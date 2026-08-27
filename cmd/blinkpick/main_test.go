package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/idealland-apps/Blink/internal/model"
)

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer

	exitCode := run([]string{"--help"}, &bytes.Buffer{}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", exitCode, stderr.String())
	}
	if got := stdout.String(); got == "" {
		t.Fatal("help output is empty")
	}
}

func TestConfigPathDefaultsBesideExecutable(t *testing.T) {
	t.Setenv("BLINKPICK_CONFIG_PATH", "")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(executable), "blinkpick.config.json")
	if got := configPath(); got != want {
		t.Fatalf("configPath() = %q, want %q", got, want)
	}
}

func TestActionLabelsReflectEntryState(t *testing.T) {
	unread := actionLabels(model.Entry{Status: "unread"})
	if got, want := strings.Join(unread, "|"), "Open original|Mark read|Next|Star|Quit"; got != want {
		t.Fatalf("unread labels = %q, want %q", got, want)
	}

	readStarred := actionLabels(model.Entry{Status: "read", Starred: true})
	if got, want := strings.Join(readStarred, "|"), "Open original|Mark unread|Next|Unstar|Quit"; got != want {
		t.Fatalf("read/starred labels = %q, want %q", got, want)
	}
}

func TestActionBarUnderlinesShortcutLetter(t *testing.T) {
	var output bytes.Buffer

	renderActionBar(&output, model.Entry{}, actionOpen, true)

	got := output.String()
	if !strings.Contains(got, "\x1b[4mO\x1b[24mpen original") {
		t.Fatalf("open shortcut is not underlined: %q", got)
	}
	if !strings.Contains(got, "Mark \x1b[4mr\x1b[24mead") {
		t.Fatalf("mark-read shortcut is not underlined: %q", got)
	}
}

func TestRenderCardUsesSemanticANSIStylesAndSelection(t *testing.T) {
	entry := model.Entry{ID: 1, Title: "Styled article", ReadingTime: 3, PublishedAt: time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)}
	entry.Feed.Title = "Example Feed"
	entry.Feed.Category.Title = "AI"
	var output bytes.Buffer

	renderCard(&output, entry, cardView{Color: true, Selected: 1})

	got := output.String()
	if !strings.Contains(got, "\x1b[1m") || !strings.Contains(got, "\x1b[7m Mark \x1b[4mr\x1b[24mead \x1b[0m") {
		t.Fatalf("rendered card lacks expected ANSI semantic styles: %q", got)
	}
}

func TestRenderCardShowsGoldStarredBadgeOnlyForStarredEntries(t *testing.T) {
	var starred, plain bytes.Buffer
	renderCard(&starred, model.Entry{ID: 1, Title: "Starred", Starred: true}, cardView{Color: true})
	renderCard(&plain, model.Entry{ID: 2, Title: "Plain"}, cardView{Color: true})

	if !strings.Contains(starred.String(), "\x1b[33m★ STARRED\x1b[0m") {
		t.Fatalf("starred badge missing or not gold: %q", starred.String())
	}
	if strings.Contains(plain.String(), "STARRED") {
		t.Fatalf("unstarred card shows starred badge: %q", plain.String())
	}
}

func TestRenderCardShowsColoredReadableStatusBadge(t *testing.T) {
	entry := model.Entry{ID: 1, Title: "Status article", Status: "read"}
	var output bytes.Buffer

	renderCard(&output, entry, cardView{Color: true})

	got := output.String()
	if !strings.Contains(got, "\x1b[32m✓ READ\x1b[0m") {
		t.Fatalf("read status badge missing or not green: %q", got)
	}
}

func TestAlternateScreenRestoresMainScrollbackOnExit(t *testing.T) {
	var output bytes.Buffer
	leave := enterAlternateScreen(&output)
	leave()

	if got, want := output.String(), "\x1b[?1049h\x1b[H\x1b[?1049l"; got != want {
		t.Fatalf("alternate screen sequence = %q, want %q", got, want)
	}
}

func TestRedrawActionBarOnlyRepaintsTheActionLine(t *testing.T) {
	var output bytes.Buffer

	redrawActionBar(&output, model.Entry{}, 1, true)

	got := output.String()
	if !strings.HasPrefix(got, "\x1b[1A\r\x1b[2K") || strings.Contains(got, "\x1b[2J") {
		t.Fatalf("redraw = %q, want a single-line repaint without full clear", got)
	}
	if !strings.Contains(got, "\x1b[7m Mark \x1b[4mr\x1b[24mead \x1b[0m") {
		t.Fatalf("selected action missing from redraw: %q", got)
	}
}

func TestConfigWizardCanCancelWithoutWritingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("BLINKPICK_CONFIG_PATH", path)
	input := strings.NewReader("1\nhttps://rss.example.test\n1\ntoken\nn\n")
	var stdout, stderr bytes.Buffer

	exitCode := run([]string{"config"}, input, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %s", exitCode, stderr.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config was written after cancellation: %v", err)
	}
}
