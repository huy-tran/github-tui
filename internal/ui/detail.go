package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/huy-tran/github-tui/internal/gh"
)

type tab int

const (
	tabPRs tab = iota
	tabWorkflows
	tabIssues
	tabSecurity
)

var tabNames = []string{"PRs", "Workflows", "Issues", "Security"}

// detailHeaderLines is the fixed number of rows above the table (repo line,
// tab bar, sub-header). Keeping it constant stops the table from shifting.
const detailHeaderLines = 3

type detailModel struct {
	repo    gh.Repo
	login   string
	account string
	theme   Theme

	active tab

	prTable    DataTable
	runTable   DataTable
	issueTable DataTable
	secTable   DataTable

	dispatch dispatchModel // "run a workflow" form (Workflows tab)
	flash    string        // transient note (e.g. workflow dispatched)

	allPRs        []gh.PR
	runs          []gh.Run
	issues        []gh.Issue
	alerts        []gh.SecurityAlert
	secUnavail    []string
	onlyMyReviews bool

	loadingPRs    bool
	loadingRuns   bool
	loadingIssues bool
	loadingSec    bool
	prErr         error
	runErr        error
	issueErr      error
	secErr        error
	secDisabled   bool
	prLoaded      time.Time
	runLoaded     time.Time
	issueLoaded   time.Time
	secLoaded     time.Time

	width  int
	height int // body height (between title bar and footer)
}

func newDetailModel(repo gh.Repo, login, account string, theme Theme) detailModel {
	prCols := []Column{
		{Title: "#", Align: lipgloss.Right, Sort: SortNumeric},
		{Title: "Title", Flex: true, Sort: SortString},
		{Title: "Author", Sort: SortString},
		{Title: "Reviewers", Sort: SortString},
		{Title: "Updated", Align: lipgloss.Right, Sort: SortTime},
	}
	runCols := []Column{
		{Title: "Status", Sort: SortString},
		{Title: "Workflow", Flex: true, Sort: SortString},
		{Title: "Branch", Sort: SortString},
		{Title: "Event", Sort: SortString},
		{Title: "Created", Align: lipgloss.Right, Sort: SortTime},
	}
	issueCols := []Column{
		{Title: "#", Align: lipgloss.Right, Sort: SortNumeric},
		{Title: "Title", Flex: true, Sort: SortString},
		{Title: "Author", Sort: SortString},
		{Title: "Labels", Sort: SortString},
		{Title: "Updated", Align: lipgloss.Right, Sort: SortTime},
	}
	secCols := []Column{
		{Title: "Severity", Sort: SortNumeric},
		{Title: "Source", Sort: SortString},
		{Title: "Detail", Flex: true, Sort: SortString},
		{Title: "Location", Sort: SortString},
		{Title: "Created", Align: lipgloss.Right, Sort: SortTime},
	}
	pt := NewDataTable(prCols)
	pt.SetTheme(theme)
	rt := NewDataTable(runCols)
	rt.SetTheme(theme)
	it := NewDataTable(issueCols)
	it.SetTheme(theme)
	it.SetEmptyMessage("No open issues.")
	st := NewDataTable(secCols)
	st.SetTheme(theme)
	st.SetEmptyMessage("No open security alerts. ")

	return detailModel{
		repo:          repo,
		login:         login,
		account:       account,
		theme:         theme,
		active:        tabPRs,
		prTable:       pt,
		runTable:      rt,
		issueTable:    it,
		secTable:      st,
		dispatch:      newDispatchModel(theme),
		onlyMyReviews: true,
		loadingPRs:    true,
		loadingRuns:   true,
		loadingIssues: true,
		loadingSec:    true,
	}
}

func (m *detailModel) initCmd() tea.Cmd {
	return tea.Batch(
		loadPRsCmd(m.repo.NameWithOwner),
		loadRunsCmd(m.repo.NameWithOwner),
		loadIssuesCmd(m.repo.NameWithOwner),
		loadSecurityCmd(m.repo.NameWithOwner),
	)
}

func (m *detailModel) setSize(w, bodyH int) {
	m.width, m.height = w, bodyH
	th := m.tableHeight()
	m.prTable.SetSize(w, th)
	m.runTable.SetSize(w, th)
	m.issueTable.SetSize(w, th)
	m.secTable.SetSize(w, th)
	m.dispatch.setSize(w, th)
}

