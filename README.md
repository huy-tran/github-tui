# github-tui

A fast, keyboard-driven terminal UI for GitHub, built on top of the official
[`gh`](https://cli.github.com/) CLI and [Bubble Tea](https://github.com/charmbracelet/bubbletea).

Browse your repositories, triage the PRs awaiting your review, watch GitHub
Actions deployments live, read & act on issues, and see vulnerability alerts -
without leaving the terminal.

```text
                                  Github - TUI - v0.0.10
 / filter · enter open · p my PRs · n notifications · v scan vulns · ctrl+f refresh · ? help

 ┌─────────────────────────────────┬────────────┬──────┬──────┬─────┬─────┬──────────┐
 │ Repository                      │ Visibility │ Crit │ High │ Med │ Low │ Updated  │
 ├─────────────────────────────────┼────────────┼──────┼──────┼─────┼─────┼──────────┤
 │ octocat/payments-api            │ private    │    1 │   11 │  13 │   5 │  2h ago  │
 │ octocat/hello-world             │ public     │    · │    · │   · │   · │ 30m ago  │
 └─────────────────────────────────┴────────────┴──────┴──────┴─────┴─────┴──────────┘

 octocat · repos · 42 items · loaded 3s ago
```

## Features

- **Repository list** - every repo you can access (owned, org, collaborator),
  ordered by recent activity, with at-a-glance Dependabot alert counts per
  severity. Cached to disk for instant startup.
- **My PRs dashboard** - a cross-repo view of pull requests awaiting your review
  (and optionally ones you authored).
- **Notifications inbox** - unread notifications across all repos, with filter,
  load-more, mark-one / mark-all read.
- **Per-repo detail** with tabs:
  - **PRs** - info, unified diff, conversation (reviews + comments), and actions:
    approve, request changes / comment, merge (commit/squash/rebase), close.
  - **Workflows** - recent Actions runs; drill into a run for jobs, steps, and
    logs; re-run or cancel; **live auto-refresh** while a run is in progress.
  - **Issues** - browse, read (body + comments), comment, and close.
  - **Security** - unified Dependabot + code-scanning + secret-scanning alerts.
- **Command palette** (`ctrl+k`) - fuzzy "go to" any repo or screen.
- **Everywhere**: `/` fuzzy filter, `s` sortable tables, `ctrl+o` open in
  browser, `ctrl+f` refresh, `?` help overlay, dark/light themes.

## Requirements

- The GitHub CLI [`gh`](https://cli.github.com/), installed and authenticated
  (`gh auth login`). github-tui shells out to `gh`, so it uses your existing
  credentials - there are no separate tokens to manage.
- To build from source: [Go](https://go.dev/dl/) 1.25+.

## Install

### Go install

```sh
go install github.com/huy-tran/github-tui@latest
```

This installs a binary named `github-tui` into your Go bin directory (ensure
`$(go env GOPATH)/bin` is on your `PATH`). Run it with:

```sh
github-tui
```

### From source

```sh
git clone https://github.com/huy-tran/github-tui
cd github-tui
go build -o github-tui .   # or: go run .
```

On Windows, `install.ps1` builds a short-named `gh-tui` binary and copies it
onto your `PATH`:

```powershell
.\install.ps1
gh-tui
```

(There's also a `Makefile` with `make install`, `make build`, `make run`.)

## Usage

```sh
github-tui                 # launch
github-tui --theme=light   # force the light theme (dark | light | auto)
github-tui --version       # print version
```

The theme can also be set with the `GITHUB_TUI_THEME` environment variable.

### Global keys

| Key | Action |
| --- | --- |
| `↑`/`k` `↓`/`j`, `pgup`/`pgdn`, `g`/`G` | Navigate / scroll tables |
| `/` | Fuzzy-filter the current table |
| `s` | Sort (then a digit `1..N` or a column's first letter; re-pick flips order) |
| `enter` | Drill into a detail screen (where one exists) |
| `ctrl+o` | Open the current selection in the browser |
| `ctrl+f` | Refresh the current screen |
| `ctrl+k` | Command palette (go to a repo / screen) |
| `?` | Toggle the help overlay |
| `esc` | Back |
| `q` / `ctrl+c` | Quit |

### Repositories

| Key | Action |
| --- | --- |
| `enter` | Open the repo detail |
| `p` | My PRs dashboard · `n` Notifications inbox |
| `v` | Re-scan the Dependabot alert columns (cached otherwise) |

### Repo detail (tabbed: `1`/`2`/`3`/`4` or `tab`)

- **PRs** - `enter` view detail; `t` toggle the "awaiting my review" filter.
  In a PR: `tab` switch Info/Diff/Conversation, `ctrl+a` approve, `ctrl+r`
  request-changes/comment, `ctrl+y` merge, `ctrl+x` close, `ctrl+o` browser.
- **Workflows** - `enter` view a run's jobs/steps; `enter`/`ctrl+l` a job's logs;
  `ctrl+r` re-run, `ctrl+x` cancel. A `● live` footer badge shows auto-refresh
  while a run is in progress.
- **Issues** - `enter` view detail; `ctrl+r` comment, `ctrl+x` close.
- **Security** - open Dependabot / code-scanning / secret-scanning alerts,
  most severe first; `ctrl+o` opens the advisory. Always fetched live.

### Notifications

`enter` opens a PR notification's detail; `x` mark read; `A` mark all read;
`f` cycle the reason filter; `m` load more; `ctrl+o` open any item in the browser.

## How it works

github-tui is a thin, read-mostly UI over the `gh` CLI and GitHub REST/GraphQL
APIs. It never stores your credentials - it invokes `gh`, which holds your auth.

Two things are cached on disk under your user cache dir (`gh-tui/`):

- the **repository list** (so the first screen is instant; refreshed in the
  background), and
- the **per-repo Dependabot alert counts** shown on the repo table (re-scanned
  on demand with `v`).

Mutating actions (approve, merge, close, comment, re-run, cancel, mark-read) are
always confirmed with a prompt and run live against GitHub.

## Develop

```sh
go test ./...     # unit/render tests (no network required)
go vet ./...
gofmt -l .        # should print nothing
```

### Layout

```
main.go                    entry point, theme flag, gh preflight
internal/gh/               gh CLI wrapper + GitHub API types
internal/cache/            on-disk caches (repos, vuln counts)
internal/ui/               Bubble Tea models
  model.go                 root model: screen routing + chrome
  datatable.go             bordered, filterable, sortable table component
  repos.go  detail.go  prdetail.go  rundetail.go  issuedetail.go
  myprs.go  notifs.go  palette.go  help.go  statusbar.go  theme.go  styles.go
```

## Contributing

Issues and PRs welcome. Please run `gofmt`, `go vet`, and `go test ./...` before
opening a PR.

## License

[MIT](LICENSE) © Huy Tran
