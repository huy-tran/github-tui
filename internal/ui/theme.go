package ui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme holds only the roles that genuinely shift between dark and light
// terminals. Every other color is a fixed semantic literal (see styles.go).
type Theme struct {
	StatusBg        lipgloss.Color
	StatusFg        lipgloss.Color
	StatusContextFg lipgloss.Color
	BorderFg        lipgloss.Color
	MutedFg         lipgloss.Color
}

var darkTheme = Theme{
	StatusBg:        lipgloss.Color("237"),
	StatusFg:        lipgloss.Color("252"),
	StatusContextFg: lipgloss.Color("252"),
	BorderFg:        lipgloss.Color("240"),
	MutedFg:         lipgloss.Color("241"),
}

var lightTheme = Theme{
	StatusBg:        lipgloss.Color("254"),
	StatusFg:        lipgloss.Color("235"),
	StatusContextFg: lipgloss.Color("236"),
	BorderFg:        lipgloss.Color("247"),
	MutedFg:         lipgloss.Color("244"),
}

// SelectTheme resolves a theme from an explicit value ("dark"|"light"|"auto").
// "auto" (or anything unrecognized) probes the terminal background.
func SelectTheme(pref string) Theme {
	switch strings.ToLower(strings.TrimSpace(pref)) {
	case "dark":
		return darkTheme
	case "light":
		return lightTheme
	default: // "auto" / unset
		if lipgloss.HasDarkBackground() {
			return darkTheme
		}
		return lightTheme
	}
}

// ThemePref reads the theme preference from a flag value, falling back to the
// GITHUB_TUI_THEME environment variable, then "auto".
func ThemePref(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if env := os.Getenv("GITHUB_TUI_THEME"); env != "" {
		return env
	}
	return "auto"
}