// busy reports states in which the root must not steal q/esc (sort ribbon,
// filter input, or the run-a-workflow form).
func (m *detailModel) busy() bool {
	return m.dispatch.active || m.activeTable().Sorting() || m.activeTable().Filtering()
}

func (m *detailModel) tableHeight() int {
	if m.height-detailHeaderLines-1 < 1 {
		return 1
	}
	return m.height - detailHeaderLines - 1 // -1 for the blank spacer before the table
}

func (m *detailModel) activeTable() *DataTable {
	switch m.active {
	case tabPRs:
		return &m.prTable
	case tabIssues:
		return &m.issueTable
	case tabSecurity:
		return &m.secTable
	default:
		return &m.runTable
	}
}

func (m *detailModel) setPRs(prs []gh.PR) {
	m.allPRs = prs
	m.loadingPRs = false
	m.prErr = nil
	m.prLoaded = time.Now()
	m.rebuildPRs()
}

func (m *detailModel) rebuildPRs() {
	muted := mutedStyleFor(m.theme)
	var rows, keys [][]string
	var ids []string
	for _, pr := range m.allPRs {
		if m.onlyMyReviews && m.login != "" && !pr.AwaitsReviewFrom(m.login) {
			continue
		}
		title := pr.Title
		if pr.IsDraft {
			title = muted.Render("(draft) ") + title
		}
		reviewers := strings.Join(pr.ReviewerLogins(), ", ")
		if reviewers == "" {
			reviewers = muted.Render("-")
		}
		rows = append(rows, []string{
			"#" + strconv.Itoa(pr.Number), title, "@" + pr.Author.Login,
			reviewers, humanizeTime(pr.UpdatedAt),
		})
		keys = append(keys, []string{
			strconv.Itoa(pr.Number), pr.Title, pr.Author.Login,
			reviewers, pr.UpdatedAt.Format(sortTimeLayout),
		})
		ids = append(ids, pr.URL)
	}
	if m.onlyMyReviews {
		m.prTable.SetEmptyMessage("No PRs awaiting your review. Press 't' to show all open PRs.")
	} else {
		m.prTable.SetEmptyMessage("No open pull requests.")
	}
	m.prTable.SetRows(rows, keys, ids)
	m.prTable.SetSize(m.width, m.tableHeight())
}

func (m *detailModel) setRuns(runs []gh.Run) {
	m.runs = runs
	m.loadingRuns = false
	m.runErr = nil
	m.runLoaded = time.Now()

	rows := make([][]string, len(runs))
	keys := make([][]string, len(runs))
	ids := make([]string, len(runs))
	for i, r := range runs {
		status := runStatusIcon(r.Status, r.Conclusion) + " " + runStatusCell(r.Status, r.Conclusion)
		name := r.WorkflowName
		if name == "" {
			name = r.DisplayTitle
		}
		rows[i] = []string{status, name, r.HeadBranch, r.Event, humanizeTime(r.CreatedAt)}
		statusKey := r.Conclusion
		if r.Status != "completed" {
			statusKey = r.Status
		}
		keys[i] = []string{statusKey, name, r.HeadBranch, r.Event, r.CreatedAt.Format(sortTimeLayout)}
		ids[i] = r.URL
	}
	m.runTable.SetEmptyMessage("No workflow runs found for this repository.")
	m.runTable.SetRows(rows, keys, ids)
	m.runTable.SetSize(m.width, m.tableHeight())
}

func (m *detailModel) setIssues(issues []gh.Issue) {
	m.issues = issues
	m.loadingIssues = false
	m.issueErr = nil
	m.issueLoaded = time.Now()

	muted := mutedStyleFor(m.theme)
	rows := make([][]string, len(issues))
	keys := make([][]string, len(issues))
	ids := make([]string, len(issues))
	for i, is := range issues {
		labels := make([]string, len(is.Labels))
		for j, l := range is.Labels {
			labels[j] = l.Name
		}
		labelStr := strings.Join(labels, ", ")
		if labelStr == "" {
			labelStr = muted.Render("-")
		}
		rows[i] = []string{
			"#" + strconv.Itoa(is.Number), is.Title, "@" + is.Author.Login,
			labelStr, humanizeTime(is.UpdatedAt),
		}
		keys[i] = []string{
			strconv.Itoa(is.Number), is.Title, is.Author.Login,
			strings.Join(labels, ", "), is.UpdatedAt.Format(sortTimeLayout),
		}
		ids[i] = is.URL
	}
	m.issueTable.SetRows(rows, keys, ids)
	m.issueTable.SetSize(m.width, m.tableHeight())
}

