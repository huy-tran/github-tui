package ui

import (
	"fmt"
	"regexp"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// App identity shown in the top bar.
const (
	AppName = "Github - TUI"
	Version = "v0.0.12" // empty or "dev" => render bare app name
)

// Fixed semantic colors — identical on light and dark backgrounds. Only the
// five roles in Theme are background-dependent; everything here is literal.
var (
	colorTitleFg = lipgloss.Color("231") // near-white
	colorTitleBg = lipgloss.Color("57")  // indigo
	colorText    = lipgloss.Color("252") // header / primary text
	colorBand    = lipgloss.Color("235") // active-row background band
	colorAccent  = lipgloss.Color("214") // action / keys / sort prompt (amber)
	colorOverlay = lipgloss.Color("213") // overlay border + cursor-highlight (magenta)

	colorGreen  = lipgloss.Color("2") // healthy
	colorRed    = lipgloss.Color("1") // error
	colorYellow = lipgloss.Color("3") // warning (also see colorAccent for amber)
)

// ansiRE strips SGR escape sequences so we can measure/truncate plain text.
var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

// truncateToWidth clamps a (possibly styled) single line to w visible columns,
// preserving ANSI styling and appending an ellipsis when it must cut.
func truncateToWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// titleBar renders the persistent top bar: bold, centered, full width,
// fg 231 on bg 57, with an optional [DRY-RUN] badge.
func titleBar(width int, dryRun bool) string {
	label := AppName
	if Version != "" && Version != "dev" {
		label = AppName + " - " + Version
	}
	if dryRun {
		badge := lipgloss.NewStyle().Bold(true).
			Foreground(colorAccent).Background(colorTitleBg).
			Render(" [DRY-RUN]")
		label += badge
	}
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitleFg).
		Background(colorTitleBg).
		Width(width).
		Align(lipgloss.Center).
		Render(label)
}

// mutedStyle / errorStyle / helpStyle are convenience styles bound to a theme
// where relevant; muted text uses the theme's MutedFg.
func mutedStyleFor(t Theme) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.MutedFg)
}

var (
	accentStyle = lipgloss.NewStyle().Foreground(colorAccent)
	errorStyle  = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
	textStyle   = lipgloss.NewStyle().Foreground(colorText)
)

// humanizeDuration renders a friendly "time ago" for table cells.
func humanizeDuration(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/24/30))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/24/365))
	}
}

// freshness renders the short relative units used by the status footer:
// "12s ago", "4m ago", "2h ago", "3d ago".
func freshness(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// sortTimeLayout is the layout used for Time-kind sort keys.
const sortTimeLayout = "2006-01-02 15:04:05"

// duration renders the elapsed time between start and end and returns the total
// seconds (for numeric sorting). When end is zero the item is still running and
// elapsed is measured from now.
func duration(start, end time.Time) (string, int) {
	if start.IsZero() {
		return "-", 0
	}
	if end.IsZero() {
		end = time.Now()
	}
	d := end.Sub(start)
	if d < 0 {
		d = 0
	}
	secs := int(d.Seconds())
	switch {
	case secs < 60:
		return fmt.Sprintf("%ds", secs), secs
	case secs < 3600:
		return fmt.Sprintf("%dm %ds", secs/60, secs%60), secs
	default:
		return fmt.Sprintf("%dh %dm", secs/3600, (secs%3600)/60), secs
	}
}

// runStatusCell returns a colored status word for a workflow run.
func runStatusCell(status, conclusion string) string {
	if status != "completed" {
		return lipgloss.NewStyle().Foreground(colorYellow).Render(status)
	}
	switch conclusion {
	case "success":
		return lipgloss.NewStyle().Foreground(colorGreen).Render("success")
	case "failure", "timed_out", "startup_failure":
		return lipgloss.NewStyle().Foreground(colorRed).Render(conclusion)
	case "":
		return "-"
	default:
		return lipgloss.NewStyle().Foreground(colorText).Render(conclusion)
	}
}

// severityCell renders a Dependabot severity in its risk color.
func severityCell(sev string) string {
	switch sev {
	case "critical":
		return lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("critical")
	case "high":
		return lipgloss.NewStyle().Foreground(colorRed).Render("high")
	case "medium":
		return lipgloss.NewStyle().Foreground(colorAccent).Render("medium")
	case "low":
		return lipgloss.NewStyle().Foreground(colorYellow).Render("low")
	case "":
		return "-"
	default:
		return sev
	}
}

// runStatusIcon returns a small colored glyph for the leading status column.
func runStatusIcon(status, conclusion string) string {
	if status != "completed" {
		return lipgloss.NewStyle().Foreground(colorYellow).Render("●")
	}
	switch conclusion {
	case "success":
		return lipgloss.NewStyle().Foreground(colorGreen).Render("✓")
	case "failure", "timed_out", "startup_failure":
		return lipgloss.NewStyle().Foreground(colorRed).Render("✗")
	case "cancelled", "skipped", "neutral", "stale":
		return lipgloss.NewStyle().Foreground(colorYellow).Render("○")
	default:
		return lipgloss.NewStyle().Foreground(colorOverlay).Render("•")
	}
}
