package ui

import (
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/mattn/go-runewidth"
	"github.com/sahilm/fuzzy"
)

// SortKind controls how a column's values are compared.
type SortKind int

const (
	SortString  SortKind = iota // case-insensitive lexicographic (default)
	SortNumeric                 // compares the leading numeric prefix
	SortTime                    // parses sortTimeLayout
)

// Column describes one table column.
type Column struct {
	Title string
	Width int  // soft minimum; 0 derives from content
	Flex  bool // absorbs leftover horizontal width
	Align lipgloss.Position
	Sort  SortKind
}

// DataTable is a bordered, scrollable table that owns its cursor and viewport.
//
// Rows carry display strings (which may contain ANSI styling). An optional
// parallel sortKeys matrix supplies the value used for comparison when a cell's
// rendered text differs from its sortable form (e.g. "4m ago" displayed, an
// absolute timestamp used for Time sorting).
type DataTable struct {
	cols []Column

	// The full, unfiltered data. rows/sortKeys/ids below are the visible subset
	// after the fuzzy filter is applied.
	allRows  [][]string
	allKeys  [][]string
	allIDs   []string
	rows     [][]string
	sortKeys [][]string // optional; len 0 => use display cells
	ids      []string   // optional opaque per-row id (e.g. URL), travels with sorts

	filtering   bool
	filterInput textinput.Model

	cursor int
	offset int

	width  int
	height int
	theme  Theme

	sortCol int // -1 = unsorted
	sortAsc bool
	sorting bool // sort ribbon is raised

	emptyMsg string // shown under the header when there are no rows
}

// NewDataTable builds a table for the given columns.
func NewDataTable(cols []Column) DataTable {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.Placeholder = "filter…"
	ti.PromptStyle = accentStyle
	ti.Cursor.Style = accentStyle
	return DataTable{cols: cols, sortCol: -1, sortAsc: true, filterInput: ti}
}

func (d *DataTable) SetTheme(t Theme) { d.theme = t }

// SetEmptyMessage sets the text shown beneath the header when there are no rows.
func (d *DataTable) SetEmptyMessage(s string) { d.emptyMsg = s }

func (d *DataTable) SetSize(w, h int) { d.width, d.height = w, h }

// SetRows replaces the data. sortKeys and ids may be nil. The current fuzzy
// filter (if any) and sort are re-applied; the cursor is preserved by id (or
// value) when possible, otherwise clamped.
func (d *DataTable) SetRows(rows [][]string, sortKeys [][]string, ids []string) {
	prevID := d.SelectedID()
	prev := d.SelectedRow()
	d.allRows = rows
	d.allKeys = sortKeys
	d.allIDs = ids
	d.applyFilterAndSort()
	if prevID != "" && d.restoreByID(prevID) {
		return
	}
	d.restoreCursor(prev)
}

// applyFilterAndSort rebuilds the visible rows/sortKeys/ids from the full set by
// applying the fuzzy filter (preserving original order) and then the sort.
func (d *DataTable) applyFilterAndSort() {
	q := strings.TrimSpace(d.filterInput.Value())
	hasKeys := len(d.allKeys) == len(d.allRows)
	hasIDs := len(d.allIDs) == len(d.allRows)

	if q == "" {
		d.rows = d.allRows
		d.sortKeys = d.allKeys
		d.ids = d.allIDs
	} else {
		vals := make([]string, len(d.allRows))
		for i, r := range d.allRows {
			vals[i] = stripANSI(strings.Join(r, " "))
		}
		keep := make([]bool, len(d.allRows))
		for _, mt := range fuzzy.Find(q, vals) {
			keep[mt.Index] = true
		}
		var rows, keys [][]string
		var ids []string
		for i := range d.allRows {
			if !keep[i] {
				continue
			}
			rows = append(rows, d.allRows[i])
			if hasKeys {
				keys = append(keys, d.allKeys[i])
			}
			if hasIDs {
				ids = append(ids, d.allIDs[i])
			}
		}
		d.rows, d.sortKeys, d.ids = rows, keys, ids
	}
	if d.sortCol >= 0 {
		d.applySort()
	}
}

// SelectedID returns the opaque id of the current row, or "" when unavailable.
func (d *DataTable) SelectedID() string {
	if d.cursor < 0 || d.cursor >= len(d.ids) {
		return ""
	}
	return d.ids[d.cursor]
}

func (d *DataTable) restoreByID(id string) bool {
	for i, rid := range d.ids {
		if rid == id {
			d.cursor = i
			d.clamp()
			return true
		}
	}
	return false
}

// Len reports the number of rows.
func (d *DataTable) Len() int { return len(d.rows) }

