package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/huy-tran/github-tui/internal/gh"
)

const issueDetailHeaderLines = 3

// issueDetailModel shows a single issue (body + comments) with comment and
// close actions.
type issueDetailModel struct {
	repo    string
	issue   gh.Issue // summary from the list (header/url before load)
	account string
	theme   Theme

	detail  gh.IssueDetail
	loading bool
	err     error
	loaded  time.Time

	vp viewport.Model

	composing    bool
	input        textinput.Model
	pendingClose bool
	working      bool
	actionMsg    string
	actionErr    bool

	width  int
	height int
}

func newIssueDetailModel(repo string, issue gh.Issue, account string, theme Theme) issueDetailModel {
	ti := textinput.New()
	ti.Prompt = ""
	ti.PromptStyle = accentStyle
	ti.Cursor.Style = accentStyle
	return issueDetailModel{
		repo:    repo,
		issue:   issue,
		account: account,
		theme:   theme,
		vp:      viewport.New(0, 0),
		input:   ti,
		loading: true,
	}
}

func (m *issueDetailModel) initCmd() tea.Cmd {
	return loadIssueDetailCmd(m.repo, m.issue.Number)
}

func (m *issueDetailModel) setSize(w, bodyH int) {
	m.width, m.height = w, bodyH
	m.vp.Width = w
	m.vp.Height = m.vpHeight()
	m.input.Width = maxInt(10, w-30)
	m.vp.SetContent(m.renderBody())
}

func (m *issueDetailModel) vpHeight() int {
	if m.height-issueDetailHeaderLines-1 < 1 {
		return 1
	}
	return m.height - issueDetailHeaderLines - 1 // -1 for the blank spacer
}

func (m *issueDetailModel) setDetail(d gh.IssueDetail) {
	m.detail = d
	m.loading = false
	m.err = nil
	m.loaded = time.Now()
	m.vp.Width = m.width
	m.vp.Height = m.vpHeight()
	m.vp.SetContent(m.renderBody())
	m.vp.SetYOffset(0)
}

func (m *issueDetailModel) browserURL() string {
	if m.detail.URL != "" {
		return m.detail.URL
	}
	return m.issue.URL
}

func (m *issueDetailModel) isOpen() bool { return strings.EqualFold(m.detail.State, "OPEN") }

// capturing reports whether a prompt/composer holds the keyboard.
func (m *issueDetailModel) capturing() bool { return m.composing || m.pendingClose }

func (m *issueDetailModel) Loading() (bool, string) { return m.loading, "issue" }

func (m *issueDetailModel) snapshot() Snapshot {
	msg := m.actionMsg
	if m.working {
		msg = "working…"
	}
	return Snapshot{
		Profile:    m.account,
		Region:     m.repo,
		View:       "issue #" + strconv.Itoa(m.issue.Number),
		Items:      -1,
		LastLoaded: m.loaded,
		Message:    msg,
	}
}

func (m *issueDetailModel) helpSections() []helpSection {
	return []helpSection{{
		title: "Issue",
		keys: []helpKey{
			{"ctrl+r", "add a comment"},
			{"ctrl+x", "close the issue"},
			{"ctrl+o", "open in browser"},
			{"↑/k ↓/j", "scroll"},
			{"esc", "back to issues"},
		},
	}}
}

