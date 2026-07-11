package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestTimelineRowsPlaceMarkersProportionally(t *testing.T) {
	markers := []LiveMarker{
		{Minute: 0, Kind: "CHANCE", Side: "HOME"},
		{Minute: 45, Kind: "GOAL", Side: "HOME"},
		{Minute: 90, Kind: "CARD", Side: "AWAY"},
		{Minute: 95, Kind: "GOAL", Side: "AWAY"}, // beyond the cursor: hidden
	}
	rows := timelineRows(markers, 90, 61)
	if len(rows) != 2 {
		t.Fatalf("timeline rows = %d, want 2", len(rows))
	}
	home, away := []rune(rows[0]), []rune(rows[1])
	if len(home) != 61 || len(away) != 61 {
		t.Fatalf("timeline widths = %d/%d, want 61", len(home), len(away))
	}
	if home[0] != 'o' {
		t.Fatalf("kickoff chance glyph = %q, want o", home[0])
	}
	if home[30] != 'G' {
		t.Fatalf("45' goal glyph at col 30 = %q, want G", home[30])
	}
	if away[60] != 'x' {
		t.Fatalf("90' card glyph at col 60 = %q, want x", away[60])
	}
	if !strings.ContainsRune(rows[0]+rows[1], '┼') {
		t.Fatal("timeline lost its 15-minute ticks")
	}

	early := timelineRows(markers[:1], 30, 61)
	homeEarly := []rune(early[0])
	cursor := 30 * 60 / 90
	if homeEarly[cursor] != '┤' {
		t.Fatalf("play head at 30' col %d = %q, want ┤", cursor, homeEarly[cursor])
	}
	for x := cursor + 1; x < len(homeEarly); x++ {
		if homeEarly[x] != ' ' {
			t.Fatalf("ruler drawn past the play head at col %d: %q", x, early[0])
		}
	}

	if rows := timelineRows(markers, 90, timelineMinWidth-1); rows != nil {
		t.Fatalf("narrow timeline should be omitted, got %q", rows)
	}

	// A marker landing on the play-head cell keeps both visible: the event
	// takes the cell and the head shifts one column right.
	fresh := timelineRows([]LiveMarker{{Minute: 45, Kind: "GOAL", Side: "HOME"}}, 45, 61)
	freshHome := []rune(fresh[0])
	if freshHome[30] != 'G' || freshHome[31] != '┤' {
		t.Fatalf("fresh goal should keep glyph and nudged play head: %q", fresh[0])
	}
}

func TestMomentumRowsMirrorSides(t *testing.T) {
	rows := momentumRows([]int{-1, 0, 2, 3, 0, -2, 1, 0, 0}, 36)
	if len(rows) != 2 {
		t.Fatalf("momentum rows = %d, want 2", len(rows))
	}
	if lipgloss.Width(rows[0]) != lipgloss.Width(rows[1]) {
		t.Fatalf("momentum rows unaligned: %d vs %d", lipgloss.Width(rows[0]), lipgloss.Width(rows[1]))
	}
	if !strings.ContainsRune(rows[0], '█') || !strings.ContainsRune(rows[0], '▆') {
		t.Fatalf("home momentum lost its peaks: %q", rows[0])
	}
	if !strings.ContainsRune(rows[1], '▆') || !strings.ContainsRune(rows[1], '▃') {
		t.Fatalf("away momentum lost its buckets: %q", rows[1])
	}
	if strings.ContainsRune(rows[1], '█') {
		t.Fatalf("away momentum shows a peak that belongs to nobody: %q", rows[1])
	}
	if momentumRows(nil, 36) != nil {
		t.Fatal("empty momentum should render nothing")
	}
	if momentumRows([]int{1, 2}, 1) != nil {
		t.Fatal("momentum narrower than its buckets should render nothing")
	}
}

func TestLiveModalShowsTimelineAndMomentumOnTallLayouts(t *testing.T) {
	m := liveModel(140, 36)
	m.UI["ui.match.timeline"] = "Timeline"
	m.UI["ui.match.momentum"] = "Momentum"
	m.Matches[0].Markers = []LiveMarker{
		{Minute: 12, Kind: "GOAL", Side: "HOME"},
		{Minute: 40, Kind: "CARD", Side: "AWAY"},
	}
	m.Matches[0].Momentum = []int{1, 2, -1, 0, 3, 0, -2, 0, 0}
	v := m.View()
	for _, want := range []string{"Timeline H ", " A ", "Momentum H "} {
		if !strings.Contains(v, want) {
			t.Fatalf("live modal missing broadcast row %q:\n%s", want, v)
		}
	}
	if !strings.Contains(v, "G") || !strings.ContainsRune(v, '█') {
		t.Fatalf("live modal missing timeline glyph or momentum peak:\n%s", v)
	}

	short := liveModel(100, 24)
	short.UI["ui.match.timeline"] = "Timeline"
	short.Matches[0].Momentum = []int{1, 2, -1, 0, 3, 0, -2, 0, 0}
	if v := short.View(); strings.Contains(v, "Timeline H ") {
		t.Fatalf("short live modal should omit the timeline:\n%s", v)
	}
}