// Cursor returns the current row index, or -1 when empty.
func (d *DataTable) Cursor() int {
	if len(d.rows) == 0 {
		return -1
	}
	return d.cursor
}

// SelectedRow returns the highlighted row's display cells, or nil when empty.
func (d *DataTable) SelectedRow() []string {
	if d.cursor < 0 || d.cursor >= len(d.rows) {
		return nil
	}
	return d.rows[d.cursor]
}

// Sorting reports whether the sort ribbon is currently raised.
func (d *DataTable) Sorting() bool { return d.sorting }

// Filtering reports whether the filter input is focused (typing).
func (d *DataTable) Filtering() bool { return d.filtering }

// FilterActive reports whether a filter query is in effect.
func (d *DataTable) FilterActive() bool { return d.filterInput.Value() != "" }

// FilterView renders the filter input line (shown in a screen's top row).
func (d *DataTable) FilterView() string { return d.filterInput.View() }

// refilter re-applies the filter/sort and keeps the cursor on the same row.
func (d *DataTable) refilter() {
	prevID := d.SelectedID()
	prev := d.SelectedRow()
	d.applyFilterAndSort()
	if prevID != "" && d.restoreByID(prevID) {
		return
	}
	d.restoreCursor(prev)
}

// Update handles filtering, navigation, and sorting keys. It returns true when
// it consumed the key, so callers can decide whether to act on it themselves.
func (d *DataTable) Update(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}

	if d.filtering {
		switch key.String() {
		case "esc":
			d.filtering = false
			d.filterInput.SetValue("")
			d.filterInput.Blur()
			d.refilter()
		case "enter":
			d.filtering = false
			d.filterInput.Blur()
		default:
			var cmd tea.Cmd
			d.filterInput, cmd = d.filterInput.Update(msg)
			d.refilter()
			return cmd, true
		}
		return nil, true
	}

	if d.sorting {
		d.handleSortKey(key)
		return nil, true
	}

	switch key.String() {
	case "/":
		d.filtering = true
		d.filterInput.Focus()
		return textinput.Blink, true
	case "s":
		if len(d.cols) > 0 {
			d.sorting = true
		}
		return nil, true
	case "up", "k":
		d.move(-1)
	case "down", "j":
		d.move(1)
	case "pgup", "ctrl+b":
		d.move(-d.page())
	case "pgdown", " ":
		d.move(d.page())
	case "home", "g":
		d.cursor = 0
	case "end", "G":
		d.cursor = len(d.rows) - 1
	default:
		return nil, false
	}
	d.clamp()
	return nil, true
}

// handleSortKey resolves a column choice (digit or first-letter) or cancels.
func (d *DataTable) handleSortKey(key tea.KeyMsg) {
	s := key.String()
	if s == "esc" {
		d.sorting = false
		return
	}
	// Digit 1..N.
	if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= len(d.cols) {
		d.chooseSort(n - 1)
		return
	}
	// First letter of a column title.
	if len(s) == 1 {
		r := unicode.ToLower(rune(s[0]))
		for i, c := range d.cols {
			if len(c.Title) > 0 && unicode.ToLower(rune(c.Title[0])) == r {
				d.chooseSort(i)
				return
			}
		}
	}
}

// chooseSort selects a sort column, flipping direction if already active.
func (d *DataTable) chooseSort(col int) {
	if d.sortCol == col {
		d.sortAsc = !d.sortAsc
	} else {
		d.sortCol = col
		d.sortAsc = true
	}
	d.sorting = false
	prev := d.SelectedRow()
	d.applySort()
	d.restoreCursor(prev)
}

// applySort stably reorders rows (and parallel sortKeys) by the active column.
func (d *DataTable) applySort() {
	if d.sortCol < 0 || d.sortCol >= len(d.cols) {
		return
	}
	col := d.sortCol
	kind := d.cols[col].Sort

	idx := make([]int, len(d.rows))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		less := compareCells(d.sortValue(idx[a], col), d.sortValue(idx[b], col), kind)
		if !d.sortAsc {
			return !less
		}
		return less
	})

	rows := make([][]string, len(d.rows))
	for i, j := range idx {
		rows[i] = d.rows[j]
	}
	d.rows = rows
	if len(d.sortKeys) == len(rows) {
		keys := make([][]string, len(d.sortKeys))
		for i, j := range idx {
			keys[i] = d.sortKeys[j]
		}
		d.sortKeys = keys
	}
	if len(d.ids) == len(rows) {
		ids := make([]string, len(d.ids))
		for i, j := range idx {
			ids[i] = d.ids[j]
		}
		d.ids = ids
	}
}

