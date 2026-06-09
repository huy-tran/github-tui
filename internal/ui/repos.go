package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/huy-tran/github-tui/internal/gh"
)

// reposModel is the first screen: a filterable, sortable repo table.
type reposModel struct {
	table DataTable
	repos []gh.Repo // full set, source order (by activity)

	vulns         map[string]gh.VulnCounts // per-repo alert counts (cached; rescanned on 'v')
	vulnsLoaded   bool                     // have counts (from cache or a scan)
	vulnsScanning bool                     // a live re-scan is in flight
	vulnsAt       time.Time                // when the shown counts were produced

	authors         map[string]gh.LastCommit // per-repo last committer (cached; rescanned on 'c')
	authorsLoaded   bool                     // have last-committer info (from cache or a scan)
	authorsScanning bool                     // a live re-scan is in flight
	authorsAt       time.Time                // when the shown info was produced

	loading    bool
	lastLoaded time.Time
	account    string
	theme      Theme

	fresh      bool      // network data has arrived (cache must not overwrite it)
	refreshing bool      // showing cached data while the network fetch runs
	cachedAt   time.Time // when the shown cache was saved

	width  int
	height int
}

func newReposModel(theme Theme) reposModel {
	cols := []Column{
		{Title: "Repository", Flex: true, Sort: SortString},
		{Title: "Visibility", Sort: SortString},
		{Title: "Crit", Align: lipgloss.Right, Sort: SortNumeric},
		{Title: "High", Align: lipgloss.Right, Sort: SortNumeric},
		{Title: "Med", Align: lipgloss.Right, Sort: SortNumeric},
		{Title: "Low", Align: lipgloss.Right, Sort: SortNumeric},
		{Title: "Last by", Sort: SortString},
		{Title: "Updated", Align: lipgloss.Right, Sort: SortTime},
	}
	t := NewDataTable(cols)
	t.SetTheme(theme)
	t.SetEmptyMessage("No repositories found.")
	return reposModel{table: t, loading: true, theme: theme}
}

func (m *reposModel) setSize(w, h int) {
	m.width, m.height = w, h
	m.table.SetSize(w, m.tableHeight())
}

// tableHeight reserves the top row for the filter/sort/hint line plus a blank
// spacer row, so the table never shifts when those toggle.
func (m *reposModel) tableHeight() int {
	if m.height-2 < 1 {
		return 1
	}
	return m.height - 2
}

// setRepos applies authoritative network data.
func (m *reposModel) setRepos(repos []gh.Repo) {
	m.repos = repos
	m.loading = false
	m.fresh = true
	m.refreshing = false
	m.lastLoaded = time.Now()
	m.rebuild()
}

// setReposFromCache shows cached data at startup. It is a no-op once fresh
// network data has arrived, so a late cache read can't clobber newer repos.
func (m *reposModel) setReposFromCache(repos []gh.Repo, savedAt time.Time) {
	if m.fresh || len(repos) == 0 {
		return
	}
	m.repos = repos
	m.loading = false
	m.refreshing = true
	m.cachedAt = savedAt
	m.rebuild()
}

func (m *reposModel) setAccount(login string) { m.account = login }

// setVulnCounts stores freshly-scanned alert counts and refreshes rows.
func (m *reposModel) setVulnCounts(counts map[string]gh.VulnCounts) {
	m.vulns = counts
	m.vulnsLoaded = true
	m.vulnsScanning = false
	m.vulnsAt = time.Now()
	m.rebuild()
}

// setVulnCountsFromCache applies cached counts at startup (no-op if empty).
func (m *reposModel) setVulnCountsFromCache(counts map[string]gh.VulnCounts, savedAt time.Time) {
	if len(counts) == 0 {
		return
	}
	m.vulns = counts
	m.vulnsLoaded = true
	m.vulnsAt = savedAt
	m.rebuild()
}

// beginVulnScan marks a live re-scan as starting.
func (m *reposModel) beginVulnScan() { m.vulnsScanning = true }

// setLastCommits stores freshly-scanned last-committer info and refreshes rows.
func (m *reposModel) setLastCommits(commits map[string]gh.LastCommit) {
	m.authors = commits
	m.authorsLoaded = true
	m.authorsScanning = false
	m.authorsAt = time.Now()
	m.rebuild()
}

// setLastCommitsFromCache applies cached last-committer info at startup (no-op
// if empty).
func (m *reposModel) setLastCommitsFromCache(commits map[string]gh.LastCommit, savedAt time.Time) {
	if len(commits) == 0 {
		return
	}
	m.authors = commits
	m.authorsLoaded = true
	m.authorsAt = savedAt
	m.rebuild()
}

// beginAuthorScan marks a live re-scan as starting.
func (m *reposModel) beginAuthorScan() { m.authorsScanning = true }

