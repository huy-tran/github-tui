package ui

import (
	"sort"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/huy-tran/github-tui/internal/gh"
)

// myPRsModel is a cross-repo dashboard of pull requests that involve the
// current user: those awaiting their review and (optionally) those they
// authored.
type myPRsModel struct {
	table DataTable

	review   []gh.SearchPR
	authored []gh.SearchPR

	includeAuthored bool // toggle; default false => only review requests

	loading bool
	err     error
	loaded  time.Time

	account string
	theme   Theme

	width  int
	height int
}

func newMyPRsModel(theme Theme) myPRsModel {
	cols := []Column{
		{Title: "Repository", Sort: SortString},
		{Title: "#", Align: lipgloss.Right, Sort: SortNumeric},
		{Title: "Title", Flex: true, Sort: SortString},
		{Title: "Author", Sort: SortString},
		{Title: "Role", Sort: SortString},
		{Title: "Updated", Align: lipgloss.Right, Sort: SortTime},
	}
	t := NewDataTable(cols)
	t.SetTheme(theme)
	return myPRsModel{table: t, theme: theme}
}

func (m *myPRsModel) setSize(w, h int) {
	m.width, m.height = w, h
	m.table.SetSize(w, m.tableHeight())
}

func (m *myPRsModel) tableHeight() int {
	if m.height-2 < 1 {
		return 1
	}
	return m.height - 2
}

func (m *myPRsModel) setData(review, authored []gh.SearchPR) {
	m.review = review
	m.authored = authored
	m.loading = false
	m.err = nil
	m.loaded = time.Now()
	m.rebuild()
}

// prRow pairs a search result with the role it appears under.
type prRow struct {
	pr   gh.SearchPR
	role string
}

// visibleRows returns the PRs to show, newest first.
func (m *myPRsModel) visibleRows() []prRow {
	rows := make([]prRow, 0, len(m.review)+len(m.authored))
	for _, p := range m.review {
		rows = append(rows, prRow{pr: p, role: "review"})
	}
	if m.includeAuthored {
		for _, p := range m.authored {
			rows = append(rows, prRow{pr: p, role: "author"})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].pr.UpdatedAt.After(rows[j].pr.UpdatedAt)
	})
	return rows
}

func (m *myPRsModel) rebuild() {
	muted := mutedStyleFor(m.theme)
	items := m.visibleRows()

	rows := make([][]string, len(items))
	keys := make([][]string, len(items))
	ids := make([]string, len(items))
	for i, it := range items {
		p := it.pr
		title := p.Title
		if p.IsDraft {
			title = muted.Render("(draft) ") + title
		}
		rows[i] = []string{
			p.RepoName(), "#" + strconv.Itoa(p.Number), title,
			"@" + p.Author.Login, roleCell(it.role, muted), humanizeTime(p.UpdatedAt),
		}
		keys[i] = []string{
			p.RepoName(), strconv.Itoa(p.Number), p.Title,
			p.Author.Login, it.role, p.UpdatedAt.Format(sortTimeLayout),
		}
		ids[i] = p.URL
	}
	m.table.SetEmptyMessage(m.emptyMessage())
	m.table.SetRows(rows, keys, ids)
	m.table.SetSize(m.width, m.tableHeight())
}

func roleCell(role string, muted lipgloss.Style) string {
	if role == "review" {
		return lipgloss.NewStyle().Foreground(colorAccent).Render("review")
	}
	return muted.Render("author")
}

// selected resolves the highlighted row's repo + a PR stub for drill-in.
func (m *myPRsModel) selected() (string, gh.PR, bool) {
	id := m.table.SelectedID()
	all := append(append([]gh.SearchPR{}, m.review...), m.authored...)
	for _, p := range all {
		if p.URL == id {
			return p.RepoName(), gh.PR{Number: p.Number, Title: p.Title, URL: p.URL}, true
		}
	}
	return "", gh.PR{}, false
}

func (m *myPRsModel) toggleAuthored() {
	m.includeAuthored = !m.includeAuthored
	m.rebuild()
}

func (m *myPRsModel) Loading() (bool, string) { return m.loading, "your pull requests" }

func (m *myPRsModel) shownCount() int {
	n := len(m.review)
	if m.includeAuthored {
		n += len(m.authored)
	}
	return n
}

func (m *myPRsModel) snapshot() Snapshot {
	items := -1
	if !m.loading {
		items = m.shownCount()
	}
	return Snapshot{
		Profile:    m.account,
		View:       "my PRs",
		Items:      items,
		LastLoaded: m.loaded,
	}
}

func (m *myPRsModel) helpSections() []helpSection {
	return []helpSection{{
		title: "My PRs",
		keys: []helpKey{
			{"enter", "open PR detail"},
			{"t", "include PRs you authored"},
			{"ctrl+o", "open in browser"},
			{"ctrl+f", "refresh"},
			{"s", "sort"},
			{"esc", "back to repositories"},
		},
	}}
}

func (m *myPRsModel) Update(msg tea.Msg) tea.Cmd {
	cmd, _ := m.table.Update(msg)
	return cmd
}

func (m *myPRsModel) View() string {
	if m.loading {
		return "" // root renders the spinner
	}
	var topLine string
	switch {
	case m.table.Sorting():
		topLine = m.table.sortRibbon()
	case m.table.Filtering() || m.table.FilterActive():
		topLine = m.table.FilterView()
	default:
		topLine = m.statusLine()
	}
	body := m.table.View() // header + centered empty message when there are no rows
	if m.err != nil {
		body = m.centered(errorStyle.Render("Failed to load: " + m.err.Error()))
	}
	return truncateToWidth(topLine, m.width) + "\n\n" + body
}

func (m *myPRsModel) statusLine() string {
	muted := mutedStyleFor(m.theme)
	mode := "review requests"
	if m.includeAuthored {
		mode = "review requests + authored"
	}
	badge := lipgloss.NewStyle().Foreground(colorTitleFg).Background(colorAccent).Bold(true).
		Render(" " + mode + " ")
	return badge + muted.Render("  enter open · t toggle authored · / filter · s sort · ctrl+f refresh · esc back")
}

func (m *myPRsModel) emptyMessage() string {
	if !m.includeAuthored {
		return "No PRs awaiting your review. Press 't' to include PRs you authored."
	}
	return "No open PRs awaiting your review or authored by you."
}

func (m *myPRsModel) centered(s string) string {
	return lipgloss.Place(maxInt(m.width, 1), m.tableHeight(), lipgloss.Center, lipgloss.Center, s)
}