func (m *issueDetailModel) Update(msg tea.Msg) tea.Cmd {
	if km, ok := msg.(tea.KeyMsg); ok {
		if m.composing {
			return m.handleComposeKey(km, msg)
		}
		if m.pendingClose {
			return m.handleCloseKey(km)
		}
		switch km.String() {
		case "ctrl+r":
			m.composing = true
			m.input.Reset()
			m.input.Focus()
			return textinput.Blink
		case "ctrl+x":
			if !m.isOpen() {
				m.flash("issue is already closed", true)
				return nil
			}
			m.pendingClose = true
			return nil
		}
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return cmd
}

func (m *issueDetailModel) handleComposeKey(km tea.KeyMsg, raw tea.Msg) tea.Cmd {
	switch km.String() {
	case "esc":
		m.composing = false
		m.input.Blur()
		m.input.Reset()
		return nil
	case "enter":
		body := strings.TrimSpace(m.input.Value())
		if body == "" {
			return nil
		}
		m.composing = false
		m.input.Blur()
		m.working = true
		m.actionMsg = ""
		return commentIssueCmd(m.repo, m.issue.Number, body)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(raw)
	return cmd
}

func (m *issueDetailModel) handleCloseKey(km tea.KeyMsg) tea.Cmd {
	switch km.String() {
	case "y":
		m.pendingClose = false
		m.working = true
		m.actionMsg = ""
		return closeIssueCmd(m.repo, m.issue.Number)
	case "n", "esc":
		m.pendingClose = false
	}
	return nil
}

func (m *issueDetailModel) actionDone(action string, err error) tea.Cmd {
	m.working = false
	if err != nil {
		m.flash(action+" failed: "+firstLine(err.Error()), true)
		return nil
	}
	switch action {
	case "comment":
		m.flash("Comment posted ✓", false)
	case "close":
		m.flash("Issue closed ✓", false)
	}
	return loadIssueDetailCmd(m.repo, m.issue.Number) // refresh to reflect the change
}

func (m *issueDetailModel) flash(msg string, isErr bool) {
	m.actionMsg = msg
	m.actionErr = isErr
}

// --- rendering ------------------------------------------------------------

func (m *issueDetailModel) View() string {
	return m.header() + "\n\n" + m.body()
}

func (m *issueDetailModel) header() string {
	muted := mutedStyleFor(m.theme)
	num, title := m.issue.Number, m.issue.Title
	if m.detail.Number != 0 {
		num, title = m.detail.Number, m.detail.Title
	}
	line1 := lipgloss.NewStyle().Bold(true).Foreground(colorAccent).
		Render(fmt.Sprintf("%s  ·  #%d %s", m.repo, num, title))

	var line2 string
	if m.loading {
		line2 = muted.Render("loading issue…")
	} else {
		labels := make([]string, len(m.detail.Labels))
		for i, l := range m.detail.Labels {
			labels[i] = l.Name
		}
		labelStr := muted.Render("no labels")
		if len(labels) > 0 {
			labelStr = strings.Join(labels, ", ")
		}
		line2 = strings.Join([]string{
			issueStateBadge(m.detail.State),
			"@" + m.detail.Author.Login,
			labelStr,
			humanizeTime(m.detail.UpdatedAt),
		}, muted.Render("  ·  "))
	}

	line3 := m.promptLine(muted)

	lines := []string{line1, line2, line3}
	for i, ln := range lines {
		lines[i] = truncateToWidth(ln, m.width)
	}
	return strings.Join(lines, "\n")
}

func (m *issueDetailModel) promptLine(muted lipgloss.Style) string {
	switch {
	case m.composing:
		return accentStyle.Bold(true).Render("Comment: ") + m.input.View() + muted.Render("   enter submit · esc cancel")
	case m.pendingClose:
		return accentStyle.Bold(true).Render(fmt.Sprintf("Close issue #%d?", m.issue.Number)) +
			muted.Render("   [y] yes   [n] no")
	case m.actionMsg != "":
		if m.actionErr {
			return errorStyle.Render(m.actionMsg)
		}
		return lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(m.actionMsg)
	default:
		return muted.Render("ctrl+r comment  ·  ctrl+x close  ·  ctrl+o browser  ·  esc back  ·  ? help")
	}
}

func (m *issueDetailModel) body() string {
	switch {
	case m.err != nil:
		return m.centered(errorStyle.Render("Failed to load issue: " + m.err.Error()))
	case m.loading:
		return "" // root renders the spinner
	default:
		return m.vp.View()
	}
}

func (m *issueDetailModel) centered(s string) string {
	return lipgloss.Place(maxInt(m.width, 1), m.vpHeight(), lipgloss.Center, lipgloss.Center, s)
}

// renderBody builds the scrollable description + comment timeline.
func (m *issueDetailModel) renderBody() string {
	muted := mutedStyleFor(m.theme)
	head := func(s string) string { return lipgloss.NewStyle().Bold(true).Foreground(colorText).Render(s) }
	wrap := func(s string) string { return hardWrap(s, maxInt(m.vp.Width-2, 10)) }

	var b strings.Builder
	b.WriteString(head("Description") + "\n")
	if body := strings.TrimSpace(m.detail.Body); body != "" {
		b.WriteString(wrap(body))
	} else {
		b.WriteString(muted.Render("No description provided."))
	}
	b.WriteString("\n\n")

	b.WriteString(head(fmt.Sprintf("Comments (%d)", len(m.detail.Comments))) + "\n")
	if len(m.detail.Comments) == 0 {
		b.WriteString(muted.Render("No comments yet."))
		return b.String()
	}
	comments := append([]gh.Comment{}, m.detail.Comments...)
	sort.SliceStable(comments, func(i, j int) bool { return comments[i].CreatedAt.Before(comments[j].CreatedAt) })
	for i, c := range comments {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("@"+c.Author.Login) + muted.Render("  "+humanizeTime(c.CreatedAt)) + "\n")
		if body := strings.TrimSpace(c.Body); body != "" {
			b.WriteString(indentLines(wrap(body), "  ") + "\n")
		}
	}
	return b.String()
}

func issueStateBadge(state string) string {
	s := strings.ToUpper(state)
	bg := colorGreen
	if s == "CLOSED" {
		bg = colorOverlay
	}
	return lipgloss.NewStyle().Bold(true).Foreground(colorTitleFg).Background(bg).Render(" " + s + " ")
}
