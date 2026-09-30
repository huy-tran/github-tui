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
	dispatchPick   dispatchStage = iota // choosing a workflow
	dispatchRef                         // entering the ref
	dispatchInputs                      // filling workflow_dispatch inputs, then run
)

// dispatchField is one editable workflow_dispatch input in the form.
type dispatchField struct {
	input  gh.WorkflowInput
	text   textinput.Model // string / number / environment inputs
	choice int             // index into input.Options (choice inputs)
	on     bool            // boolean inputs
}

// isChoice reports whether the field cycles through fixed options.
func (f *dispatchField) isChoice() bool { return f.input.Type == "choice" && len(f.input.Options) > 0 }

func (f *dispatchField) isBool() bool { return f.input.Type == "boolean" }

// value returns the field's current value as it would be sent to gh.
func (f *dispatchField) value() string {
	switch {
	case f.isChoice():
		return f.input.Options[f.choice]
	case f.isBool():
		if f.on {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(f.text.Value())
	}
}

// dispatchModel is the "run a workflow" form shown over the Workflows tab: pick
// a workflow, choose a ref, fill any workflow_dispatch inputs, and trigger a run.
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

	loadingInputs bool // reading the workflow file for its inputs
	fields        []dispatchField
	field         int // focused field on the inputs stage

	width  int
	height int
}

func newDispatchModel(theme Theme) dispatchModel {
	return dispatchModel{theme: theme, ref: newDispatchTextInput(32)}
}

func newDispatchTextInput(width int) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.PromptStyle = accentStyle
	ti.Cursor.Style = accentStyle
	ti.Width = width
	return ti
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
	m.blurFields()
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

// setInputs receives the workflow's inputs for the ref entered. With no inputs
// the workflow is dispatched straight away; otherwise the inputs stage opens.
// A stale reply (different workflow or ref) is ignored.
func (m *dispatchModel) setInputs(msg dispatchInputsLoadedMsg) tea.Cmd {
	wf, ok := m.selectedWorkflow()
	if !m.active || !m.loadingInputs || !ok || wf.ID != msg.workflowID || msg.ref != strings.TrimSpace(m.ref.Value()) {
		return nil
	}
	m.loadingInputs = false
	if msg.err != nil {
		m.msg = "could not read workflow inputs: " + firstLine(msg.err.Error())
		m.msgErr = true
		return nil
	}
	if len(msg.inputs) == 0 {
		return m.dispatch(wf, msg.ref, nil)
	}
	m.fields = make([]dispatchField, 0, len(msg.inputs))
	for _, in := range msg.inputs {
		f := dispatchField{input: in}
		switch {
		case f.isChoice():
			for i, opt := range in.Options {
				if opt == in.Default {
					f.choice = i
				}
			}
		case f.isBool():
			f.on = strings.EqualFold(in.Default, "true")
		default:
			f.text = newDispatchTextInput(32)
			f.text.SetValue(in.Default)
		}
		m.fields = append(m.fields, f)
	}
	m.field = 0
	m.stage = dispatchInputs
	m.ref.Blur()
	return m.focusField()
}

// dispatch fires the workflow run and marks the form as working.
func (m *dispatchModel) dispatch(wf gh.Workflow, ref string, inputs []gh.DispatchInput) tea.Cmd {
	m.working = true
	m.msg = ""
	return dispatchWorkflowCmd(m.repo, wf.ID, wf.Name, ref, inputs)
}

// inputValues collects the values to send; empty optional inputs are omitted
// so GitHub applies their defaults. Returns the name of the first required
// input left empty, if any.
func (m *dispatchModel) inputValues() ([]gh.DispatchInput, string) {
	var out []gh.DispatchInput
	for i := range m.fields {
		f := &m.fields[i]
		v := f.value()
		if v == "" {
			if f.input.Required {
				return nil, f.input.Name
			}
			continue
		}
		out = append(out, gh.DispatchInput{Name: f.input.Name, Value: v})
	}
	return out, ""
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
	m.blurFields()
}

func (m *dispatchModel) selectedWorkflow() (gh.Workflow, bool) {
	if m.cursor < 0 || m.cursor >= len(m.workflows) {
		return gh.Workflow{}, false
	}
	return m.workflows[m.cursor], true
}

// focusField focuses the text input of the current field (if it has one) and
// blurs the rest.
func (m *dispatchModel) focusField() tea.Cmd {
	var cmd tea.Cmd
	for i := range m.fields {
		f := &m.fields[i]
		if f.isChoice() || f.isBool() {
			continue
		}
		if i == m.field {
			cmd = f.text.Focus()
		} else {
			f.text.Blur()
		}
	}
	return cmd
}

func (m *dispatchModel) blurFields() {
	for i := range m.fields {
		m.fields[i].text.Blur()
	}
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
	case m.working, m.loadingInputs:
		return nil
	}

	switch m.stage {
	case dispatchPick:
		return m.updatePick(km)
	case dispatchRef:
		return m.updateRef(km)
	default:
		return m.updateInputs(km)
	}
}

