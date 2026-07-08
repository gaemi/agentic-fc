package tui

import (
	"errors"
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
		"ui.app.title":                             "Agentic FC",
		"ui.tab.media":                             "Media",
		"ui.tab.table":                             "Table",
		"ui.tab.clubs":                             "Clubs",
		"ui.tab.fixtures":                          "Fixtures",
		"ui.tab.admin_settings":                    "Settings",
		"ui.col.pos":                               "Pos",
		"ui.col.club":                              "Club",
		"ui.col.div":                               "Div",
		"ui.col.security":                          "Security",
		"ui.col.source":                            "Source",
		"ui.col.article":                           "Article",
		"ui.col.name":                              "Name",
		"ui.col.age":                               "Age",
		"ui.col.position":                          "Pos",
		"ui.col.attributes":                        "Attributes",
		"ui.col.contract":                          "Contract",
		"ui.col.played":                            "P",
		"ui.col.won":                               "W",
		"ui.col.drawn":                             "D",
		"ui.col.lost":                              "L",
		"ui.col.gf":                                "GF",
		"ui.col.ga":                                "GA",
		"ui.col.pts":                               "Pts",
		"ui.col.round":                             "Rd",
		"ui.col.kickoff":                           "Kick-off",
		"ui.col.fixture":                           "Fixture",
		"ui.col.status":                            "Status",
		"ui.table.live":                            "Live standings",
		"ui.fixtures.empty":                        "No fixtures",
		"ui.fixture.live":                          "Live",
		"ui.fixture.scheduled":                     "Soon",
		"ui.fixture.result":                        "Result",
		"ui.fixture.replay":                        "Replay",
		"ui.fixture.archived":                      "Archive",
		"ui.fixture.scheduled_notice":              "Not kicked off",
		"ui.match.loading":                         "Loading",
		"ui.match.live":                            "Live match",
		"ui.match.ended":                           "Match ended",
		"ui.match.waiting_result":                  "Waiting result",
		"ui.match.modal.close":                     "Esc close",
		"ui.match.modal.replay_help":               "PgUp/PgDn",
		"ui.match.away":                            "Away",
		"ui.match.winner":                          "Winner",
		"ui.match.scorers":                         "Scorers",
		"ui.match.cards":                           "Cards",
		"ui.match.subs":                            "Subs",
		"ui.match.replay":                          "Replay log",
		"ui.match.replay.more":                     "More",
		"ui.match.replay.archived":                 "Archived no replay",
		"ui.match.ratings":                         "Ratings",
		"ui.match.stat.shots":                      "Shots",
		"ui.match.stat.cards":                      "Cards",
		"ui.match.stat.subs":                       "Subs",
		"ui.match.stat.chance_mix":                 "Chance mix",
		"term.chance_type.COUNTER":                 "Counters",
		"term.chance_type.CUTBACK":                 "Cutbacks",
		"ui.header.division":                       "Division {tier}",
		"ui.help.keys":                             "help",
		"ui.help.keys_admin":                       "admin help",
		"ui.admin.token_required":                  "Admin token required",
		"ui.admin.settings.loading":                "Loading settings",
		"ui.admin.settings.title":                  "Runtime Settings",
		"ui.admin.settings.help":                   "adjust settings",
		"ui.admin.settings.setting":                "Setting",
		"ui.admin.settings.value":                  "Value",
		"ui.admin.settings.allowed":                "Allowed",
		"ui.admin.settings.game_speed":             "Game speed",
		"ui.admin.settings.idle_acceleration":      "Idle acceleration",
		"ui.admin.settings.offseason_acceleration": "Off-season acceleration",
		"ui.admin.settings.determinism":            "Determinism",
		"ui.admin.settings.rebuild_required":       "New world required",
		"ui.admin.settings.saved":                  "Saved",
		"ui.media.empty":                           "No press",
		"ui.media.recent":                          "Recent press",
		"ui.notice.news":                           "Fresh story: {title}",
		"ui.notice.match":                          "{count} live match window(s) just opened.",
		"ui.clubs.empty":                           "No clubs",
		"ui.club.caretaker":                        "Caretaker",
		"ui.club.manager":                          "Manager",
		"ui.club.predicted":                        "Predicted",
		"ui.club.objective":                        "Objective",
		"ui.club.confidence":                       "Board",
		"ui.club.security":                         "Job",
		"ui.club.fan_mood":                         "Fans",
		"ui.club.balance":                          "Balance",
		"ui.club.wage_bill":                        "Wages",
		"ui.club.transfer_budget":                  "Transfer",
		"ui.club.squad":                            "Squad",
		"ui.player.empty":                          "No player",
		"ui.player.dossier":                        "Player dossier",
		"ui.player.group":                          "Unit",
		"ui.player.body":                           "Body",
		"ui.player.foot":                           "Foot",
		"ui.player.familiarity":                    "Familiarity",
		"ui.player.profile":                        "Attribute profile",
		"ui.player.youth":                          "Academy player",
		"ui.terminal.too_small":                    "too small {min_cols}x{min_rows} now {cols}x{rows}",
		"attr.PACE":                                "Pace",
		"attr.FINISHING":                           "Finishing",
		"attr.PASSING":                             "Passing",
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
		{ID: 6, Status: "RESULT", Archived: true, Season: 0, Round: 0, KickoffText: "Last season", Home: "E", Away: "F", HomeGoals: 0, AwayGoals: 0},
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
	m = update(m, key("5"))
	if m.Tab != tabFixtures {
		t.Fatalf("removed match tab changed tab to %d", m.Tab)
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

func TestAdminSettingsTabAndAdjustments(t *testing.T) {
	m := testModel()
	m.AdminMode = true
	m.Settings = AdminSettings{
		Runtime: RuntimeSettings{GameSpeed: 15, IdleAcceleration: 16, OffseasonAcceleration: 96},
		Schema: SettingsSchema{
			GameSpeedOptions:     []int{5, 15, 30, 60, 120},
			IdleAccelerationMin:  2,
			IdleAccelerationMax:  64,
			OffseasonAccelMin:    2,
			OffseasonAccelMax:    240,
			Determinism:          "Pacing only",
			RequiresWorldRebuild: []string{"seed"},
		},
	}
	m = update(m, key("5"))
	if m.Tab != tabAdminSettings {
		t.Fatalf("admin settings tab = %d", m.Tab)
	}
	v := m.View()
	for _, want := range []string{"Runtime Settings", "Game speed", "15x", "Idle acceleration", "Pacing only", "admin help"} {
		if !strings.Contains(v, want) {
			t.Fatalf("admin settings missing %q:\n%s", want, v)
		}
	}
	m = update(m, key("+"))
	if m.Settings.Runtime.GameSpeed != 30 {
		t.Fatalf("game speed = %d, want 30", m.Settings.Runtime.GameSpeed)
	}
	m.Settings.Runtime.GameSpeed = 60
	m = update(m, key("+"))
	if m.Settings.Runtime.GameSpeed != 120 {
		t.Fatalf("game speed should use schema options, got %d", m.Settings.Runtime.GameSpeed)
	}
	m = update(m, SettingsMsg{Settings: AdminSettings{Runtime: RuntimeSettings{GameSpeed: 5}}})
	if m.Settings.Runtime.GameSpeed != 120 {
		t.Fatalf("poll should not overwrite dirty settings, got %d", m.Settings.Runtime.GameSpeed)
	}
	m = update(m, ErrMsg{Err: errors.New("news poll failed")})
	if !m.SettingsDirty {
		t.Fatal("unrelated errors should not clear settings dirty state")
	}
	m = update(m, SettingsMsg{Settings: AdminSettings{Runtime: RuntimeSettings{GameSpeed: 60, IdleAcceleration: 16, OffseasonAcceleration: 96}}, Updated: true})
	if !m.SettingsDirty || m.Settings.Runtime.GameSpeed != 120 {
		t.Fatalf("stale update response should keep dirty local settings, dirty=%v runtime=%+v", m.SettingsDirty, m.Settings.Runtime)
	}
	m = update(m, SettingsMsg{Settings: AdminSettings{Runtime: RuntimeSettings{GameSpeed: 120, IdleAcceleration: 16, OffseasonAcceleration: 96}}, Updated: true})
	if m.SettingsDirty {
		t.Fatal("updated settings response should clear dirty flag")
	}
	m = update(m, key("down"))
	m = update(m, key("-"))
	if m.Settings.Runtime.IdleAcceleration != 15 {
		t.Fatalf("idle acceleration = %d, want 15", m.Settings.Runtime.IdleAcceleration)
	}
	m = update(m, key("down"))
	m.Settings.Runtime.OffseasonAcceleration = 2
	m = update(m, key("-"))
	if m.Settings.Runtime.OffseasonAcceleration != 2 {
		t.Fatalf("offseason acceleration should clamp at 2, got %d", m.Settings.Runtime.OffseasonAcceleration)
	}
}

func TestAdminSettingsLoadingDoesNotPatchDefaults(t *testing.T) {
	m := testModel()
	m.AdminMode = true
	m.Tab = tabAdminSettings
	next, cmd := m.Update(key("+"))
	m = next.(Model)
	if cmd != nil {
		t.Fatal("loading settings adjustment should not issue a PATCH command")
	}
	if m.SettingsDirty {
		t.Fatal("loading settings adjustment should not mark settings dirty")
	}
	if m.Settings.Runtime.GameSpeed != 0 {
		t.Fatalf("loading settings adjustment changed runtime settings: %+v", m.Settings.Runtime)
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
	m.UI["ui.help.keys"] = "1 매체 · 2 순위표 · 3 클럽 · 4 일정 · q 종료"
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

func TestKoreanClubViewKeepsWidthsAndAttributeColumns(t *testing.T) {
	m := testModel()
	m.Tab = tabClubs
	m.UI["ui.tab.media"] = "매체"
	m.UI["ui.tab.table"] = "순위표"
	m.UI["ui.tab.clubs"] = "클럽"
	m.UI["ui.tab.fixtures"] = "일정/결과"
	m.UI["ui.help.keys"] = "1 매체 · 2 순위표 · 3 클럽 · 4 결과 · q 종료"
	m.UI["ui.col.position"] = "포지션"
	m.UI["ui.col.attributes"] = "능력치"
	m.UI["ui.player.dossier"] = "선수 파일"
	m.UI["ui.player.profile"] = "능력치 프로필"
	m.UI["ui.club.manager"] = "감독"
	m.UI["ui.club.predicted"] = "예상"
	m.UI["ui.club.objective"] = "목표"
	m.UI["ui.club.confidence"] = "보드 신뢰"
	m.UI["ui.club.security"] = "직위"
	m.UI["attr.COMPOSURE"] = "침착성"
	m.UI["attr.REFLEXES"] = "반사신경"
	m.UI["attr.SWEEPING"] = "스위핑"
	m.UI["attr.CONCENTRATION"] = "집중력"
	m.UI["attr.AERIAL_REACH"] = "공중 리치"
	m.Club.Squad[0].Attributes = map[string]int{
		"COMPOSURE": 14, "REFLEXES": 13, "SWEEPING": 12, "CONCENTRATION": 11, "AERIAL_REACH": 9,
	}

	for _, size := range []struct {
		width  int
		height int
	}{{80, 24}, {92, 36}, {172, 36}} {
		m.Width, m.Height = size.width, size.height
		v := m.View()
		for i, line := range strings.Split(v, "\n") {
			if got := lipgloss.Width(line); got != size.width {
				t.Fatalf("club view line %d width = %d, want %d: %q\n%s", i, got, size.width, line, v)
			}
		}
		if size.width == 80 && !strings.Contains(v, "선수 파일") {
			t.Fatalf("compact club view should reserve selected player detail:\n%s", v)
		}
	}

	attrLines := m.playerAttrLines(m.Club.Squad[0], 54)
	barCol := -1
	for _, line := range attrLines {
		col := cellIndexAny(line, "█░")
		if col < 0 {
			t.Fatalf("attribute line missing bar: %q", line)
		}
		if barCol < 0 {
			barCol = col
			continue
		}
		if col != barCol {
			t.Fatalf("attribute bar column = %d, want %d:\n%s", col, barCol, strings.Join(attrLines, "\n"))
		}
	}
}

func TestPlayerAttributeProfileUsesFixedOrder(t *testing.T) {
	m := testModel()
	m.UI["attr.FINISHING"] = "Finishing"
	m.UI["attr.PASSING"] = "Passing"
	m.UI["attr.VISION"] = "Vision"
	m.UI["attr.ACCELERATION"] = "Acceleration"
	m.UI["attr.PACE"] = "Pace"
	p := Player{
		Attributes: map[string]int{
			"PACE":         20,
			"ACCELERATION": 19,
			"VISION":       9,
			"PASSING":      10,
			"FINISHING":    1,
		},
	}

	lines := plain(strings.Join(m.playerAttrLines(p, 72), "\n"))
	positions := []int{
		strings.Index(lines, "Finishing"),
		strings.Index(lines, "Passing"),
		strings.Index(lines, "Vision"),
		strings.Index(lines, "Acceleration"),
		strings.Index(lines, "Pace"),
	}
	for i, pos := range positions {
		if pos < 0 {
			t.Fatalf("missing attribute %d in:\n%s", i, lines)
		}
		if i > 0 && pos <= positions[i-1] {
			t.Fatalf("attributes are not in fixed profile order: %v\n%s", positions, lines)
		}
	}
}

func cellIndexAny(s, chars string) int {
	idx := strings.IndexAny(s, chars)
	if idx < 0 {
		return -1
	}
	return lipgloss.Width(s[:idx])
}

func TestViewTooSmall(t *testing.T) {
	m := testModel()
	m.Width, m.Height = 40, 10
	v := m.View()
	if !strings.Contains(v, "too small 60x16 now 40x10") {
		t.Fatalf("XS view = %q", v)
	}
}

func TestUIFallbacksAvoidRawKeysForNewerClient(t *testing.T) {
	m := liveModel(100, 24)
	delete(m.UI, "ui.match.current_scene")
	delete(m.UI, "ui.match.history")
	delete(m.UI, "ui.match.goalflash")
	delete(m.UI, "ui.match.scene.build")

	v := m.liveMatchModal(100, 24)
	for _, raw := range []string{"ui.match.current_scene", "ui.match.history", "ui.match.goalflash", "ui.match.scene.build"} {
		if strings.Contains(v, raw) {
			t.Fatalf("raw fallback key leaked %q:\n%s", raw, v)
		}
	}
	for _, want := range []string{"Current scene", "Earlier flow", "Build-up"} {
		if !strings.Contains(v, want) {
			t.Fatalf("fallback label missing %q:\n%s", want, v)
		}
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
	for _, want := range []string{"o-o", "/|\\", "Fresh story", "Alderton"} {
		if !strings.Contains(v, want) {
			t.Fatalf("notice overlay missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "◖") || strings.Contains(v, "▔") {
		t.Fatalf("notice overlay still contains unstable wide glyphs:\n%s", v)
	}

	for range noticeTicks {
		m = update(m, tickMsg{})
	}
	if m.Notice != "" || strings.Contains(m.View(), "Fresh story") {
		t.Fatal("notice should expire after its TTL")
	}
}

func TestCompactNoticeUsesInlineStatus(t *testing.T) {
	m := testModel()
	m.Width, m.Height = 80, 24
	m.Notice = "새 기사: Stanton Albion이 중요한 소식을 전했습니다."
	v := m.View()
	if !strings.Contains(v, "새 기사: Stanton Albion") {
		t.Fatalf("compact notice missing:\n%s", v)
	}
	if strings.Contains(v, "o-o") || strings.Contains(v, "/|\\") {
		t.Fatalf("compact notice should not render mascot overlay:\n%s", v)
	}
	if lines := strings.Split(v, "\n"); len(lines) != 24 {
		t.Fatalf("compact notice changed frame height = %d, want 24:\n%s", len(lines), v)
	}

	m.Err = "network stalled"
	v = m.View()
	if !strings.Contains(v, "network stalled") || !strings.Contains(v, "새 기사: Stanton Albion") {
		t.Fatalf("compact status should include both error and notice:\n%s", v)
	}
	if strings.Contains(v, "o-o") || strings.Contains(v, "/|\\") {
		t.Fatalf("compact error+notice should not render mascot overlay:\n%s", v)
	}

	m.Err = ""
	m.Width = 100
	v = m.View()
	if !strings.Contains(v, "o-o") || !strings.Contains(v, "/|\\") {
		t.Fatalf("width 100 should use overlay notice path:\n%s", v)
	}
}

func TestNoticeOverlaySupportsKoreanText(t *testing.T) {
	cases := []string{
		"새 기사: Stanpool Rovers의 Ronnie Foster, 부상으로 수 주간 이탈.",
		"새 기사: 가나다라마바 사아자차카타파하 가나다라마바 사아자차카타파하",
	}
	for _, text := range cases {
		lines := mascotBubble(text, 34)
		joined := strings.Join(lines, "\n")
		for _, want := range []string{"o-o", "새 기사"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("notice bubble missing %q:\n%s", want, joined)
			}
		}
		for _, line := range lines {
			if got := lipgloss.Width(line); got != 34 {
				t.Fatalf("notice bubble line width = %d, want 34: %q", got, line)
			}
		}
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
	m = update(m, tea.MouseMsg{X: 5, Y: 7, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.FixtureIdx != 0 || m.MatchModal != modalReplay {
		t.Fatalf("mouse fixture click selected fixture %d modal %q, want fixture 0 replay", m.FixtureIdx, m.MatchModal)
	}
	m = update(m, tea.MouseMsg{X: 5, Y: 8, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.FixtureIdx != 0 {
		t.Fatalf("mouse click pierced open modal, selected fixture %d", m.FixtureIdx)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	m = update(m, tea.MouseMsg{X: 5, Y: 8, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.FixtureIdx != 1 || m.Notice != "Not kicked off" {
		t.Fatalf("mouse scheduled click selected fixture %d notice %q, want fixture 1 scheduled notice", m.FixtureIdx, m.Notice)
	}

	m.AdminMode = true
	m.Tab = tabAdminSettings
	m.Settings = AdminSettings{Runtime: RuntimeSettings{GameSpeed: 15, IdleAcceleration: 16, OffseasonAcceleration: 96}}
	m = update(m, tea.MouseMsg{X: 5, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.SettingsIdx != 0 {
		t.Fatalf("mouse admin first row selected %d, want 0", m.SettingsIdx)
	}
	m = update(m, tea.MouseMsg{X: 5, Y: 11, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.SettingsIdx != 1 {
		t.Fatalf("mouse admin second row selected %d, want 1", m.SettingsIdx)
	}
	m = update(m, tea.MouseMsg{X: 5, Y: 12, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.SettingsIdx != 2 {
		t.Fatalf("mouse admin third row selected %d, want 2", m.SettingsIdx)
	}
}

func TestFixtureResultsScreenShowsReplay(t *testing.T) {
	m := testModel()
	m.Width, m.Height = 140, 40
	m.Tab = tabFixtures
	v := m.View()
	for _, want := range []string{"Replay", "A 2-1 B", "C - D"} {
		if !strings.Contains(v, want) {
			t.Fatalf("fixtures/results view missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "Chance mix Counters 2") || strings.Contains(v, "Scorers") {
		t.Fatalf("fixture list should not render the old side detail pane:\n%s", v)
	}

	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.MatchModal != modalReplay {
		t.Fatalf("enter did not open replay modal: %q", m.MatchModal)
	}
	v = m.View()
	for _, want := range []string{"A 2-1 B", "Chance mix Counters 2", "Scorers", "Rae Quinn", "Cards", "Lee Ward", "Subs", "Fresh Legs", "Ratings", "7.8 Rae Quinn", "Replay log", "lashes it home"} {
		if !strings.Contains(v, want) {
			t.Fatalf("replay modal missing %q:\n%s", want, v)
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

	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.MatchModal != modalNone {
		t.Fatalf("Esc did not close replay modal: %q", m.MatchModal)
	}
	m = update(m, key("down"))
	if m.FixtureIdx != 1 {
		t.Fatalf("fixture selection = %d, want 1", m.FixtureIdx)
	}
	if v := m.viewFixtures(80, 20); strings.Contains(v, "Not kicked off") {
		t.Fatalf("scheduled detail should not render as side pane:\n%s", v)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.MatchModal != modalNone || m.Notice != "Not kicked off" {
		t.Fatalf("scheduled enter opened modal %q notice %q", m.MatchModal, m.Notice)
	}
}

func TestCompactReplayModalPrioritizesCommentary(t *testing.T) {
	m := testModel()
	m.Width, m.Height = 80, 24
	m.Tab = tabFixtures

	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.MatchModal != modalReplay {
		t.Fatalf("enter did not open replay modal: %q", m.MatchModal)
	}
	v := m.View()
	for _, want := range []string{"A 2-1 B", "Scorers", "Rae Quinn", "Replay log", "lashes it home"} {
		if !strings.Contains(v, want) {
			t.Fatalf("compact replay missing %q:\n%s", want, v)
		}
	}
	for _, hidden := range []string{"Cards", "Lee Ward", "Ratings", "7.8 Rae Quinn"} {
		if strings.Contains(v, hidden) {
			t.Fatalf("compact replay rendered secondary %q:\n%s", hidden, v)
		}
	}
}

func TestReplayModalAtEightyCellBoxPrioritizesCommentary(t *testing.T) {
	m := testModel()
	m.Width, m.Height = 82, 30
	m.Tab = tabFixtures

	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.MatchModal != modalReplay {
		t.Fatalf("enter did not open replay modal: %q", m.MatchModal)
	}
	v := m.View()
	if !strings.Contains(v, "Replay log") || !strings.Contains(v, "lashes it home") {
		t.Fatalf("80-cell replay box should still show commentary:\n%s", v)
	}
	if strings.Contains(v, "Ratings") || strings.Contains(v, "7.8 Rae Quinn") {
		t.Fatalf("80-cell replay box should omit secondary ratings:\n%s", v)
	}
}

func TestFixtureResultsEnterAndSpaceOpenLiveModal(t *testing.T) {
	m := testModel()
	m.Tab = tabFixtures
	m.FixtureIdx = 1
	m.Matches = []LiveMatchView{{Fixture: 8, Home: "C", Away: "D", HomeGoals: 1, AwayGoals: 0, Minute: 25}}

	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.MatchModal != modalLive || m.MatchModalID != 8 {
		t.Fatalf("enter did not open live modal: modal=%q id=%d", m.MatchModal, m.MatchModalID)
	}

	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	m = update(m, tea.KeyMsg{Type: tea.KeySpace})
	if m.MatchModal != modalLive || m.MatchModalID != 8 {
		t.Fatalf("space did not open live modal: modal=%q id=%d", m.MatchModal, m.MatchModalID)
	}
}

func TestFixtureListLiveStatusOpensWaitingThenLive(t *testing.T) {
	m := testModel()
	m.Tab = tabFixtures
	m.Fixtures = []Fixture{{ID: 9, Status: "LIVE", Round: 2, KickoffText: "Now", Home: "Alpha", Away: "Beta"}}
	m.Matches = nil
	v := m.View()
	if !strings.Contains(v, "Live") {
		t.Fatalf("fixture LIVE status did not render live label:\n%s", v)
	}

	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.MatchModal != modalWaiting || m.MatchModalID != 9 {
		t.Fatalf("live fixture without detail should open waiting modal: modal=%q id=%d", m.MatchModal, m.MatchModalID)
	}
	m = update(m, MatchesMsg{{Fixture: 9, Home: "Alpha", Away: "Beta", HomeGoals: 1, AwayGoals: 0}})
	if m.MatchModal != modalLive || m.MatchIdx != 0 {
		t.Fatalf("waiting live fixture did not promote to live modal: modal=%q idx=%d", m.MatchModal, m.MatchIdx)
	}
}

func TestFixtureListLiveStatusClosesWaitingWhenLiveMissing(t *testing.T) {
	m := testModel()
	m.Tab = tabFixtures
	m.Fixtures = []Fixture{{ID: 9, Status: "LIVE", Round: 2, KickoffText: "Now", Home: "Alpha", Away: "Beta"}}
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.MatchModal != modalWaiting {
		t.Fatalf("live fixture should open waiting modal first: %q", m.MatchModal)
	}

	m = update(m, MatchesMsg{})
	if m.MatchModal != modalNone || m.Notice != m.ui("ui.match.ended") {
		t.Fatalf("missing live match should close waiting modal: modal=%q notice=%q", m.MatchModal, m.Notice)
	}
}

func TestArchivedResultOpensLedgerModal(t *testing.T) {
	m := testModel()
	m.Width, m.Height = 80, 24
	m.Tab = tabFixtures
	m.FixtureIdx = 2
	m.MatchDetail = MatchDetail{
		Fixture: 6, Status: "RESULT", Archived: true, Competition: "LEAGUE", KickoffText: "Last season",
		Home: "E", Away: "F", HomeGoals: 0, AwayGoals: 0, HomeShots: 5, AwayShots: 4,
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.MatchModal != modalReplay || m.MatchModalID != 6 {
		t.Fatalf("archived result did not open replay/ledger modal: modal=%q id=%d notice=%q", m.MatchModal, m.MatchModalID, m.Notice)
	}
	v := m.View()
	for _, want := range []string{"E 0-0 F", "Shots H 5 · A 4", "Archived no replay"} {
		if !strings.Contains(v, want) {
			t.Fatalf("archived ledger modal missing %q:\n%s", want, v)
		}
	}

	overlay, ok := m.matchModalOverlay(52, 18)
	if !ok || !strings.Contains(strings.Join(overlay.Lines, "\n"), "E 0-0 F") {
		t.Fatalf("small modal fallback failed: ok=%v overlay=%+v", ok, overlay)
	}
}

func TestMatchModalOverlayUsesNearlyFullScreen(t *testing.T) {
	m := liveModel(160, 44)

	overlay, ok := m.matchModalOverlay(160, 44)
	if !ok {
		t.Fatal("modal overlay missing")
	}
	if overlay.X != 1 {
		t.Fatalf("overlay x = %d, want 1", overlay.X)
	}
	if len(overlay.Lines) != 38 {
		t.Fatalf("overlay height = %d, want 38", len(overlay.Lines))
	}
	for i, line := range overlay.Lines {
		if got := lipgloss.Width(line); got != 158 {
			t.Fatalf("overlay line %d width = %d, want 158: %q", i, got, line)
		}
	}
}

func TestSmallMatchModalKeepsEssentialsAndOmitsSecondarySections(t *testing.T) {
	m := liveModel(64, 18)
	m.Matches[0].Stats = LiveStats{
		HomeShots: 7, AwayShots: 3, HomeCards: 1, AwayCards: 2, HomeSubs: 2, AwaySubs: 0,
		ChanceTypes: map[string]int{"CUTBACK": 2},
		Diagnostics: MatchDiagnostics{
			ShotQuality: map[string]int{"HIGH": 1},
		},
	}
	m.Matches[0].Ratings = []LiveRating{{Side: "HOME", Name: "Hero", RatingX10: 76}}

	overlay, ok := m.matchModalOverlay(64, 18)
	if !ok {
		t.Fatal("modal overlay missing")
	}
	got := strings.Join(overlay.Lines, "\n")
	for _, want := range []string{"Alpha 2-1 Beta", "61' · LEAGUE", "Shots H 7 · A 3", "Chance mix Cutbacks 2", "line two"} {
		if !strings.Contains(got, want) {
			t.Fatalf("small modal missing essential %q:\n%s", want, got)
		}
	}
	for _, hidden := range []string{"RATINGS", "Quality", "7.6 Hero"} {
		if strings.Contains(got, hidden) {
			t.Fatalf("small modal rendered secondary %q:\n%s", hidden, got)
		}
	}
}

func TestReplayModalTracksFixtureIDAcrossFixtureRefresh(t *testing.T) {
	m := testModel()
	m.Width, m.Height = 100, 28
	m.Tab = tabFixtures
	m.FixtureIdx = 0
	m.MatchModal = modalReplay
	m.MatchModalID = 7

	m = update(m, FixturesMsg([]Fixture{
		{ID: 9, Status: "SCHEDULED", Round: 1, KickoffText: "Now", Home: "Live", Away: "New"},
		{ID: 7, Status: "RESULT", Round: 1, KickoffText: "Aug 16, 15:00", Home: "A", Away: "B", HomeGoals: 2, AwayGoals: 1, HasReplay: true},
		{ID: 8, Status: "SCHEDULED", Round: 2, KickoffText: "Aug 23, 15:00", Home: "C", Away: "D"},
	}))
	if m.FixtureIdx != 1 {
		t.Fatalf("replay modal did not resync fixture index by id: %d", m.FixtureIdx)
	}
	if v := m.View(); !strings.Contains(v, "A 2-1 B") || strings.Contains(v, "Loading") {
		t.Fatalf("replay modal drifted after fixture refresh:\n%s", v)
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
	m.Tab = tabFixtures
	m.MatchModal = modalLive
	m.MatchModalID = 9
	m.UI = map[string]string{
		"ui.app.title":               "Agentic FC",
		"ui.header.division":         "Division {tier}",
		"ui.tab.media":               "Media",
		"ui.tab.table":               "Table",
		"ui.tab.clubs":               "Clubs",
		"ui.tab.fixtures":            "Fixtures",
		"ui.fixtures.empty":          "No fixtures",
		"ui.fixture.live":            "Live",
		"ui.match.none":              "quiet",
		"ui.match.live":              "Live match",
		"ui.match.ended":             "Match ended",
		"ui.match.waiting_result":    "Waiting result",
		"ui.match.goalflash":         "GOAL",
		"ui.match.current_scene":     "Current scene",
		"ui.match.history":           "Earlier flow",
		"ui.match.scene.goal":        "Goal scene",
		"ui.match.scene.chance":      "Chance building",
		"ui.match.scene.save":        "Keeper's save",
		"ui.match.scene.cross":       "Wide delivery",
		"ui.match.scene.cutback":     "Cut-back",
		"ui.match.scene.through":     "Through ball",
		"ui.match.scene.longshot":    "From range",
		"ui.match.scene.setpiece":    "Set piece",
		"ui.match.scene.counter":     "Counter attack",
		"ui.match.scene.scramble":    "Six-yard scramble",
		"ui.match.scene.dribble":     "Dribble",
		"ui.match.scene.card":        "Referee's book",
		"ui.match.scene.injury":      "Stoppage",
		"ui.match.scene.sub":         "Technical area",
		"ui.match.scene.build":       "Build-up",
		"ui.match.modal.close":       "Esc close",
		"ui.match.modal.replay_help": "PgUp/PgDn",
		"ui.match.replay":            "COMMENTARY",
		"ui.match.ratings":           "RATINGS",
		"ui.match.stat.shots":        "Shots",
		"ui.match.stat.cards":        "Cards",
		"ui.match.stat.subs":         "Subs",
		"ui.match.stat.chance_mix":   "Chance mix",
		"ui.match.stat.quality":      "Quality",
		"ui.match.stat.aerial":       "Aerial",
		"ui.match.stat.press":        "Press",
		"ui.match.stat.setpieces":    "Set pieces",
		"ui.help.keys":               "help",
		"term.chance_type.CUTBACK":   "Cutbacks",
		"term.quality.HIGH":          "High",
		"term.quality.MEDIUM":        "Medium",
	}
	m.Fixtures = []Fixture{{ID: 9, Status: "SCHEDULED", Round: 2, KickoffText: "Now", Home: "Alpha", Away: "Beta"}}
	m.Matches = []LiveMatchView{{
		Fixture: 9,
		Home:    "Alpha", Away: "Beta", HomeGoals: 2, AwayGoals: 1, Minute: 61,
		Competition: "LEAGUE",
		Commentary:  []string{"line one", "line two"},
		Markers: []LiveMarker{
			{Minute: 12, Kind: "GOAL", Side: "HOME"},
			{Minute: 40, Kind: "CARD", Side: "AWAY"},
		},
	}}
	return m
}

func TestLiveMatchModalShowsBoardAndNoPitch(t *testing.T) {
	m := liveModel(140, 36)
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
	v := m.View()
	for _, want := range []string{"Alpha 2-1 Beta", "61' · LEAGUE", "Shots H 7 · A 3", "Chance mix Cutbacks 2", "Quality High 1", "Aerial H 2/3", "Press H 1", "Build-up", "Current scene", "Earlier flow", "line two"} {
		if !strings.Contains(v, want) {
			t.Fatalf("live modal missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "HOME 7.6") || strings.Contains(v, "AWAY 6.1") {
		t.Fatalf("live modal should not render raw side enums:\n%s", v)
	}
	if strings.Contains(v, "+--") || strings.Contains(v, "legend") {
		t.Fatalf("live modal should not render the old pitch:\n%s", v)
	}
	foundFrame := false
	for _, line := range strings.Split(v, "\n") {
		if !strings.Contains(line, "Build-up") {
			continue
		}
		foundFrame = true
		corner := strings.LastIndex(line, "╮")
		if corner < 0 || lipgloss.Width(line[:corner]) < 60 {
			t.Fatalf("scene frame title row collapsed:\n%s", line)
		}
	}
	if !foundFrame {
		t.Fatalf("live modal did not render a scene frame:\n%s", v)
	}
}

func TestMatchSceneClassificationCoversVariedMoments(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{"A high delivery hangs perfectly; Rao rises above everyone.", "cross"},
		{"Alpha swing it in early; Rao gets there, but the header loops over.", "cross"},
		{"Rao attacks the far-post ball for Alpha, glancing it just beyond the upright.", "cross"},
		{"The winger is crossing into the box with two runners waiting.", "cross"},
		{"A header flashes wide after the delivery bends in.", "cross"},
		{"The pull-back is on a plate and the shot flashes wide.", "cutback"},
		{"A threaded pass splits the defence and Kim races clear.", "through"},
		{"No one closes him down and he lets fly from distance.", "longshot"},
		{"The crowd urges Rao to shoot, and the strike whistles over the bar.", "longshot"},
		{"Alpha settle for the long shot, but Rao cannot keep it down.", "longshot"},
		{"He hits a long-distance drive that bends late.", "longshot"},
		{"The distance strike forces everyone to turn.", "longshot"},
		{"The dead ball drops into danger from the set piece.", "setpiece"},
		{"Alpha burst forward on the counter with grass ahead.", "counter"},
		{"The ball ricochets around the six-yard box in chaos.", "scramble"},
		{"The runner darts between two shirts and keeps going.", "dribble"},
		{"Rao is denied — a fine stop keeps Alpha out.", "save"},
		{"The goalkeeper reacts fast to palm it away.", "save"},
		{"The keeper claws through bodies to save.", "save"},
		{"Rao races clear on the break, but the goalkeeper reads it and makes the stop.", "save"},
		{"The keeper throws it out to launch a counter.", "counter"},
		{"They battle to save the point in midfield.", "build"},
		{"일대일 상황, 골키퍼가 크게 버티며 막아냅니다.", "save"},
		{"중거리 슛을 골키퍼가 손끝으로 밀어냅니다.", "save"},
		{"세트피스가 위험했지만 골키퍼가 몸들 사이로 걷어냅니다.", "save"},
		{"혼전 속에서 골키퍼가 어떻게든 막아냅니다.", "save"},
		{"역습을 골키퍼가 읽고 나와 막아냅니다.", "save"},
		{"구석이 열린 줄 알았지만 슛이 마지막 순간 걷어 올려집니다.", "save"},
		{"A clean shot opens up on the edge of the area.", "chance"},
		{"The goalkeeper watches it fizz wide.", "chance"},
		{"Goal! Rao finds the net for Alpha.", "goal"},
		{"ordinary midfield exchange", "build"},
		{"The players keep their distance while the referee talks.", "build"},
		{"Booked — the referee shows yellow.", "card"},
		{"경고 — 수비수가 카드를 받습니다.", "card"},
		{"정확한 컷백, 늦게 들어온 선수가 마무리합니다.", "cutback"},
		{"수비수가 슛을 막아냅니다.", "chance"},
		{"그 대신 공을 뒤로 돌립니다.", "build"},
		{"막바지에 양 팀이 서로를 살핍니다.", "build"},
		{"The ball runs across the six-yard box in chaos.", "scramble"},
		{"The staff arrange the wall before the restart.", "build"},
		{"The manager hangs a new header in the staff room.", "build"},
	}
	for _, tc := range cases {
		if got := matchSceneFromLine(tc.line, nil).kind; got != tc.want {
			t.Fatalf("scene for %q = %q, want %q", tc.line, got, tc.want)
		}
	}
	if got := matchSceneFromLine("ordinary midfield exchange", &LiveMarker{Kind: "GOAL"}).kind; got != "goal" {
		t.Fatalf("goal marker scene = %q, want goal", got)
	}
	if got := matchSceneFromLive(LiveMatchView{
		Markers: []LiveMarker{{Minute: 20, Kind: "CHANCE", Side: "HOME"}},
	}, "").kind; got != "chance" {
		t.Fatalf("bare chance marker scene = %q, want chance", got)
	}
	if got := matchSceneFromLive(LiveMatchView{
		Markers: []LiveMarker{{Minute: 20, Kind: "CARD", Side: "HOME"}},
	}, "").kind; got != "card" {
		t.Fatalf("bare card marker scene = %q, want card", got)
	}
	if got := matchSceneFromLive(LiveMatchView{
		Commentary: []string{"Goal! Rao finds the net.", "The goalkeeper reacts fast to palm it away."},
		Markers:    []LiveMarker{{Minute: 70, Kind: "GOAL", Side: "HOME"}},
	}, "The goalkeeper reacts fast to palm it away.").kind; got != "save" {
		t.Fatalf("stale goal marker should not override latest line: got %q, want save", got)
	}
	m := liveModel(120, 32)
	if frame := sceneFrame(m, matchSceneFromLine("ordinary midfield exchange", nil), 26, 9); len(frame) != 0 {
		t.Fatalf("narrow scene frame should be omitted: %q", frame)
	}
	if frame := sceneFrame(m, matchSceneFromLine("Goal! Rao finds the net for Alpha.", nil), 40, 9); len(frame) != 0 {
		t.Fatalf("too-narrow art frame should be omitted instead of truncated:\n%s", strings.Join(frame, "\n"))
	}
	if frame := sceneFrame(m, matchSceneFromLine("Goal! Rao finds the net for Alpha.", nil), 80, 9); len(frame) != 9 {
		t.Fatalf("goal scene frame height = %d, want 9:\n%s", len(frame), strings.Join(frame, "\n"))
	}
	if history := recentHistory(nil, 3); len(history) != 0 {
		t.Fatalf("empty history = %q, want none", history)
	}
	if history := recentHistory([]string{"current only"}, 3); len(history) != 0 {
		t.Fatalf("single-line history = %q, want none", history)
	}
}

func TestLiveMatchModalSkipsHistoryWhenOnlyCurrentLine(t *testing.T) {
	m := liveModel(120, 32)
	m.Matches[0].Commentary = []string{"The runner darts between two shirts and keeps going."}

	got := m.liveMatchModal(100, 24)
	if !strings.Contains(got, "Current scene") || !strings.Contains(got, "▶ The runner darts") {
		t.Fatalf("live modal missing current scene:\n%s", got)
	}
	if strings.Contains(got, "Earlier flow") || strings.Contains(got, "· -") {
		t.Fatalf("live modal rendered empty history:\n%s", got)
	}
}

func TestLiveMatchModalFallbackUsesLocalizedSceneLabel(t *testing.T) {
	m := liveModel(120, 32)
	m.UI["ui.match.scene.card"] = "심판 수첩"
	m.Matches[0].Commentary = nil
	m.Matches[0].Markers = []LiveMarker{{Minute: 40, Kind: "CARD", Side: "AWAY"}}

	got := m.liveMatchModal(100, 24)
	if !strings.Contains(got, "▶ 심판 수첩") {
		t.Fatalf("live modal fallback did not use localized scene label:\n%s", got)
	}
	if strings.Contains(got, "▶ REFEREE'S BOOK") {
		t.Fatalf("live modal fallback leaked internal scene title:\n%s", got)
	}
}

func TestReplayMatchModalShowsCurrentSceneFallback(t *testing.T) {
	m := liveModel(120, 32)
	m.MatchModal = modalReplay
	m.MatchModalID = 9
	m.MatchDetail = MatchDetail{
		Fixture: 9, Status: "RESULT", Competition: "LEAGUE", KickoffText: "Now",
		Home: "Alpha", Away: "Beta", HomeGoals: 0, AwayGoals: 0,
	}
	got := m.replayMatchModal(100, 24)
	for _, want := range []string{"Current scene", "▶ Build-up"} {
		if !strings.Contains(got, want) {
			t.Fatalf("replay modal missing fallback %q:\n%s", want, got)
		}
	}
	m.MatchDetail.Commentary = []string{"A clean shot opens up on the edge of the area.", "The ball is cleared."}
	got = m.replayMatchModal(100, 24)
	for _, want := range []string{"Current scene", "▶ A clean shot opens up", "COMMENTARY", "· The ball is cleared."} {
		if !strings.Contains(got, want) {
			t.Fatalf("replay modal missing non-empty scene %q:\n%s", want, got)
		}
	}
}

func TestLiveMatchModalDoesNotOverflowTightFrame(t *testing.T) {
	m := liveModel(90, 20)
	m.Matches[0].Ratings = []LiveRating{{Side: "HOME", Name: "Hero", RatingX10: 78}}
	m.Matches[0].Commentary = []string{
		"Goal! Rao finds the net.",
		"A high delivery hangs perfectly.",
		"The ball ricochets around the six-yard box in chaos.",
		"The goalkeeper reacts fast to palm it away.",
		"The runner darts between two shirts and keeps going.",
	}
	got := m.liveMatchModal(90, 20)
	if lines := strings.Split(got, "\n"); len(lines) != 20 {
		t.Fatalf("live modal line count = %d, want 20:\n%s", len(lines), got)
	}
	if !strings.Contains(got, "Dribble") || !strings.Contains(got, "Current scene") {
		t.Fatalf("tight modal lost current scene:\n%s", got)
	}
}

func TestLiveMatchModalGoalFlashAndClose(t *testing.T) {
	m := liveModel(140, 36)
	m.Matches[0].Markers = append(m.Matches[0].Markers, LiveMarker{Minute: 62, Kind: "GOAL", Side: "AWAY"})
	v := m.View()
	if !strings.Contains(v, "GOAL") || !strings.Contains(v, "62'") || !strings.Contains(v, "Beta") || !strings.Contains(v, "█") {
		t.Fatalf("latest-goal flash missing:\n%s", v)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.MatchModal != modalNone {
		t.Fatalf("Esc did not close live modal: %q", m.MatchModal)
	}
}

func TestLiveMatchModalTracksFixtureAcrossRefresh(t *testing.T) {
	m := liveModel(140, 36)
	m.Matches = append([]LiveMatchView{
		{Fixture: 10, Home: "Gamma", Away: "Delta", HomeGoals: 0, AwayGoals: 0, Minute: 12},
	}, m.Matches...)
	m.MatchIdx = 1
	m.MatchModalID = 9

	m = update(m, MatchesMsg{
		{Fixture: 9, Home: "Alpha", Away: "Beta", HomeGoals: 3, AwayGoals: 1, Minute: 70},
		{Fixture: 10, Home: "Gamma", Away: "Delta", HomeGoals: 0, AwayGoals: 0, Minute: 14},
	})
	if m.MatchModal != modalLive || m.MatchIdx != 0 || m.MatchModalID != 9 {
		t.Fatalf("live modal did not remap by fixture: modal=%q idx=%d id=%d", m.MatchModal, m.MatchIdx, m.MatchModalID)
	}
	if got := m.liveMatchModal(90, 18); !strings.Contains(got, "Alpha 3-1 Beta") {
		t.Fatalf("live modal drifted to wrong match:\n%s", got)
	}
}

func TestLiveMatchModalTransitionsToReplayWhenResultArrives(t *testing.T) {
	m := liveModel(140, 36)
	m.Fixtures[0].Status = "RESULT"
	m.Fixtures[0].HasReplay = true
	m.Fixtures[0].HomeGoals = 2
	m.Fixtures[0].AwayGoals = 1

	next, cmd := m.Update(MatchesMsg{})
	m = next.(Model)
	if m.MatchModal != modalReplay || m.MatchModalID != 9 || m.FixtureIdx != 0 {
		t.Fatalf("live modal did not transition to replay: modal=%q id=%d fixture=%d", m.MatchModal, m.MatchModalID, m.FixtureIdx)
	}
	if cmd != nil {
		t.Fatal("nil test client should not schedule replay fetch")
	}
}

func TestLiveMatchModalWaitsForFixtureResultSkew(t *testing.T) {
	m := liveModel(140, 36)

	m = update(m, MatchesMsg{})
	if m.MatchModal != modalWaiting || m.MatchModalID != 9 || m.FixtureIdx != 0 {
		t.Fatalf("missing live match should wait for result: modal=%q id=%d fixture=%d", m.MatchModal, m.MatchModalID, m.FixtureIdx)
	}
	m.World.Divisions = 3
	m.Tier = 2
	m = update(m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.Tier != 2 {
		t.Fatalf("left key leaked through waiting modal, tier=%d", m.Tier)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyRight})
	if m.Tier != 2 {
		t.Fatalf("right key leaked through waiting modal, tier=%d", m.Tier)
	}
	if v := m.View(); !strings.Contains(v, "Waiting result") {
		t.Fatalf("waiting modal missing:\n%s", v)
	}

	m = update(m, FixturesMsg([]Fixture{{ID: 9, Status: "RESULT", HasReplay: true, Home: "Alpha", Away: "Beta", HomeGoals: 2, AwayGoals: 1}}))
	if m.MatchModal != modalReplay || m.MatchModalID != 9 {
		t.Fatalf("result fixture did not promote waiting modal to replay: modal=%q id=%d", m.MatchModal, m.MatchModalID)
	}
}

func TestWaitingMatchModalClosesWhenFixtureDisappears(t *testing.T) {
	m := liveModel(140, 36)
	m.MatchModal = modalWaiting
	m.MatchModalID = 9

	m = update(m, FixturesMsg([]Fixture{{ID: 10, Status: "SCHEDULED", Home: "Other", Away: "Match"}}))
	if m.MatchModal != modalNone || m.MatchModalID != 0 {
		t.Fatalf("missing fixture did not close waiting modal: modal=%q id=%d", m.MatchModal, m.MatchModalID)
	}
	if m.Notice != "Match ended" {
		t.Fatalf("missing fixture notice = %q", m.Notice)
	}
}
