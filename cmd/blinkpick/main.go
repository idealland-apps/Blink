package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/idealland-apps/Blink/internal/config"
	"github.com/idealland-apps/Blink/internal/miniflux"
	"github.com/idealland-apps/Blink/internal/model"
	"github.com/idealland-apps/Blink/internal/picker"
	"github.com/idealland-apps/Blink/internal/state"
	"golang.org/x/term"
)

const usage = `Blink — random reading picks for the time between tasks.

Usage:
  blinkpick [selection flags]       Open the interactive reading card.
  blinkpick suggest [flags]         Emit a noninteractive recommendation.
  blinkpick mark-read <entry-id>    Mark a Miniflux entry as read.
  blinkpick save <entry-id>         Save/star a Miniflux entry.
  blinkpick unsave <entry-id>       Remove an entry from saved/starred.
  blinkpick config [flags]          Configure a provider (bare command starts a wizard).
  blinkpick doctor                  Check configuration and provider access.

Selection flags: --minutes N, --category NAME, --fresh 7d, --all
`

type selectionFlags struct {
	minutes         int
	category, fresh string
	all             bool
}

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, in io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return interactive(in, stdout, stderr, selectionFlags{})
	}
	switch args[0] {
	case "--help", "-h", "help":
		_, _ = fmt.Fprint(stdout, usage)
		return 0
	case "config":
		return runConfig(args[1:], in, stdout, stderr)
	case "suggest":
		return runSuggest(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(stdout, stderr)
	case "mark-read", "save", "unsave":
		return runMutation(args, stderr)
	default:
		flags, err := parseSelection(args)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 2
		}
		return interactive(in, stdout, stderr, flags)
	}
}

