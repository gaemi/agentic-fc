package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Broadcast furniture for the live and replay match pop-ups: a marker
// timeline and a momentum strip built only from public match data
// (docs/07 §4.1). Both render as fixed-width preformatted rows.

const (
	timelineMinWidth = 30
	fullTimeMinute   = 90
)

func timelineGlyph(kind string) rune {
	switch strings.ToUpper(kind) {
	case "GOAL":
		return 'G'
	case "CHANCE":
		return 'o'
	case "CARD":
		return 'x'
	case "INJURY":
		return '+'
	case "SUB":
		return 's'
	case "SHOOTOUT":
		return '!'
	default:
		return '·'
	}
}

// timelineRows plots match markers on a shared minute ruler, home events on
// the top row and away events below. The ruler is scaled to the elapsed
// minute (at least a full match), ticks mark every 15 minutes, and the play
// head sits at the current minute.
func timelineRows(markers []LiveMarker, minute, width int) []string {
	if width < timelineMinWidth {
		return nil
	}
	span := fullTimeMinute
	if minute > span {
		span = minute
	}
	col := func(min int) int {
		c := min * (width - 1) / span
		if c < 0 {
			c = 0
		}
		if c > width-1 {
			c = width - 1
		}
		return c
	}
	home := []rune(strings.Repeat("─", width))
	away := []rune(strings.Repeat("─", width))
	for tick := 15; tick < span; tick += 15 {
		home[col(tick)] = '┼'
		away[col(tick)] = '┼'
	}
	cursor := col(minute)
	for x := cursor + 1; x < width; x++ {
		home[x] = ' '
		away[x] = ' '
	}
	home[cursor] = '┤'
	away[cursor] = '┤'
	for _, mk := range markers {
		if mk.Minute < 0 || mk.Minute > minute {
			continue
		}
		row := home
		if mk.Side == matchSideAway {
			row = away
		}
		row[col(mk.Minute)] = timelineGlyph(mk.Kind)
	}
	return []string{string(home), string(away)}
}

// momentumBlock maps one signed momentum bucket to a bar glyph.
func momentumBlock(v int) rune {
	switch {
	case v >= 3:
		return '█'
	case v == 2:
		return '▆'
	case v == 1:
		return '▃'
	default:
		return ' '
	}
}

// momentumRows renders the signed ten-minute momentum buckets as mirrored
// home/away rows, each bucket wide enough to fill the given width.
func momentumRows(momentum []int, width int) []string {
	if len(momentum) == 0 || width < len(momentum) {
		return nil
	}
	cell := width / len(momentum)
	if cell > 4 {
		cell = 4
	}
	gap := 1
	if cell < 2 {
		gap = 0
	}
	var home, away strings.Builder
	for i, v := range momentum {
		hc, ac := ' ', ' '
		if v > 0 {
			hc = momentumBlock(v)
		}
		if v < 0 {
			ac = momentumBlock(-v)
		}
		home.WriteString(strings.Repeat(string(hc), cell-gap))
		away.WriteString(strings.Repeat(string(ac), cell-gap))
		if gap > 0 && i < len(momentum)-1 {
			home.WriteRune(' ')
			away.WriteRune(' ')
		}
	}
	return []string{home.String(), away.String()}
}

// labeledPairRows prefixes two H/A rows with a section label, keeping every
// glyph column aligned between the two rows.
func labeledPairRows(label string, rows []string, width int) []string {
	if len(rows) != 2 || width <= 0 {
		return nil
	}
	labelWidth := lipgloss.Width(label)
	prefixWidth := labelWidth + 3 // label + space + side letter + space
	if prefixWidth+timelineMinWidth > width {
		return nil
	}
	pad := strings.Repeat(" ", labelWidth)
	out := []string{
		preformattedLinePrefix + fitLine(label+" H "+rows[0], width, alignLeft),
		preformattedLinePrefix + fitLine(pad+" A "+rows[1], width, alignLeft),
	}
	return out
}

// matchPhaseLabel tags the running minute as first or second half.
func (m Model) matchPhaseLabel(minute int) string {
	if minute <= 45 {
		return m.ui("ui.match.phase.first")
	}
	return m.ui("ui.match.phase.second")
}

// broadcastRows renders the timeline and momentum strips for the live modal.
func (m Model) broadcastRows(mv LiveMatchView, width int) []string {
	label := m.ui("ui.match.timeline")
	momentumLabel := m.ui("ui.match.momentum")
	labelWidth := maxInt(lipgloss.Width(label), lipgloss.Width(momentumLabel))
	stripWidth := width - labelWidth - 3
	if stripWidth > 64 {
		stripWidth = 64
	}
	out := []string{}
	if rows := timelineRows(mv.Markers, mv.Minute, stripWidth); rows != nil {
		out = append(out, labeledPairRows(fitLine(label, labelWidth, alignLeft), rows, width)...)
	}
	if rows := momentumRows(mv.Momentum, stripWidth); rows != nil {
		out = append(out, labeledPairRows(fitLine(momentumLabel, labelWidth, alignLeft), rows, width)...)
	}
	return out
}
