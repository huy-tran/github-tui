# Changelog

All notable changes to this project are documented here. This project follows
[Semantic Versioning](https://semver.org/) (pre-1.0: minor/patch only).

## v0.0.16
- Repo list is now **scoped by default**: only repos you own or directly
  collaborate on, showing the 50 most recently active (plus any pinned). Avoids
  loading and scanning every accessible repo. Press `a` to toggle the full
  org-inclusive "show all" view.
- **Pin repos** with `*`: pinned repos get a ★, sort to the top, persist across
  sessions, and always show - even org repos outside the scoped filter.
- Vulnerability (`v`) and last-committer (`c`) scans now cover only the visible
  set, so cross-repo ranking reflects the repos you actually manage.

## v0.0.15
- Per-repo **Commits tab**: recent commits on the default branch (SHA, message,
  author, when), fuzzy-filterable and sortable; `ctrl+o` opens a commit on
  github.com.
- Repo list gains a **"Last by"** column showing who made each repo's most
  recent default-branch commit, fetched via batched GraphQL across all repos,
  cached to disk and re-scanned on demand with `c` (mirrors the `v` vuln scan).

## v0.0.14
- Absolute timestamps now append the local zone abbreviation (e.g.
  `Jun 2, 14:30 AEST`) so the time is unambiguous.

## v0.0.13
- Timestamps now switch from relative to an absolute date once an item is older
  than three days (e.g. `2d ago`, then `Jun 2, 14:30`; absolute dates show in
  local time, and prior years include the year). Applies across every list,
  detail header, and comment/review; the status-bar "loaded ... ago" stays
  relative since it tracks data freshness.

## v0.0.12
- **@-mention autocomplete** in the PR review composer (`ctrl+r`): typing `@`
  opens a picker of PR participants first, then the repo's mentionable users;
  `↑/↓` select, `tab`/`enter` insert, `esc` dismisses.

## v0.0.11
- Trigger workflows (`workflow_dispatch`) directly from the Workflows tab.

## v0.0.10
- Keybinding consistency: `ctrl+o` is the single "open in browser" key on every
  screen; `enter` only drills into a detail screen. On the Security tab and for
  non-PR notifications, `enter` is now a no-op (use `ctrl+o`).

## v0.0.9
- Unified **Security tab**: Dependabot + code-scanning + secret-scanning alerts
  merged into one severity-sorted view, with per-source "unavailable" handling.
- Long centered messages now wrap instead of overflowing the body width.

## v0.0.8
- Per-repo **Security tab** (Dependabot alerts), severity-colored.
- Security alerts paginate fully (no fixed limit).
- Repo table gains **Crit/High/Med/Low** vulnerability columns, fetched via
  batched GraphQL across all repos, cached to disk and re-scanned on demand
  with `v`.

## v0.0.7
- `/` fuzzy filter on every table (repos, PRs/Workflows/Issues, My PRs,
  Notifications), built into the shared table component.

## v0.0.6
- Layout breathing room between the title bar, hints, table, and footer.
- Notifications power-ups: mark-all-read (`A`), reason filter (`f`), load more (`m`).
- Command palette (`ctrl+k`) - fuzzy "go to" any repo or screen.

## v0.0.5
- **Issues** tab in the repo detail, plus an issue detail (comment, close).
- Merge moved to `ctrl+y` (Ctrl+M is indistinguishable from Enter in a terminal).

## v0.0.4
- Faster startup: the repo list is cached to disk and shown instantly while a
  fresh copy loads in the background.
- **PR conversation** view (reviews + comments) alongside Info and Diff.
- **Notifications inbox** (`n`).

## v0.0.3
- Cross-repo **My PRs** dashboard (`p`).

## v0.0.2
- `-v` / `--version` flags; version shown in the title bar.
- Repo list now includes every accessible repo (owned, org, collaborator).

## v0.0.1
- Initial release: repository list with fuzzy search; per-repo detail with PRs
  (reviewer filter) and Workflows tabs.
