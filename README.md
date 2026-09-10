English | [简体中文](README_zh.md)

# ued-uninstaller

A TUI uninstaller written in Go, for cleaning up the data left behind by `unieditdept` apps in three locations.

It scans two kinds of targets.

**1. Data belonging to UniEditDept apps** (the main target):

| Location | Environment variable |
| --- | --- |
| Roaming | `%APPDATA%` |
| Local | `%LOCALAPPDATA%` |
| Temp | `%TEMP%` |

A subdirectory named `unieditdept/<name>` under each location counts as one app's data. **Directories sharing a name across the three locations are merged into a single app**: their sizes are summed and they are removed together.

**2. Data left behind by other apps** (found incidentally, for reference only):

A **folder** under the **root** of `%APPDATA%` whose name ends in `.exe` (a folder, not a file) and which contains an `EBWebView` subdirectory. These are usually data directories created by other WebView2-based apps, and have nothing to do with this program.

They get their own section in the list, separated by a notice:

> These apps are not made by UniEditDept — clean them with care.

The `.exe` suffix is stripped from the displayed name, their size is measured asynchronously like everything else, and pressing `D` removes them — the **entire folder** goes away.

## Interaction

The whole program is a single screen: the app list on the left, the task panel on the right. It surfaces only what is useful at the current stage, and action hints live in exactly one place — the status bar at the bottom.

1. **Listed on entry**: startup only enumerates directory names (near-instant), so the list is usable immediately. It has a header row; each entry shows name, size, and creation date (the earliest of the three locations). The size and date columns are fixed width — only the name column flexes. As the window narrows, the date is first shortened, then dropped, so numbers never get pushed off screen. If "other apps" data directories are found, they get their own section below the list, separated by a notice.
2. **Asynchronous size measurement**: measurement runs concurrently in the background, and each directory's result is filled into its row as soon as it completes; rows still pending show "pending". The list sorts by name while measuring, then switches to size once finished. The right panel shows live progress.
3. **Two-step removal**:
   - Press `D` with apps selected to enter the confirm state;
   - The bottom hint changes from "remove" to "confirm", and a highlight band sweeps slowly across the selected rows and the confirm hint (about 14 columns per second, deliberately gentle), for 6 seconds;
   - Press `D` (or `Y`) again to actually run it; **any other key, or the timeout, cancels**;
   - After cancelling, the right panel briefly shows "Cancelled" and the status bar returns to the normal hints.
4. **Right-hand task panel**: shows measurement progress while scanning, a summary of the pending removal (app count / projected space freed) once something is selected, a progress bar with space freed and per-item status while removing, and a result (folders removed, space freed, elapsed time, failure details) when done. Left blank when nothing is selected.

After a removal completes, successfully deleted apps disappear from the list while failures remain, so they can be retried once whatever was holding them is closed.

Visually: a slowly breathing star precedes the title (primary colour `#ff6699`, one brightness cycle every 4 seconds); the rule under the title advances only every 3 frames, so the shimmer stays unhurried; the progress bar and the dissolve animation are gradients built from that same primary colour.

