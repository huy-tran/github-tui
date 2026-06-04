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

// prDetailHeaderLines is the fixed number of rows above the scroll viewport.
const prDetailHeaderLines = 4

type prView int

const (
	prViewInfo prView = iota
	prViewDiff
	prViewConversation
)

type prAction int

const (
	prActionNone prAction = iota
	prActionApprove
	prActionMerge
	prActionClose
)

// prDetailModel is the pull-request detail screen: an info view and a diff
// view, with approve/merge actions guarded by a confirmation prompt.
type prDetailModel struct {
	repo    string
	pr      gh.PR // summary from the list (header/url before load)
	account string
	theme   Theme

	detail  gh.PRDetail
	loading bool
	err     error
	loaded  time.Time

	view prView
	vp   viewport.Model

	diff        string
	diffLoaded  bool
	diffLoading bool
	diffErr     error

	pending   prAction // confirmation in progress
	working   bool     // an approve/merge request is in flight
	actionMsg string   // transient result/explanation shown in the footer
	actionErr bool

	// Review composer (request-changes / comment).
	composing     bool
	composeTyping bool // false = choosing kind, true = entering body
	composeKind   gh.ReviewKind
	input         textinput.Model

	width  int
	height int
}

func newPRDetailModel(repo string, pr gh.PR, account string, theme Theme) prDetailModel {
	vp := viewport.New(0, 0)
	ti := textinput.New()
	ti.Prompt = ""
	ti.PromptStyle = accentStyle
	ti.Cursor.Style = accentStyle
	return prDetailModel{
		repo:    repo,
		pr:      pr,
		account: account,
		theme:   theme,
		vp:      vp,
		input:   ti,
		loading: true,
	}
}

func (m *prDetailModel) initCmd() tea.Cmd {
	return loadPRDetailCmd(m.repo, m.pr.Number)
}

func (m *prDetailModel) setSize(w, bodyH int) {
	m.width, m.height = w, bodyH
	m.vp.Width = w
	m.vp.Height = m.vpHeight()
	m.input.Width = maxInt(10, w-46) // leave room for label + submit hint
	m.refreshContent()
}

func (m *prDetailModel) vpHeight() int {
	if m.height-prDetailHeaderLines-1 < 1 {
		return 1
	}
	return m.height - prDetailHeaderLines - 1 // -1 for the blank spacer
}

func (m *prDetailModel) setDetail(d gh.PRDetail) {
	m.detail = d
	m.loading = false
	m.err = nil
	m.loaded = time.Now()
	m.refreshContent()
}

func (m *prDetailModel) setDiff(diff string) {
	m.diff = diff
	m.diffLoaded = true
	m.diffLoading = false
	m.diffErr = nil
	if m.view == prViewDiff {
		m.refreshContent()
	}
}

// refreshContent rebuilds the viewport text for the active view.
func (m *prDetailModel) refreshContent() {
	m.vp.Width = m.width
	m.vp.Height = m.vpHeight()
	switch m.view {
	case prViewDiff:
		m.vp.SetContent(m.renderDiff())
	case prViewConversation:
		m.vp.SetContent(m.renderConversation())
	default:
		m.vp.SetContent(m.renderInfo())
	}
	m.vp.SetYOffset(0)
}

func (m *prDetailModel) browserURL() string {
	if m.detail.URL != "" {
		return m.detail.URL
	}
	return m.pr.URL
}

func (m *prDetailModel) Loading() (bool, string) { return m.loading, "pull request" }

func (m *prDetailModel) snapshot() Snapshot {
	view := "PR #" + strconv.Itoa(m.pr.Number)
	switch m.view {
	case prViewDiff:
		view += " · diff"
	case prViewConversation:
		view += " · conversation"
	}
	msg := m.actionMsg
	if m.working {
		msg = "working…"
	}
	return Snapshot{
		Profile:    m.account,
		Region:     m.repo,
		View:       view,
		Items:      -1,
		LastLoaded: m.loaded,
		Message:    msg,
	}
}