func (m *detailModel) setSecurity(alerts []gh.SecurityAlert, unavailable []string) {
	m.alerts = alerts
	m.secUnavail = unavailable
	m.loadingSec = false
	m.secErr = nil
	// Disabled only when every source was unavailable (nothing to show).
	m.secDisabled = len(alerts) == 0 && len(unavailable) >= 3
	m.secLoaded = time.Now()

	muted := mutedStyleFor(m.theme)
	rows := make([][]string, len(alerts))
	keys := make([][]string, len(alerts))
	ids := make([]string, len(alerts))
	for i, a := range alerts {
		loc := a.Location
		if loc == "" {
			loc = muted.Render("-")
		}
		rows[i] = []string{
			securitySeverityCell(a.Severity), securitySourceCell(a.Source),
			a.Detail, loc, humanizeTime(a.CreatedAt),
		}
		keys[i] = []string{
			strconv.Itoa(gh.SeverityRank(a.Severity)), a.Source, a.Detail,
			a.Location, a.CreatedAt.Format(sortTimeLayout),
		}
		ids[i] = a.HTMLURL
	}
	m.secTable.SetRows(rows, keys, ids)
	m.secTable.SetSize(m.width, m.tableHeight())
}

// securitySourceCell colors the alert source.
func securitySourceCell(source string) string {
	switch source {
	case "dependabot":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render("dependabot")
	case "code":
		return lipgloss.NewStyle().Foreground(colorOverlay).Render("code-scan")
	case "secret":
		return lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("secret")
	default:
		return source
	}
}

// securitySeverityCell renders severity, treating "high" secrets etc. uniformly.
func securitySeverityCell(sev string) string { return severityCell(sev) }

// selectedIssue resolves the highlighted issue via the table id (URL).
func (m *detailModel) selectedIssue() (gh.Issue, bool) {
	id := m.issueTable.SelectedID()
	for _, is := range m.issues {
		if is.URL == id {
			return is, true
		}
	}
	return gh.Issue{}, false
}

func (m *detailModel) Loading() (bool, string) {
	switch m.active {
	case tabPRs:
		return m.loadingPRs, "pull requests"
	case tabIssues:
		return m.loadingIssues, "issues"
	case tabSecurity:
		return m.loadingSec, "vulnerabilities"
	default:
		return m.loadingRuns, "workflow runs"
	}
}

func (m *detailModel) snapshot() Snapshot {
	items := -1
	var loaded time.Time
	switch m.active {
	case tabPRs:
		if !m.loadingPRs {
			items = m.prTable.Len()
		}
		loaded = m.prLoaded
	case tabIssues:
		if !m.loadingIssues {
			items = m.issueTable.Len()
		}
		loaded = m.issueLoaded
	case tabSecurity:
		if !m.loadingSec && !m.secDisabled {
			items = m.secTable.Len()
		}
		loaded = m.secLoaded
	default:
		if !m.loadingRuns {
			items = m.runTable.Len()
		}
		loaded = m.runLoaded
	}
	msg := ""
	if m.active == tabWorkflows {
		msg = m.flash
	}
	return Snapshot{
		Profile:    m.account,
		Region:     m.repo.NameWithOwner,
		View:       tabNames[m.active],
		Items:      items,
		LastLoaded: loaded,
		Message:    msg,
		Live:       m.active == tabWorkflows && !m.loadingRuns && m.hasActiveRun(),
	}
}

// dispatchDone records a workflow-dispatch result; on success it flashes a note
// and reloads the runs list so the new run appears.
func (m *detailModel) dispatchDone(name string, err error) tea.Cmd {
	m.dispatch.finish(err)
	if err != nil {
		return nil
	}
	m.flash = "dispatched '" + name + "' ✓"
	return loadRunsCmd(m.repo.NameWithOwner)
}

