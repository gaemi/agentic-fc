package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plain(s string) string { return ansiRE.ReplaceAllString(s, "") }

func testModel() Model {
	m := NewModel(nil)
	m.Width, m.Height = 80, 24
	m.UI = map[string]string{
		"ui.app.title":                "Agentic FC",
		"ui.tab.media":                "Media",
		"ui.tab.table":                "Table",
		"ui.tab.clubs":                "Clubs",
		"ui.tab.fixtures":             "Fixtures",
		"ui.tab.match":                "Match",
		"ui.col.pos":                  "Pos",
		"ui.col.club":                 "Club",
		"ui.col.div":                  "Div",
		"ui.col.security":             "Security",
		"ui.col.source":               "Source",
		"ui.col.article":              "Article",
		"ui.col.name":                 "Name",
		"ui.col.age":                  "Age",
		"ui.col.position":             "Pos",
		"ui.col.attributes":           "Attributes",
		"ui.col.contract":             "Contract",
		"ui.col.played":               "P",
		"ui.col.won":                  "W",
		"ui.col.drawn":                "D",
		"ui.col.lost":                 "L",
		"ui.col.gf":                   "GF",
		"ui.col.ga":                   "GA",
		"ui.col.pts":                  "Pts",
		"ui.col.round":                "Rd",
		"ui.col.kickoff":              "Kick-off",
		"ui.col.fixture":              "Fixture",
		"ui.col.status":               "Status",
		"ui.table.live":               "Live standings",
		"ui.fixtures.empty":           "No fixtures",
		"ui.fixture.scheduled":        "Soon",
		"ui.fixture.result":           "Result",
		"ui.fixture.replay":           "Replay",
		"ui.fixture.archived":         "Archive",
		"ui.fixture.scheduled_detail": "Not kicked off",
		"ui.match.loading":            "Loading",
		"ui.match.away":               "Away",
		"ui.match.winner":             "Winner",
		"ui.match.scorers":            "Scorers",
		"ui.match.cards":              "Cards",
		"ui.match.subs":               "Subs",
		"ui.match.replay":             "Replay log",
		"ui.match.replay.more":        "More",
		"ui.match.replay.archived":    "Archived no replay",
		"ui.match.ratings":            "Ratings",
		"ui.match.stat.shots":         "Shots",
		"ui.match.stat.cards":         "Cards",
		"ui.match.stat.subs":          "Subs",
		"ui.match.stat.chance_mix":    "Chance mix",
		"term.chance_type.COUNTER":    "Counters",
		"term.chance_type.CUTBACK":    "Cutbacks",
		"ui.header.division":          "Division {tier}",
		"ui.help.keys":                "help",
		"ui.media.empty":              "No press",
		"ui.media.recent":             "Recent press",
		"ui.notice.news":              "Fresh story: {title}",
		"ui.notice.match":             "{count} live match window(s) just opened.",
		"ui.clubs.empty":              "No clubs",
		"ui.club.caretaker":           "Caretaker",
		"ui.club.manager":             "Manager",
		"ui.club.predicted":           "Predicted",
		"ui.club.objective":           "Objective",
		"ui.club.confidence":          "Board",
		"ui.club.security":            "Job",
		"ui.club.fan_mood":            "Fans",
		"ui.club.balance":             "Balance",
		"ui.club.wage_bill":           "Wages",
		"ui.club.transfer_budget":     "Transfer",
		"ui.club.squad":               "Squad",
		"ui.player.empty":             "No player",
		"ui.player.dossier":           "Player dossier",
		"ui.player.group":             "Unit",
		"ui.player.body":              "Body",
		"ui.player.foot":              "Foot",
		"ui.player.familiarity":       "Familiarity",
		"ui.player.profile":           "Attribute profile",
		"ui.player.youth":             "Academy player",
		"ui.terminal.too_small":       "too small {min_cols}x{min_rows} now {cols}x{rows}",
		"attr.PACE":                   "Pace",
		"attr.FINISHING":              "Finishing",
		"attr.PASSING":                "Passing",
	}
	m.World = WorldInfo{Name: "Testshire League", ClockText: "Aug 16, 15:00 · Season 1",
		TempoLabel: "Idle", Divisions: 2}
	m.Table = Table{Tier: 1, Label: "Last season standings", Rows: []TableRow{
		{Pos: 1, Club: "Alderton Athletic", Played: 22, Won: 15, Drawn: 4, Lost: 3,
			GF: 40, GA: 18, Points: 49},
	}}
	m.News = []NewsArticle{{
		Source: "Club Desk", TimeText: "Aug 16, 15:00", Category: "board",
		Title: "Alderton appoint Lee Carter",
		Deck:  "Boardroom pressure has produced a fresh development.",
		Body:  "Alderton appoint Lee Carter. Supporters will expect a quick response.",
	}}
	m.Clubs = []ClubSummary{{ID: 10, Name: "Alderton Athletic", Tier: 1, Manager: "Lee Carter", Security: "Stable"}}
	m.Club = ClubDetail{
		ClubSummary:          m.Clubs[0],
		PredictedFinish:      4,
		BoardObjectiveFinish: 6,
		Board:                map[string]string{"confidence": "Watchful", "security": "Stable", "fan_mood": "Steady"},
		Finances:             map[string]string{"cash": "cr2M", "salary_bill": "cr50k", "market_funds": "cr400k"},
		Squad: []Player{{
			Name: "Rae Quinn", Age: 22, HeightCm: 188, WeightKg: 84, Position: "ST", Foot: "RIGHT", WeakFootLabel: "Useful", ContractExpirySeason: 2,
			Attributes: map[string]int{"PACE": 14, "FINISHING": 13, "PASSING": 8},
		}, {
			Name: "Mina Holt", Age: 19, HeightCm: 174, WeightKg: 68, Position: "CM", Foot: "LEFT", WeakFootLabel: "Limited", ContractExpirySeason: 3,
			Attributes: map[string]int{"PASSING": 12, "VISION": 11, "STAMINA": 10},
		}},
	}
	m.Fixtures = []Fixture{
		{ID: 7, Status: "RESULT", Round: 1, KickoffText: "Aug 16, 15:00", Home: "A", Away: "B", HomeGoals: 2, AwayGoals: 1, HasReplay: true},
		{ID: 8, Status: "SCHEDULED", Round: 2, KickoffText: "Aug 23, 15:00", Home: "C", Away: "D"},
	}
	m.MatchDetail = MatchDetail{
		Fixture: 7, Status: "RESULT", Competition: "LEAGUE", KickoffText: "Aug 16, 15:00",
		Home: "A", Away: "B", HomeGoals: 2, AwayGoals: 1, HomeShots: 8, AwayShots: 3,
		ChanceTypes: map[string]int{"COUNTER": 2, "CUTBACK": 1},
		Scorers:     []MatchEvent{{Minute: 12, Club: "A", Player: "Rae Quinn"}},
		Cards:       []MatchEvent{{Minute: 70, Club: "B", Player: "Lee Ward", Detail: "YELLOW"}},
		Subs:        []MatchSub{{Minute: 65, Club: "A", Off: "Old Legs", On: "Fresh Legs", Reason: "TACTICAL"}},
		Ratings:     []LiveRating{{Name: "Rae Quinn", RatingX10: 78}},
		Commentary:  []string{"A work the ball through midfield.", "Rae Quinn lashes it home."},
	}
	return m
}

