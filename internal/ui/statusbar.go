package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Snapshot is what a view reports for the status footer.
type Snapshot struct {
	Profile    string    // for us: the authenticated account
	Region     string    // for us: the active repo (empty on the list screen)
	View       string    // e.g. "repos", "PRs", "Workflows"
	Items      int       // item count; < 0 to omit
	LastLoaded time.Time // zero => no freshness shown
	Message    string    // transient action message (takes priority)
	Live       bool      // auto-refresh is active for this screen
}

// statusFooter renders the single bottom row from a Snapshot.
func statusFooter(width int, t Theme, s Snapshot) string {
	muted := mutedStyleFor(t)
	sep := muted.Render("  ·  ")

	var segs []string

	// 1. Context: profile · region · view (bold), non-empty parts only.
	ctxStyle := lipgloss.NewStyle().Bold(true).Foreground(t.StatusContextFg)
	var ctx []string
	for _, p := range []string{s.Profile, s.Region, s.View} {
		if p != "" {
			ctx = append(ctx, p)
		}
	}
	if len(ctx) > 0 {
		segs = append(segs, ctxStyle.Render(strings.Join(ctx, " · ")))
	}

	// 2. Item count, muted.
	if s.Items >= 0 {
		segs = append(segs, muted.Render(plural(s.Items, "item", "items")))
	}

	// Live (auto-refresh) indicator.
	if s.Live {
		segs = append(segs, lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render("● live"))
	}

	// 3. Transient message (amber, bold) OR freshness (muted).
	switch {
	case s.Message != "":
		segs = append(segs, lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render(s.Message))
	case !s.LastLoaded.IsZero():
		segs = append(segs, muted.Render("loaded "+freshness(s.LastLoaded)))
	}

	// Clamp to a single row: truncate before the style pads to full width,
	// otherwise long context wraps onto a second line.
	body := truncateToWidth(strings.Join(segs, sep), maxInt(width-2, 0))
	return lipgloss.NewStyle().
		Background(t.StatusBg).
		Foreground(t.StatusFg).
		Width(width).
		MaxHeight(1).
		Padding(0, 1).
		Render(body)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return itoa(n) + " " + many
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
