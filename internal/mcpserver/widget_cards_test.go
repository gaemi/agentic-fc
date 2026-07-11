package mcpserver

import (
	"fmt"
	"html"
	"strings"
	"testing"

	"github.com/gaemi/agentic-fc/internal/focus"
	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/narrative"
)

// TestWriteCardsRenderFromEnvelope covers the shaping-write cards: each renders
// the DECISION from the result envelope (the mindset structs the call produced),
// with localized labels AND localized enum values, en then ko.
func TestWriteCardsRenderFromEnvelope(t *testing.T) {
	g, _, _, _ := newGateway(t)
	en := narrative.LocaleEN
	meta := map[string]any{"mindset_version": 3, "focus": map[string]any{"spent": 12, "balance": 40}, "game_time": "1925-08-01T00:00"}

	pc := prioritiesCard(g, en, setPrioritiesIn{}, map[string]any{"ok": true, "meta": meta,
		"data": map[string]any{"priorities": []mindset.Priority{{Rank: 1, Goal: mindset.GoalWinLeague}, {Rank: 2, Goal: mindset.GoalCupRun}}}})
	for _, w := range []string{"Decided", "Set the priorities.", "Win the league"} {
		if !strings.Contains(pc, w) {
			t.Fatalf("priorities card missing %q:\n%s", w, pc)
		}
	}

	ac := addDirectiveCard(g, en, addDirectiveIn{}, map[string]any{"ok": true, "meta": meta,
		"data": map[string]any{"directive": mindset.Directive{Verb: mindset.VerbStart, Strength: mindset.StrengthLean}, "active_directives": 2}})
	for _, w := range []string{"Added a directive.", "Start", "Lean", "Active directives"} {
		if !strings.Contains(ac, w) {
			t.Fatalf("directive card missing %q:\n%s", w, ac)
		}
	}

	rc := removeDirectiveCard(g, en, removeDirectiveIn{}, map[string]any{"ok": true, "meta": meta,
		"data": map[string]any{"removed": "d1", "active_directives": 1}})
	if !strings.Contains(rc, "Removed a directive.") {
		t.Fatalf("remove card:\n%s", rc)
	}

	tenv := map[string]any{"ok": true, "meta": meta,
		"data": map[string]any{"tactical_plan": mindset.TacticalPlan{
			Formation: "4-4-2", Mentality: "ATTACKING", Pressing: "HIGH",
			Tempo: "FAST", Width: "WIDE", Directness: "DIRECT", Counter: true,
		}}}
	counter := true
	patch := updateTacticalPlanIn{
		Formation: "4-4-2", Mentality: "ATTACKING", Pressing: "HIGH",
		Tempo: "FAST", Width: "WIDE", Directness: "DIRECT", Counter: &counter,
	}
	tc := tacticalCard(g, en, patch, tenv)
	for _, w := range []string{
		"Adjusted the tactical plan.", "4-4-2", "Attacking", "High press",
		"Fast tempo", "Wide", "Direct passing", "Counter attack", "yes",
	} {
		if !strings.Contains(tc, w) {
			t.Fatalf("tactical card missing %q:\n%s", w, tc)
		}
	}
	if got := strings.Count(tc, `class="nfw-r nfw-r--ch"`); got != 7 {
		t.Fatalf("full tactical patch highlighted %d rows, want 7:\n%s", got, tc)
	}
	partial := tacticalCard(g, en, updateTacticalPlanIn{Tempo: "FAST"}, tenv)
	if got := strings.Count(partial, `class="nfw-r nfw-r--ch"`); got != 1 {
		t.Fatalf("tempo-only patch highlighted %d rows, want 1:\n%s", got, partial)
	}
	tko := tacticalCard(g, narrative.LocaleKO, patch, tenv)
	for _, w := range []string{"전술을 조정했습니다.", "공격적", "하이 프레스", "빠른 템포", "넓은 폭", "다이렉트 패스", "역습", "예"} {
		if !strings.Contains(tko, w) {
			t.Fatalf("tactical ko card missing %q:\n%s", w, tko)
		}
	}
	if strings.Contains(tko, "Adjusted") {
		t.Fatalf("tactical ko not localized:\n%s", tko)
	}
}