func (m *prDetailModel) helpSections() []helpSection {
	return []helpSection{{
		title: "Pull request",
		keys: []helpKey{
			{"tab / ← →", "switch Info / Diff / Conversation"},
			{"ctrl+d", "jump to diff"},
			{"ctrl+a", "approve"},
			{"ctrl+r", "request changes / comment"},
			{"ctrl+y", "merge (then pick method)"},
			{"ctrl+x", "close without merging"},
			{"ctrl+o", "open in browser"},
			{"↑/k ↓/j", "scroll"},
			{"esc", "back to PRs"},
		},
	}}
}

// confirming reports whether a confirmation prompt is showing.
func (m *prDetailModel) confirming() bool { return m.pending != prActionNone }

// capturing reports whether the screen is holding a confirmation prompt or the
// review composer, so the root forwards keys (incl. esc) instead of acting on
// them, and blocks the help overlay.
func (m *prDetailModel) capturing() bool { return m.confirming() || m.composing }

func (m *prDetailModel) Update(msg tea.Msg) tea.Cmd {
	if km, ok := msg.(tea.KeyMsg); ok {
		// The review composer captures all keys until submitted/cancelled.
		if m.composing {
			return m.handleComposeKey(km, msg)
		}
		// Confirmation prompt consumes keys until resolved.
		if m.pending != prActionNone {
			return m.handleConfirmKey(km)
		}
		switch km.String() {
		case "tab", "right":
			return m.cycleView(1)
		case "shift+tab", "left":
			return m.cycleView(-1)
		case "ctrl+d":
			return m.toggleDiff()
		case "ctrl+a":
			m.beginApprove()
			return nil
		case "ctrl+r":
			m.beginReview()
			return nil
		case "ctrl+y":
			// Not ctrl+m: terminals send carriage return for Ctrl+M, which is
			// indistinguishable from Enter. ctrl+y is a distinct control code.
			m.beginMerge()
			return nil
		case "ctrl+x":
			m.beginClose()
			return nil
		}
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return cmd
}

// cycleView moves through Info → Diff → Conversation, fetching the diff lazily.
func (m *prDetailModel) cycleView(delta int) tea.Cmd {
	const n = 3
	m.view = prView((int(m.view) + delta + n) % n)
	return m.enterView()
}

// toggleDiff jumps to the diff view, or back to info from it.
func (m *prDetailModel) toggleDiff() tea.Cmd {
	if m.view == prViewDiff {
		m.view = prViewInfo
	} else {
		m.view = prViewDiff
	}
	return m.enterView()
}

// enterView refreshes content for the current view, triggering a diff fetch the
// first time the diff view is shown.
func (m *prDetailModel) enterView() tea.Cmd {
	if m.view == prViewDiff && !m.diffLoaded && !m.diffLoading {
		m.diffLoading = true
		m.refreshContent()
		return loadPRDiffCmd(m.repo, m.pr.Number)
	}
	m.refreshContent()
	return nil
}

// renderConversation builds a chronological timeline of reviews and comments.
func (m *prDetailModel) renderConversation() string {
	muted := mutedStyleFor(m.theme)

	type entry struct {
		when time.Time
		head string
		body string
	}
	var entries []entry
	for _, r := range m.detail.Reviews {
		entries = append(entries, entry{
			when: r.SubmittedAt,
			head: reviewStateText(r.State) + "  @" + r.Author.Login + muted.Render("  "+freshness(r.SubmittedAt)),
			body: strings.TrimSpace(r.Body),
		})
	}
	for _, c := range m.detail.Comments {
		entries = append(entries, entry{
			when: c.CreatedAt,
			head: lipgloss.NewStyle().Bold(true).Render("@"+c.Author.Login) + muted.Render("  commented  "+freshness(c.CreatedAt)),
			body: strings.TrimSpace(c.Body),
		})
	}
	if len(entries) == 0 {
		return muted.Render("No conversation yet.")
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].when.Before(entries[j].when) })

	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(e.head + "\n")
		if e.body != "" {
			b.WriteString(indentLines(hardWrap(e.body, maxInt(m.vp.Width-2, 10)), "  "))
			b.WriteString("\n")
		}
	}
	return b.String()
}