func parseSelection(args []string) (selectionFlags, error) {
	var result selectionFlags
	fs := flag.NewFlagSet("blinkpick", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.IntVar(&result.minutes, "minutes", 0, "maximum preferred reading minutes")
	fs.StringVar(&result.category, "category", "", "Miniflux category title")
	fs.StringVar(&result.fresh, "fresh", "", "freshness duration, for example 7d")
	fs.BoolVar(&result.all, "all", false, "allow read entries")
	if err := fs.Parse(args); err != nil {
		return result, err
	}
	if fs.NArg() != 0 {
		return result, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if result.minutes < 0 {
		return result, errors.New("--minutes must be positive")
	}
	return result, nil
}

func configPath() string {
	if value := os.Getenv("BLINKPICK_CONFIG_PATH"); value != "" {
		return value
	}
	executable, err := os.Executable()
	if err != nil {
		return "blinkpick.config.json"
	}
	return filepath.Join(filepath.Dir(executable), "blinkpick.config.json")
}

func statePath() string {
	if value := os.Getenv("BLINKPICK_STATE_PATH"); value != "" {
		return value
	}
	executable, err := os.Executable()
	if err != nil {
		return "blinkpick-state.json"
	}
	return filepath.Join(filepath.Dir(executable), "blinkpick-state.json")
}

func loadConfig() (config.Config, error) {
	c, err := config.Load(configPath())
	if os.IsNotExist(err) {
		return config.Config{}, fmt.Errorf("not configured; run `blinkpick config`")
	}
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return config.Config{}, fmt.Errorf("invalid config: %w", err)
	}
	return c, nil
}

func runConfig(args []string, in io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.SetOutput(stderr)
	provider := fs.String("provider", "", "provider name")
	url := fs.String("url", "", "Miniflux base URL")
	username := fs.String("username", "", "Miniflux username")
	token := fs.String("token", "", "Miniflux API token")
	password := fs.String("password", "", "Miniflux password")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "config accepts flags only")
		return 2
	}
	if len(args) == 0 {
		return configWizard(in, stdout, stderr)
	}
	current, err := config.Load(configPath())
	if err != nil && !os.IsNotExist(err) {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if current.Provider == "" {
		current.Provider = "miniflux"
	}
	if *provider != "" {
		current.Provider = *provider
	}
	if *url != "" {
		current.URL = *url
	}
	if *username != "" {
		current.Username = *username
	}
	if *token != "" {
		current.Token = *token
		current.Password = ""
	}
	if *password != "" {
		current.Password = *password
		current.Token = ""
	}
	if err := config.Save(configPath(), current); err != nil {
		_, _ = fmt.Fprintf(stderr, "save config: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Saved Blink configuration to %s\n", configPath())
	return 0
}

func configWizard(in io.Reader, stdout, stderr io.Writer) int {
	scanner := bufio.NewScanner(in)
	ask := func(label, fallback string) (string, bool) {
		if fallback != "" {
			fmt.Fprintf(stdout, "%s [%s]: ", label, fallback)
		} else {
			fmt.Fprintf(stdout, "%s: ", label)
		}
		if !scanner.Scan() {
			return "", false
		}
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			return fallback, true
		}
		return text, true
	}
	_, _ = fmt.Fprintln(stdout, "Blink configuration wizard")
	_, _ = fmt.Fprintln(stdout)
	provider, ok := ask("Provider (1 = Miniflux)", "miniflux")
	if !ok {
		return 1
	}
	if provider == "1" {
		provider = "miniflux"
	}
	if provider != "miniflux" {
		_, _ = fmt.Fprintln(stderr, "Only Miniflux is currently supported.")
		return 2
	}
	url, ok := ask("Miniflux URL", "")
	if !ok {
		return 1
	}
	auth, ok := ask("Authentication (1 = API token, 2 = username/password)", "1")
	if !ok {
		return 1
	}
	c := config.Config{Provider: provider, URL: url}
	if auth == "1" || strings.EqualFold(auth, "token") {
		c.Token, ok = ask("API token", "")
	} else if auth == "2" || strings.EqualFold(auth, "basic") {
		c.Username, ok = ask("Username", "")
		if ok {
			c.Password, ok = ask("Password", "")
		}
	} else {
		_, _ = fmt.Fprintln(stderr, "Choose 1 or 2.")
		return 2
	}
	if !ok {
		return 1
	}
	confirm, ok := ask("Save this configuration? [Y/n]", "y")
	if !ok {
		return 1
	}
	if strings.EqualFold(confirm, "n") || strings.EqualFold(confirm, "no") {
		_, _ = fmt.Fprintln(stdout, "Configuration not saved.")
		return 0
	}
	if err := config.Save(configPath(), c); err != nil {
		_, _ = fmt.Fprintf(stderr, "save config: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "\nSaved Blink configuration to %s\n", configPath())
	return 0
}

func parseFresh(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	if strings.HasSuffix(value, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
		return time.Duration(days) * 24 * time.Hour, err
	}
	return time.ParseDuration(value)
}
func selectEntry(flags selectionFlags) (model.Entry, *miniflux.Client, error) {
	c, err := loadConfig()
	if err != nil {
		return model.Entry{}, nil, err
	}
	fresh, err := parseFresh(flags.fresh)
	if err != nil {
		return model.Entry{}, nil, fmt.Errorf("parse --fresh: %w", err)
	}
	client := miniflux.New(c)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	entries, err := client.ListEntries(ctx, miniflux.ListOptions{UnreadOnly: !flags.all, Limit: 200, Category: flags.category, Freshness: fresh})
	if err != nil {
		return model.Entry{}, nil, err
	}
	local, err := state.Load(statePath())
	if err != nil {
		return model.Entry{}, nil, err
	}
	entry, err := picker.Pick(entries, picker.Options{Now: time.Now(), Freshness: fresh, Minutes: flags.minutes, RecentEntryIDs: local.EntrySet(), RecentFeedIDs: local.FeedSet()})
	if err != nil {
		return model.Entry{}, nil, err
	}
	local.Record(entry.ID, entry.FeedID)
	if err := state.Save(statePath(), local); err != nil {
		return model.Entry{}, nil, err
	}
	return entry, client, nil
}

func runSuggest(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("suggest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "one-line", "one-line or json")
	oneLine := fs.Bool("one-line", false, "one-line output")
	jsonOutput := fs.Bool("json", false, "JSON output")
	minutes := fs.Int("minutes", 0, "maximum preferred reading minutes")
	category := fs.String("category", "", "category")
	fresh := fs.String("fresh", "", "freshness")
	all := fs.Bool("all", false, "allow read entries")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *oneLine {
		*format = "one-line"
	}
	if *jsonOutput {
		*format = "json"
	}
	entry, _, err := selectEntry(selectionFlags{minutes: *minutes, category: *category, fresh: *fresh, all: *all})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "blinkpick suggest:", err)
		return 1
	}
	if *format == "json" {
		if err := json.NewEncoder(stdout).Encode(entry); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if *format != "one-line" {
		_, _ = fmt.Fprintln(stderr, "--format must be one-line or json")
		return 2
	}
	_, _ = fmt.Fprintf(stdout, "%s · %d min · %s · %s\n", entry.Feed.Category.Title, entry.ReadingTime, entry.Title, entry.URL)
	return 0
}

func runMutation(args []string, stderr io.Writer) int {
	if len(args) != 2 {
		_, _ = fmt.Fprintf(stderr, "usage: blinkpick %s <entry-id>\n", args[0])
		return 2
	}
	id, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil || id <= 0 {
		_, _ = fmt.Fprintln(stderr, "entry-id must be a positive integer")
		return 2
	}
	c, err := loadConfig()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	client := miniflux.New(c)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	switch args[0] {
	case "mark-read":
		err = client.MarkRead(ctx, id)
	case "save":
		err = client.SetStarred(ctx, id, true)
	case "unsave":
		err = client.SetStarred(ctx, id, false)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Miniflux update:", err)
		return 1
	}
	return 0
}

func runDoctor(stdout, stderr io.Writer) int {
	c, err := loadConfig()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	client := miniflux.New(c)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = client.ListEntries(ctx, miniflux.ListOptions{UnreadOnly: true, Limit: 1})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Miniflux check:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "OK: Miniflux is reachable at %s\n", c.URL)
	return 0
}

type cardView struct {
	Color    bool
	Selected int
	Notice   string
}

const (
	actionOpen = iota
	actionMarkRead
	actionNext
	actionSave
	actionQuit
)

func actionLabels(entry model.Entry) []string {
	readAction := "Mark read"
	if strings.EqualFold(entry.Status, "read") {
		readAction = "Mark unread"
	}
	starAction := "Star"
	if entry.Starred {
		starAction = "Unstar"
	}
	return []string{"Open original", readAction, "Next", starAction, "Quit"}
}

func interactive(in io.Reader, stdout, stderr io.Writer, flags selectionFlags) int {
	reader := bufio.NewReader(in)
	color, raw := enableInteractiveTerminal(in, stdout)
	if raw != nil {
		defer raw()
	}
	for {
		entry, client, err := selectEntry(flags)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "blinkpick:", err)
			return 1
		}
		selected, notice := 0, ""
		needsFullRender := true
		for {
			if needsFullRender {
				if color {
					_, _ = fmt.Fprint(stdout, "\x1b[2J\x1b[H")
				}
				renderCard(stdout, entry, cardView{Color: color, Selected: selected, Notice: notice})
				needsFullRender = false
			}
			key, ok := readKey(reader)
			if !ok {
				return 0
			}
			switch key {
			case "left":
				selected = (selected + len(actionLabels(entry)) - 1) % len(actionLabels(entry))
				if color {
					redrawActionBar(stdout, entry, selected, true)
				} else {
					needsFullRender = true
				}
				continue
			case "right":
				selected = (selected + 1) % len(actionLabels(entry))
				if color {
					redrawActionBar(stdout, entry, selected, true)
				} else {
					needsFullRender = true
				}
				continue
			case "o":
				selected = 0
			case "s":
				selected = actionSave
			case "r":
				selected = actionMarkRead
			case "n":
				selected = actionNext
			case "q":
				selected = actionQuit
			case "enter":
				// Run the highlighted action.
			default:
				notice = "Use ←/→ and Enter, or o/s/r/n/q shortcuts."
				needsFullRender = true
				continue
			}
			switch selected {
			case actionOpen:
				if err := openURL(entry.URL); err != nil {
					notice = "Could not open browser: " + err.Error()
				} else {
					notice = "Opened original URL."
				}
			case actionMarkRead:
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				var err error
				if strings.EqualFold(entry.Status, "read") {
					err = client.MarkUnread(ctx, entry.ID)
				} else {
					err = client.MarkRead(ctx, entry.ID)
				}
				cancel()
				if err != nil {
					notice = "Update read status failed: " + err.Error()
				} else if strings.EqualFold(entry.Status, "read") {
					entry.Status = "unread"
					notice = "Marked unread in Miniflux."
				} else {
					entry.Status = "read"
					notice = "Marked read in Miniflux."
				}
			case actionNext:
				break
			case actionSave:
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				err := client.SetStarred(ctx, entry.ID, !entry.Starred)
				cancel()
				if err != nil {
					notice = "Save failed: " + err.Error()
				} else {
					entry.Starred = !entry.Starred
					notice = map[bool]string{true: "Saved in Miniflux.", false: "Removed from saved."}[entry.Starred]
				}
			case actionQuit:
				return 0
			}
			needsFullRender = true
			if selected == actionNext {
				break
			}
		}
	}
}