// sortValue returns the comparison string for a cell.
func (d *DataTable) sortValue(row, col int) string {
	if len(d.sortKeys) == len(d.rows) && col < len(d.sortKeys[row]) {
		return d.sortKeys[row][col]
	}
	if col < len(d.rows[row]) {
		return stripANSI(d.rows[row][col])
	}
	return ""
}

func compareCells(a, b string, kind SortKind) bool {
	switch kind {
	case SortNumeric:
		return numericPrefix(a) < numericPrefix(b)
	case SortTime:
		ta, _ := time.Parse(sortTimeLayout, strings.TrimSpace(a))
		tb, _ := time.Parse(sortTimeLayout, strings.TrimSpace(b))
		return ta.Before(tb)
	default:
		return strings.ToLower(a) < strings.ToLower(b)
	}
}

// numericPrefix extracts a leading number, e.g. "5d" -> 5, "1.2 G" -> 1.2.
func numericPrefix(s string) float64 {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) {
		c := s[end]
		if (c >= '0' && c <= '9') || c == '.' || (end == 0 && (c == '-' || c == '+')) {
			end++
			continue
		}
		break
	}
	if end == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(s[:end], 64)
	return v
}

// --- cursor / viewport ----------------------------------------------------

func (d *DataTable) move(delta int) { d.cursor += delta }

func (d *DataTable) clamp() {
	if d.cursor < 0 {
		d.cursor = 0
	}
	if d.cursor > len(d.rows)-1 {
		d.cursor = len(d.rows) - 1
	}
	if d.cursor < 0 {
		d.cursor = 0
	}
}

// page returns the rows-per-page used for pgup/pgdown.
func (d *DataTable) page() int {
	p := d.visibleGuess()
	if p < 1 {
		return 1
	}
	return p
}

// visibleGuess estimates how many data rows fit, accounting for the boxed
// borders (top + header + separators, two lines per data row).
func (d *DataTable) visibleGuess() int {
	n := (d.height - 3) / 2
	if n < 1 {
		n = 1
	}
	return n
}

// ensureVisible scrolls the viewport so the cursor is within a window of vr.
func (d *DataTable) ensureVisible(vr int) {
	d.clamp()
	if d.cursor < d.offset {
		d.offset = d.cursor
	}
	if d.cursor >= d.offset+vr {
		d.offset = d.cursor - vr + 1
	}
	if d.offset < 0 {
		d.offset = 0
	}
}

// restoreCursor re-finds prev (by value) after a reorder, else clamps.
func (d *DataTable) restoreCursor(prev []string) {
	if prev == nil {
		d.clamp()
		return
	}
	for i, row := range d.rows {
		if rowsEqual(row, prev) {
			d.cursor = i
			d.clamp()
			return
		}
	}
	d.clamp()
}

func rowsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- rendering ------------------------------------------------------------

// View renders the table, fitting within the configured height.
func (d *DataTable) View() string {
	if len(d.cols) == 0 {
		return ""
	}
	widths := d.columnWidths()
	if len(d.rows) == 0 {
		return d.renderEmpty(widths)
	}
	for vr := d.visibleGuess(); vr >= 1; vr-- {
		d.ensureVisible(vr)
		out := d.render(widths, vr)
		if lipgloss.Height(out) <= d.height || vr == 1 {
			return out
		}
	}
	return d.render(widths, 1)
}

// renderEmpty draws the header-only table plus a centered empty message, so a
// screen with no rows still clearly looks like its table.
func (d *DataTable) renderEmpty(widths []int) string {
	headers := make([]string, len(d.cols))
	for i := range d.cols {
		headers[i] = formatCell(d.headerLabel(i), widths[i], d.cols[i].Align)
	}
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderRow(true).
		BorderColumn(true).
		BorderStyle(lipgloss.NewStyle().Foreground(d.theme.BorderFg)).
		Headers(headers...).
		StyleFunc(func(row, col int) lipgloss.Style {
			st := lipgloss.NewStyle().Padding(0, 1)
			if row == table.HeaderRow {
				return st.Bold(true).Foreground(colorText)
			}
			return st
		})
	out := t.Render()

	msg := d.emptyMsg
	if msg == "" {
		msg = "Nothing to show."
	}
	rem := d.height - lipgloss.Height(out)
	if rem < 1 {
		return out
	}
	placed := lipgloss.Place(maxInt(d.width, 1), rem, lipgloss.Center, lipgloss.Center,
		mutedStyleFor(d.theme).Render(msg))
	return out + "\n" + placed
}

