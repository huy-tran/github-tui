package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/huy-tran/github-tui/internal/gh"
)

// scopedRepoLimit caps how many non-pinned repos the scoped view keeps (the
// most recently active ones). Pinned repos are always shown on top of this.
const scopedRepoLimit = 50

// reposModel is the first screen: a filterable, sortable repo table.
type reposModel struct {
	table DataTable
	repos []gh.Repo // loaded set, source order (by activity)

	pins       []string        // pinned "owner/name", in pin order (persisted)
	pinSet     map[string]bool // membership lookup for pins
	pinsLoaded bool            // pins have been read from disk
	showAll    bool            // true => full org-inclusive set; false => scoped

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

// setRepos applies authoritative network data. all reports whether this is the
// full "show all" set (vs the scoped owner/collaborator set).
func (m *reposModel) setRepos(repos []gh.Repo, all bool) {
	m.repos = repos
	m.showAll = all
	m.loading = false
	m.fresh = true
	m.refreshing = false
	m.lastLoaded = time.Now()
	m.rebuild()
}

// setPins stores the pinned-repo list (from disk) and refreshes ordering.
func (m *reposModel) setPins(pins []string) {
	m.pins = pins
	m.pinSet = make(map[string]bool, len(pins))
	for _, p := range pins {
		m.pinSet[p] = true
	}
	m.pinsLoaded = true
	m.rebuild()
}

// mergePinnedRepos folds in pinned repos fetched individually because they fell
// outside the scoped list, then refreshes.
func (m *reposModel) mergePinnedRepos(repos []gh.Repo) {
	have := make(map[string]bool, len(m.repos))
	for _, r := range m.repos {
		have[r.NameWithOwner] = true
	}
	for _, r := range repos {
		if !have[r.NameWithOwner] {
			m.repos = append(m.repos, r)
		}
	}
	m.rebuild()
}

// missingPins returns pinned repos absent from the currently loaded set, so the
// caller can fetch them individually.
func (m *reposModel) missingPins() []string {
	have := make(map[string]bool, len(m.repos))
	for _, r := range m.repos {
		have[r.NameWithOwner] = true
	}
	var missing []string
	for _, p := range m.pins {
		if !have[p] {
			missing = append(missing, p)
		}
	}
	return missing
}

// isPinned reports whether a repo is pinned.
func (m *reposModel) isPinned(nameWithOwner string) bool { return m.pinSet[nameWithOwner] }

// togglePin pins or unpins the selected repo and returns the new pin list (for
// persistence) plus whether anything changed.
func (m *reposModel) togglePin() ([]string, bool) {
	r, ok := m.selected()
	if !ok {
		return m.pins, false
	}
	nwo := r.NameWithOwner
	if m.pinSet[nwo] {
		delete(m.pinSet, nwo)
		out := m.pins[:0:0]
		for _, p := range m.pins {
			if p != nwo {
				out = append(out, p)
			}
		}
		m.pins = out
	} else {
		if m.pinSet == nil {
			m.pinSet = map[string]bool{}
		}
		m.pinSet[nwo] = true
		m.pins = append(m.pins, nwo)
	}
	m.rebuild()
	return m.pins, true
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

// rebuild refreshes the table rows from the loaded repo set: pinned repos are
// listed first, then the rest by activity; in the scoped view only the top
// scopedRepoLimit non-pinned repos are kept. The table applies its own fuzzy
// filter on top.
func (m *reposModel) rebuild() {
	display := m.displayRepos()
	muted := mutedStyleFor(m.theme)
	rows := make([][]string, len(display))
	keys := make([][]string, len(display))
	ids := make([]string, len(display))
	for i, r := range display {
		vis := "public"
		if r.IsPrivate {
			vis = muted.Render("private")
		}
		pinned := m.isPinned(r.NameWithOwner)
		name := r.NameWithOwner
		if pinned {
			name = pinStyle.Render("★ ") + name
		}
		vc := m.vulns[r.NameWithOwner]
		lc := m.authors[r.NameWithOwner]
		rows[i] = []string{
			name, vis,
			m.vulnCell(vc.Critical, colorRed, vc.Known),
			m.vulnCell(vc.High, colorRed, vc.Known),
			m.vulnCell(vc.Medium, colorAccent, vc.Known),
			m.vulnCell(vc.Low, colorYellow, vc.Known),
			m.authorCell(lc),
			humanizeTime(r.Activity()),
		}
		// Sort key: pinned repos sort ahead via a leading marker; the table's
		// default (unsorted) order already places them first.
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

// displayRepos returns the repos to show: pinned first (in activity order),
// then the rest by activity. In the scoped view the non-pinned tail is capped
// to scopedRepoLimit; "show all" keeps everything. m.repos is assumed to be in
// activity order already (the fetch sorts it).
func (m *reposModel) displayRepos() []gh.Repo {
	pinned := make([]gh.Repo, 0, len(m.pins))
	others := make([]gh.Repo, 0, len(m.repos))
	for _, r := range m.repos {
		if m.isPinned(r.NameWithOwner) {
			pinned = append(pinned, r)
		} else {
			others = append(others, r)
		}
	}
	if !m.showAll && len(others) > scopedRepoLimit {
		others = others[:scopedRepoLimit]
	}
	return append(pinned, others...)
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
	case m.showAll:
		msg = "showing all repos"
	default:
		msg = "scoped to owner/collaborator (a: show all)"
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
			{"*", "pin / unpin the selected repo (pinned repos always show)"},
			{"a", "toggle showing all repos vs owner/collaborator only"},
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
		"/ filter · enter open · p my PRs · n notifications · v scan vulns · c scan committers · * pin · a all · ctrl+f refresh · ? help") +
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