func enableInteractiveTerminal(in io.Reader, out io.Writer) (bool, func()) {
	input, inputOK := in.(*os.File)
	output, outputOK := out.(*os.File)
	if !inputOK || !outputOK || !term.IsTerminal(int(input.Fd())) || !term.IsTerminal(int(output.Fd())) {
		return false, nil
	}
	leaveScreen := enterAlternateScreen(out)
	oldState, err := term.MakeRaw(int(input.Fd()))
	color := os.Getenv("NO_COLOR") == ""
	if err != nil {
		return color, func() { leaveScreen(); _, _ = fmt.Fprint(out, "\x1b[0m\n") }
	}
	return color, func() {
		_ = term.Restore(int(input.Fd()), oldState)
		_, _ = fmt.Fprint(out, "\x1b[0m")
		leaveScreen()
		_, _ = fmt.Fprint(out, "\n")
	}
}

func enterAlternateScreen(out io.Writer) func() {
	_, _ = fmt.Fprint(out, "\x1b[?1049h\x1b[H")
	return func() { _, _ = fmt.Fprint(out, "\x1b[?1049l") }
}

func readKey(reader *bufio.Reader) (string, bool) {
	key, err := reader.ReadByte()
	if err != nil {
		return "", false
	}
	if key == '\x1b' {
		left, err1 := reader.ReadByte()
		right, err2 := reader.ReadByte()
		if err1 == nil && err2 == nil && left == '[' {
			if right == 'D' {
				return "left", true
			}
			if right == 'C' {
				return "right", true
			}
		}
		return "", true
	}
	if key == '\r' || key == '\n' {
		return "enter", true
	}
	return strings.ToLower(string(key)), true
}