// render builds the lipgloss table for a window of vr rows from the offset.
func (d *DataTable) render(widths []int, vr int) string {
	end := d.offset + vr
	if end > len(d.rows) {
		end = len(d.rows)
	}
	active := d.cursor - d.offset

	headers := make([]string, len(d.cols))
	for i := range d.cols {
		headers[i] = formatCell(d.headerLabel(i), widths[i], d.cols[i].Align)
	}

	var data [][]string
	for r := d.offset; r < end; r++ {
		cells := make([]string, len(d.cols))
		for i := range d.cols {
			cell := ""
			if i < len(d.rows[r]) {
				cell = d.rows[r][i]
			}
			cells[i] = formatCell(cell, widths[i], d.cols[i].Align)
		}
		data = append(data, cells)
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderRow(true).
		BorderColumn(true).
		BorderStyle(lipgloss.NewStyle().Foreground(d.theme.BorderFg)).
		Headers(headers...).
		Rows(data...).
		StyleFunc(func(row, col int) lipgloss.Style {
			st := lipgloss.NewStyle().Padding(0, 1)
			if row == table.HeaderRow {
				return st.Bold(true).Foreground(colorText)
			}
			if row == active {
				// Subtle background band only; per-cell fg is preserved.
				st = st.Background(colorBand)
			}
			return st
		})

	return t.Render()
}

// headerLabel adds the sort-direction arrow to the active sort column.
func (d *DataTable) headerLabel(i int) string {
	title := d.cols[i].Title
	if i == d.sortCol {
		if d.sortAsc {
			return title + " ↑"
		}
		return title + " ↓"
	}
	return title
}

// columnWidths derives per-column content widths, giving leftover to the flex
// column and shrinking it (then the widest others) when space is tight.
func (d *DataTable) columnWidths() []int {
	n := len(d.cols)
	nat := make([]int, n)
	for i := range d.cols {
		w := lipgloss.Width(d.headerLabel(i))
		if d.cols[i].Width > w {
			w = d.cols[i].Width
		}
		for _, row := range d.rows {
			if i < len(row) {
				if cw := lipgloss.Width(row[i]); cw > w {
					w = cw
				}
			}
		}
		nat[i] = w
	}

	chrome := (n + 1) + n*2 // vertical borders + per-cell padding(1 each side)
	budget := d.width - chrome
	if budget < n {
		for i := range nat {
			nat[i] = 1
		}
		return nat
	}

	flex := d.flexCol()
	sum := 0
	for _, w := range nat {
		sum += w
	}
	switch {
	case sum < budget:
		nat[flex] += budget - sum
	case sum > budget:
		over := sum - budget
		const minFlex = 6
		if nat[flex]-over >= minFlex {
			nat[flex] -= over
		} else {
			over -= nat[flex] - minFlex
			nat[flex] = minFlex
			for over > 0 {
				wi, ww := -1, 0
				for i := 0; i < n; i++ {
					if i == flex {
						continue
					}
					if nat[i] > ww {
						ww, wi = nat[i], i
					}
				}
				if wi < 0 || nat[wi] <= 3 {
					break
				}
				nat[wi]--
				over--
			}
		}
	}
	return nat
}

// flexCol returns the flex column index, defaulting to 0.
func (d *DataTable) flexCol() int {
	for i, c := range d.cols {
		if c.Flex {
			return i
		}
	}
	return 0
}

// formatCell fits s to exactly width visible columns, aligned. Styled cells
// that must be shortened are stripped before truncation so escapes aren't cut.
func formatCell(s string, width int, align lipgloss.Position) string {
	if width < 1 {
		width = 1
	}
	vis := lipgloss.Width(s)
	if vis > width {
		plain := stripANSI(s)
		return runewidth.Truncate(plain, width, "…")
	}
	pad := width - vis
	switch align {
	case lipgloss.Right:
		return strings.Repeat(" ", pad) + s
	case lipgloss.Center:
		left := pad / 2
		return strings.Repeat(" ", left) + s + strings.Repeat(" ", pad-left)
	default:
		return s + strings.Repeat(" ", pad)
	}
}

// sortRibbon renders the sort prompt shown while choosing a column.
func (d *DataTable) sortRibbon() string {
	var b strings.Builder
	b.WriteString(accentStyle.Bold(true).Render("sort by: "))
	parts := make([]string, len(d.cols))
	for i, c := range d.cols {
		parts[i] = accentStyle.Bold(true).Render(strconv.Itoa(i+1) + ":" + c.Title)
	}
	b.WriteString(strings.Join(parts, accentStyle.Render("  ")))
	b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("(esc to cancel)"))
	return b.String()
}
