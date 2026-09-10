English | [简体中文](README_zh.md)

# ued-uninstaller

A TUI uninstaller in Go, for cleaning up the data that `unieditdept` apps leave in three locations.

## What it scans

**UniEditDept apps** — every `unieditdept/<name>` subdirectory under the three locations below. Directories sharing a name are merged into one app: sizes are summed, and they are removed together.

| Location | Variable |
| --- | --- |
| Roaming | `%APPDATA%` |
| Local | `%LOCALAPPDATA%` |
| Temp | `%TEMP%` |

**Other apps** (found incidentally) — a *folder* under the `%APPDATA%` root whose name ends in `.exe` and which contains `EBWebView`. These belong to other programs (usually WebView2-based), not to UniEditDept, so they get a separate section behind a caution notice. `.exe` is stripped from the displayed name and `D` removes the entire folder.

## Usage

```powershell
.\build.ps1            # build, stamping today's date as the version
.\ued-uninstaller.exe
```

```
-namespace string   namespace directory holding app data (default "unieditdept")
-lang string        interface language: zh or en (default zh)
-dry-run            measure reclaimable space without deleting
-no-anim            disable animations
-no-prune           keep empty namespace directories
-log string         log file path; empty disables logging
-version            print version
```

## Keys

| Key | Action |
| --- | --- |
| `↑` `↓` / `k` `j` | move the cursor |
| `PgUp` `PgDn` / `Home` `End` | page, jump to first / last |
| `Space` | select or deselect |
| `A` / `I` | select all or none / invert selection |
| `S` | cycle sorting (size / name / date) |
| `/` | filter by name, `Esc` clears |
| `D` | remove: confirm state, then `D`/`Y` to run |
| `L` | switch language |
| `P` | open the GitHub repository |
| `R` | rescan |
| `?` | help |
| `Q` / `Esc` / `Ctrl+C` | quit |

## Mouse

| Action | Effect |
| --- | --- |
| Click a row | move the cursor there and toggle selection |
| Click a bottom key hint | same as pressing that key |
| Click `P` / `L` / a language | open the repo / next language / that language |
| Click `GitHub` | open the repository |
| Click the title | back to the main view (closes help, cancels a confirm) |
| Wheel | move the cursor |

## Interface

- **Listed instantly**: startup only enumerates directory names; sizes are measured concurrently and filled in per row.
- **Two-step removal**: `D` enters a 6-second confirm state where the selected rows and the confirm hint glow; `D`/`Y` runs it, and any other key or the timeout cancels.
- **Right panel** tracks the stage: scan progress → pending summary → removal progress → result.
- Non-UniEditDept entries are deliberately styled apart — grey text, and a blue-purple band instead of pink while confirming.
- `Enter` is unbound, so it can't be confused with confirming filter input.

## Building

The version number is the build date in `yyMMdd`. `build.ps1` stamps it and generates the Windows VERSIONINFO resource — that resource is what fills Explorer → Properties → Details, which Go leaves blank by default. The generated `resource_windows_*.syso` isn't committed, but `go build` links it automatically from the project root, so run `build.ps1` once before building manually.

## Project layout

```
main.go                     entry point
internal/app                root model: holds the screen, dispatches messages
internal/config             runtime configuration
internal/core               domain layer: paths, model, scanner, remover (UI-agnostic)
internal/i18n               UI strings and language switching
internal/version            version number (build date)
internal/platform           OS-specific file operations
internal/logging            optional file logging
internal/ui                 Screen abstraction and cross-screen messages
internal/ui/theme           palette and lipgloss styles
internal/ui/ascii           gradient, wave, spinner, dissolve animations
internal/ui/components      boxes, progress bar, help line, text clipping
internal/ui/screens/home    the only screen
```

## Development

- `internal/core` depends on no UI code; scanning and removal talk to the UI through an event stream and take a `context.Context` (a rescan shuts the old task down).
- Unreadable directories are skipped, read-only attributes are cleared before deletion, and failures are recorded per item.
- The layout never overflows between 80×24 and 200×60.
- **All width math uses `charmbracelet/x/ansi`** — never `mattn/go-runewidth`, which counts ambiguous characters as 2 columns on Chinese Windows while rendering positions them at 1; the mismatch truncates rows in the middle of an ANSI sequence. Always truncate through `ansi.Truncate`.

## Tests

```bash
go test ./...
```

`UED_DUMP=<path> go test ./internal/app -run TestDumpHome` exports plain-text snapshots of every state.
