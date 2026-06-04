package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/huy-tran/github-tui/internal/gh"
)

// notifsModel is the notification inbox: unread items across all repos.
type notifsModel struct {
	table DataTable

	notifs []gh.Notification
	page   int // highest page loaded (for "load more")

	reasonFilter   string // "" = all
	pendingMarkAll bool
	loadingMore    bool

	loading bool
	err     error
	loaded  time.Time
	message string // transient (e.g. mark-read errors)

	account string
	theme   Theme

	width  int
	height int
}

func newNotifsModel(theme Theme) notifsModel {
	cols := []Column{
		{Title: "Repository", Sort: SortString},
		{Title: "Type", Sort: SortString},
		{Title: "Reason", Sort: SortString},
		{Title: "Title", Flex: true, Sort: SortString},
		{Title: "Updated", Align: lipgloss.Right, Sort: SortTime},
	}
	t := NewDataTable(cols)
	t.SetTheme(theme)
	t.SetEmptyMessage("Inbox zero - no unread notifications. ")
	return notifsModel{table: t, theme: theme}
}

func (m *notifsModel) setSize(w, h int) {
	m.width, m.height = w, h
	m.table.SetSize(w, m.tableHeight())
}

func (m *notifsModel) tableHeight() int {
	if m.height-2 < 1 {
		return 1
	}
	return m.height - 2
}

// setData stores a loaded page: page 1 replaces, later pages append.
func (m *notifsModel) setData(notifs []gh.Notification, page int) {
	if page <= 1 {
		m.notifs = notifs
		m.page = 1
	} else {
		m.notifs = append(m.notifs, notifs...)
		m.page = page
	}
	m.loading = false
	m.loadingMore = false
	m.err = nil
	m.loaded = time.Now()
	m.rebuild()
}

// visible returns the notifications matching the active reason filter.
func (m *notifsModel) visible() []gh.Notification {
	if m.reasonFilter == "" {
		return m.notifs
	}
	out := make([]gh.Notification, 0, len(m.notifs))
	for _, n := range m.notifs {
		if n.Reason == m.reasonFilter {
			out = append(out, n)
		}
	}
	return out
}

// reasonCycle is the set of reason filters cycled through with 'f'.
var reasonCycle = []string{"", "review_requested", "mention", "assign", "ci_activity", "subscribed"}

func (m *notifsModel) cycleFilter() {
	cur := 0
	for i, r := range reasonCycle {
		if r == m.reasonFilter {
			cur = i
			break
		}
	}
	m.reasonFilter = reasonCycle[(cur+1)%len(reasonCycle)]
	m.rebuild()
}

func (m *notifsModel) rebuild() {
	muted := mutedStyleFor(m.theme)
	vis := m.visible()
	rows := make([][]string, len(vis))
	keys := make([][]string, len(vis))
	ids := make([]string, len(vis))
	for i, n := range vis {
		rows[i] = []string{
			n.RepoName(),
			notifTypeCell(n.Subject.Type),
			muted.Render(humanizeReason(n.Reason)),
			n.Subject.Title,
			humanizeDuration(n.UpdatedAt),
		}
		keys[i] = []string{
			n.RepoName(), n.Subject.Type, n.Reason, n.Subject.Title,
			n.UpdatedAt.Format(sortTimeLayout),
		}
		ids[i] = n.ID
	}
	if m.reasonFilter != "" {
		m.table.SetEmptyMessage("No '" + humanizeReason(m.reasonFilter) + "' notifications. Press 'f' to change the filter.")
	} else {
		m.table.SetEmptyMessage("Inbox zero - no unread notifications. ")
	}
	m.table.SetRows(rows, keys, ids)
	m.table.SetSize(m.width, m.tableHeight())
}

func notifTypeCell(t string) string {
	switch t {
	case "PullRequest":
		return "PR"
	case "Issue":
		return "issue"
	case "CheckSuite":
		return lipgloss.NewStyle().Foreground(colorYellow).Render("CI")
	case "Release":
		return "release"
	case "":
		return "-"
	default:
		return t
	}
}

// humanizeReason makes GitHub's notification reason readable.
func humanizeReason(r string) string {
	return strings.ReplaceAll(r, "_", " ")
}

// selected returns the highlighted notification.
func (m *notifsModel) selected() (gh.Notification, bool) {
	id := m.table.SelectedID()
	for _, n := range m.notifs {
		if n.ID == id {
			return n, true
		}
	}
	return gh.Notification{}, false
}

// removeByID drops a notification from the list (after marking it read).
func (m *notifsModel) removeByID(id string) {
	out := m.notifs[:0]
	for _, n := range m.notifs {
		if n.ID != id {
			out = append(out, n)
		}
	}
	m.notifs = out
	m.rebuild()
}

func (m *notifsModel) Loading() (bool, string) { return m.loading, "notifications" }

func (m *notifsModel) shownCount() int { return len(m.visible()) }

// markedAll clears the list after a successful mark-all-read, or flashes the error.
func (m *notifsModel) markedAll(err error) {
	if err != nil {
		m.message = "mark all read failed: " + firstLine(err.Error())
		return
	}
	m.notifs = nil
	m.message = "all marked read ✓"
	m.rebuild()
}

func (m *notifsModel) snapshot() Snapshot {
	items := -1
	if !m.loading {
		items = m.shownCount()
	}
	msg := m.message
	if m.loadingMore {
		msg = "loading more…"
	}
	return Snapshot{
		Profile:    m.account,
		View:       "notifications",
		Items:      items,
		LastLoaded: m.loaded,
		Message:    msg,
	}
}

func (m *notifsModel) helpSections() []helpSection {
	return []helpSection{{
		title: "Notifications",
		keys: []helpKey{
			{"enter", "open PR detail (PR notifications only)"},
			{"ctrl+o", "open in browser (any item)"},
			{"x", "mark as read"},
			{"A", "mark ALL read (confirm)"},
			{"f", "cycle reason filter"},
			{"m", "load more (next 50)"},
			{"ctrl+f", "refresh · s sort · esc back"},
		},
	}}
}

func (m *notifsModel) Update(msg tea.Msg) tea.Cmd {
	cmd, _ := m.table.Update(msg)
	return cmd
}

func (m *notifsModel) View() string {
	if m.loading {
		return "" // root renders the spinner
	}
	muted := mutedStyleFor(m.theme)
	var topLine string
	switch {
	case m.pendingMarkAll:
		topLine = accentStyle.Bold(true).Render(fmt.Sprintf("Mark all %d notifications read?", len(m.notifs))) +
			muted.Render("   [y] yes   [n] no")
	case m.table.Sorting():
		topLine = m.table.sortRibbon()
	case m.table.Filtering() || m.table.FilterActive():
		topLine = m.table.FilterView()
	default:
		label := "unread"
		if m.reasonFilter != "" {
			label = humanizeReason(m.reasonFilter)
		}
		badge := lipgloss.NewStyle().Foreground(colorTitleFg).Background(colorAccent).Bold(true).Render(" " + label + " ")
		topLine = badge + muted.Render("  enter PR · ctrl+o open · x read · A all · f filter · m more · esc back")
	}
	body := m.table.View()
	if m.err != nil {
		body = m.centered(errorStyle.Render("Failed to load notifications: " + m.err.Error()))
	}
	return truncateToWidth(topLine, m.width) + "\n\n" + body
}

func (m *notifsModel) centered(s string) string {
	return lipgloss.Place(maxInt(m.width, 1), m.tableHeight(), lipgloss.Center, lipgloss.Center, s)
}