func renderCard(out io.Writer, entry model.Entry, view cardView) {
	category := entry.Feed.Category.Title
	if category == "" {
		category = "Uncategorized"
	}
	bold, accent, muted, reset := "", "", "", ""
	if view.Color {
		bold, accent, muted, reset = "\x1b[1m", "\x1b[36m", "\x1b[2m", "\x1b[0m"
	}
	statusText, statusColor := "● UNREAD", ""
	if strings.EqualFold(entry.Status, "read") {
		statusText, statusColor = "✓ READ", "\x1b[32m"
	} else if view.Color {
		statusColor = "\x1b[33m"
	}
	if !view.Color {
		statusColor = ""
	}
	fmt.Fprintf(out, "%s%s%s%s  ", bold, statusColor, statusText, reset)
	fmt.Fprintf(out, "%s%s%s%s", bold, accent, category, reset)
	fmt.Fprintf(out, "  %s· %s%s  %s· %s%d min%s  %s· %s%s", muted, entry.Feed.Title, reset, muted, bold, entry.ReadingTime, reset, muted, entry.PublishedAt.Local().Format("2006-01-02 15:04"), reset)
	if entry.Starred {
		starColor := ""
		if view.Color {
			starColor = "\x1b[33m"
		}
		fmt.Fprintf(out, "  %s★ STARRED%s", starColor, reset)
	}
	fmt.Fprint(out, "\n\n")
	fmt.Fprintf(out, "%s%s%s\n\n%s\n\n%s%s%s\n", bold, entry.Title, reset, preview(entry.Content, 700), muted, entry.URL, reset)
	if view.Notice != "" {
		fmt.Fprintf(out, "\n%s%s%s\n", muted, view.Notice, reset)
	}
	fmt.Fprint(out, "\n")
	renderActionBar(out, entry, view.Selected, view.Color)
	fmt.Fprint(out, "\n")
}

func renderActionBar(out io.Writer, entry model.Entry, selected int, color bool) {
	for index, action := range actionLabels(entry) {
		if index == selected && color {
			fmt.Fprintf(out, " \x1b[7m %s \x1b[0m", action)
		} else if index == selected {
			fmt.Fprintf(out, " [ %s ]", action)
		} else {
			fmt.Fprintf(out, "   %s  ", action)
		}
	}
}

func redrawActionBar(out io.Writer, entry model.Entry, selected int, color bool) {
	_, _ = fmt.Fprint(out, "\x1b[1A\r\x1b[2K")
	renderActionBar(out, entry, selected, color)
	_, _ = fmt.Fprint(out, "\n")
}

func preview(html string, limit int) string {
	var b strings.Builder
	inTag := false
	lastSpace := false
	for _, r := range html {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			b.WriteByte(' ')
			continue
		}
		if inTag {
			continue
		}
		if unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastSpace = false
		if b.Len() >= limit {
			b.WriteString("…")
			break
		}
	}
	return strings.TrimSpace(b.String())
}
func openURL(raw string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", raw)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", raw)
	default:
		command = exec.Command("xdg-open", raw)
	}
	return command.Start()
}
