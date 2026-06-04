package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"github.com/huy-tran/github-tui/internal/gh"
)

// paletteKind distinguishes a jump target.
type paletteKind int

const (
	paletteScreen paletteKind = iota
	paletteRepo
)

// paletteItem is one fuzzy-searchable jump target.
type paletteItem struct {
	label  string // searched + displayed
	hint   string // muted right-side tag
	kind   paletteKind
	screen screen  // for paletteScreen
	repo   gh.Repo // for paletteRepo
}

// paletteModel is the command palette overlay: fuzzy "go to" across screens and
// repositories.
type paletteModel struct {
	active bool
	input  textinput.Model
	items  []paletteItem
	match  []paletteItem
	cursor int
	theme  Theme
}

func newPaletteModel(theme Theme) paletteModel {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.Placeholder = "go to a repo or screen…"
	ti.PromptStyle = accentStyle
	ti.Cursor.Style = accentStyle
	return paletteModel{input: ti, theme: theme}
}

// open (re)builds the target list from the current repos and shows the palette.
func (m *paletteModel) open(repos []gh.Repo) {
	items := []paletteItem{
		{label: "Repositories", hint: "screen", kind: paletteScreen, screen: screenRepos},
		{label: "My PRs", hint: "screen", kind: paletteScreen, screen: screenMyPRs},
		{label: "Notifications", hint: "screen", kind: paletteScreen, screen: screenNotifs},
	}
	for _, r := range repos {
		items = append(items, paletteItem{label: r.NameWithOwner, hint: "repo", kind: paletteRepo, repo: r})
	}
	m.items = items
	m.active = true
	m.cursor = 0
	m.input.SetValue("")
	m.input.Focus()
	m.filter()
}

func (m *paletteModel) close() {
	m.active = false
	m.input.Blur()
}

// filter recomputes matches for the current query.
func (m *paletteModel) filter() {
	q := strings.TrimSpace(m.input.Value())
	if q == "" {
		m.match = m.items
	} else {
		labels := make([]string, len(m.items))
		for i, it := range m.items {
			labels[i] = it.label
		}
		res := fuzzy.Find(q, labels)
		m.match = make([]paletteItem, len(res))
		for i, r := range res {
			m.match[i] = m.items[r.Index]
		}
	}
	m.clampCursor()
}

func (m *paletteModel) clampCursor() {
	if m.cursor >= len(m.match) {
		m.cursor = len(m.match) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *paletteModel) move(d int) {
	m.cursor += d
	m.clampCursor()
}

func (m *paletteModel) selected() (paletteItem, bool) {
	if m.cursor < 0 || m.cursor >= len(m.match) {
		return paletteItem{}, false
	}
	return m.match[m.cursor], true
}

// updateInput forwards a key to the text input and refilters.
func (m *paletteModel) updateInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.filter()
	return cmd
}

// View renders the palette as a centered overlay within the given area.
func (m *paletteModel) View(width, height int) string {
	muted := mutedStyleFor(m.theme)
	innerW := minInt(60, maxInt(width-10, 24)) // content width inside the box
	m.input.Width = maxInt(innerW-3, 8)

	title := lipgloss.NewStyle().Bold(true).Foreground(colorOverlay).Render("Go to")
	input := m.input.View()

	// Show up to a bounded number of matches, keeping the cursor visible.
	const maxRows = 10
	start := 0
	if m.cursor >= maxRows {
		start = m.cursor - maxRows + 1
	}
	end := minInt(start+maxRows, len(m.match))

	selStyle := lipgloss.NewStyle().Foreground(colorTitleFg).Background(colorAccent).Bold(true)
	row := func(it paletteItem, sel bool) string {
		avail := maxInt(innerW-lipgloss.Width(it.hint)-1, 1)
		label := truncateToWidth(it.label, avail)
		pad := maxInt(innerW-lipgloss.Width(label)-lipgloss.Width(it.hint), 1)
		if sel {
			return selStyle.Render(label + strings.Repeat(" ", pad) + it.hint)
		}
		return label + strings.Repeat(" ", pad) + muted.Render(it.hint)
	}

	var rows []string
	for i := start; i < end; i++ {
		rows = append(rows, row(m.match[i], i == m.cursor))
	}
	if len(rows) == 0 {
		rows = append(rows, muted.Render("no matches"))
	}

	footer := muted.Render("↑↓ move · enter go · esc cancel")
	content := strings.Join([]string{title, input, "", strings.Join(rows, "\n"), "", footer}, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorOverlay).
		Padding(1, 2).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
