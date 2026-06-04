package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/huy-tran/github-tui/internal/gh"
)

// runDetailHeaderLines is the fixed number of rows above the body.
const runDetailHeaderLines = 3

type runView int

const (
	runViewJobs runView = iota
	runViewLogs
)

type runAction int

const (
	runActionNone runAction = iota
	runActionRerun
	runActionCancel
)

// runDetailModel shows jobs and steps for one workflow run, a per-job log
// viewer, and rerun/cancel actions guarded by a confirmation prompt.
type runDetailModel struct {
	repo    string
	run     gh.Run // summary from the list (header fallback + url before load)
	account string
	theme   Theme

	detail gh.RunDetail
	table  DataTable

	loading bool
	err     error
	loaded  time.Time

	view runView
	vp   viewport.Model

	logs        string
	logsJobID   int64
	logsJobName string
	logsLoaded  bool
	logsLoading bool
	logsErr     error

	pending   runAction
	working   bool
	actionMsg string
	actionErr bool

	width  int
	height int
}

func newRunDetailModel(repo string, run gh.Run, account string, theme Theme) runDetailModel {
	cols := []Column{
		{Title: "Job / Step", Flex: true, Sort: SortString},
		{Title: "Status", Sort: SortString},
		{Title: "Duration", Align: lipgloss.Right, Sort: SortNumeric},
		{Title: "Started", Align: lipgloss.Right, Sort: SortTime},
	}
	t := NewDataTable(cols)
	t.SetTheme(theme)
	t.SetEmptyMessage("This run has no jobs.")
	return runDetailModel{
		repo:    repo,
		run:     run,
		account: account,
		theme:   theme,
		table:   t,
		vp:      viewport.New(0, 0),
		loading: true,
	}
}

func (m *runDetailModel) initCmd() tea.Cmd {
	return loadRunDetailCmd(m.repo, m.run.DatabaseID)
}

func (m *runDetailModel) setSize(w, bodyH int) {
	m.width, m.height = w, bodyH
	m.table.SetSize(w, m.bodyH())
	m.vp.Width = w
	m.vp.Height = m.bodyH()
	if m.view == runViewLogs {
		m.vp.SetContent(m.renderLog())
	}
}

func (m *runDetailModel) bodyH() int {
	if m.height-runDetailHeaderLines-1 < 1 {
		return 1
	}
	return m.height - runDetailHeaderLines - 1 // -1 for the blank spacer
}

func (m *runDetailModel) setDetail(d gh.RunDetail) {
	m.detail = d
	m.loading = false
	m.err = nil
	m.loaded = time.Now()
	m.rebuild()
}

// rebuild flattens jobs and their steps into table rows. Each row's id is its
// parent job's database id, so selecting a step opens that job's logs.
func (m *runDetailModel) rebuild() {
	muted := mutedStyleFor(m.theme)
	var rows, keys [][]string
	var ids []string

	for _, job := range m.detail.Jobs {
		jobID := strconv.FormatInt(job.DatabaseID, 10)
		jobName := lipgloss.NewStyle().Bold(true).Render(job.Name)
		dispDur, secs := duration(job.StartedAt, job.CompletedAt)
		rows = append(rows, []string{
			jobName,
			runStatusIcon(job.Status, job.Conclusion) + " " + runStatusCell(job.Status, job.Conclusion),
			dispDur,
			humanizeDuration(job.StartedAt),
		})
		keys = append(keys, []string{
			job.Name, jobStatusKey(job.Status, job.Conclusion),
			strconv.Itoa(secs), job.StartedAt.Format(sortTimeLayout),
		})
		ids = append(ids, jobID)

		for _, st := range job.Steps {
			d2, s2 := duration(st.StartedAt, st.CompletedAt)
			rows = append(rows, []string{
				muted.Render("  └ " + st.Name),
				runStatusIcon(st.Status, st.Conclusion) + " " + runStatusCell(st.Status, st.Conclusion),
				d2,
				humanizeDuration(st.StartedAt),
			})
			keys = append(keys, []string{
				st.Name, jobStatusKey(st.Status, st.Conclusion),
				strconv.Itoa(s2), st.StartedAt.Format(sortTimeLayout),
			})
			ids = append(ids, jobID)
		}
	}
	m.table.SetRows(rows, keys, ids)
	m.table.SetSize(m.width, m.bodyH())
}

func jobStatusKey(status, conclusion string) string {
	if status != "completed" {
		return status
	}
	return conclusion
}

// browserURL is the run's web URL (prefers the loaded detail).
func (m *runDetailModel) browserURL() string {
	if m.detail.URL != "" {
		return m.detail.URL
	}
	return m.run.URL
}

func (m *runDetailModel) inLogs() bool     { return m.view == runViewLogs }
func (m *runDetailModel) confirming() bool { return m.pending != runActionNone }