func TestMatchPhaseLabel(t *testing.T) {
	m := liveModel(140, 36)
	m.UI["ui.match.phase.first"] = "1H"
	m.UI["ui.match.phase.second"] = "2H"
	if got := m.matchPhaseLabel(45); got != "1H" {
		t.Fatalf("45' phase = %q, want 1H", got)
	}
	if got := m.matchPhaseLabel(46); got != "2H" {
		t.Fatalf("46' phase = %q, want 2H", got)
	}
}

func TestElsewhereTickerListsOtherMatchesAndFlagsFreshGoals(t *testing.T) {
	m := liveModel(140, 36)
	m.UI["ui.match.latest"] = "Elsewhere"
	m.Matches = append(m.Matches,
		LiveMatchView{Fixture: 11, Home: "Gamma", Away: "Delta", HomeGoals: 2, AwayGoals: 0, Minute: 61,
			Markers: []LiveMarker{{Minute: 60, Kind: "GOAL", Side: "HOME"}}},
		LiveMatchView{Fixture: 12, Home: "Epsilon", Away: "Zeta", HomeGoals: 0, AwayGoals: 1, Minute: 62,
			Markers: []LiveMarker{{Minute: 40, Kind: "GOAL", Side: "AWAY"}}},
	)
	current := m.Matches[0].Fixture
	ticker := m.elsewhereTicker(current, 120)
	if !strings.Contains(ticker, "Elsewhere") {
		t.Fatalf("ticker missing label: %q", ticker)
	}
	if strings.Contains(ticker, m.Matches[0].Home+" ") && strings.Contains(ticker, fmt.Sprintf("%s %d-%d", m.Matches[0].Home, m.Matches[0].HomeGoals, m.Matches[0].AwayGoals)) {
		t.Fatalf("ticker should exclude the match being watched: %q", ticker)
	}
	if !strings.Contains(ticker, "G! Gamma 2-0 Delta") {
		t.Fatalf("fresh goal not highlighted: %q", ticker)
	}
	if strings.Contains(ticker, "G! Epsilon") {
		t.Fatalf("stale goal wrongly highlighted: %q", ticker)
	}
	// A chance right after a fresh goal must not age the highlight out.
	m.Matches[1].Markers = append(m.Matches[1].Markers, LiveMarker{Minute: 61, Kind: "CHANCE", Side: "AWAY"})
	if ticker := m.elsewhereTicker(current, 120); !strings.Contains(ticker, "G! Gamma 2-0 Delta") {
		t.Fatalf("fresh goal lost behind a newer marker: %q", ticker)
	}
	if m.elsewhereTicker(current, 0) != "" {
		t.Fatal("zero-width ticker should be empty")
	}
	solo := liveModel(140, 36)
	if solo.elsewhereTicker(solo.Matches[0].Fixture, 120) != "" {
		t.Fatal("single live match should not render a ticker")
	}

	v := m.View()
	if !strings.Contains(v, "Elsewhere") || !strings.Contains(v, "G! Gamma 2-0 Delta") {
		t.Fatalf("live modal missing elsewhere ticker:\n%s", v)
	}

	// Narrow-but-tall terminals keep the ticker: tallness is the only gate.
	narrow := m
	narrow.Width, narrow.Height = 72, 30
	if v := narrow.View(); !strings.Contains(v, "Elsewhere") {
		t.Fatalf("narrow-but-tall live modal lost the ticker:\n%s", v)
	}
}

func TestFixtureListShowsLiveMinute(t *testing.T) {
	m := testModel()
	m.Width, m.Height = 140, 40
	m.Tab = tabFixtures
	m.Matches = []LiveMatchView{{Fixture: m.Fixtures[1].ID, Home: "C", Away: "D", Minute: 63}}
	v := m.View()
	if !strings.Contains(v, "Live 63'") {
		t.Fatalf("fixture list missing live minute:\n%s", v)
	}
}