// TestSetWidgetMode confirms the daemon-facing seam maps the flag string to the
// unexported mode (unknown → official MCP Apps default).
func TestSetWidgetMode(t *testing.T) {
	g, _, _, _ := newGateway(t)
	if g.WidgetMode != widgetApps {
		t.Fatal("zero-value widget mode should default to MCP Apps")
	}
	g.SetWidgetMode("apps")
	if g.WidgetMode != widgetApps {
		t.Fatal(`SetWidgetMode("apps") did not select MCP Apps`)
	}
	g.SetWidgetMode("meta")
	if g.WidgetMode != widgetMeta {
		t.Fatal(`SetWidgetMode("meta") did not select _meta`)
	}
	g.SetWidgetMode("content")
	if g.WidgetMode != widgetContentBlock {
		t.Fatal(`SetWidgetMode("content") did not select content-block`)
	}
	g.SetWidgetMode("nonsense")
	if g.WidgetMode != widgetApps {
		t.Fatal("unknown mode should default to MCP Apps")
	}
}

// TestFanoutCardsRenderFromEnvelope drives every fan-out renderer with a
// real envelope from its tool and confirms the action card carries the right
// badge + behaviour headline — i.e. each observation/scout call produces a
// human-readable card of what the agent did.
func TestFanoutCardsRenderFromEnvelope(t *testing.T) {
	g, host, _, man := newGateway(t)
	mid := firstManagerID(man)
	pid := host.world.Players[0].ID
	fid := host.world.Fixtures[0].ID

	ok := func(name string, env map[string]any) map[string]any {
		if env["ok"] != true {
			t.Fatalf("%s call failed: %+v", name, env)
		}
		return env
	}
	want := func(name, card, badge, headline string) {
		if !strings.Contains(card, badge) {
			t.Errorf("%s: card missing badge %q:\n%s", name, badge, card)
		}
		if !strings.Contains(card, headline) {
			t.Errorf("%s: card missing headline %q:\n%s", name, headline, card)
		}
	}

	en := narrative.LocaleEN
	want("situation", situationCard(g, en, emptyIn{}, ok("situation", g.getSituation(mid, "", emptyIn{}))),
		"Observed", "Checked the dashboard.")
	want("news", newsCard(g, en, getNewsIn{}, ok("news", g.getNews(mid, "", getNewsIn{}))),
		"Read", "Checked the press room; no fresh stories.")
	want("club", clubCard(g, en, getClubIn{}, ok("club", g.getClub(mid, "", getClubIn{}))),
		"Observed", "Opened a club dossier.")
	want("squad", squadCard(g, en, getSquadIn{}, ok("squad", g.getSquad(mid, "", getSquadIn{}))),
		"Observed", "Reviewed the squad sheet.")
	pin := getPersonIn{Ref: personRef{Player: pid}}
	want("person", personCard(g, en, pin, ok("person", g.getPerson(mid, "", pin))),
		"Observed", "Opened a player dossier.")
	min := getMatchIn{Fixture: fid}
	want("match", matchCard(g, en, min, ok("match", g.getMatch(mid, "", min))),
		"Observed", "Checked a match.")
	want("search", searchCard(g, en, searchPlayersIn{}, ok("search", g.searchPlayers(mid, "", searchPlayersIn{}))),
		"Observed", "Searched the player market.")
	sin := scoutIn{Profile: "ST"}
	want("scout", scoutCard(g, en, sin, ok("scout", g.scout(mid, "", sin))),
		"Scouted", "Commissioned a scouting report.")
}

// TestFanoutCardsLocalizeKorean confirms the fan-out cards render fully in the
// spectator's locale — Korean chrome, no English leakage (FR-35c).
func TestFanoutCardsLocalizeKorean(t *testing.T) {
	g, host, _, man := newGateway(t)
	mid := firstManagerID(man)
	pin := getPersonIn{Ref: personRef{Player: host.world.Players[0].ID}}
	env := g.getPerson(mid, "", pin)
	if env["ok"] != true {
		t.Fatalf("get_person failed: %+v", env)
	}
	ko := personCard(g, narrative.LocaleKO, pin, env)
	for _, want := range []string{"관찰", "선수 자료를 열었습니다.", "포지션"} {
		if !strings.Contains(ko, want) {
			t.Fatalf("person card missing ko %q:\n%s", want, ko)
		}
	}
	if strings.Contains(ko, "Opened a player dossier.") {
		t.Fatalf("ko card leaked English chrome:\n%s", ko)
	}
}

