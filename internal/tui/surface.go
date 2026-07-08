package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type cellAlign int

const (
	alignLeft cellAlign = iota
	alignRight
	alignCenter
)

// Overlay is a terminal graphic layer for width-1 TUI art. Box art, pitch
// markers, mascots, badges, and later help panels can share this composition
// path without touching simulation state.
type Overlay struct {
	X     int
	Y     int
	Z     int
	Lines []string
}

func textOverlay(x, y, z int, text string) Overlay {
	return Overlay{X: x, Y: y, Z: z, Lines: strings.Split(text, "\n")}
}

func applyOverlays(lines []string, overlays []Overlay) []string {
	if len(overlays) == 0 {
		return lines
	}
	out := append([]string{}, lines...)
	sort.SliceStable(overlays, func(i, j int) bool { return overlays[i].Z < overlays[j].Z })
	for _, ov := range overlays {
		for dy, text := range ov.Lines {
			y := ov.Y + dy
			if y < 0 || y >= len(out) {
				continue
			}
			out[y] = writeCells(out[y], ov.X, text)
		}
	}
	return out
}

func writeCells(base string, x int, text string) string {
	if x < 0 {
		text = dropCells(text, -x)
		x = 0
	}
	if text == "" {
		return base
	}
	width := lipgloss.Width(base)
	// Overlays are clipped to the fixed-width frame surface. Drawing fully past
	// the right edge is intentionally a no-op rather than a line extension.
	if x >= width {
		return base
	}
	if lipgloss.Width(text) > width-x {
		text = truncate(text, width-x)
	}
	out := takeCells(base, x) + text + dropCells(base, x+lipgloss.Width(text))
	return fitLine(out, width, alignLeft)
}

func takeCells(s string, n int) string {
	if n <= 0 {
		return ""
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		w := lipgloss.Width(string(r))
		if w <= 0 {
			continue
		}
		if used+w > n {
			break
		}
		b.WriteRune(r)
		used += w
	}
	if used < n {
		b.WriteString(strings.Repeat(" ", n-used))
	}
	return b.String()
}

func dropCells(s string, n int) string {
	if n <= 0 {
		return s
	}
	used := 0
	runes := []rune(s)
	for i, r := range runes {
		w := lipgloss.Width(string(r))
		if w <= 0 {
			continue
		}
		if used+w <= n {
			used += w
			continue
		}
		if used < n {
			return strings.Repeat(" ", used+w-n) + string(runes[i+1:])
		}
		return string(runes[i:])
	}
	return ""
}

type appFrame struct {
	Width    int
	Height   int
	Title    string
	Header   string
	Tabs     string
	Body     string
	Error    string
	Footer   string
	Overlays []Overlay
}

func (f appFrame) Render() string {
	if f.Width <= 0 {
		return strings.TrimRight(f.Body, "\n")
	}
	if f.Height <= 0 {
		f.Height = lipgloss.Height(f.Body) + 6
	}
	if f.Height < 6 || f.Width < 4 {
		return strings.TrimRight(f.Body, "\n")
	}

	inner := f.Width - 2
	errRows := 0
	if f.Error != "" {
		errRows = 1
	}
	bodyRows := f.Height - 6 - errRows
	if bodyRows < 0 {
		bodyRows = 0
	}

	lines := make([]string, 0, f.Height)
	lines = append(lines, borderLine("┌", "─", "┐", inner))
	lines = append(lines, frameContentLine(f.Header, inner))
	lines = append(lines, frameContentLine(f.Tabs, inner))
	lines = append(lines, borderLine("├", "─", "┤", inner))
	for _, line := range fixedLines(f.Body, bodyRows) {
		lines = append(lines, frameContentLine(line, inner))
	}
	if f.Error != "" {
		lines = append(lines, frameContentLine(f.Error, inner))
	}
	lines = append(lines, frameContentLine(f.Footer, inner))
	lines = append(lines, borderLine("└", "─", "┘", inner))

	title := strings.TrimSpace(f.Title)
	if title != "" && lipgloss.Width(title)+2 < f.Width {
		label := " " + title + " "
		lines = applyOverlays(lines, append(f.Overlays, textOverlay((f.Width-lipgloss.Width(label))/2, 0, 100, label)))
	} else {
		lines = applyOverlays(lines, f.Overlays)
	}
	for i := range lines {
		lines[i] = fitLine(lines[i], f.Width, alignLeft)
	}
	return strings.Join(lines, "\n")
}

