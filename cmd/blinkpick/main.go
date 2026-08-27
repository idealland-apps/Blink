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

func interactive(in io.Reader, stdout, stderr io.Writer, flags selectionFlags) int {
	scanner := bufio.NewScanner(in)
	for {
		entry, client, err := selectEntry(flags)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "blinkpick:", err)
			return 1
		}
		renderCard(stdout, entry)
		fmt.Fprint(stdout, "[o]pen [s]ave [r]ead [n]ext [q]uit [?]help > ")
		if !scanner.Scan() {
			return 0
		}
		switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
		case "", "n":
			continue
		case "q":
			return 0
		case "?":
			fmt.Fprintln(stdout, "o opens the original URL; s toggles saved; r marks read; n picks another; q quits.")
		case "o":
			if err := openURL(entry.URL); err != nil {
				fmt.Fprintln(stderr, "open browser:", err)
			}
		case "s":
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			err := client.SetStarred(ctx, entry.ID, !entry.Starred)
			cancel()
			if err != nil {
				fmt.Fprintln(stderr, "save:", err)
			} else {
				fmt.Fprintln(stdout, "Saved state updated.")
			}
		case "r":
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			err := client.MarkRead(ctx, entry.ID)
			cancel()
			if err != nil {
				fmt.Fprintln(stderr, "mark read:", err)
			} else {
				fmt.Fprintln(stdout, "Marked read.")
			}
		default:
			fmt.Fprintln(stdout, "Unknown action. Press ? for help.")
		}
	}
}

func renderCard(out io.Writer, entry model.Entry) {
	category := entry.Feed.Category.Title
	if category == "" {
		category = "Uncategorized"
	}
	fmt.Fprintf(out, "\n%s · %s · %d min · %s\n\n%s\n\n%s\n\n%s\n\n", category, entry.Feed.Title, entry.ReadingTime, entry.PublishedAt.Local().Format("2006-01-02 15:04"), entry.Title, preview(entry.Content, 700), entry.URL)
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