func (m *dispatchModel) updatePick(km tea.KeyMsg) tea.Cmd {
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

func (m *dispatchModel) updateRef(km tea.KeyMsg) tea.Cmd {
	switch km.String() {
	case "esc":
		m.stage = dispatchPick
		m.ref.Blur()
		m.msg = ""
		return nil
	case "enter":
		wf, ok := m.selectedWorkflow()
		ref := strings.TrimSpace(m.ref.Value())
		if !ok || ref == "" {
			return nil
		}
		m.loadingInputs = true
		m.msg = ""
		return loadDispatchInputsCmd(m.repo, wf, ref)
	default:
		var cmd tea.Cmd
		m.ref, cmd = m.ref.Update(km)
		return cmd
	}
}

func (m *dispatchModel) updateInputs(km tea.KeyMsg) tea.Cmd {
	f := &m.fields[m.field]
	switch km.String() {
	case "esc":
		m.stage = dispatchRef
		m.blurFields()
		m.msg = ""
		m.ref.Focus()
		return textinput.Blink
	case "enter":
		wf, ok := m.selectedWorkflow()
		if !ok {
			return nil
		}
		values, missing := m.inputValues()
		if missing != "" {
			m.msg = "'" + missing + "' is required"
			m.msgErr = true
			return nil
		}
		return m.dispatch(wf, strings.TrimSpace(m.ref.Value()), values)
	case "tab", "down":
		if m.field < len(m.fields)-1 {
			m.field++
		}
		return m.focusField()
	case "shift+tab", "up":
		if m.field > 0 {
			m.field--
		}
		return m.focusField()
	}

	switch {
	case f.isChoice():
		n := len(f.input.Options)
		switch km.String() {
		case "right", "l", " ":
			f.choice = (f.choice + 1) % n
		case "left", "h":
			f.choice = (f.choice + n - 1) % n
		}
		return nil
	case f.isBool():
		switch km.String() {
		case "right", "left", "l", "h", " ":
			f.on = !f.on
		}
		return nil
	default:
		var cmd tea.Cmd
		f.text, cmd = f.text.Update(km)
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
	case m.stage == dispatchRef:
		wf, _ := m.selectedWorkflow()
		b.WriteString("Workflow:  " + lipgloss.NewStyle().Bold(true).Render(wf.Name) + "\n")
		b.WriteString("Ref:       " + m.ref.View() + "\n")
		if m.loadingInputs {
			b.WriteString("\n" + muted.Render("reading workflow inputs…"))
		}
		b.WriteString(m.message())
		b.WriteString("\n\n" + muted.Render("enter next · esc back"))
	default: // dispatchInputs
		wf, _ := m.selectedWorkflow()
		b.WriteString("Workflow:  " + lipgloss.NewStyle().Bold(true).Render(wf.Name) + "\n")
		b.WriteString("Ref:       " + strings.TrimSpace(m.ref.Value()) + "\n\n")
		b.WriteString(m.inputsForm(muted))
		b.WriteString(m.message())
		b.WriteString("\n\n" + muted.Render("enter run · tab/↑↓ field · ←→ change · esc back"))
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorOverlay).
		Padding(1, 2).
		Render(b.String())
	return lipgloss.Place(maxInt(m.width, 1), maxInt(m.height, 1), lipgloss.Center, lipgloss.Center, box)
}

// message renders the status / error line, if any.
func (m *dispatchModel) message() string {
	if m.msg == "" {
		return ""
	}
	style := lipgloss.NewStyle().Foreground(colorGreen)
	if m.msgErr {
		style = errorStyle
	}
	return "\n" + style.Render(m.msg)
}

// inputsForm renders one row per workflow_dispatch input, with the focused
// row highlighted and its description shown beneath the list.
func (m *dispatchModel) inputsForm(muted lipgloss.Style) string {
	labelW := 0
	for i := range m.fields {
		if w := lipgloss.Width(m.fieldLabel(i)); w > labelW {
			labelW = w
		}
	}

	var b strings.Builder
	for i := range m.fields {
		f := &m.fields[i]
		label := m.fieldLabel(i)
		label += strings.Repeat(" ", labelW-lipgloss.Width(label))
		var val string
		switch {
		case f.isChoice():
			val = "‹ " + f.input.Options[f.choice] + " ›"
			if i == m.field {
				val = accentStyle.Render(val)
			}
		case f.isBool():
			val = "[ ] false"
			if f.on {
				val = "[x] true"
			}
			if i == m.field {
				val = accentStyle.Render(val)
			}
		default:
			val = f.text.View()
		}
		if i == m.field {
			b.WriteString(accentStyle.Render("› ") + lipgloss.NewStyle().Bold(true).Render(label) + "  " + val + "\n")
		} else {
			b.WriteString("  " + label + "  " + val + "\n")
		}
	}
	if d := m.fields[m.field].input.Description; d != "" {
		b.WriteString("\n" + muted.Render(truncateToWidth(d, maxInt(minInt(60, m.width-8), 12))))
	}
	return b.String()
}

// fieldLabel is the input's name, starred when required.
func (m *dispatchModel) fieldLabel(i int) string {
	f := &m.fields[i]
	if f.input.Required {
		return f.input.Name + "*"
	}
	return f.input.Name
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