func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func key(s string) tea.KeyMsg {
	if s == "left" {
		return tea.KeyMsg{Type: tea.KeyLeft}
	}
	if s == "right" {
		return tea.KeyMsg{Type: tea.KeyRight}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestTabsAndDivisionSwitch(t *testing.T) {
	m := testModel()
	m = update(m, key("2"))
	if m.Tab != tabTable {
		t.Fatalf("tab = %d", m.Tab)
	}
	m = update(m, key("3"))
	if m.Tab != tabClubs {
		t.Fatalf("tab = %d", m.Tab)
	}
	m = update(m, key("4"))
	if m.Tab != tabFixtures {
		t.Fatalf("tab = %d", m.Tab)
	}
	// Division bounds: 1..World.Divisions.
	m = update(m, key("left"))
	if m.Tier != 1 {
		t.Fatalf("tier went below 1: %d", m.Tier)
	}
	m = update(m, key("right"))
	if m.Tier != 2 {
		t.Fatalf("tier = %d", m.Tier)
	}
	m = update(m, key("right"))
	if m.Tier != 2 {
		t.Fatalf("tier exceeded divisions: %d", m.Tier)
	}
}

func TestViewRendersChrome(t *testing.T) {
	m := testModel()
	v := m.View()
	for _, want := range []string{"Testshire League", "Aug 16, 15:00", "Division 1",
		"Agentic FC", "Alderton appoint Lee Carter", "Club Desk", "help"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q", want)
		}
	}
	lines := strings.Split(v, "\n")
	if len(lines) != m.Height {
		t.Fatalf("view lines = %d, want %d:\n%s", len(lines), m.Height, v)
	}
	if !strings.HasPrefix(lines[0], "┌") || !strings.HasSuffix(lines[len(lines)-1], "┘") {
		t.Fatalf("view frame missing:\n%s", v)
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got != m.Width {
			t.Fatalf("line %d width = %d, want %d: %q", i, got, m.Width, line)
		}
	}
}

func TestKoreanTableKeepsColumnWidths(t *testing.T) {
	m := testModel()
	m.Width, m.Height = 96, 24
	m.UI["ui.tab.media"] = "매체"
	m.UI["ui.tab.table"] = "순위표"
	m.UI["ui.tab.clubs"] = "클럽"
	m.UI["ui.tab.fixtures"] = "일정"
	m.UI["ui.tab.match"] = "경기"
	m.UI["ui.header.division"] = "{tier}부 리그"
	m.UI["ui.col.pos"] = "순위"
	m.UI["ui.col.club"] = "클럽"
	m.UI["ui.col.played"] = "경기"
	m.UI["ui.col.won"] = "승"
	m.UI["ui.col.drawn"] = "무"
	m.UI["ui.col.lost"] = "패"
	m.UI["ui.col.gf"] = "득점"
	m.UI["ui.col.ga"] = "실점"
	m.UI["ui.col.pts"] = "승점"
	m.UI["ui.help.keys"] = "1 매체 · 2 순위표 · 3 클럽 · 4 일정 · 5 경기 · q 종료"
	m.Table.Rows = []TableRow{
		{Pos: 1, Club: "서울 유나이티드", Played: 2, Won: 1, Drawn: 1, Lost: 0, GF: 4, GA: 2, Points: 4},
		{Pos: 2, Club: "Rot-Weiss Lindenbach", Played: 2, Won: 1, Drawn: 0, Lost: 1, GF: 3, GA: 3, Points: 3},
	}

	m.Tab = tabTable
	v := m.View()
	if !strings.Contains(v, "│순위│") || !strings.Contains(v, "서울 유나이티드") {
		t.Fatalf("localized table missing expected cells:\n%s", v)
	}
	for _, line := range strings.Split(v, "\n") {
		if strings.Contains(line, "│") && lipgloss.Width(line) != m.Width {
			t.Fatalf("framed table line width = %d, want %d: %q", lipgloss.Width(line), m.Width, line)
		}
	}
}

func TestViewTooSmall(t *testing.T) {
	m := testModel()
	m.Width, m.Height = 40, 10
	v := m.View()
	if !strings.Contains(v, "too small 60x16 now 40x10") {
		t.Fatalf("XS view = %q", v)
	}
}

func TestMediaAndClubScreens(t *testing.T) {
	m := testModel()
	if v := m.View(); !strings.Contains(v, "Alderton appoint Lee Carter") || !strings.Contains(v, "Recent press") {
		t.Fatalf("media view missing article:\n%s", v)
	}
	if v := m.mediaDetail(90, 20, m.News[0]); !strings.Contains(v, "AGENTIC FC MEDIA") || !strings.Contains(v, "╔") {
		t.Fatalf("media detail missing masthead:\n%s", v)
	}
	m.News[0].Body = "Opening line of the article sets the scene.\nSecond line carries the local reaction.\nThird line reaches the archive scroll target."
	m.ArticleOffset = 2
	if v := plain(m.mediaDetail(70, 12, m.News[0])); strings.Contains(v, "Opening line") || !strings.Contains(v, "Third line") {
		t.Fatalf("media detail did not honor article scroll offset:\n%s", v)
	}
	m.ArticleOffset = 0
	m = update(m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.ArticleOffset == 0 {
		t.Fatal("PageDown did not advance media article scroll")
	}
	m.News = append(m.News, NewsArticle{Source: "Transfer Wire", Title: "Second item", Body: "Follow-up."})
	m = update(m, key("down"))
	if m.ArticleOffset != 0 {
		t.Fatalf("news selection did not reset article scroll: %d", m.ArticleOffset)
	}
	m.Tab = tabClubs
	m.Width, m.Height = 140, 36
	v := m.View()
	for _, want := range []string{"Alderton Athletic", "AGENTIC", "Lee Carter", "Watchful", "Rae Quinn", "Player dossier", "188cm / 84kg", "Pace 14", "████"} {
		if !strings.Contains(v, want) {
			t.Fatalf("club view missing %q:\n%s", want, v)
		}
	}
}

func TestMascotNoticeOverlay(t *testing.T) {
	m := testModel()
	m.Width, m.Height = 120, 32
	m.Notice = "Fresh story: Alderton appoint Lee Carter"
	m.NoticeTTL = noticeTicks
	v := m.View()
	for _, want := range []string{"◖●●◗", "Fresh story", "Alderton"} {
		if !strings.Contains(v, want) {
			t.Fatalf("notice overlay missing %q:\n%s", want, v)
		}
	}

	for range noticeTicks {
		m = update(m, tickMsg{})
	}
	if m.Notice != "" || strings.Contains(m.View(), "Fresh story") {
		t.Fatal("notice should expire after its TTL")
	}
}

func TestMouseSelectsTabsAndRows(t *testing.T) {
	m := testModel()
	m.Width, m.Height = 140, 36
	var clubClick int
	for _, span := range m.tabSpans() {
		if span.tab == tabClubs {
			clubClick = (span.start + span.end) / 2
			break
		}
	}
	m = update(m, tea.MouseMsg{X: clubClick, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.Tab != tabClubs {
		t.Fatalf("mouse tab click selected tab %d, want clubs", m.Tab)
	}
	for _, span := range m.tabSpans() {
		label := map[int]string{
			tabMedia:    "1 Media",
			tabTable:    "2 Table",
			tabClubs:    "3 Clubs",
			tabFixtures: "4 Fixtures",
			tabMatch:    "5 Match",
		}[span.tab]
		tabLine := plain(strings.Split(m.View(), "\n")[2])
		byteIdx := strings.Index(tabLine, label)
		if byteIdx < 0 {
			t.Fatalf("tab %q missing in %q", label, tabLine)
		}
		if got := lipgloss.Width(tabLine[:byteIdx]); got != span.start {
			t.Fatalf("tab %q rendered at cell %d, span starts at %d in %q", label, got, span.start, tabLine)
		}
		clicked := update(m, tea.MouseMsg{X: (span.start + span.end) / 2, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if clicked.Tab != span.tab {
			t.Fatalf("tab span %+v selected tab %d", span, clicked.Tab)
		}
	}
	m.Tab = tabClubs
	m = update(m, tea.MouseMsg{X: 86, Y: 16, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.PlayerIdx != 1 {
		t.Fatalf("mouse squad click selected player %d, want 1", m.PlayerIdx)
	}

	m.Tab = tabFixtures
	m = update(m, tea.MouseMsg{X: 5, Y: 8, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.FixtureIdx != 1 {
		t.Fatalf("mouse fixture click selected fixture %d, want 1", m.FixtureIdx)
	}
}

func TestFixtureResultsScreenShowsReplay(t *testing.T) {
	m := testModel()
	m.Width, m.Height = 140, 40
	m.Tab = tabFixtures
	v := m.View()
	for _, want := range []string{"Replay", "A 2-1 B", "Chance mix Counters 2", "Scorers", "Rae Quinn", "Replay log", "lashes it home"} {
		if !strings.Contains(v, want) {
			t.Fatalf("fixtures/results view missing %q:\n%s", want, v)
		}
	}

	m.MatchDetail.Commentary = append(m.MatchDetail.Commentary,
		"12' The crowd rises.",
		"18' A second wave of pressure.",
		"25' The keeper punches clear.",
		"31' Midfielders trade possession.",
		"40' The away bench is restless.",
		"52' The match opens up.",
		"68' A late tackle brings whistles.",
		"75' Fresh legs arrive.",
		"88' The last attack fades.",
	)
	m = update(m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.ReplayOffset == 0 {
		t.Fatal("PageDown did not advance replay scroll")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.ReplayOffset != 0 {
		t.Fatalf("PageUp did not rewind replay scroll: %d", m.ReplayOffset)
	}

	m = update(m, key("down"))
	if m.FixtureIdx != 1 {
		t.Fatalf("fixture selection = %d, want 1", m.FixtureIdx)
	}
	if v := m.viewFixtures(80, 20); !strings.Contains(v, "Not kicked off") {
		t.Fatalf("scheduled detail missing:\n%s", v)
	}
}

func TestSelectedRowsStayVisible(t *testing.T) {
	if got := visibleStart(18, 30, 10); got != 13 {
		t.Fatalf("middle selected start = %d, want 13", got)
	}
	if got := visibleStart(29, 30, 10); got != 20 {
		t.Fatalf("tail selected start = %d, want 20", got)
	}
	if got := visibleStart(2, 30, 10); got != 0 {
		t.Fatalf("head selected start = %d, want 0", got)
	}
}

func liveModel(width, height int) Model {
	m := NewModel(nil)
	m.Width, m.Height = width, height
	m.Tab = tabMatch
	m.UI = map[string]string{
		"ui.match.none":       "quiet",
		"ui.match.legend":     "legend",
		"ui.match.scoreboard": "SCOREBOARD",
		"ui.match.goalflash":  "GOAL",
		"ui.match.replay":     "COMMENTARY",
	}
	m.Matches = []LiveMatchView{{
		Home: "Alpha", Away: "Beta", HomeGoals: 2, AwayGoals: 1, Minute: 61,
		Competition: "LEAGUE",
		Commentary:  []string{"line one", "line two"},
		Markers: []LiveMarker{
			{Minute: 12, Kind: "GOAL", Side: "HOME"},
			{Minute: 40, Kind: "CARD", Side: "AWAY"},
		},
	}}
	return m
}

// TestMatchScreenTiers locks the docs/07 §4.1 degradation ladder: tier S is
// pure commentary (no pitch border at all), M draws the strip and 'p' folds
// it away, L renders the full band with the event glyphs on it.
func TestMatchScreenTiers(t *testing.T) {
	small := liveModel(70, 24) // TierS
	if v := small.View(); strings.Contains(v, "+--") {
		t.Fatal("tier S must stay pure commentary — no pitch border")
	} else if !strings.Contains(v, "line two") {
		t.Fatal("tier S lost the commentary itself")
	}

	mid := liveModel(110, 30) // TierM
	if v := mid.View(); !strings.Contains(v, "+--") {
		t.Fatal("tier M should draw the compact strip by default")
	}
	toggled, _ := mid.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if v := toggled.(Model).View(); strings.Contains(v, "+--") {
		t.Fatal("'p' must fold the M-tier strip back to commentary")
	}

	large := liveModel(150, 40) // TierL
	v := large.View()
	if !strings.Contains(v, "+--") || !strings.Contains(v, "G") || !strings.Contains(v, "x") {
		t.Fatal("tier L must render the full band with goal and card glyphs")
	}
	if !strings.Contains(v, "Alpha 2 - 1 Beta") {
		t.Fatal("score header missing")
	}
	if !strings.Contains(v, "SCOREBOARD") || !strings.Contains(v, "█████") {
		t.Fatal("tier L must render the big scoreboard")
	}
	if strings.Contains(v, ">>>  GOAL") {
		t.Fatal("goal flash should only render when the latest marker is a goal")
	}
	if !strings.Contains(v, "legend") {
		t.Fatal("marker legend missing beside the pitch")
	}
}

func TestMatchGoalFlash(t *testing.T) {
	m := liveModel(150, 40)
	m.Matches[0].Markers = append(m.Matches[0].Markers, LiveMarker{Minute: 62, Kind: "GOAL", Side: "AWAY"})
	v := m.View()
	if !strings.Contains(v, ">>>  GOAL  62'  Beta  <<<") {
		t.Fatalf("latest-goal flash missing:\n%s", v)
	}
	if !strings.Contains(v, "▓") || !strings.Contains(v, "SCOREBOARD GOAL") {
		t.Fatalf("latest-goal flash missing burst graphics:\n%s", v)
	}

	small := liveModel(70, 24)
	small.Matches[0].Markers = append(small.Matches[0].Markers, LiveMarker{Minute: 62, Kind: "GOAL", Side: "AWAY"})
	if v := small.View(); strings.Contains(v, ">>>  GOAL") || strings.Contains(v, "SCOREBOARD") {
		t.Fatal("tier S should remain pure commentary without scoreboard or flash")
	}
}

// TestMatchScreenEmptyAndGoalSides locks the empty state and the goalmouth
// geometry: a HOME goal lands in the right half of the field, an AWAY goal in
// the left half.
func TestMatchScreenEmptyAndGoalSides(t *testing.T) {
	empty := liveModel(150, 40)
	empty.Matches = nil
	if v := empty.View(); !strings.Contains(v, "quiet") {
		t.Fatal("empty state must show ui.match.none")
	}

	pitch := renderPitch(80, 9, []LiveMarker{{Kind: "GOAL", Side: "HOME"}})
	for _, line := range strings.Split(pitch, "\n") {
		if i := strings.IndexRune(line, 'G'); i >= 0 && i <= 40 {
			t.Fatalf("HOME goal glyph at col %d — must sit in the right half", i)
		}
	}
	pitch = renderPitch(80, 9, []LiveMarker{{Kind: "GOAL", Side: "AWAY"}})
	for _, line := range strings.Split(pitch, "\n") {
		if i := strings.IndexRune(line, 'G'); i >= 40 {
			t.Fatalf("AWAY goal glyph at col %d — must sit in the left half", i)
		}
	}
}

// paneModel is liveModel plus the §4.1 side-pane data: stats, ratings for
// both sides, a momentum curve, and a second match for the ticker.
func paneModel(width, height int) Model {
	m := liveModel(width, height)
	m.UI["ui.match.stats"] = "MATCH-STATS"
	m.UI["ui.match.ratings"] = "RATINGS"
	m.UI["ui.match.latest"] = "LATEST"
	m.UI["ui.match.momentum"] = "MOMENTUM"
	m.UI["ui.match.stat.shots"] = "Shots"
	m.UI["ui.match.stat.quality"] = "Quality"
	m.UI["ui.match.stat.aerial"] = "Aerial"
	m.UI["ui.match.stat.press"] = "Press"
	m.UI["ui.match.stat.setpieces"] = "Set pieces"
	m.UI["term.chance_type.CUTBACK"] = "Cutbacks"
	m.UI["term.quality.HIGH"] = "High"
	m.UI["term.quality.MEDIUM"] = "Medium"
	m.Matches[0].Stats = LiveStats{
		HomeShots: 7, AwayShots: 3, HomeCards: 1, AwayCards: 2, HomeSubs: 2, AwaySubs: 0,
		ChanceTypes: map[string]int{"CUTBACK": 2},
		Diagnostics: MatchDiagnostics{
			ShotQuality:    map[string]int{"HIGH": 1, "MEDIUM": 2},
			AerialDuels:    map[string]int{"HOME": 3, "AWAY": 1},
			AerialWins:     map[string]int{"HOME": 2},
			PressTurnovers: map[string]int{"HOME": 1},
		},
	}
	m.Matches[0].Ratings = []LiveRating{
		{Side: "HOME", Name: "Hero", RatingX10: 76},
		{Side: "HOME", Name: "Solid", RatingX10: 68},
		{Side: "AWAY", Name: "Villain", RatingX10: 61},
	}
	m.Matches[0].Momentum = []int{0, 3, 1, -1, 0, 0, 4, 0, 0}
	m.Matches = append(m.Matches, LiveMatchView{
		Home: "Gamma", Away: "Delta", HomeGoals: 0, AwayGoals: 0, Minute: 30,
	})
	return m
}

// TestMatchSidePanes locks the §4.1 side-pane matrix: L shows stats + ratings
// (ticker only when tall), XL keeps the ticker column, M shows one toggleable
// pane ('r' cycles stats → ratings → hidden), S swaps the main block, and the
// momentum sparkline sits under the header from L up.
func TestMatchSidePanes(t *testing.T) {
	large := paneModel(150, 45) // TierL, tall → ticker
	v := large.View()
	for _, want := range []string{"MATCH-STATS", "Cutbacks 2", "Quality High 1", "Aerial H 2/3", "Press H 1", "RATINGS", "7.6 Hero", "6.1 Villain", "MOMENTUM", "LATEST", "Gamma 0-0 Delta 30'"} {
		if !strings.Contains(v, want) {
			t.Fatalf("tier L tall missing %q:\n%s", want, v)
		}
	}
	if !strings.Contains(v, "△") || !strings.Contains(v, "▲") || !strings.Contains(v, "▽") {
		t.Fatal("momentum glyphs missing (goal ▲, light pressure △/▽)")
	}

	shortL := paneModel(150, 30) // TierL, short → no ticker
	if v := shortL.View(); strings.Contains(v, "LATEST") {
		t.Fatal("tier L below the tall threshold must drop the ticker")
	} else if !strings.Contains(v, "MATCH-STATS") {
		t.Fatal("tier L short must keep the stats pane")
	}

	xl := paneModel(190, 30) // TierXL → persistent ticker even short
	if v := xl.View(); !strings.Contains(v, "LATEST") {
		t.Fatal("tier XL must keep the ticker column")
	}

	mid := paneModel(110, 30) // TierM: one pane, 'r' cycles
	if v := mid.View(); !strings.Contains(v, "MATCH-STATS") || strings.Contains(v, "RATINGS") {
		t.Fatal("tier M default must show the stats pane alone")
	}
	r1, _ := mid.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if v := r1.(Model).View(); !strings.Contains(v, "RATINGS") || strings.Contains(v, "MATCH-STATS") {
		t.Fatal("tier M 'r' must swap to the ratings pane")
	}
	r2, _ := r1.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if v := r2.(Model).View(); strings.Contains(v, "RATINGS") || strings.Contains(v, "MATCH-STATS") {
		t.Fatal("tier M second 'r' must hide the pane")
	}
	if v := r2.(Model).View(); strings.Contains(v, "MOMENTUM") {
		t.Fatal("the momentum sparkline is L/XL only")
	}

	small := paneModel(70, 24) // TierS: 'r' swaps the main block
	s1, _ := small.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if v := s1.(Model).View(); !strings.Contains(v, "MATCH-STATS") || strings.Contains(v, "line two") {
		t.Fatal("tier S 'r' must swap commentary for the stats block")
	}
	s2, _ := s1.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if v := s2.(Model).View(); !strings.Contains(v, "RATINGS") {
		t.Fatal("tier S second 'r' must show the ratings block")
	}
	s3, _ := s2.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if v := s3.(Model).View(); !strings.Contains(v, "line two") {
		t.Fatal("tier S third 'r' must return to commentary")
	}
}