func borderLine(left, fill, right string, width int) string {
	if width < 0 {
		width = 0
	}
	return left + strings.Repeat(fill, width) + right
}

func frameContentLine(s string, width int) string {
	return "│" + fitLine(s, width, alignLeft) + "│"
}

func fixedLines(s string, rows int) []string {
	out := make([]string, 0, rows)
	if s != "" && rows > 0 {
		for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
			if len(out) >= rows {
				break
			}
			out = append(out, line)
		}
	}
	for len(out) < rows {
		out = append(out, "")
	}
	return out
}

func fitLine(s string, width int, align cellAlign) string {
	if width <= 0 {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	if lipgloss.Width(s) > width {
		s = truncate(s, width)
	}
	pad := width - lipgloss.Width(s)
	if pad <= 0 {
		return s
	}
	switch align {
	case alignRight:
		return strings.Repeat(" ", pad) + s
	case alignCenter:
		left := pad / 2
		return strings.Repeat(" ", left) + s + strings.Repeat(" ", pad-left)
	default:
		return s + strings.Repeat(" ", pad)
	}
}

type tableColumn struct {
	Header   string
	Width    int
	MinWidth int
	Flex     bool
	Align    cellAlign
}

func renderTextTable(width int, columns []tableColumn, rows [][]string) string {
	if width < 1 || len(columns) == 0 {
		return ""
	}
	cols := resolveTableColumns(width, columns)
	lines := []string{
		tableRule("┌", "┬", "┐", cols),
		tableRow(cols, headers(cols), true),
		tableRule("├", "┼", "┤", cols),
	}
	for _, row := range rows {
		lines = append(lines, tableRow(cols, row, false))
	}
	lines = append(lines, tableRule("└", "┴", "┘", cols))
	for i := range lines {
		lines[i] = fitLine(lines[i], width, alignLeft)
	}
	return strings.Join(lines, "\n")
}

func resolveTableColumns(width int, columns []tableColumn) []tableColumn {
	cols := append([]tableColumn{}, columns...)
	cellBudget := width - (len(cols) + 1)
	if cellBudget < len(cols) {
		cellBudget = len(cols)
	}
	fixed := 0
	flexMin := 0
	flexCount := 0
	for i := range cols {
		min := cols[i].MinWidth
		if min == 0 {
			min = lipgloss.Width(cols[i].Header)
		}
		if min < 1 {
			min = 1
		}
		cols[i].MinWidth = min
		if cols[i].Flex {
			flexMin += min
			flexCount++
			continue
		}
		w := cols[i].Width
		if w < min {
			w = min
		}
		cols[i].Width = w
		fixed += w
	}
	remaining := cellBudget - fixed
	if flexCount == 0 {
		return cols
	}
	if remaining < flexMin {
		remaining = flexMin
	}
	extra := remaining - flexMin
	share := extra / flexCount
	leftover := extra % flexCount
	for i := range cols {
		if !cols[i].Flex {
			continue
		}
		cols[i].Width = cols[i].MinWidth + share
		if leftover > 0 {
			cols[i].Width++
			leftover--
		}
	}
	return cols
}

func headers(cols []tableColumn) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Header
	}
	return out
}

func tableRule(left, mid, right string, cols []tableColumn) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = strings.Repeat("─", c.Width)
	}
	return left + strings.Join(parts, mid) + right
}

func tableRow(cols []tableColumn, row []string, header bool) string {
	cells := make([]string, len(cols))
	for i, c := range cols {
		value := ""
		if i < len(row) {
			value = row[i]
		}
		align := c.Align
		if header {
			align = alignCenter
		}
		cells[i] = fitLine(value, c.Width, align)
	}
	return "│" + strings.Join(cells, "│") + "│"
}

func intCell(v int) string { return fmt.Sprint(v) }

func colWidth(label string, min int) int {
	if w := lipgloss.Width(label); w > min {
		return w
	}
	return min
}