**Non-UniEditDept entries deliberately use a different palette**: the notice is dark grey and the entries are light grey, a notch quieter than the primary scheme (this is incidentally discovered information, and shouldn't compete with the apps this program manages); the cursor row is inverted; and the band that sweeps across them during confirm is blue-purple (deep blue → blue → blue-purple), instantly distinguishable from the pink band on UniEditDept entries. The bright end of that band is deliberately given lower perceived brightness than the pink one, so the cool colour doesn't end up harsher — a test guards that constraint.

Animations can be disabled entirely with `-no-anim`.

## Keys

| Key | Action |
| --- | --- |
| `↑` `↓` / `k` `j` | move the cursor |
| `PgUp` `PgDn` / `Home` `End` | page up / down, jump to first / last |
| `Space` | select or deselect |
| `A` | select all / none |
| `I` | invert the selection |
| `S` | cycle sorting (size / name / date) |
| `/` | filter by name, `Esc` clears |
| `D` | remove: enter the confirm state, press again to run |
| `L` | switch interface language (简体中文 / English) |
| `P` | open the GitHub repository |
| `R` | rescan |
| `?` | help. The help page is read-only: it only responds to `?` (back), `L` (language) and `q` (quit); it also has a "← Back" button at the top, and clicking the title exits too |
| `Q` / `Esc` / `Ctrl+C` | quit |

`Enter` is unbound on purpose: it neither selects, nor triggers removal, nor quits — so it can't be confused with confirming input while filtering with `/`.

## Mouse

| Action | Effect |
| --- | --- |
| Click an app row | move the cursor there and **toggle its selection** |
| Click a bottom key hint | same as pressing that key (`Space` `A` `D` `/` `S` `I` `R` `?` `Q`, plus "any key" to cancel while confirming) |
| Click the `L` badge | switch to the next language |
| Click the `P` badge | open the repository (same as pressing `P`) |
| Click 简体中文 / English | switch straight to that language |
| Click `GitHub` | open the repository in the default browser |
| Click the title | go back to the main view: close help / cancel a pending confirm / dismiss the result |
| Wheel | move the cursor |

Hit testing uses the layout recorded while rendering, so clickable regions always match what you see. The arrow-key hints have no click binding (use the wheel or click a row directly).

## Interface language

The top right shows, from left to right: the version number, a `P` hint, the `GitHub` link, and the language switcher — e.g. `260910    P  GitHub    L  简体中文 | English`. `GitHub` is underlined to signal it's clickable (opens the repo in your browser); the `P` chip immediately to its left does the same thing and is styled identically to `L`. Both are neutral-grey chips, as is the current language (unselected languages carry no background) — consistent with the bottom key hints, but kept neutral so it doesn't pull attention from the list. Press `P` or `L` to act instantly, no restart needed.

You can also pick the initial language with `-lang en`. All UI strings live in `internal/i18n`; adding a language means adding one `Strings` value — a test reflects over the struct and fails if any field is untranslated.

English counting nouns are always written as `app(s)` / `folder(s)` (`1 app(s)`, `3 app(s)`), because those numbers are frequently 1 and English has no plural form that reads correctly for both 0/1 and N — hardcoding the plural produces "1 apps".

## Version

The version number is the build date in `yyMMdd` form (e.g. `260910`). It's shown in the top right of the TUI, to the left of the GitHub link, and is also what `--version` prints.

The repo ships a `build.ps1` that injects today's date and generates the Windows version resource:

```powershell
.\build.ps1
```

That step is also what fills in the file description, product name, company, copyright, and file/product version shown in Explorer → Properties → Details. Go writes no VERSIONINFO by default, so without it those fields stay blank.

The resource text lives in [versioninfo.json](versioninfo.json), where `__MAJOR__` / `__MINOR__` / `__PATCH__` / `__VERSION_DOTTED__` are replaced by the script with today's date (`260910` → `26.9.10`). The generated `resource_windows_*.syso` is not committed, but `go build` links it automatically whenever it sits in the project root — so run `build.ps1` once before building manually.

If you use plain `go build` without injection, the TUI version falls back to the executable's build time (its file modification time), which still yields the day it was built.

## Usage

```powershell
.\build.ps1            # build and stamp today's version
.\ued-uninstaller.exe
```

Or build manually (after running `build.ps1` once on Windows so the version resource exists):

```bash
go build -o ued-uninstaller.exe .
./ued-uninstaller.exe
```

Command-line flags:

```
-namespace string   name of the namespace directory holding app data (default "unieditdept")
-lang string        interface language: zh or en (default zh)
-dry-run            dry run: measure reclaimable space without deleting anything
-no-anim            disable animations (for weak terminals or remote sessions)
-no-prune           keep the empty namespace directories after removal
-log string         path to a log file; empty means no logging
-version            print version information
```

## Project layout

```
main.go                     command-line entry point
internal/app                root model: holds the current screen and dispatches messages
internal/config             runtime configuration
internal/core               domain layer: path resolution, data model, scanner, remover (UI-agnostic)
internal/i18n               UI strings and language switching
internal/version            version number (build date)
internal/platform           OS-specific file operations (creation time, clearing read-only)
internal/logging            optional file logging
internal/ui                 Screen abstraction and cross-screen messages
internal/ui/theme           primary palette and lipgloss styles
internal/ui/ascii           gradient shading, wave, spinner, dissolve animations
internal/ui/components      shared building blocks (box, progress bar, help line, text clipping)
internal/ui/screens/home    the one and only main screen
```

## Design notes

- **Domain and UI separated**: `internal/core` depends on no UI code and is testable on its own; scanning and removal talk to the UI through an event stream (`Event`).
- **Two-phase scanning**: enumerate first (synchronous, instant), then measure (asynchronous, concurrent), so the UI is usable at every moment; results are filled back by "app name + location", with no need to re-sort or rebuild the list.
- **Interruptible**: both scanning and removal take a `context.Context` and stop emitting events once cancelled; a rescan shuts the old task down to avoid leaking goroutines.
- **Robustness**: unreadable directories are skipped and the walk continues; read-only attributes are cleared before deletion, a failure is retried once, and failures are recorded per item.
- **Adaptive layout**: the interface never overflows between 80×24 and 200×60, and the key hints are dropped by width (covered by unit tests).
- **One width definition only**: every layout computation uses `charmbracelet/x/ansi` width (matching how lipgloss / bubbletea render). **Do not introduce `mattn/go-runewidth`** — on Chinese Windows it counts ambiguous characters like `↑` `↓` `·` `│` as 2 columns while rendering positions them at 1, and the mismatch makes row widths wrong, which then truncates in the middle of an ANSI escape sequence and shows up as the bottom hint bar losing everything after `↑/`. Every truncation must go through `ansi.Truncate` (which won't break escape sequences), and never hand-clip an already-styled string by rune.

## Tests

```bash
go test ./...
```

Set `UED_DUMP=<path>` and run `go test ./internal/app -run TestDumpHome` to export snapshots of each screen state (plain text, for eyeballing the layout).