// isActive reports whether the run is still running (worth auto-refreshing).
func (m *runDetailModel) isActive() bool { return isActiveRunStatus(m.detail.Status) }

// busy reports states in which the root should not steal q/esc.
func (m *runDetailModel) busy() bool { return m.table.Sorting() || m.confirming() }

func (m *runDetailModel) backToJobs() {
	m.view = runViewJobs
}

func (m *runDetailModel) Loading() (bool, string) { return m.loading, "workflow run" }

func (m *runDetailModel) snapshot() Snapshot {
	items := -1
	view := "run"
	if n := m.detail.Number; n > 0 {
		view = "run #" + strconv.Itoa(n)
	}
	if m.view == runViewLogs {
		view += " · logs"
	} else if !m.loading {
		items = len(m.detail.Jobs)
	}
	msg := m.actionMsg
	if m.working {
		msg = "working…"
	}
	return Snapshot{
		Profile:    m.account,
		Region:     m.repo,
		View:       view,
		Items:      items,
		LastLoaded: m.loaded,
		Message:    msg,
		Live:       !m.loading && m.isActive(),
	}
}

func (m *runDetailModel) helpSections() []helpSection {
	return []helpSection{{
		title: "Workflow run",
		keys: []helpKey{
			{"enter / ctrl+l", "view selected job's logs"},
			{"ctrl+r", "re-run (failed or all jobs)"},
			{"ctrl+x", "cancel an in-progress run"},
			{"ctrl+o", "open run in browser"},
			{"↑/k ↓/j", "move / scroll"},
			{"s", "sort"},
			{"esc", "back (logs → jobs → workflows)"},
		},
	}}
}

func (m *runDetailModel) Update(msg tea.Msg) tea.Cmd {
	km, isKey := msg.(tea.KeyMsg)
	if isKey {
		if m.pending != runActionNone {
			return m.handleConfirmKey(km)
		}
		if m.view == runViewLogs {
			switch km.String() {
			case "ctrl+l", "enter":
				m.backToJobs()
				return nil
			}
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return cmd
		}
		// Jobs view.
		switch km.String() {
		case "enter", "ctrl+l":
			return m.openLogs()
		case "ctrl+r":
			m.beginRerun()
			return nil
		case "ctrl+x":
			m.beginCancel()
			return nil
		}
		cmd, _ := m.table.Update(msg)
		return cmd
	}
	return nil
}

// selectedJobID returns the database id of the highlighted row's job.
func (m *runDetailModel) selectedJobID() int64 {
	id, _ := strconv.ParseInt(m.table.SelectedID(), 10, 64)
	return id
}

func (m *runDetailModel) jobName(id int64) string {
	for _, j := range m.detail.Jobs {
		if j.DatabaseID == id {
			return j.Name
		}
	}
	return "run"
}

func (m *runDetailModel) openLogs() tea.Cmd {
	jobID := m.selectedJobID()
	m.view = runViewLogs
	m.logsJobName = m.jobName(jobID)
	if m.logsLoaded && m.logsJobID == jobID {
		m.vp.SetContent(m.renderLog())
		m.vp.SetYOffset(0)
		return nil
	}
	m.logsJobID = jobID
	m.logsLoaded = false
	m.logsLoading = true
	m.logsErr = nil
	m.vp.SetContent("")
	return loadRunLogCmd(m.repo, m.run.DatabaseID, jobID)
}

func (m *runDetailModel) setLog(jobID int64, log string) {
	m.logsJobID = jobID
	m.logs = log
	m.logsLoaded = true
	m.logsLoading = false
	m.logsErr = nil
	if m.view == runViewLogs {
		m.vp.Width = m.width
		m.vp.Height = m.bodyH()
		m.vp.SetContent(m.renderLog())
		m.vp.SetYOffset(0)
	}
}

func (m *runDetailModel) beginRerun() {
	m.pending = runActionRerun
}

func (m *runDetailModel) beginCancel() {
	if strings.EqualFold(m.detail.Status, "completed") {
		m.flash("run already finished - nothing to cancel", true)
		return
	}
	m.pending = runActionCancel
}

func (m *runDetailModel) handleConfirmKey(km tea.KeyMsg) tea.Cmd {
	s := km.String()
	if s == "esc" || s == "n" {
		m.pending = runActionNone
		return nil
	}
	switch m.pending {
	case runActionRerun:
		switch s {
		case "f":
			m.pending = runActionNone
			m.working = true
			m.actionMsg = ""
			return rerunRunCmd(m.repo, m.run.DatabaseID, true)
		case "a":
			m.pending = runActionNone
			m.working = true
			m.actionMsg = ""
			return rerunRunCmd(m.repo, m.run.DatabaseID, false)
		}
	case runActionCancel:
		if s == "y" {
			m.pending = runActionNone
			m.working = true
			m.actionMsg = ""
			return cancelRunCmd(m.repo, m.run.DatabaseID)
		}
	}
	return nil
}