func TestNewsCardRendersArticle(t *testing.T) {
	g, _, _, _ := newGateway(t)
	env := map[string]any{
		"ok": true,
		"meta": map[string]any{
			"focus": map[string]any{"spent": 1, "balance": 99},
			"tool":  string(focus.GetNews),
		},
		"data": map[string]any{"items": []map[string]any{{
			"category": "board",
			"headline": map[string]any{
				"key": "news.board.appointed",
				"params": map[string]any{
					"club": "Atletico Cerro Palma", "manager": "David Keller",
				},
			},
		}}},
	}
	card := newsCard(g, narrative.LocaleEN, getNewsIn{}, env)
	for _, want := range []string{"Read", "Club Desk", "Atletico Cerro Palma appoint David Keller", "Boardroom pressure"} {
		if !strings.Contains(card, want) {
			t.Fatalf("news article card missing %q:\n%s", want, card)
		}
	}
	ko := newsCard(g, narrative.LocaleKO, getNewsIn{}, env)
	for _, want := range []string{"읽음", "클럽 데스크", "Atletico Cerro Palma, David Keller 신임 감독 선임", "보드룸 압박"} {
		if !strings.Contains(ko, want) {
			t.Fatalf("ko news article card missing %q:\n%s", want, ko)
		}
	}
}

func TestNewsCardKeepsStructuredArticleAngle(t *testing.T) {
	g, _, _, _ := newGateway(t)
	params := map[string]any{"club": "Alderton Athletic", "player": "Alex North"}
	env := map[string]any{
		"ok": true,
		"meta": map[string]any{
			"focus": map[string]any{"spent": 1, "balance": 99},
			"tool":  string(focus.GetNews),
		},
		"data": map[string]any{"items": []map[string]any{{
			"id": 2, "category": "injury",
			"headline": map[string]any{"key": "news.injury.weeks", "params": params},
		}}},
	}
	article := g.newsArticle("injury", "news.injury.weeks", params, narrative.LocaleEN, 2)
	card := newsCard(g, narrative.LocaleEN, getNewsIn{}, env)
	bodyParagraphs := strings.Split(fmt.Sprint(article["body"]), "\n\n")
	if len(bodyParagraphs) < 2 {
		t.Fatalf("variant article has no editorial body paragraph: %+v", article)
	}
	for field, want := range map[string]string{
		"deck": html.EscapeString(fmt.Sprint(article["deck"])),
		"body": html.EscapeString(bodyParagraphs[1]),
	} {
		if !strings.Contains(card, want) {
			t.Fatalf("news card lost id-selected %s angle %q:\n%s", field, want, card)
		}
	}
}

func TestMatchdayNewsArticleUsesGroupedBody(t *testing.T) {
	g, _, _, _ := newGateway(t)
	params := map[string]any{
		"count":        2,
		"month":        8,
		"day":          16,
		"kickoff_time": "15:00",
		"results": []map[string]any{
			{"home": "AFC Castleden", "away": "Eastvale Town", "home_goals": 2, "away_goals": 1},
			{"home": "Stanton Albion", "away": "Union Steindorf", "home_goals": 0, "away_goals": 0},
		},
		"table": []map[string]any{
			{"division": 1, "club": "AFC Castleden", "points": 6},
		},
		"story": map[string]any{
			"best_margin": 1,
			"best_home":   "AFC Castleden",
			"best_away":   "Eastvale Town",
			"home_goals":  2,
			"away_goals":  1,
			"draws":       1,
		},
	}
	article := g.newsArticle("match", "feed.matchday.results", params, narrative.LocaleEN, 0)
	for _, want := range []string{"Results desk", "Results:", "Table picture:", "AFC Castleden 2-1 Eastvale Town", "Draws: 1"} {
		if !strings.Contains(fmt.Sprint(article["body"]), want) && !strings.Contains(fmt.Sprint(article["title"]), want) {
			t.Fatalf("matchday article missing %q: %+v", want, article)
		}
	}
	ko := g.newsArticle("match", "feed.matchday.results", params, narrative.LocaleKO, 0)
	for _, want := range []string{"결과 데스크", "결과:", "순위표 흐름:", "AFC Castleden 2-1 Eastvale Town", "무승부 1경기"} {
		if !strings.Contains(fmt.Sprint(ko["body"]), want) && !strings.Contains(fmt.Sprint(ko["title"]), want) {
			t.Fatalf("ko matchday article missing %q: %+v", want, ko)
		}
	}
	for _, notWant := range []string{"Draws:", "Table picture:"} {
		if strings.Contains(fmt.Sprint(ko["body"]), notWant) {
			t.Fatalf("ko matchday article leaked English %q: %+v", notWant, ko)
		}
	}
	params["story"] = map[string]any{
		"best_margin": 2,
		"best_home":   "AFC Castleden",
		"best_away":   "Eastvale Town",
		"home_goals":  3,
		"away_goals":  1,
		"draws":       0,
	}
	article = g.newsArticle("match", "feed.matchday.results", params, narrative.LocaleEN, 0)
	if !strings.Contains(fmt.Sprint(article["body"]), "No draw softened the table movement") {
		t.Fatalf("all-winners story missing: %+v", article)
	}
}

