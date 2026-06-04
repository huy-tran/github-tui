package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// helpKey is a single key/description pair.
type helpKey struct{ key, desc string }

// helpSection groups related bindings under a heading.
type helpSection struct {
	title string
	keys  []helpKey
}

// globalHelp is always present, regardless of view.
func globalHelp() helpSection {
	return helpSection{
		title: "Global",
		keys: []helpKey{
			{"?", "toggle this help"},
			{"ctrl+k", "command palette (go to repo/screen)"},
			{"tab / ← →", "switch tabs"},
			{"s", "sort table"},
			{"/", "filter"},
			{"enter", "drill in / open"},
			{"ctrl+o", "open in browser"},
			{"ctrl+f", "refresh data"},
			{"esc", "back"},
			{"q / ctrl+c", "quit"},
		},
	}
}

// renderHelpOverlay draws the centered help box over the given area.
func renderHelpOverlay(width, height int, sections []helpSection) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorOverlay)
	headStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)
	keyStyle := lipgloss.NewStyle().Foreground(colorAccent)
	descStyle := lipgloss.NewStyle().Foreground(colorText)
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	// Column width for keys so descriptions align.
	keyW := 0
	for _, sec := range sections {
		for _, k := range sec.keys {
			if w := lipgloss.Width(k.key); w > keyW {
				keyW = w
			}
		}
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Help"))
	b.WriteString("\n")
	for _, sec := range sections {
		b.WriteString("\n")
		b.WriteString(headStyle.Render(sec.title))
		b.WriteString("\n")
		for _, k := range sec.keys {
			pad := strings.Repeat(" ", keyW-lipgloss.Width(k.key))
			b.WriteString("  " + keyStyle.Render(k.key) + pad + "  " + descStyle.Render(k.desc) + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("esc or ? to close"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorOverlay).
		Padding(1, 2).
		Render(b.String())

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