func (m *detailModel) helpSections() []helpSection {
	return []helpSection{
		{title: "PRs tab", keys: []helpKey{
			{"t", "toggle reviewer filter"},
			{"enter", "view PR details"},
			{"ctrl+o", "open PR in browser"},
		}},
		{title: "Workflows tab", keys: []helpKey{
			{"enter", "view run jobs & steps"},
			{"r", "run a workflow (workflow_dispatch)"},
			{"ctrl+o", "open run in browser"},
		}},
		{title: "Issues tab", keys: []helpKey{
			{"enter", "view issue (comment / close)"},
			{"ctrl+o", "open issue in browser"},
		}},
		{title: "Security tab", keys: []helpKey{
			{"ctrl+o", "open the advisory (Dependabot / code / secret)"},
		}},
		{title: "Table", keys: []helpKey{
			{"↑/k ↓/j", "move"},
			{"pgup/pgdn", "page"},
			{"g / G", "top / bottom"},
			{"s", "sort"},
		}},
	}
}

func (m *detailModel) Update(msg tea.Msg) tea.Cmd {
	if km, ok := msg.(tea.KeyMsg); ok {
		// The run-a-workflow form captures all keys while open.
		if m.dispatch.active {
			return m.dispatch.Update(km)
		}
		// While the sort ribbon or filter input is up, all keys belong to the table.
		at := m.activeTable()
		if !at.Sorting() && !at.Filtering() {
			switch km.String() {
			case "r":
				if m.active == tabWorkflows {
					return m.dispatch.open(m.repo.NameWithOwner)
				}
			case "tab", "right", "l":
				m.switchTab(1)
				return nil
			case "shift+tab", "left", "h":
				m.switchTab(-1)
				return nil
			case "1":
				m.active = tabPRs
				return nil
			case "2":
				m.active = tabWorkflows
				return nil
			case "3":
				m.active = tabIssues
				return nil
			case "4":
				m.active = tabSecurity
				return nil
			case "t":
				if m.active == tabPRs {
					m.onlyMyReviews = !m.onlyMyReviews
					m.rebuildPRs()
				}
				return nil
			case "enter":
				// Each tab drills into its own detail screen.
				switch m.active {
				case tabWorkflows:
					if run, ok := m.selectedRun(); ok {
						return func() tea.Msg { return openRunMsg{repo: m.repo.NameWithOwner, run: run} }
					}
				case tabIssues:
					if is, ok := m.selectedIssue(); ok {
						return func() tea.Msg { return openIssueMsg{repo: m.repo.NameWithOwner, issue: is} }
					}
				case tabSecurity:
					// No drill-in screen for an alert; ctrl+o opens the advisory.
					return nil
				default:
					if pr, ok := m.selectedPR(); ok {
						return func() tea.Msg { return openPRMsg{repo: m.repo.NameWithOwner, pr: pr} }
					}
				}
				return nil
			}
		}
	}

	cmd, _ := m.activeTable().Update(msg)
	return cmd
}

// selectedPR resolves the highlighted pull request via the table id (URL).
func (m *detailModel) selectedPR() (gh.PR, bool) {
	id := m.prTable.SelectedID()
	for _, pr := range m.allPRs {
		if pr.URL == id {
			return pr, true
		}
	}
	return gh.PR{}, false
}

// isActiveRunStatus reports whether a run status means "not finished" (so it's
// worth polling): queued, in_progress, waiting, requested, pending, ...
func isActiveRunStatus(status string) bool {
	return status != "" && !strings.EqualFold(status, "completed")
}

// hasActiveRun reports whether any loaded run is still running.
func (m *detailModel) hasActiveRun() bool {
	for _, r := range m.runs {
		if isActiveRunStatus(r.Status) {
			return true
		}
	}
	return false
}

// selectedRun resolves the highlighted workflow run via the table id (URL).
func (m *detailModel) selectedRun() (gh.Run, bool) {
	id := m.runTable.SelectedID()
	for _, r := range m.runs {
		if r.URL == id {
			return r, true
		}
	}
	return gh.Run{}, false
}

func (m *detailModel) switchTab(delta int) {
	n := len(tabNames)
	m.active = tab((int(m.active) + delta + n) % n)
}

// --- rendering ------------------------------------------------------------

func (m *detailModel) View() string {
	return m.header() + "\n\n" + m.body()
}