// indentLines prefixes every line of s with the given pad.
func indentLines(s, pad string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = pad + ln
	}
	return strings.Join(lines, "\n")
}

// beginApprove / beginMerge raise the confirmation prompt (or explain why not).
func (m *prDetailModel) beginApprove() {
	if !m.isOpen() {
		m.flash(fmt.Sprintf("PR is %s - cannot approve", m.stateText()), true)
		return
	}
	m.pending = prActionApprove
}

func (m *prDetailModel) beginMerge() {
	if !m.isOpen() {
		m.flash(fmt.Sprintf("PR is %s - cannot merge", m.stateText()), true)
		return
	}
	m.pending = prActionMerge
}

func (m *prDetailModel) beginClose() {
	if !m.isOpen() {
		m.flash(fmt.Sprintf("PR is already %s", m.stateText()), true)
		return
	}
	m.pending = prActionClose
}

func (m *prDetailModel) beginReview() {
	if !m.isOpen() {
		m.flash(fmt.Sprintf("PR is %s - cannot review", m.stateText()), true)
		return
	}
	m.composing = true
	m.composeTyping = false
	m.composeKind = ""
	m.input.Reset()
}

// handleComposeKey drives the review composer: first pick a kind, then type the
// body and submit with enter (esc cancels at any point).
func (m *prDetailModel) handleComposeKey(km tea.KeyMsg, raw tea.Msg) tea.Cmd {
	if km.String() == "esc" {
		m.cancelCompose()
		return nil
	}
	if !m.composeTyping {
		switch km.String() {
		case "r":
			m.composeKind = gh.ReviewRequestChanges
		case "c":
			m.composeKind = gh.ReviewComment
		default:
			return nil
		}
		m.composeTyping = true
		m.input.Reset()
		m.input.Focus()
		return textinput.Blink
	}
	// Typing the body.
	if km.String() == "enter" {
		body := strings.TrimSpace(m.input.Value())
		if body == "" {
			return nil // both kinds require a non-empty body
		}
		kind := m.composeKind
		m.cancelCompose()
		m.working = true
		m.actionMsg = ""
		return reviewPRCmd(m.repo, m.pr.Number, kind, body)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(raw)
	return cmd
}

func (m *prDetailModel) cancelCompose() {
	m.composing = false
	m.composeTyping = false
	m.input.Blur()
	m.input.Reset()
}

func (m *prDetailModel) handleConfirmKey(km tea.KeyMsg) tea.Cmd {
	s := km.String()
	if s == "esc" || s == "n" {
		m.pending = prActionNone
		return nil
	}
	switch m.pending {
	case prActionApprove:
		if s == "y" {
			m.pending = prActionNone
			m.working = true
			m.actionMsg = ""
			return approvePRCmd(m.repo, m.pr.Number)
		}
	case prActionClose:
		if s == "y" {
			m.pending = prActionNone
			m.working = true
			m.actionMsg = ""
			return closePRCmd(m.repo, m.pr.Number)
		}
	case prActionMerge:
		var method gh.MergeMethod
		switch s {
		case "m":
			method = gh.MergeCommit
		case "s":
			method = gh.MergeSquash
		case "r":
			method = gh.MergeRebase
		default:
			return nil
		}
		m.pending = prActionNone
		m.working = true
		m.actionMsg = ""
		return mergePRCmd(m.repo, m.pr.Number, method)
	}
	return nil
}

// actionDone records the result and re-fetches the PR to reflect new state.
func (m *prDetailModel) actionDone(action string, err error) tea.Cmd {
	m.working = false
	if err != nil {
		m.flash(action+" failed: "+firstLine(err.Error()), true)
		return nil
	}
	switch action {
	case "approve":
		m.flash("Approved ✓", false)
	case "merge":
		m.flash("Merged ✓", false)
	case "close":
		m.flash("Closed ✓", false)
	case string(gh.ReviewRequestChanges):
		m.flash("Changes requested ✓", false)
	case string(gh.ReviewComment):
		m.flash("Comment posted ✓", false)
	}
	return loadPRDetailCmd(m.repo, m.pr.Number) // refresh state silently
}

func (m *prDetailModel) flash(msg string, isErr bool) {
	m.actionMsg = msg
	m.actionErr = isErr
}

func (m *prDetailModel) isOpen() bool { return strings.EqualFold(m.detail.State, "OPEN") }

func (m *prDetailModel) stateText() string {
	if m.detail.State == "" {
		return "unknown"
	}
	return strings.ToLower(m.detail.State)
}

// --- rendering ------------------------------------------------------------

func (m *prDetailModel) View() string {
	return m.header() + "\n\n" + m.body()
}

func (m *prDetailModel) header() string {
	muted := mutedStyleFor(m.theme)
	num := m.pr.Number
	title := m.pr.Title
	if m.detail.Number != 0 {
		num = m.detail.Number
		title = m.detail.Title
	}
	line1 := lipgloss.NewStyle().Bold(true).Foreground(colorAccent).
		Render(fmt.Sprintf("%s  ·  #%d %s", m.repo, num, title))

	var line2 string
	if m.loading {
		line2 = muted.Render("loading pull request…")
	} else {
		adds := lipgloss.NewStyle().Foreground(colorGreen).Render("+" + strconv.Itoa(m.detail.Additions))
		dels := lipgloss.NewStyle().Foreground(colorRed).Render("-" + strconv.Itoa(m.detail.Deletions))
		line2 = strings.Join([]string{
			prStateBadge(m.detail),
			reviewDecisionText(m.detail.ReviewDecision),
			"@" + m.detail.Author.Login,
			m.detail.BaseRefName + " ← " + m.detail.HeadRefName,
			adds + " " + dels,
			fmt.Sprintf("%d files", m.detail.ChangedFiles),
			freshness(m.detail.UpdatedAt),
		}, muted.Render("  ·  "))
	}

	line3 := m.viewTabs(muted)
	line4 := m.promptLine(muted)

	lines := []string{line1, line2, line3, line4}
	for i, ln := range lines {
		lines[i] = truncateToWidth(ln, m.width)
	}
	return strings.Join(lines, "\n")
}

func (m *prDetailModel) viewTabs(muted lipgloss.Style) string {
	active := lipgloss.NewStyle().Bold(true).Foreground(colorTitleFg).Background(colorAccent).Padding(0, 1)
	inactive := lipgloss.NewStyle().Foreground(m.theme.MutedFg).Padding(0, 1)
	tab := func(label string, v prView) string {
		if m.view == v {
			return active.Render(label)
		}
		return inactive.Render(label)
	}
	tabs := tab("Info", prViewInfo) + " " + tab("Diff", prViewDiff) + " " + tab("Conversation", prViewConversation)
	scroll := ""
	if !m.loading {
		scroll = muted.Render(fmt.Sprintf("  %3.0f%%  (tab to switch)", m.vp.ScrollPercent()*100))
	}
	return tabs + scroll
}

// promptLine shows the review composer, a confirmation prompt, an action
// result, or the key hints.
func (m *prDetailModel) promptLine(muted lipgloss.Style) string {
	if m.composing {
		if !m.composeTyping {
			return accentStyle.Bold(true).Render(fmt.Sprintf("Review PR #%d:", m.pr.Number)) +
				muted.Render("   [r] request changes   [c] comment   esc cancel")
		}
		label := "Comment"
		if m.composeKind == gh.ReviewRequestChanges {
			label = "Request changes"
		}
		return accentStyle.Bold(true).Render(label+": ") + m.input.View() + muted.Render("   enter submit · esc cancel")
	}
	switch m.pending {
	case prActionApprove:
		return accentStyle.Bold(true).Render(fmt.Sprintf("Approve PR #%d?", m.pr.Number)) +
			muted.Render("   [y] yes   [n] no")
	case prActionMerge:
		return accentStyle.Bold(true).Render(fmt.Sprintf("Merge PR #%d?", m.pr.Number)) +
			muted.Render("   [m] merge commit   [s] squash   [r] rebase   esc cancel")
	case prActionClose:
		return accentStyle.Bold(true).Render(fmt.Sprintf("Close PR #%d without merging?", m.pr.Number)) +
			muted.Render("   [y] yes   [n] no")
	}
	if m.actionMsg != "" {
		if m.actionErr {
			return errorStyle.Render(m.actionMsg)
		}
		return lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(m.actionMsg)
	}
	return muted.Render("tab views  ·  ctrl+a approve  ·  ctrl+r review  ·  ctrl+y merge  ·  ctrl+x close  ·  ctrl+o browser  ·  esc back")
}

func (m *prDetailModel) body() string {
	switch {
	case m.err != nil:
		return m.centered(errorStyle.Render("Failed to load PR: " + m.err.Error()))
	case m.loading:
		return "" // root renders the spinner
	case m.view == prViewDiff && m.diffErr != nil:
		return m.centered(errorStyle.Render("Failed to load diff: " + m.diffErr.Error()))
	case m.view == prViewDiff && m.diffLoading:
		return m.centered(mutedStyleFor(m.theme).Render("loading diff…"))
	default:
		return m.vp.View()
	}
}

func (m *prDetailModel) centered(s string) string {
	return lipgloss.Place(maxInt(m.width, 1), m.vpHeight(), lipgloss.Center, lipgloss.Center, s)
}

// renderInfo builds the scrollable overview text.
func (m *prDetailModel) renderInfo() string {
	muted := mutedStyleFor(m.theme)
	head := func(s string) string { return lipgloss.NewStyle().Bold(true).Foreground(colorText).Render(s) }
	var b strings.Builder

	b.WriteString(head("Description"))
	b.WriteString("\n")
	body := strings.TrimSpace(m.detail.Body)
	if body == "" {
		b.WriteString(muted.Render("No description provided."))
	} else {
		b.WriteString(body)
	}
	b.WriteString("\n\n")

	// Checks.
	b.WriteString(head(fmt.Sprintf("Checks (%d)", len(m.detail.Checks))))
	b.WriteString("\n")
	if len(m.detail.Checks) == 0 {
		b.WriteString(muted.Render("No status checks."))
	} else {
		for _, c := range m.detail.Checks {
			b.WriteString("  " + checkIcon(c.Result()) + " " + c.DisplayName() + muted.Render("  "+c.Result()) + "\n")
		}
	}
	b.WriteString("\n\n")

	// Reviews.
	b.WriteString(head(fmt.Sprintf("Reviews (%d)", len(m.detail.Reviews))))
	b.WriteString("\n")
	if len(m.detail.Reviews) == 0 {
		b.WriteString(muted.Render("No reviews yet."))
	} else {
		for _, r := range m.detail.Reviews {
			line := "  " + reviewStateText(r.State) + " @" + r.Author.Login
			if rb := strings.TrimSpace(r.Body); rb != "" {
				line += muted.Render("  — " + firstLine(rb))
			}
			b.WriteString(line + "\n")
		}
	}
	b.WriteString("\n")

	// Files.
	b.WriteString(head(fmt.Sprintf("Files changed (%d)", len(m.detail.Files))))
	b.WriteString("\n")
	if len(m.detail.Files) == 0 {
		b.WriteString(muted.Render("No files."))
	} else {
		for _, f := range m.detail.Files {
			stat := lipgloss.NewStyle().Foreground(colorGreen).Render("+"+strconv.Itoa(f.Additions)) + " " +
				lipgloss.NewStyle().Foreground(colorRed).Render("-"+strconv.Itoa(f.Deletions))
			b.WriteString("  " + f.Path + muted.Render("  (") + stat + muted.Render(")") + "\n")
		}
	}
	return b.String()
}

// renderDiff colorizes a unified diff.
func (m *prDetailModel) renderDiff() string {
	if strings.TrimSpace(m.diff) == "" {
		return mutedStyleFor(m.theme).Render("(empty diff)")
	}
	add := lipgloss.NewStyle().Foreground(colorGreen)
	del := lipgloss.NewStyle().Foreground(colorRed)
	hunk := lipgloss.NewStyle().Foreground(colorOverlay)
	meta := lipgloss.NewStyle().Bold(true).Foreground(colorText)

	var b strings.Builder
	for _, ln := range strings.Split(m.diff, "\n") {
		switch {
		case strings.HasPrefix(ln, "+++") || strings.HasPrefix(ln, "---"):
			b.WriteString(meta.Render(ln))
		case strings.HasPrefix(ln, "diff ") || strings.HasPrefix(ln, "index "):
			b.WriteString(meta.Render(ln))
		case strings.HasPrefix(ln, "@@"):
			b.WriteString(hunk.Render(ln))
		case strings.HasPrefix(ln, "+"):
			b.WriteString(add.Render(ln))
		case strings.HasPrefix(ln, "-"):
			b.WriteString(del.Render(ln))
		default:
			b.WriteString(ln)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// --- small helpers --------------------------------------------------------

func prStateBadge(d gh.PRDetail) string {
	state := strings.ToUpper(d.State)
	label, bg := state, colorYellow
	switch {
	case d.IsDraft && state == "OPEN":
		label, bg = "DRAFT", colorYellow
	case state == "OPEN":
		bg = colorGreen
	case state == "MERGED":
		bg = colorOverlay
	case state == "CLOSED":
		bg = colorRed
	}
	return lipgloss.NewStyle().Bold(true).Foreground(colorTitleFg).Background(bg).Render(" " + label + " ")
}

func reviewDecisionText(d string) string {
	switch strings.ToUpper(d) {
	case "APPROVED":
		return lipgloss.NewStyle().Foreground(colorGreen).Render("approved")
	case "CHANGES_REQUESTED":
		return lipgloss.NewStyle().Foreground(colorRed).Render("changes requested")
	case "REVIEW_REQUIRED":
		return lipgloss.NewStyle().Foreground(colorYellow).Render("review required")
	case "":
		return lipgloss.NewStyle().Foreground(colorYellow).Render("no review")
	default:
		return strings.ToLower(d)
	}
}

func reviewStateText(s string) string {
	switch strings.ToUpper(s) {
	case "APPROVED":
		return lipgloss.NewStyle().Foreground(colorGreen).Render("✓ approved")
	case "CHANGES_REQUESTED":
		return lipgloss.NewStyle().Foreground(colorRed).Render("✗ changes requested")
	case "COMMENTED":
		return lipgloss.NewStyle().Foreground(colorText).Render("• commented")
	default:
		return strings.ToLower(s)
	}
}

func checkIcon(result string) string {
	switch result {
	case "success", "passing":
		return lipgloss.NewStyle().Foreground(colorGreen).Render("✓")
	case "failure", "error", "failing":
		return lipgloss.NewStyle().Foreground(colorRed).Render("✗")
	case "pending", "expected", "in_progress", "queued":
		return lipgloss.NewStyle().Foreground(colorYellow).Render("●")
	default:
		return lipgloss.NewStyle().Foreground(colorOverlay).Render("•")
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