// actionDone records the result and re-fetches the run to reflect new state.
func (m *runDetailModel) actionDone(action string, err error) tea.Cmd {
	m.working = false
	if err != nil {
		m.flash(action+" failed: "+firstLine(err.Error()), true)
		return nil
	}
	switch action {
	case "rerun":
		m.flash("Re-run triggered ✓", false)
	case "cancel":
		m.flash("Cancellation requested ✓", false)
	}
	m.logsLoaded = false // logs will change; re-fetch on next open
	return loadRunDetailCmd(m.repo, m.run.DatabaseID)
}

func (m *runDetailModel) flash(msg string, isErr bool) {
	m.actionMsg = msg
	m.actionErr = isErr
}

// --- rendering ------------------------------------------------------------

func (m *runDetailModel) View() string {
	return m.header() + "\n\n" + m.body()
}

func (m *runDetailModel) header() string {
	muted := mutedStyleFor(m.theme)
	name := m.detail.WorkflowName
	if name == "" {
		name = m.run.WorkflowName
	}
	line1 := lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render(m.repo+"  ·  "+name) +
		logsSuffix(m.view, muted, m.logsJobName)

	var line2 string
	if m.loading {
		line2 = muted.Render("loading run details…")
	} else {
		status := runStatusIcon(m.detail.Status, m.detail.Conclusion) + " " +
			runStatusCell(m.detail.Status, m.detail.Conclusion)
		dispDur, _ := duration(m.detail.CreatedAt, m.detail.UpdatedAt)
		sha := m.detail.HeadSha
		if len(sha) > 7 {
			sha = sha[:7]
		}
		line2 = strings.Join([]string{
			status,
			"#" + strconv.Itoa(m.detail.Number),
			m.detail.HeadBranch,
			"on " + m.detail.Event,
			dispDur,
			freshness(m.detail.CreatedAt),
			sha,
		}, muted.Render("  ·  "))
	}

	line3 := m.promptLine(muted)

	lines := []string{line1, line2, line3}
	for i, ln := range lines {
		lines[i] = truncateToWidth(ln, m.width)
	}
	return strings.Join(lines, "\n")
}

func logsSuffix(v runView, muted lipgloss.Style, job string) string {
	if v == runViewLogs {
		return muted.Render("  ·  logs: ") + job
	}
	return ""
}

// promptLine shows the confirmation prompt, an action result, or key hints.
func (m *runDetailModel) promptLine(muted lipgloss.Style) string {
	switch m.pending {
	case runActionRerun:
		return accentStyle.Bold(true).Render("Re-run this run?") +
			muted.Render("   [f] failed jobs   [a] all jobs   esc cancel")
	case runActionCancel:
		return accentStyle.Bold(true).Render("Cancel this run?") +
			muted.Render("   [y] yes   [n] no")
	}
	if m.actionMsg != "" {
		if m.actionErr {
			return errorStyle.Render(m.actionMsg)
		}
		return lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(m.actionMsg)
	}
	if m.view == runViewLogs {
		return muted.Render("↑/↓ scroll  ·  ctrl+l/enter back to jobs  ·  ctrl+o browser  ·  esc back")
	}
	return muted.Render("enter logs  ·  ctrl+r rerun  ·  ctrl+x cancel  ·  ctrl+o browser  ·  ctrl+f refresh  ·  esc back")
}

func (m *runDetailModel) body() string {
	if m.view == runViewLogs {
		switch {
		case m.logsErr != nil:
			return m.centered(errorStyle.Render("Failed to load logs: " + m.logsErr.Error()))
		case m.logsLoading:
			return m.centered(mutedStyleFor(m.theme).Render("loading logs…"))
		default:
			return m.vp.View()
		}
	}
	switch {
	case m.err != nil:
		return m.centered(errorStyle.Render("Failed to load run: " + m.err.Error()))
	case m.loading:
		return "" // root renders the spinner
	default:
		return m.table.View() // header + centered empty message when no jobs
	}
}

// renderLog soft-wraps the log to the viewport width (tabs → spaces) so long
// lines don't overflow the layout.
func (m *runDetailModel) renderLog() string {
	if strings.TrimSpace(m.logs) == "" {
		return mutedStyleFor(m.theme).Render("(no logs)")
	}
	return hardWrap(strings.ReplaceAll(m.logs, "\t", "  "), m.vp.Width)
}

func (m *runDetailModel) centered(s string) string {
	return lipgloss.Place(maxInt(m.width, 1), m.bodyH(), lipgloss.Center, lipgloss.Center, s)
}

// hardWrap wraps each line of s to at most width runes.
func hardWrap(s string, width int) string {
	if width < 1 {
		width = 1
	}
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		r := []rune(line)
		for len(r) > width {
			b.WriteString(string(r[:width]))
			b.WriteByte('\n')
			r = r[width:]
		}
		b.WriteString(string(r))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