func (m *detailModel) header() string {
	repoLine := lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render(m.repo.NameWithOwner)

	tabActive := lipgloss.NewStyle().Bold(true).
		Foreground(colorTitleFg).Background(colorAccent).Padding(0, 2)
	tabInactive := lipgloss.NewStyle().Foreground(m.theme.MutedFg).Padding(0, 2)
	var tabs []string
	for i, name := range tabNames {
		if tab(i) == m.active {
			tabs = append(tabs, tabActive.Render(name))
		} else {
			tabs = append(tabs, tabInactive.Render(name))
		}
	}
	bar := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)

	lines := []string{repoLine, bar, m.subHeader()}
	for i, ln := range lines {
		lines[i] = truncateToWidth(ln, m.width)
	}
	return strings.Join(lines, "\n")
}

func (m *detailModel) subHeader() string {
	at := m.activeTable()
	if at.Sorting() {
		return at.sortRibbon()
	}
	if at.Filtering() || at.FilterActive() {
		return at.FilterView()
	}
	muted := mutedStyleFor(m.theme)
	switch m.active {
	case tabPRs:
		badge := badgeOff(m.theme, " reviewer filter: off ")
		if m.onlyMyReviews {
			badge = badgeOn(" reviewer filter: on ")
		}
		return badge + muted.Render(fmt.Sprintf("  %d of %d open · t toggle · enter details · / filter · ctrl+o browser · ? help", m.prTable.Len(), len(m.allPRs)))
	case tabIssues:
		return muted.Render(fmt.Sprintf("open issues · %d shown · enter details · / filter · ctrl+o browser · ctrl+f refresh", m.issueTable.Len()))
	case tabSecurity:
		if m.secDisabled {
			return muted.Render("security alerts")
		}
		line := fmt.Sprintf("security alerts · %d shown · ctrl+o advisory · / filter · s sort · ctrl+f refresh", m.secTable.Len())
		if len(m.secUnavail) > 0 {
			line += "  (" + strings.Join(m.secUnavail, ", ") + " unavailable)"
		}
		return muted.Render(line)
	default:
		return muted.Render(fmt.Sprintf("recent runs · %d shown · enter details · r run workflow · / filter · ctrl+o browser · ctrl+f refresh", m.runTable.Len()))
	}
}

func (m *detailModel) body() string {
	switch {
	case m.active == tabWorkflows && m.dispatch.active:
		return m.dispatch.View()
	case m.active == tabPRs && m.prErr != nil:
		return m.centered(errorStyle.Render("Failed to load PRs: " + m.prErr.Error()))
	case m.active == tabPRs && m.loadingPRs:
		return "" // root renders the spinner
	case m.active == tabPRs:
		return m.prTable.View() // header + centered empty message when no rows
	case m.active == tabWorkflows && m.runErr != nil:
		return m.centered(errorStyle.Render("Failed to load runs: " + m.runErr.Error()))
	case m.active == tabWorkflows && m.loadingRuns:
		return ""
	case m.active == tabWorkflows:
		return m.runTable.View()
	case m.active == tabIssues && m.issueErr != nil:
		return m.centered(errorStyle.Render("Failed to load issues: " + m.issueErr.Error()))
	case m.active == tabIssues && m.loadingIssues:
		return ""
	case m.active == tabIssues:
		return m.issueTable.View()
	case m.active == tabSecurity && m.secDisabled:
		return m.centered(mutedStyleFor(m.theme).Render("No security data - Dependabot, code & secret scanning are disabled or inaccessible."))
	case m.active == tabSecurity && m.secErr != nil:
		return m.centered(errorStyle.Render("Failed to load vulnerabilities: " + m.secErr.Error()))
	case m.active == tabSecurity && m.loadingSec:
		return ""
	default:
		return m.secTable.View()
	}
}

func (m *detailModel) centered(s string) string {
	w := maxInt(m.width, 1)
	// Wrap to width so long messages don't overflow the body.
	wrapped := lipgloss.NewStyle().Width(w).Align(lipgloss.Center).Render(s)
	return lipgloss.Place(w, m.tableHeight(), lipgloss.Center, lipgloss.Center, wrapped)
}

func badgeOn(s string) string {
	return lipgloss.NewStyle().Bold(true).
		Foreground(colorTitleFg).Background(colorGreen).Render(s)
}

func badgeOff(t Theme, s string) string {
	return lipgloss.NewStyle().Foreground(t.MutedFg).Render(s)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
