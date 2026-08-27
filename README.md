# Blink

**Blink** is a small terminal companion for the gaps between agent tasks: it picks one suitable article from your reading backlog and gives you a compact decision card.

The released command is **`blinkpick`**, not `blink`: `blink` is already used by other well-known CLI tools.

Blink is deliberately not an RSS reader or manager. Your provider owns feeds, categories, entry content, read state, and saved state. Blink owns a little local history so it can make a useful random pick without immediately repeating itself.

## v1

- **Provider:** Miniflux only (via its API)
- **Interfaces:** a small terminal card for people, or deterministic noninteractive output for hooks and scripts
- **Persistence:** local JSON config and recent-pick history
- **Not included:** GUI, browser UI, daemon, notifications, feed management, accounts, or AI summaries

## Install

Download the binary matching your platform from this repository's GitHub Releases and place it on your `PATH`:

```bash
blinkpick --help
```

For local development, Go 1.27 or newer is required:

```bash
go test ./...
go build -o ./bin/blinkpick ./cmd/blinkpick
./bin/blinkpick --help
```

## Maintainer releases

Releases are **manual only**. Normal commits and pushes never publish a GitHub Release.

After `.github/workflows/release.yml` is on the default branch, open **Actions → Release Blinkpick → Run workflow** in GitHub. Select the source branch/commit, enter a new tag such as `v0.1.0`, optionally select **prerelease**, then run it.

The workflow runs tests and `go vet`, cross-compiles these six standalone binaries, and only then creates the tag and GitHub Release:

```text
blinkpick_linux_amd64
blinkpick_linux_arm64
blinkpick_darwin_amd64
blinkpick_darwin_arm64
blinkpick_windows_amd64.exe
blinkpick_windows_arm64.exe
```

An existing tag intentionally makes the release job fail rather than overwriting a published release.

## Configure Miniflux

Run the wizard once:

```bash
blinkpick config
```

It asks for:

1. provider (currently `miniflux`);
2. the base Miniflux URL;
3. API token (recommended) or username/password; and
4. confirmation before it writes the configuration.

Create an API token in Miniflux at **Settings → API Keys → Create a new API key**. Blinkpick is portable by default: it saves its configuration next to the executable you run:

```text
<directory containing blinkpick>/blinkpick.config.json
```

The recent-pick history is stored alongside it as `blinkpick-state.json`. This makes an extracted release folder self-contained and easy to move between machines. Keep the folder private because `blinkpick.config.json` contains your Miniflux credential.

If the executable directory is intentionally read-only (for example a system-wide installation), set explicit locations with environment variables:

```bash
BLINKPICK_CONFIG_PATH=/path/to/config.json
BLINKPICK_STATE_PATH=/path/to/state.json
```

For noninteractive setup or a single field update:

```bash
blinkpick config --url https://rss.example.com --token 'your-api-token'
blinkpick config --username anthony --password 'your-password'
```

Use `blinkpick doctor` to check the saved configuration and Miniflux API connectivity. Blink never prints credentials in errors or normal output.

## Read in the terminal

```bash
# Open a random article card.
blinkpick

# Prefer short content, restrict category, or limit freshness.
blinkpick --minutes 3
blinkpick --category AI --fresh 7d
blinkpick --all
```

The card shows category, feed, estimated reading time, publication time, title, a short plaintext preview, and the original URL. It intentionally sends full reading to the original site rather than implementing an HTML browser.

The card uses ANSI semantic styles rather than fixed RGB colors: category/title use bold and the terminal's cyan palette, metadata is dimmed, and the selected action uses reverse video. This respects the user-selected terminal theme. Set `NO_COLOR=1` to opt out of colors and retain a plain-text selected button.

At the action bar, use **Left/Right Arrow** to select an action and **Enter** to run it. Keyboard shortcuts remain available:

| Key | Action |
|---|---|
| `←` / `→` | Move the highlighted action button |
| Enter | Run the highlighted action |
| `o` | Select and open the original URL with the OS default browser |
| `s` | Select and toggle Miniflux saved/starred state |
| `r` | Select and mark the entry read in Miniflux |
| `n` | Select and pick another article |
| `?` | Show help |
| `q` | Quit |

Save, mark-read, and open actions keep the current article visible and show a result notice. Only **Next** fetches a new card.

## Agent hooks and scripts

`blinkpick suggest` is explicitly noninteractive: it never opens a browser, waits for input, or emits terminal control sequences.

```bash
blinkpick suggest --one-line
blinkpick suggest --minutes 3 --one-line
blinkpick suggest --category AI --fresh 7d --json
```

The JSON form emits one Miniflux-compatible entry object on stdout, making it suitable for Claude Code hooks, Codex wrappers, CI logs, or any shell integration. The caller decides whether and where to present it.

## State commands

```bash
blinkpick mark-read 4812
blinkpick save 4812
blinkpick unsave 4812
```

These mutate Miniflux only after the API accepts the update. Seeing a Blink preview or suggestion never automatically marks an article read.

## Selection behavior

Blink fetches a bounded candidate list from Miniflux and then selects locally:

1. unread entries by default (`--all` permits read entries);
2. freshness and category filtering;
3. exclusion of recently suggested / picked entries;
4. avoidance of feeds that appeared recently when alternatives exist;
5. preference for content within `--minutes` when alternatives exist;
6. a random pick among the remaining candidates.

Miniflux remains the source of truth. Local history only prevents repetitive recommendations and is stored separately from config in the OS cache directory.

## Security boundary

This is a single-user local CLI. v1 keeps Miniflux credentials in a local mode-0600 JSON config file so the binary has no external service dependency. Treat that file as a secret. Future system-keychain support is possible but intentionally not required for the first portable version.

## License

[MIT](LICENSE)