// rebuild refreshes the table rows from the full repo set (the table applies
// its own fuzzy filter on top).
func (m *reposModel) rebuild() {
	muted := mutedStyleFor(m.theme)
	rows := make([][]string, len(m.repos))
	keys := make([][]string, len(m.repos))
	ids := make([]string, len(m.repos))
	for i, r := range m.repos {
		vis := "public"
		if r.IsPrivate {
			vis = muted.Render("private")
		}
		vc := m.vulns[r.NameWithOwner]
		lc := m.authors[r.NameWithOwner]
		rows[i] = []string{
			r.NameWithOwner, vis,
			m.vulnCell(vc.Critical, colorRed, vc.Known),
			m.vulnCell(vc.High, colorRed, vc.Known),
			m.vulnCell(vc.Medium, colorAccent, vc.Known),
			m.vulnCell(vc.Low, colorYellow, vc.Known),
			m.authorCell(lc),
			humanizeTime(r.Activity()),
		}
		keys[i] = []string{
			r.NameWithOwner, vis,
			itoa(vc.Critical), itoa(vc.High), itoa(vc.Medium), itoa(vc.Low),
			lc.Author,
			r.Activity().Format(sortTimeLayout),
		}
		ids[i] = r.NameWithOwner
	}
	m.table.SetRows(rows, keys, ids)
	m.table.SetSize(m.width, m.tableHeight())
}

// vulnCell renders one severity count: "…" while scanning, "?" before any scan,
// "-" when unknown, a muted "·" for zero, and a colored number otherwise.
func (m *reposModel) vulnCell(n int, color lipgloss.Color, known bool) string {
	muted := mutedStyleFor(m.theme)
	switch {
	case m.vulnsScanning:
		return muted.Render("…")
	case !m.vulnsLoaded:
		return muted.Render("?")
	case !known:
		return muted.Render("-")
	case n == 0:
		return muted.Render("·")
	}
	return lipgloss.NewStyle().Foreground(color).Bold(true).Render(itoa(n))
}

// authorCell renders the last committer: "…" while scanning, "?" before any
// scan, a muted "-" when unknown, and the login/name otherwise.
func (m *reposModel) authorCell(lc gh.LastCommit) string {
	muted := mutedStyleFor(m.theme)
	switch {
	case m.authorsScanning:
		return muted.Render("…")
	case !m.authorsLoaded:
		return muted.Render("?")
	case !lc.Known || lc.Author == "":
		return muted.Render("-")
	}
	return lc.Author
}

// selected returns the highlighted repo, resolving through the table id.
func (m *reposModel) selected() (gh.Repo, bool) {
	id := m.table.SelectedID()
	for _, r := range m.repos {
		if r.NameWithOwner == id {
			return r, true
		}
	}
	return gh.Repo{}, false
}

func (m *reposModel) Loading() (bool, string) { return m.loading, "repositories" }

func (m *reposModel) snapshot() Snapshot {
	msg := ""
	switch {
	case m.table.FilterActive():
		msg = "filter: " + m.table.filterInput.Value()
	case m.vulnsScanning:
		msg = "scanning vulnerabilities…"
	case m.authorsScanning:
		msg = "scanning last committers…"
	case m.refreshing:
		msg = "refreshing…"
	}
	items := -1
	if !m.loading {
		items = m.table.Len()
	}
	loaded := m.lastLoaded
	if !m.fresh && m.refreshing {
		loaded = m.cachedAt // "loaded <ago>" reflects the cache age until fresh data lands
	}
	return Snapshot{
		Profile:    m.account,
		View:       "repos",
		Items:      items,
		LastLoaded: loaded,
		Message:    msg,
	}
}

func (m *reposModel) helpSections() []helpSection {
	return []helpSection{{
		title: "Repositories",
		keys: []helpKey{
			{"↑/k ↓/j", "move"},
			{"g / G", "top / bottom"},
			{"/", "fuzzy filter"},
			{"enter", "open repo"},
			{"p", "my PRs dashboard"},
			{"n", "notifications inbox"},
			{"v", "re-scan vulnerability counts (cached otherwise)"},
			{"c", "re-scan last committers (cached otherwise)"},
		},
	}}
}

func (m *reposModel) Update(msg tea.Msg) tea.Cmd {
	cmd, _ := m.table.Update(msg)
	return cmd
}

func (m *reposModel) View() string {
	if m.loading {
		return "" // root renders the spinner
	}
	return topLineFor(&m.table, m.width, m.theme,
		"/ filter · enter open · p my PRs · n notifications · v scan vulns · c scan committers · ctrl+f refresh · ? help") +
		"\n\n" + m.table.View()
}

// topLineFor renders a list screen's top row: the sort ribbon when sorting, the
// filter input when filtering/filtered, otherwise the provided hint.
func topLineFor(t *DataTable, width int, theme Theme, hint string) string {
	switch {
	case t.Sorting():
		return truncateToWidth(t.sortRibbon(), width)
	case t.Filtering() || t.FilterActive():
		return truncateToWidth(t.FilterView(), width)
	default:
		return truncateToWidth(mutedStyleFor(theme).Render(hint), width)
	}
}