func TestMatchdayArticleTitlesVaryDeterministically(t *testing.T) {
	g, _, _, _ := newGateway(t)
	params := map[string]any{"count": 6, "kickoff_time": "15:00"}
	titles := map[string]bool{}
	for id := int64(1); id <= 60; id++ {
		first := g.newsArticle("match", "feed.matchday.results", params, narrative.LocaleEN, id)["title"].(string)
		again := g.newsArticle("match", "feed.matchday.results", params, narrative.LocaleEN, id)["title"].(string)
		if first != again {
			t.Fatalf("news %d title changed between reads: %q then %q", id, first, again)
		}
		titles[first] = true
	}
	if len(titles) != 3 {
		t.Fatalf("selected %d matchday title variants, want 3: %v", len(titles), titles)
	}
}

// TestPersonCardDistinguishesManager locks the review fix: the person card reads
// its subject kind from the RESULT envelope (a manager carries a "manager" key),
// so a manager lookup never mislabels itself as a player.
func TestPersonCardDistinguishesManager(t *testing.T) {
	g, host, _, man := newGateway(t)
	mid := firstManagerID(man)

	pin := getPersonIn{Ref: personRef{Player: host.world.Players[0].ID}}
	pcard := personCard(g, narrative.LocaleEN, pin, g.getPerson(mid, "", pin))
	if !strings.Contains(pcard, "Opened a player dossier.") {
		t.Fatalf("player ref should read as a player:\n%s", pcard)
	}

	min := getPersonIn{Ref: personRef{Manager: host.world.Managers[0].ID}}
	env := g.getPerson(mid, "", min)
	if env["ok"] != true {
		t.Fatalf("get_person(manager) failed: %+v", env)
	}
	mcard := personCard(g, narrative.LocaleEN, min, env)
	if !strings.Contains(mcard, "Opened a manager dossier.") || strings.Contains(mcard, "Opened a player dossier.") {
		t.Fatalf("manager ref must read as a manager, not a player:\n%s", mcard)
	}
}

// TestSearchCardShowsMaskedValueBand is the fan-out's FR-22 guard: the search
// card must surface the masked value BAND from the envelope (a bucketed money
// range), which is all a non-own player exposes — never a raw ability figure.
// Rendering the envelope's already-masked value verbatim is what keeps it safe.
func TestSearchCardShowsMaskedValueBand(t *testing.T) {
	g, _, _, man := newGateway(t)
	in := searchPlayersIn{}
	env := g.searchPlayers(firstManagerID(man), "", in)
	if env["ok"] != true {
		t.Fatalf("search_players failed: %+v", env)
	}
	players := envList(env, "players")
	if len(players) == 0 {
		t.Skip("no players returned — nothing to mask")
	}
	vb, ok := players[0]["value_band"].(map[string]any)
	if !ok {
		t.Fatalf("top result has no masked value_band: %+v", players[0])
	}
	low := moneyDisplay(vb["low"])
	card := searchCard(g, narrative.LocaleEN, in, env)
	if low != "" && !strings.Contains(card, low) {
		t.Fatalf("search card must surface the masked band low %q from the envelope:\n%s", low, card)
	}
}

// TestEnumVocabulariesComplete is the drift lock for enum
// localization: every mindset goal, verb, strength, and tactical dial value
// must carry an enum.* entry in BOTH locales — a new enum value cannot ship
// with a silent raw-token fallback. (The fallback still exists for runtime
// safety; this test keeps it from ever being exercised.)
func TestEnumVocabulariesComplete(t *testing.T) {
	check := func(class, token string) {
		t.Helper()
		key := "enum." + class + "." + token
		for _, loc := range narrative.Supported {
			if got := narrative.Default.Render(loc, key, nil); got == key {
				t.Errorf("missing %s in locale %s", key, loc)
			}
		}
	}
	for _, gl := range mindset.AllGoals {
		check("goal", string(gl))
	}
	for _, v := range mindset.AllVerbs {
		check("verb", string(v))
	}
	for _, st := range mindset.AllStrengths {
		check("strength", string(st))
	}
	for dial, values := range mindset.DialValues() {
		for _, v := range values {
			check(dial, v)
		}
	}
}
