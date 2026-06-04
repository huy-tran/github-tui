package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/huy-tran/github-tui/internal/gh"
)

type dispatchStage int

const (
	dispatchPick dispatchStage = iota // choosing a workflow
	dispatchRef                       // entering the ref, then run
)

// dispatchModel is the "run a workflow" form shown over the Workflows tab: pick
// a workflow, choose a ref, and trigger a workflow_dispatch run.
type dispatchModel struct {
	active  bool
	repo    string
	theme   Theme
	loading bool
	err     error // failed to load the workflow list
	working bool  // dispatch request in flight
	msg     string
	msgErr  bool

	workflows []gh.Workflow
	cursor    int
	stage     dispatchStage
	ref       textinput.Model

	width  int
	height int
}

func newDispatchModel(theme Theme) dispatchModel {
	ti := textinput.New()
	ti.Prompt = ""
	ti.PromptStyle = accentStyle
	ti.Cursor.Style = accentStyle
	ti.Width = 32
	return dispatchModel{theme: theme, ref: ti}
}

func (m *dispatchModel) setSize(w, h int) { m.width, m.height = w, h }

// open starts the form for a repo and returns the command that loads its
// workflows + default branch.
func (m *dispatchModel) open(repo string) tea.Cmd {
	*m = dispatchModel{theme: m.theme, ref: m.ref, width: m.width, height: m.height}
	m.active = true
	m.repo = repo
	m.loading = true
	m.ref.SetValue("")
	return loadDispatchInfoCmd(repo)
}

func (m *dispatchModel) cancel() {
	m.active = false
	m.ref.Blur()
}

func (m *dispatchModel) setInfo(workflows []gh.Workflow, defaultBranch string, err error) {
	m.loading = false
	if err != nil {
		m.err = err
		return
	}
	m.workflows = workflows
	m.ref.SetValue(defaultBranch)
}

// finish records the dispatch result; on success it closes the form (the caller
// refreshes the runs list), on failure it shows the error in place.
func (m *dispatchModel) finish(err error) {
	m.working = false
	if err != nil {
		m.msg = "dispatch failed: " + firstLine(err.Error())
		m.msgErr = true
		return
	}
	m.active = false
}

func (m *dispatchModel) selectedWorkflow() (gh.Workflow, bool) {
	if m.cursor < 0 || m.cursor >= len(m.workflows) {
		return gh.Workflow{}, false
	}
	return m.workflows[m.cursor], true
}

func (m *dispatchModel) Update(km tea.KeyMsg) tea.Cmd {
	switch {
	case m.loading:
		if km.String() == "esc" {
			m.cancel()
		}
		return nil
	case m.err != nil, len(m.workflows) == 0:
		m.cancel() // any key dismisses the error / empty state
		return nil
	}

	if m.stage == dispatchPick {
		switch km.String() {
		case "esc":
			m.cancel()
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.workflows)-1 {
				m.cursor++
			}
		case "enter":
			if _, ok := m.selectedWorkflow(); ok {
				m.stage = dispatchRef
				m.ref.Focus()
				return textinput.Blink
			}
		}
		return nil
	}

	// dispatchRef
	switch km.String() {
	case "esc":
		m.stage = dispatchPick
		m.ref.Blur()
		m.msg = ""
		return nil
	case "enter":
		wf, ok := m.selectedWorkflow()
		ref := strings.TrimSpace(m.ref.Value())
		if !ok || ref == "" || m.working {
			return nil
		}
		m.working = true
		m.msg = ""
		return dispatchWorkflowCmd(m.repo, wf.ID, wf.Name, ref)
	default:
		var cmd tea.Cmd
		m.ref, cmd = m.ref.Update(km)
		return cmd
	}
}

// View renders the form centered in the body area.
func (m *dispatchModel) View() string {
	muted := mutedStyleFor(m.theme)
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorOverlay).Render("Run a workflow") + "\n\n")

	switch {
	case m.loading:
		b.WriteString(muted.Render("loading workflows…"))
	case m.err != nil:
		b.WriteString(errorStyle.Render("Failed to load workflows: "+firstLine(m.err.Error())) +
			"\n\n" + muted.Render("press any key to close"))
	case len(m.workflows) == 0:
		b.WriteString(muted.Render("No workflows in this repository.") + "\n\n" + muted.Render("press any key to close"))
	case m.working:
		b.WriteString(muted.Render("dispatching…"))
	case m.stage == dispatchPick:
		b.WriteString(m.workflowList(muted))
		b.WriteString("\n" + muted.Render("↑↓ select · enter next · esc cancel"))
	default: // dispatchRef
		wf, _ := m.selectedWorkflow()
		b.WriteString("Workflow:  " + lipgloss.NewStyle().Bold(true).Render(wf.Name) + "\n")
		b.WriteString("Ref:       " + m.ref.View() + "\n")
		if m.msg != "" {
			style := lipgloss.NewStyle().Foreground(colorGreen)
			if m.msgErr {
				style = errorStyle
			}
			b.WriteString("\n" + style.Render(m.msg))
		}
		b.WriteString("\n\n" + muted.Render("enter run · esc back"))
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorOverlay).
		Padding(1, 2).
		Render(b.String())
	return lipgloss.Place(maxInt(m.width, 1), maxInt(m.height, 1), lipgloss.Center, lipgloss.Center, box)
}

// workflowList renders a windowed, cursor-highlighted list of workflows.
func (m *dispatchModel) workflowList(muted lipgloss.Style) string {
	const maxRows = 10
	start := 0
	if m.cursor >= maxRows {
		start = m.cursor - maxRows + 1
	}
	end := minInt(start+maxRows, len(m.workflows))

	var b strings.Builder
	for i := start; i < end; i++ {
		name := truncateToWidth(m.workflows[i].Name, maxInt(minInt(48, m.width-8), 12))
		if i == m.cursor {
			b.WriteString(accentStyle.Render("› ") + lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render(name) + "\n")
		} else {
			b.WriteString("  " + name + "\n")
		}
	}
	if end < len(m.workflows) {
		b.WriteString(muted.Render("  …more\n"))
	}
	return b.String()
}
