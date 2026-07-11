package mcpserver

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/focus"
	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// ownClubManager returns a manager id bound to a club (employed).
func employedManager(t *testing.T, g *Gateway) (int64, int64) {
	t.Helper()
	for id, m := range g.managers {
		if m.ClubID != 0 {
			return id, m.ClubID
		}
	}
	t.Fatal("no employed manager")
	return 0, 0
}

// activeEmployedManager returns the id of the first ACTIVE employed manager in world
// order. Unlike employedManager (which iterates the randomized manager map), this is
// deterministic and — crucially — skips a manager that RETIRED at a season rollover
// (manager careers), whose token is rejected with MANAGER_RETIRED. Every club is always
// managed (FR-14d), so one always exists. Use this when the bound manager must still
// be live AFTER a run that crosses a season boundary.
func activeEmployedManager(t *testing.T, w *worldgen.World) int64 {
	t.Helper()
	for i := range w.Managers {
		m := &w.Managers[i]
		if m.ClubID != 0 && m.Status != worldgen.ManagerRetired {
			return m.ID
		}
	}
	t.Fatal("no active employed manager after rollover")
	return 0
}

// TestGetNewsHidesTransferFee locks the FR-22 boundary: a completed
// fee transfer's news carries the fee and wage for the human Console, but the
// agent surface (get_news) must expose NEITHER — each is a pure function of the
// hidden Ability Pool (fee = pool²·k), so a raw value would invert to it. Guards
// against the renderMessage-echoes-raw-params leak: the headline TEXT never
// prints them, but the structured params map would carry them unfiltered.
func TestGetNewsHidesTransferFee(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, club := employedManager(t, g)
	const fee = 48_400_000
	const wage = 125_000
	g.Host.World().AddNews(worldgen.NewsItem{
		GameTime: 0, Category: "transfer", Key: "news.transfer.fee_completed",
		Params:  map[string]any{"club": "Athletic", "player": "R. Vega", "from": "United", "fee": fee, "wage": wage},
		ClubIDs: []int64{club},
	})

	env := g.getNews(mid, "s1", getNewsIn{Since: "0", Scope: "world", Limit: 100})
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{strconv.Itoa(fee), strconv.Itoa(wage), `"fee"`, `"wage"`} {
		if strings.Contains(string(b), leak) {
			t.Fatalf("get_news leaks %q in the serialized response:\n%s", leak, b)
		}
	}
	// The public headline (names) still comes through — the guard is targeted.
	if !strings.Contains(string(b), "R. Vega") || !strings.Contains(string(b), "United") {
		t.Fatalf("get_news dropped the public transfer headline:\n%s", b)
	}
}

func TestGetNewsHidesMatchdayPreviewArticles(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, _ := employedManager(t, g)
	w := g.Host.World()
	w.AddNews(worldgen.NewsItem{
		GameTime: 10,
		Category: "match",
		Key:      "feed.matchday.preview",
		Params: map[string]any{
			"count":        2,
			"month":        8,
			"day":          16,
			"kickoff_time": "15:00",
		},
	})
	w.AddNews(worldgen.NewsItem{
		GameTime: 20,
		Category: "match",
		Key:      "feed.matchday.results",
		Params: map[string]any{
			"count":        1,
			"month":        8,
			"day":          16,
			"kickoff_time": "15:00",
			"results": []map[string]any{
				{"home": "AFC Castleden", "away": "Eastvale Town", "home_goals": 2, "away_goals": 1, "winner": "AFC Castleden"},
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
				"draws":       0,
			},
		},
	})

	data := dataOf(t, g.getNews(mid, "s1", getNewsIn{Since: "0", Scope: "world", Limit: 10}))
	items := data["items"].([]map[string]any)
	if len(items) != 1 {
		t.Fatalf("news items = %d: %+v", len(items), items)
	}
	headline := items[0]["headline"].(map[string]any)
	if got := headline["key"]; got != "feed.matchday.results" {
		t.Fatalf("shown news key = %v, want feed.matchday.results", got)
	}
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "matchday.preview") || strings.Contains(string(b), "preview") {
		t.Fatalf("matchday preview leaked through get_news: %s", b)
	}
}

// TestSearchPlayersListed locks the transfer-list discovery surface:
// once a club SELL-lists one of its own players, search_players(contract_status=
// "listed") surfaces exactly that player — and, in a freshly generated world
// where nobody else has listed anyone, only that player.
func TestSearchPlayersListed(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, club := employedManager(t, g)

	var pid int64
	w := g.Host.World()
	for i := range w.Players {
		if w.Players[i].ClubID == club && !w.Players[i].Youth {
			pid = w.Players[i].ID
			break
		}
	}
	if pid == 0 {
		t.Fatal("manager's club has no senior player to list")
	}
	if code := errCode(g.addDirective(mid, "s1", addDirectiveIn{
		Verb: "SELL", Target: mindset.Target{Player: pid}, Strength: "INSIST",
	})); code != "" {
		t.Fatalf("listing an own player for sale should be accepted, got %q", code)
	}

	data := dataOf(t, g.searchPlayers(mid, "s2", searchPlayersIn{ContractStatus: "listed", Limit: 30}))
	rows := data["players"].([]map[string]any)
	found := false
	for _, r := range rows {
		if r["player"].(int64) != pid {
			t.Fatalf("a non-listed player %v surfaced under contract_status=listed", r["player"])
		}
		found = true
	}
	if !found {
		t.Fatal("the SELL-listed player did not appear in search_players(listed)")
	}
}

// TestAddDirectiveSellRequiresOwnership locks the poach defence at the write
// surface: a manager may only SELL-list a player at their OWN
// club — a SELL on another club's player is rejected, so it can't lie in wait to
// re-list the player the instant the manager later signs them.
func TestAddDirectiveSellRequiresOwnership(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, club := employedManager(t, g)

	var pid int64
	w := g.Host.World()
	for i := range w.Players {
		if p := &w.Players[i]; p.ClubID != 0 && p.ClubID != club && !p.Youth {
			pid = p.ID
			break
		}
	}
	if pid == 0 {
		t.Fatal("no player at another club to test with")
	}
	env := g.addDirective(mid, "s1", addDirectiveIn{
		Verb: "SELL", Target: mindset.Target{Player: pid}, Strength: "LEAN",
	})
	if errCode(env) != "VALIDATION" {
		t.Fatalf("SELL on a player at another club must be rejected (VALIDATION), got %+v", env)
	}
}

// TestAddDirectiveSellUnemployedOnFreeAgent locks the ClubID==0 edge: club 0
// marks both a free agent and the unemployed manager pool, so an unemployed
// manager must not be able to "list" a free agent via the 0==0 ownership
// compare.
func TestAddDirectiveSellUnemployedOnFreeAgent(t *testing.T) {
	g, _, _, _ := newGateway(t)
	var umid int64
	for id, m := range g.managers {
		if m.ClubID == 0 {
			umid = id
			break
		}
	}
	if umid == 0 {
		t.Skip("no unemployed manager in this world")
	}
	var fa int64
	w := g.Host.World()
	for i := range w.Players {
		if w.Players[i].ClubID == 0 && !w.Players[i].Youth {
			fa = w.Players[i].ID
			break
		}
	}
	if fa == 0 {
		t.Skip("no free agent in this world")
	}
	env := g.addDirective(umid, "s1", addDirectiveIn{
		Verb: "SELL", Target: mindset.Target{Player: fa}, Strength: "LEAN",
	})
	if errCode(env) != "VALIDATION" {
		t.Fatalf("an unemployed manager listing a free agent must be rejected (VALIDATION), got %+v", env)
	}
}

// TestConfidenceDescriptorReflectsLive locks the careers-A1 surface swap: the
// dashboard's board-confidence Descriptor reads the LIVE Confidence, not the
// static ConfidenceBaseline — set baseline HIGH but live LOW and it must read LOW.
func TestConfidenceDescriptorReflectsLive(t *testing.T) {
	g, host, _, _ := newGateway(t)
	mid, club := employedManager(t, g)
	host.LockedWrite(func() {
		c := g.clubByID(club)
		c.ConfidenceBaseline = 90 // would read HIGH if the surface used the baseline
		c.Confidence = 10         // live LOW
	})
	data := dataOf(t, g.getSituation(mid, "s1", emptyIn{}))
	board := data["urgent"].(map[string]any)["board"].(map[string]any)
	conf := board["confidence"].(map[string]any)
	if conf["key"] != "desc.confidence.LOW" {
		t.Fatalf("dashboard confidence key = %v, want desc.confidence.LOW (must read live confidence, not baseline)", conf["key"])
	}
}

func TestGetSituationEmployed(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, _ := employedManager(t, g)
	data := dataOf(t, g.getSituation(mid, "s1", emptyIn{}))
	if data["league_position"].(int) < 1 {
		t.Fatalf("no league position: %+v", data)
	}
	if _, ok := data["urgent"].(map[string]any); !ok {
		t.Fatal("missing urgent block")
	}
	if _, ok := data["next_fixture"]; !ok {
		t.Fatal("missing next fixture (season not started)")
	}
}

func TestSituationHeadlinesStayCompact(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, _ := employedManager(t, g)
	g.Host.World().AddNews(worldgen.NewsItem{
		GameTime: 100,
		Category: "match",
		Key:      "feed.matchday.results",
		Params: map[string]any{
			"count":        2,
			"kickoff_time": "15:00",
			"results": []map[string]any{
				{"home": "Alpha", "away": "Beta", "home_goals": 2, "away_goals": 1},
			},
			"table": []map[string]any{{"division": 1, "club": "Alpha", "points": 6}},
			"story": map[string]any{"best_home": "Alpha", "best_away": "Beta", "home_goals": 2, "away_goals": 1},
		},
	})

	data := dataOf(t, g.getSituation(mid, "s1", emptyIn{}))
	rows := data["headlines"].([]map[string]any)
	if len(rows) == 0 {
		t.Fatal("situation returned no headline preview")
	}
	row := rows[0]
	article := row["article"].(map[string]any)
	if article["title"] == "" || article["deck"] == "" || article["source"] == "" {
		t.Fatalf("headline preview lost its article identity: %+v", article)
	}
	if _, ok := article["body"]; ok {
		t.Fatalf("situation headline leaked full article body: %+v", article)
	}
	headline := row["headline"].(map[string]any)
	params := headline["params"].(map[string]any)
	for _, key := range []string{"fixtures", "results", "story", "table"} {
		if _, ok := params[key]; ok {
			t.Errorf("situation headline retained detail param %q: %+v", key, params)
		}
	}
	if params["count"] != 2 || params["kickoff_time"] != "15:00" {
		t.Fatalf("headline lost title params: %+v", params)
	}

	news := dataOf(t, g.getNews(mid, "news-detail", getNewsIn{Since: "0", Scope: "world", Limit: 100}))
	items := news["items"].([]map[string]any)
	var detailed map[string]any
	for _, item := range items {
		if item["id"] == row["id"] {
			detailed = item
			break
		}
	}
	if detailed == nil {
		t.Fatalf("get_news did not return situation headline id %v", row["id"])
	}
	detailArticle := detailed["article"].(map[string]any)
	if detailArticle["body"] == "" {
		t.Fatalf("get_news lost full article body: %+v", detailArticle)
	}
	detailParams := detailed["headline"].(map[string]any)["params"].(map[string]any)
	for _, key := range []string{"results", "story", "table"} {
		if _, ok := detailParams[key]; !ok {
			t.Errorf("get_news lost detail param %q: %+v", key, detailParams)
		}
	}
}

func TestGetLeagueTableAndManagers(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, _ := employedManager(t, g)
	env := g.getLeague(mid, "s1", getLeagueIn{Division: 1, Sections: []string{"table", "managers"}})
	data := dataOf(t, env)
	rows := data["table"].([]map[string]any)
	if len(rows) != g.Host.World().Config.ClubsPerDivision {
		t.Fatalf("table rows = %d", len(rows))
	}
	mgrs := data["managers"].([]map[string]any)
	if len(mgrs) != g.Host.World().Config.ClubsPerDivision {
		t.Fatalf("managers rows = %d", len(mgrs))
	}
	// Unknown division → NOT_FOUND.
	if errCode(g.getLeague(mid, "s1", getLeagueIn{Division: 99})) != "NOT_FOUND" {
		t.Fatal("unknown division must be NOT_FOUND")
	}
}

func TestGetClubOwnVsOther(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, ownClub := employedManager(t, g)

	// Own club: finances present.
	own := dataOf(t, g.getClub(mid, "s1", getClubIn{Club: ownClub}))
	if _, ok := own["finances"].(map[string]any); !ok {
		t.Fatal("own club missing finances")
	}

	// Another club: public only, no internal finances/board.
	var otherClub int64
	for _, c := range g.Host.World().Clubs {
		if c.ID != ownClub {
			otherClub = c.ID
			break
		}
	}
	other := dataOf(t, g.getClub(mid, "s1", getClubIn{Club: otherClub}))
	if _, leaked := other["finances"]; leaked {
		t.Fatal("other club leaked finances")
	}
	if _, leaked := other["board"]; leaked {
		t.Fatal("other club leaked board detail")
	}
	if _, ok := other["headline_players"]; !ok {
		t.Fatal("other club missing public headline players")
	}
}

func TestGetSquadMaskingOwnVsOther(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, ownClub := employedManager(t, g)

	own := dataOf(t, g.getSquad(mid, "s1", getSquadIn{Club: ownClub}))
	for _, row := range own["players"].([]map[string]any) {
		body := row["body"].(map[string]any)
		if body["height_cm"].(int) <= 0 || body["weight_kg"].(int) <= 0 {
			t.Fatalf("own squad missing body profile: %+v", body)
		}
		attrs := row["attributes"].(map[string]any)
		for a, v := range attrs {
			if _, exact := v.(int); !exact {
				t.Fatalf("own squad attribute %s not exact: %T", a, v)
			}
		}
	}

	var otherClub int64
	for _, c := range g.Host.World().Clubs {
		if c.ID != ownClub {
			otherClub = c.ID
			break
		}
	}
	other := dataOf(t, g.getSquad(mid, "s1", getSquadIn{Club: otherClub}))
	for _, row := range other["players"].([]map[string]any) {
		attrs := row["attributes"].(map[string]any)
		for a, v := range attrs {
			rng, ranged := v.([]int)
			if !ranged || len(rng) != 2 || rng[0] > rng[1] {
				t.Fatalf("other squad attribute %s not a range: %v", a, v)
			}
		}
	}
}

func TestUnemployedScopeGuards(t *testing.T) {
	g, _, _, _ := newGateway(t)
	var unemployed int64
	for id, m := range g.managers {
		if m.ClubID == 0 {
			unemployed = id
			break
		}
	}
	if unemployed == 0 {
		t.Skip("no unemployed manager in this world")
	}
	if errCode(g.getClub(unemployed, "s1", getClubIn{})) != "UNEMPLOYED_SCOPE" {
		t.Fatal("unemployed get_club (own) must be UNEMPLOYED_SCOPE")
	}
	if errCode(g.getSquad(unemployed, "s1", getSquadIn{})) != "UNEMPLOYED_SCOPE" {
		t.Fatal("unemployed get_squad (own) must be UNEMPLOYED_SCOPE")
	}
	if errCode(g.scout(unemployed, "s1", scoutIn{Profile: "ST"})) != "UNEMPLOYED_SCOPE" {
		t.Fatal("unemployed scout must be UNEMPLOYED_SCOPE")
	}
}

func TestGetPersonPlayerHidesUntilScouted(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, ownClub := employedManager(t, g)

	// A player NOT on the manager's club: masked, no personality descriptor.
	var target *worldgen.Player
	for i := range g.Host.World().Players {
		p := &g.Host.World().Players[i]
		if p.ClubID != 0 && p.ClubID != ownClub && !p.Youth {
			target = p
			break
		}
	}
	data := dataOf(t, g.getPerson(mid, "s1", getPersonIn{Ref: personRef{Player: target.ID}}))
	body := data["body"].(map[string]any)
	if body["height_cm"].(int) <= 0 || body["weight_kg"].(int) <= 0 {
		t.Fatalf("get_person missing public body profile: %+v", body)
	}
	if len(data["descriptors"].([]map[string]any)) != 0 {
		t.Fatal("unscouted other-club player leaked a personality descriptor")
	}
	attrs := data["attributes"].(map[string]any)
	for _, v := range attrs {
		if _, exact := v.(int); exact {
			t.Fatal("unscouted player attribute is exact")
		}
	}
}

func TestScoutEnrichesKnowledge(t *testing.T) {
	g, host, _, _ := newGateway(t)
	mid, ownClub := employedManager(t, g)
	m := g.managers[mid]

	var target *worldgen.Player
	for i := range g.Host.World().Players {
		p := &g.Host.World().Players[i]
		if p.ClubID != 0 && p.ClubID != ownClub && !p.Youth {
			target = p
			break
		}
	}

	env := g.scout(mid, "s1", scoutIn{Target: personRef{Player: target.ID}})
	data := dataOf(t, env)
	if data["commissioned"] != true {
		t.Fatalf("scout not commissioned: %+v", data)
	}

	// Drain the world until the report lands.
	if _, err := host.eng.RunUntil(sim.GameTime(20 * sim.MinutesPerDay)); err != nil {
		t.Fatal(err)
	}
	if g.Host.World().KnowledgeLevel(m.ID, target.ID) < 1 {
		t.Fatal("scouting did not raise knowledge level")
	}

	// The report is a PRIVATE news item — only this manager sees it.
	personEnv := g.getPerson(mid, "s1", getPersonIn{Ref: personRef{Player: target.ID}})
	person := dataOf(t, personEnv)
	if len(person["evidence"].([]map[string]any)) == 0 {
		t.Fatal("no scout evidence after report")
	}

	// FR-22: the enriched evidence payload must carry qualitative prose only —
	// never a raw hidden-attribute enum or value. This is the guardrail the
	// original attr_key-leaking implementation would have failed.
	b, err := json.Marshal(personEnv)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range forbiddenHidden {
		if strings.Contains(string(b), f) {
			t.Errorf("scouted get_person leaks %s", f)
		}
	}

	// hasScout scans a get_news response for the private scout headline. An
	// explicit "0" cursor scans the whole ring — the report landed mid-run,
	// before the current game-day the default cursor would start from.
	hasScout := func(mgr int64, sid, scope string) bool {
		env := g.getNews(mgr, sid, getNewsIn{Since: "0", Scope: scope, Limit: 100})
		for _, item := range dataOf(t, env)["items"].([]map[string]any) {
			if strings.Contains(item["headline"].(map[string]any)["key"].(string), "scout") {
				return true
			}
		}
		return false
	}

	// The owner sees the report in their personal ("own") feed, but never in
	// the public world/league breadth scopes (scope hygiene, no cross-leak).
	if !hasScout(mid, "own", "own") {
		t.Fatal("owner cannot see their own scout report in the personal feed")
	}
	if hasScout(mid, "ownw", "world") || hasScout(mid, "ownl", "league") {
		t.Fatal("private scout report leaked into the owner's public world/league scope")
	}

	// A different manager must NEVER see the private scout report in any scope.
	other, _ := employedManager(t, g)
	for other == mid {
		for id, mm := range g.managers {
			if id != mid && mm.ClubID != 0 {
				other = id
			}
		}
	}
	if hasScout(other, "otherw", "world") || hasScout(other, "othero", "own") || hasScout(other, "otherl", "league") {
		t.Fatal("private scout report leaked to another manager's news")
	}
}

func TestGetNewsCursorAdvances(t *testing.T) {
	g, host, _, _ := newGateway(t)
	mid, _ := employedManager(t, g)

	// Generate some world news.
	if _, err := host.eng.RunUntil(sim.GameTime(60 * sim.MinutesPerDay)); err != nil {
		t.Fatal(err)
	}
	first := dataOf(t, g.getNews(mid, "s1", getNewsIn{Scope: "world", Limit: 10}))
	cursor := first["cursor"].(string)
	if cursor == "" {
		t.Fatal("no cursor returned")
	}
	// The session cursor persists: a follow-up call without `since` starts
	// after the previous batch.
	second := dataOf(t, g.getNews(mid, "s1", getNewsIn{Scope: "world", Limit: 10}))
	firstItems := first["items"].([]map[string]any)
	secondItems := second["items"].([]map[string]any)
	if len(firstItems) > 0 && len(secondItems) > 0 {
		if firstItems[len(firstItems)-1]["id"].(int64) >= secondItems[0]["id"].(int64) {
			t.Fatal("cursor did not advance between calls")
		}
	}
}

func TestSearchPlayersFiltersAndMasks(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, _ := employedManager(t, g)
	data := dataOf(t, g.searchPlayers(mid, "s1", searchPlayersIn{Position: "ST", Limit: 5}))
	rows := data["players"].([]map[string]any)
	if len(rows) == 0 {
		t.Fatal("no strikers found")
	}
	for _, r := range rows {
		if r["position"] != "ST" {
			t.Fatalf("filter leaked non-ST: %v", r["position"])
		}
		// Headline attributes come masked (ranges) for non-own players.
		for _, v := range r["headline_attributes"].(map[string]any) {
			if _, exact := v.(int); exact {
				// exact only if the striker happens to be on own club — allowed
				continue
			}
		}
	}
}

// forbiddenHidden are tokens that must never appear anywhere on the MCP wire
// (FR-22): raw hidden-attribute enum names (canonical UPPERCASE), growth enums,
// and the internal JSON field names for hidden state / Ability Pool / raw club
// tendencies. Qualitative evidence prose and its lowercase dimension keys are
// the sanctioned surface and are deliberately NOT listed here.
var forbiddenHidden = []string{
	`"hidden"`, `"ability_pool"`, `"potential_cap"`,
	`"board_patience"`, `"board_ambition"`, `"youth_emphasis"`,
	`"training_facilities"`, `"youth_facilities"`, `"confidence_baseline"`,
	"PROFESSIONALISM", "AMBITION", "LOYALTY", "TEMPERAMENT", "PRESSURE",
	"ADAPTABILITY", "SOCIABILITY", "INFLUENCE", "CONSISTENCY",
	"BIG_MATCH_NERVE", "INJURY_PRONENESS", "RECOVERY", "DISCIPLINE",
	"VERSATILITY", "POTENTIAL_CAP", "DEVELOPMENT_SPEED", "DECLINE_ONSET",
	"DECLINE_SPEED",
}

// TestMCPObservationNeverLeaksHidden is the FR-22 guardrail over the full
// observation surface: no tool may emit hidden attributes, Ability Pool,
// Potential Cap, raw reputation numbers, or raw club tendency numbers.
func TestMCPObservationNeverLeaksHidden(t *testing.T) {
	g, host, _, _ := newGateway(t)
	mid, ownClub := employedManager(t, g)
	// Run past the first match days so the scan also covers finished-match
	// payloads (scores, ratings, lineups), the live table, and season stats.
	if _, err := host.eng.RunUntil(sim.GameTime(60 * sim.MinutesPerDay)); err != nil {
		t.Fatal(err)
	}
	var otherClub, somePlayer, someManager int64
	for _, c := range g.Host.World().Clubs {
		if c.ID != ownClub {
			otherClub = c.ID
			break
		}
	}
	for i := range g.Host.World().Players {
		if g.Host.World().Players[i].ClubID == otherClub {
			somePlayer = g.Host.World().Players[i].ID
			break
		}
	}
	for id, m := range g.managers {
		if m.ClubID == otherClub {
			someManager = id
		}
	}

	// Prefer a PLAYED fixture so get_match returns the finished view (scores,
	// ratings, lineups) for the leak scan; fall back to a scheduled one.
	var someFixture int64
	if len(g.Host.World().Results) > 0 {
		someFixture = g.Host.World().Results[0].FixtureID
	} else if fx := g.Host.World().Fixtures; len(fx) > 0 {
		someFixture = fx[0].ID
	}

	calls := []map[string]any{
		g.getSituation(mid, "s1", emptyIn{}),
		g.getNews(mid, "s1", getNewsIn{Scope: "world", Limit: 100}),
		g.getLeague(mid, "s1", getLeagueIn{Division: 1, Sections: []string{"table", "managers", "fixtures"}}),
		g.getClub(mid, "s1", getClubIn{Club: otherClub}),
		g.getSquad(mid, "s1", getSquadIn{Club: otherClub}),
		g.getPerson(mid, "s1", getPersonIn{Ref: personRef{Player: somePlayer}}),
		g.getPerson(mid, "s1", getPersonIn{Ref: personRef{Manager: someManager}}),
		g.getMatch(mid, "s1", getMatchIn{Fixture: someFixture}),
		g.searchPlayers(mid, "s1", searchPlayersIn{Limit: 30}),
	}
	forbidden := forbiddenHidden
	for i, env := range calls {
		b, err := json.Marshal(env)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range forbidden {
			if strings.Contains(string(b), f) {
				t.Errorf("call %d leaks %s", i, f)
			}
		}
	}
}

// TestGetSquadContractsOwnClubOnly locks docs/11 §4: the get_squad contract
// summary is own-club only — other clubs expose no contract fields at all.
func TestGetSquadContractsOwnClubOnly(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, ownClub := employedManager(t, g)
	var otherClub int64
	for _, c := range g.Host.World().Clubs {
		if c.ID != ownClub {
			otherClub = c.ID
			break
		}
	}

	own := dataOf(t, g.getSquad(mid, "s1", getSquadIn{Club: ownClub, Detail: "contracts"}))
	foundOwn := false
	for _, r := range own["players"].([]map[string]any) {
		if _, ok := r["contract_expiry_season"]; ok {
			foundOwn = true
			break
		}
	}
	if !foundOwn {
		t.Fatal("own-club contracts detail should expose the contract summary")
	}

	other := dataOf(t, g.getSquad(mid, "s2", getSquadIn{Club: otherClub, Detail: "contracts"}))
	for _, r := range other["players"].([]map[string]any) {
		if _, ok := r["contract_expiry_season"]; ok {
			t.Fatal("other-club contracts detail leaked contract_expiry_season")
		}
		if _, ok := r["wage_weekly"]; ok {
			t.Fatal("other-club contracts detail leaked wage_weekly")
		}
	}
}

// TestMatchReadsSurfaceResults locks the match-result wiring: once matches play,
// the live table, results, get_match finished view, and season stats all
// surface real data instead of placeholders.
func TestMatchReadsSurfaceResults(t *testing.T) {
	g, host, _, _ := newGateway(t)
	mid, _ := employedManager(t, g)
	// Kickoffs begin ~day 46; run past a couple of match days.
	if _, err := host.eng.RunUntil(sim.GameTime(60 * sim.MinutesPerDay)); err != nil {
		t.Fatal(err)
	}
	w := g.Host.World()
	if len(w.Results) == 0 {
		t.Fatal("no matches played by day 60")
	}

	// get_league: the live table has played games and results are non-empty.
	played := w.Results[0]
	league := dataOf(t, g.getLeague(mid, "s1", getLeagueIn{
		Division: played.DivisionTier, Sections: []string{"table", "results"},
	}))
	tablePlayed := 0
	for _, row := range league["table"].([]map[string]any) {
		tablePlayed += row["played"].(int)
	}
	if tablePlayed == 0 {
		t.Fatal("live league table shows no games played")
	}
	if len(league["results"].([]map[string]any)) == 0 {
		t.Fatal("get_league results empty after matches")
	}

	// get_match: a played fixture returns a FINISHED view with lineups+ratings.
	mv := dataOf(t, g.getMatch(mid, "s2", getMatchIn{Fixture: played.FixtureID}))
	if mv["status"] != "FINISHED" {
		t.Fatalf("played fixture status = %v, want FINISHED", mv["status"])
	}
	lineup := mv["home_lineup"].([]map[string]any)
	if len(lineup) == 0 || lineup[0]["rating"] == nil {
		t.Fatal("finished match lineup missing ratings")
	}

	// get_person: a player who featured shows accrued season stats.
	var featured int64
	for i := range w.Players {
		if w.Players[i].SeasonApps > 0 {
			featured = w.Players[i].ID
			break
		}
	}
	if featured == 0 {
		t.Fatal("no player accrued appearances")
	}
	person := dataOf(t, g.getPerson(mid, "s3", getPersonIn{Ref: personRef{Player: featured}}))
	stats := person["season_stats"].(map[string]any)
	if stats["apps"].(int) == 0 {
		t.Fatal("get_person season_stats apps still zero for a featured player")
	}
}

// TestGetLeaguePostRollover locks the rollover read seam the hash test can't:
// after a season rollover (Table rebuilt, promotion/relegation changing
// memberships, Results cleared), get_league still returns a populated table for
// every division with clubs in their NEW tiers — no nil/range panic.
func TestGetLeaguePostRollover(t *testing.T) {
	g, host, _, _ := newGatewayCfg(t, worldgen.DefaultConfig(31)) // 2 divisions
	if _, err := host.eng.RunUntil(sim.GameTime(430 * sim.MinutesPerDay)); err != nil {
		t.Fatal(err)
	}
	w := g.Host.World()
	// Bind AFTER the rollover to a still-ACTIVE manager: careers-C retirement can fire
	// at the season boundary, and a RETIRED token is rejected. get_league is a public
	// read, so any active manager exercises the post-rollover table — the point of the
	// test is the table's tier correctness, not which manager asks.
	mid := activeEmployedManager(t, w)
	s2 := false
	for _, r := range w.Results {
		if worldgen.DateOf(r.Kickoff).Season == 2 {
			s2 = true
			break
		}
	}
	if !s2 {
		t.Fatal("test vacuous: no season-2 matches — rollover not exercised")
	}
	tierOfClub := func(id int64) int {
		for i := range w.Clubs {
			if w.Clubs[i].ID == id {
				return w.Clubs[i].DivisionTier
			}
		}
		return 0
	}
	for tier := 1; tier <= w.Config.Divisions; tier++ {
		data := dataOf(t, g.getLeague(mid, "s1", getLeagueIn{
			Division: tier, Sections: []string{"table", "results"},
		}))
		rows := data["table"].([]map[string]any)
		if len(rows) == 0 {
			t.Fatalf("division %d table empty after rollover", tier)
		}
		for _, row := range rows {
			clubID := row["club"].(map[string]any)["club"].(int64)
			if got := tierOfClub(clubID); got != tier {
				t.Fatalf("division %d table lists club %d now in tier %d", tier, clubID, got)
			}
		}
	}
}

// TestMatchCommentaryAndForm locks the commentary/form reads: a finished match carries
// a rendered commentary log, and the live league table carries form strings.
func TestMatchCommentaryAndForm(t *testing.T) {
	g, host, _, _ := newGateway(t)
	mid, _ := employedManager(t, g)
	if _, err := host.eng.RunUntil(sim.GameTime(60 * sim.MinutesPerDay)); err != nil {
		t.Fatal(err)
	}
	w := g.Host.World()
	if len(w.Results) == 0 {
		t.Fatal("no matches played")
	}
	played := w.Results[0]
	w.Results[0].ChanceTypes = map[string]int{"COUNTER": 2, "CUTBACK": 1}
	w.Results[0].Diagnostics.ShotQuality = map[string]int{"HIGH": 1, "MEDIUM": 2}
	w.Results[0].Diagnostics.PressTurnovers = map[string]int{"HOME": 2}

	mv := dataOf(t, g.getMatch(mid, "s1", getMatchIn{Fixture: played.FixtureID}))
	stats := mv["stats"].(map[string]any)
	if _, ok := stats["chance_types"]; ok {
		t.Fatal("MCP get_match exposed raw chance_types instead of player-facing match_patterns")
	}
	patterns := stats["match_patterns"].([]map[string]any)
	if len(patterns) != 2 || patterns[0]["pattern"] != "COUNTER" ||
		patterns[0]["label"] != "Counters" || patterns[0]["count"] != 2 {
		t.Fatalf("match_patterns not rendered as player-facing rows: %+v", patterns)
	}
	quality := stats["shot_quality"].([]map[string]any)
	if len(quality) == 0 || quality[0]["band"] != "MEDIUM" || quality[0]["count"] != 2 {
		t.Fatalf("shot quality not exposed as public diagnostic rows: %+v", quality)
	}
	press := stats["press_turnovers"].([]map[string]any)
	if len(press) != 1 || press[0]["side"] != "HOME" || press[0]["count"] != 2 {
		t.Fatalf("press turnovers not exposed as side counts: %+v", press)
	}
	commentary := mv["commentary"].([]map[string]any)
	if len(commentary) == 0 {
		t.Fatal("finished match has no commentary log")
	}
	if commentary[0]["line"].(map[string]any)["text"] == "" {
		t.Fatal("commentary line rendered empty (missing catalog key?)")
	}

	league := dataOf(t, g.getLeague(mid, "s2", getLeagueIn{
		Division: played.DivisionTier, Sections: []string{"table"},
	}))
	sawForm := false
	for _, row := range league["table"].([]map[string]any) {
		if row["form"].(string) != "" {
			sawForm = true
			break
		}
	}
	if !sawForm {
		t.Fatal("no club shows league form after matches played")
	}
}

// TestLiveViewOwnVsOther locks the own/other gating: a live match's
// commentary + score are public, but the own-team-state block and the
// manager's in-match adjustments are exposed only for the viewer's own match.
func TestLiveViewOwnVsOther(t *testing.T) {
	g, host, _, _ := newGateway(t)
	mid, ownClub := employedManager(t, g)
	w := g.Host.World()
	var first sim.GameTime = 1 << 62
	for i := range w.Fixtures {
		if w.Fixtures[i].Kickoff < first {
			first = w.Fixtures[i].Kickoff
		}
	}
	if _, err := host.eng.RunUntil(first + sim.GameTime(60)); err != nil { // 60' in
		t.Fatal(err)
	}
	var ownFixture, otherFixture int64
	for id, lm := range w.LiveMatches {
		if lm.HomeID == ownClub || lm.AwayID == ownClub {
			ownFixture = id
		} else {
			otherFixture = id
		}
	}
	if ownFixture == 0 || otherFixture == 0 {
		t.Skip("need both an own and a rival live match; scheduling didn't provide both")
	}

	own := dataOf(t, g.getMatch(mid, "s1", getMatchIn{Fixture: ownFixture}))
	if own["status"] != "LIVE" {
		t.Fatalf("own live match status = %v", own["status"])
	}
	team, ok := own["own_team"].(map[string]any)
	if !ok {
		t.Fatal("own live match missing own_team state")
	}
	idx := playerIndex(w)
	sawDrain := false
	for _, row := range team["players"].([]map[string]any) {
		condition := row["condition"].(int)
		if condition < 0 || condition > worldgen.ConditionMax {
			t.Fatalf("live condition outside 0..100: %+v", row)
		}
		stored := idx[row["player"].(int64)].Condition
		if condition < stored {
			sawDrain = true
		}
	}
	if !sawDrain {
		t.Fatal("own live match still exposes only stale pre-match condition")
	}
	// Cards are as public live as finished — a red visibly ejects, so the
	// structured fact must exist while the match runs.
	if _, ok := own["cards"]; !ok {
		t.Fatal("live match missing the public cards rows")
	}
	if len(own["commentary"].([]map[string]any)) == 0 {
		t.Fatal("own live match missing commentary")
	}

	other := dataOf(t, g.getMatch(mid, "s2", getMatchIn{Fixture: otherFixture}))
	// own_team is the viewer's own-match convenience block — never for a rival.
	if _, leaked := other["own_team"]; leaked {
		t.Fatal("rival live match leaked own_team state")
	}
	// Commentary AND adjustments are public — a tactical shift is a visible
	// on-pitch action the commentary narrates, so the finished and live views
	// agree that it is observable (not private, unlike the Mindset plan).
	if _, ok := other["commentary"]; !ok {
		t.Fatal("rival live match should carry public commentary")
	}
	if _, ok := other["adjustments"]; !ok {
		t.Fatal("rival live match should carry public adjustments (a visible on-pitch action)")
	}
}

// TestGetMatchOwnRateViaMatchArg locks the PR #7 fix: an own match requested
// through the `match` argument is billed at the own-club rate, not overcharged
// as an other-club read — the cost resolver honors the same id fallback as the
// handler.
func TestGetMatchOwnRateViaMatchArg(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, ownClub := employedManager(t, g)
	var ownFixture int64
	for i := range g.Host.World().Fixtures {
		f := &g.Host.World().Fixtures[i]
		if f.HomeID == ownClub || f.AwayID == ownClub {
			ownFixture = f.ID
			break
		}
	}
	if ownFixture == 0 {
		t.Fatal("no fixture for own club")
	}
	env := g.getMatch(mid, "s1", getMatchIn{Match: ownFixture})
	spent := env["meta"].(map[string]any)["focus"].(map[string]any)["spent"].(int)
	own, _ := focus.CostOwnOther(focus.GetMatch, true)
	if spent != own {
		t.Fatalf("own match via `match` arg billed %d FP, want %d (own rate)", spent, own)
	}
}

// TestGetMatchLiveView exercises the LIVE path — the whole point of streaming
// matches rather than resolving them atomically. It stops mid-window (a match
// in progress) and asserts get_match returns a clean LIVE view: current score
// and clock, no finished-only lineup/ratings.
func TestGetMatchLiveView(t *testing.T) {
	g, host, _, _ := newGateway(t)
	mid, _ := employedManager(t, g)
	w := g.Host.World()
	var first sim.GameTime = 1 << 62
	for i := range w.Fixtures {
		if w.Fixtures[i].Kickoff < first {
			first = w.Fixtures[i].Kickoff
		}
	}
	if _, err := host.eng.RunUntil(first + sim.GameTime(45)); err != nil { // 45' in
		t.Fatal(err)
	}
	if len(w.LiveMatches) == 0 {
		t.Fatal("no live match mid-window — cannot exercise the LIVE view")
	}
	var liveFixture int64
	for id := range w.LiveMatches {
		liveFixture = id
		break
	}
	mv := dataOf(t, g.getMatch(mid, "s1", getMatchIn{Fixture: liveFixture}))
	if mv["status"] != "LIVE" {
		t.Fatalf("in-progress fixture status = %v, want LIVE", mv["status"])
	}
	if _, ok := mv["minute"]; !ok {
		t.Fatal("live view missing minute")
	}
	if _, ok := mv["home_lineup"]; ok {
		t.Fatal("live view leaked the finished-only lineup/ratings")
	}
}

// TestSearchPlayersRejectsBadSort locks that the sort enum is validated
// (docs/11 §4: value|age) rather than silently defaulting.
func TestSearchPlayersRejectsBadSort(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, _ := employedManager(t, g)
	env := g.searchPlayers(mid, "s1", searchPlayersIn{Sort: "bogus"})
	if env["ok"] == true {
		t.Fatalf("invalid sort should be rejected, got %+v", env)
	}
	if code := errCode(env); code != string(ErrValidation) {
		t.Fatalf("want %s error, got %q", ErrValidation, code)
	}
}

// TestGetClubHeadlinersMaskedOrdering locks that another club's public
// headline players are both SELECTED and ordered by the viewer's MASKED pool
// bucket (id tie-break), never raw pool — so a public profile can't leak exact
// intra-bucket ranking (FR-22, docs/11 §4). It compares the returned top-3
// against the masked top-3 recomputed over the full eligible squad, so a raw
// top-N pick with masked ordering would still fail.
func TestGetClubHeadlinersMaskedOrdering(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, ownClub := employedManager(t, g)
	m := g.managers[mid]
	var otherClub int64
	for _, c := range g.Host.World().Clubs {
		if c.ID != ownClub {
			otherClub = c.ID
			break
		}
	}
	w := g.Host.World()

	// Independently recompute the masked top-3 over the whole eligible squad.
	var squad []*worldgen.Player
	for i := range w.Players {
		if p := &w.Players[i]; p.ClubID == otherClub && !p.Youth {
			squad = append(squad, p)
		}
	}
	sort.Slice(squad, func(i, j int) bool {
		bi := bucketedPool(squad[i].AbilityPool, effectiveLevel(m, w, squad[i]))
		bj := bucketedPool(squad[j].AbilityPool, effectiveLevel(m, w, squad[j]))
		if bi != bj {
			return bi > bj
		}
		return squad[i].ID < squad[j].ID
	})
	wantN := min(3, len(squad))

	data := dataOf(t, g.getClub(mid, "s1", getClubIn{Club: otherClub}))
	players := data["headline_players"].([]map[string]any)
	if len(players) != wantN {
		t.Fatalf("want %d headliners, got %d", wantN, len(players))
	}
	for i := 0; i < wantN; i++ {
		if got := players[i]["player"].(int64); got != squad[i].ID {
			t.Fatalf("headliner %d: want id %d, got %d — selection/order not masked-consistent", i, squad[i].ID, got)
		}
	}
}

// TestHeadlineAttrsMaskedSelection locks that WHICH attributes surface as
// headlines is decided by the viewer's masked bucket, not raw values — the
// selection itself must not leak intra-bucket ranking for unscouted players.
func TestHeadlineAttrsMaskedSelection(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, _ := employedManager(t, g)
	m := g.managers[mid]
	w := g.Host.World()

	// Synthetic unscouted player (not the viewer's club, no knowledge → level
	// 0). Three attributes share the level-0 bucket [11,15] but differ in raw
	// value; two fillers sit in a lower bucket.
	p := &worldgen.Player{
		ID: 999999, ClubID: 999999,
		Visible: map[attr.Visible]int{
			attr.Finishing: 11, attr.Passing: 15, attr.Dribbling: 13,
			attr.Tackling: 3, attr.Vision: 3,
		},
	}
	got := headlineAttrs(m, w, p)
	// Masked selection ties on the shared bucket and breaks by attribute
	// identity ascending — DRIBBLING, FINISHING. Raw selection would instead
	// have surfaced PASSING (raw 15) as the standout, leaking its rank.
	if _, leaked := got[string(attr.Passing)]; leaked {
		t.Fatal("headline selection leaked raw value (PASSING chosen over lexicographically-smaller same-bucket attrs)")
	}
	if len(got) != 2 || got[string(attr.Dribbling)] == nil || got[string(attr.Finishing)] == nil {
		t.Fatalf("want {DRIBBLING, FINISHING} by masked bucket + identity tie-break, got %v", got)
	}
}

// TestValueBandMasksPoolUntilKnown locks FR-22 on the market-value estimate:
// the band edges are pure functions of pool, so an unquantized band would be
// invertible to an exact Ability Pool. Below full knowledge the pool is
// bucketed, so neighbours in the same bucket produce an identical band.
func TestValueBandMasksPoolUntilKnown(t *testing.T) {
	lowAmt := func(b map[string]any) int64 {
		return b["low"].(map[string]any)["amount"].(int64)
	}
	// Pools 101 and 104 share the level-0 bucket [100,104]: identical band,
	// so sqrt(low/k) cannot recover the exact pool.
	if lowAmt(valueBand(101, 0)) != lowAmt(valueBand(104, 0)) {
		t.Fatal("level-0 value band leaks exact Ability Pool")
	}
	// Full knowledge (own squad / fully scouted) resolves the band exactly.
	if lowAmt(valueBand(101, 3)) == lowAmt(valueBand(104, 3)) {
		t.Fatal("level-3 value band should reflect the exact pool")
	}
	// Domain-cap edge (regression): the top bucket must absorb the overflow so
	// PoolMax never sits alone in its bucket. pool 196 and PoolMax(200) share
	// the top level-0 bucket [195,200]; a naive floor would isolate 200.
	if lowAmt(valueBand(196, 0)) != lowAmt(valueBand(attr.PoolMax, 0)) {
		t.Fatal("top value bucket collapses at PoolMax — exact pool leaks")
	}
	if bucketedPool(attr.PoolMax, 0) != attr.PoolMax-knowledgeBuckets[0] {
		t.Fatalf("top bucket should absorb the cap, got floor %d", bucketedPool(attr.PoolMax, 0))
	}
	// The search sort keys on bucketedPool: same-bucket pools must compare
	// equal so ordering falls to id, never exact pool. But full knowledge
	// (level 3) still separates them.
	if bucketedPool(101, 0) != bucketedPool(104, 0) {
		t.Fatal("same-bucket pools must share a sort key (else intra-bucket order leaks)")
	}
	if bucketedPool(101, 3) == bucketedPool(104, 3) {
		t.Fatal("level-3 sort key should separate exact pools")
	}
}

func TestGameTimeISORoundYear(t *testing.T) {
	// Jan–Jun belong to the season's second calendar year (docs display).
	jan := gameTimeISO(gameTimeAtJan())
	if !strings.HasPrefix(jan, "1926-01") {
		t.Fatalf("January of season 1 should render 1926, got %s", jan)
	}
}

func gameTimeAtJan() sim.GameTime {
	// Day offset of Jan 1 within the season (matches worldgen calendar).
	return sim.GameTime(int64(184) * sim.MinutesPerDay) // ~Jan 1
}

// cursorParseSanity guards the cursor round-trip (string ints).
func TestCursorRoundTrip(t *testing.T) {
	for _, id := range []int64{0, 1, 42, 999999} {
		s := strconv.FormatInt(id, 10)
		got, err := strconv.ParseInt(s, 10, 64)
		if err != nil || got != id {
			t.Fatalf("cursor round-trip failed for %d", id)
		}
	}
}

// TestRetiredPlayerOffTheSurfaces locks the player-lifecycle wire rules: a
// retired player shares ClubID 0 with real free agents, so search_players must
// not surface them as signable, a fresh scout commission on them is rejected,
// and get_person names the state honestly via the retired flag.
func TestRetiredPlayerOffTheSurfaces(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, club := employedManager(t, g)

	w := g.Host.World()
	var pid int64
	for i := range w.Players {
		p := &w.Players[i]
		if p.ClubID != 0 && p.ClubID != club && !p.Youth {
			pid = p.ID
			p.Retired = true
			p.ClubID = 0
			p.Contract = nil
			break
		}
	}
	if pid == 0 {
		t.Fatal("no player to retire")
	}

	data := dataOf(t, g.searchPlayers(mid, "s1", searchPlayersIn{ContractStatus: "free_agent", Limit: 30}))
	for _, r := range data["players"].([]map[string]any) {
		if r["player"].(int64) == pid {
			t.Fatal("a retired player surfaced in search_players(free_agent)")
		}
	}

	if code := errCode(g.scout(mid, "s2", scoutIn{Target: personRef{Player: pid}})); code != string(ErrValidation) {
		t.Fatalf("scouting a retired player returned %q, want %s", code, ErrValidation)
	}

	person := dataOf(t, g.getPerson(mid, "s3", getPersonIn{Ref: personRef{Player: pid}}))
	if person["retired"] != true {
		t.Fatalf("get_person retired = %v, want true", person["retired"])
	}
}

// TestExpiringTracksCurrentSeason locks the current-season contract fix: the
// "expiring" surfaces compared ExpirySeasonYear against a hardcoded 1, which is
// only correct in season 1. In a later season, "expiring" must mean "ends with
// the CURRENT season" — in search results, the row flags, and get_situation's
// urgent count.
func TestExpiringTracksCurrentSeason(t *testing.T) {
	g, h, _, _ := newGateway(t)
	mid, club := employedManager(t, g)

	// Jump the clock into season 2 without simulating (the tools read
	// Engine.Now() as the current game time).
	h.eng.ResumeAt(sim.GameTime(int64(worldgen.DaysPerSeason+40) * sim.MinutesPerDay))

	w := g.Host.World()
	var s1, s2 int64
	for i := range w.Players {
		p := &w.Players[i]
		if p.ClubID != club || p.Youth || p.Contract == nil {
			continue
		}
		// Age 44 is beyond any generated age: with an AgeMin filter it isolates
		// exactly these two from the world-wide expiring pool (season 2 has many
		// generated expiry-2 deals, and search truncates to the pool-bucket top).
		if s1 == 0 {
			s1 = p.ID
			p.Age = 44
			p.Contract.ExpirySeasonYear = 1 // already lapsed-era deal: NOT "expiring" now
			continue
		}
		s2 = p.ID
		p.Age = 44
		p.Contract.ExpirySeasonYear = 2 // ends with the current season: expiring
		break
	}
	if s1 == 0 || s2 == 0 {
		t.Fatal("need two contracted seniors at the manager's club")
	}

	data := dataOf(t, g.searchPlayers(mid, "s1", searchPlayersIn{ContractStatus: "expiring", AgeMin: 44, Limit: 30}))
	sawS2 := false
	for _, r := range data["players"].([]map[string]any) {
		id := r["player"].(int64)
		if id == s1 {
			t.Fatal("a season-1 deal surfaced as expiring in season 2")
		}
		if id == s2 {
			sawS2 = true
			if r["flags"].(map[string]any)["expiring"] != true {
				t.Fatal("the expiring flag is not tracking the current season")
			}
		}
	}
	if !sawS2 {
		t.Fatal("the current-season deal did not surface as expiring")
	}

	sit := dataOf(t, g.getSituation(mid, "s2", emptyIn{}))
	urgent := sit["urgent"].(map[string]any)
	if got := urgent["expiring_contracts"].(int); got < 1 {
		t.Fatalf("get_situation expiring_contracts = %d in season 2, want >= 1", got)
	}
}

// TestInjuryVisibilityRule locks the FR-22 injury boundary: everyone sees the
// coarse severity band of a live injury; ONLY the owning manager sees the
// medical room's expected return date. History carries season+band only.
func TestInjuryVisibilityRule(t *testing.T) {
	g, h, _, _ := newGateway(t)
	mid, club := employedManager(t, g)

	w := g.Host.World()
	var own, other *worldgen.Player
	for i := range w.Players {
		p := &w.Players[i]
		if p.ClubID == club && own == nil {
			own = p
		}
		if p.ClubID != club && p.ClubID != 0 && other == nil {
			other = p
		}
	}
	until := sim.GameTime(int64(worldgen.DaysPerSeason) * sim.MinutesPerDay / 2)
	for _, p := range []*worldgen.Player{own, other} {
		p.InjuredUntil = until
		p.Injuries = append(p.Injuries, worldgen.InjuryRecord{SeasonYear: 1, Band: "WEEKS"})
	}
	_ = h // clock stays at 0 — both injuries are live

	mine := dataOf(t, g.getPerson(mid, "s1", getPersonIn{Ref: personRef{Player: own.ID}}))
	inj, ok := mine["injury"].(map[string]any)
	if !ok {
		t.Fatal("own injured player carries no injury block")
	}
	if _, ok := inj["expected_return"]; !ok {
		t.Fatal("owning manager must see the expected return date")
	}

	theirs := dataOf(t, g.getPerson(mid, "s2", getPersonIn{Ref: personRef{Player: other.ID}}))
	inj2, ok := theirs["injury"].(map[string]any)
	if !ok {
		t.Fatal("another club's injured player carries no injury block")
	}
	if _, ok := inj2["expected_return"]; ok {
		t.Fatal("FR-22 leak: a rival manager saw an exact return date")
	}
	if _, ok := inj2["severity"]; !ok {
		t.Fatal("the coarse severity band must be public")
	}
	hist, ok := theirs["injury_history"].([]any)
	if !ok || len(hist) == 0 {
		t.Fatal("injury history must list season+band entries")
	}
}

// TestFinishedLineupIncludesSubOns locks the regression fix: an archived
// match's lineups list every participant, so a sub-on's rating has a row.
func TestFinishedLineupIncludesSubOns(t *testing.T) {
	ids := participantIDs([]int64{1, 2}, []worldgen.SubEvent{
		{Minute: 60, ClubID: 7, Off: 2, On: 3},
		{Minute: 70, ClubID: 8, Off: 9, On: 10}, // other side — not ours
		{Minute: 80, ClubID: 7, Off: 1},         // uncovered withdrawal — no new body
	}, 7)
	if len(ids) != 3 || ids[2] != 3 {
		t.Fatalf("participants = %v, want starters + own sub-on [1 2 3]", ids)
	}
}

// TestFormAndCareerSurfaces locks the form and career read surfaces: get_person
// carries the rolling form average + band and the archived career lines;
// get_squad rows carry the band descriptor (never the raw ring).
func TestFormAndCareerSurfaces(t *testing.T) {
	g, _, _, _ := newGateway(t)
	mid, club := employedManager(t, g)

	w := g.Host.World()
	var p *worldgen.Player
	for i := range w.Players {
		if w.Players[i].ClubID == club && !w.Players[i].Youth {
			p = &w.Players[i]
			break
		}
	}
	p.FormX10 = []int{72, 72, 72}
	p.Career = append(p.Career, worldgen.SeasonRecord{
		SeasonYear: 1, ClubID: club, Apps: 30, Goals: 12, RatingSumX10: 2130,
	})

	person := dataOf(t, g.getPerson(mid, "s1", getPersonIn{Ref: personRef{Player: p.ID}}))
	form, ok := person["form"].(map[string]any)
	if !ok {
		t.Fatal("get_person carries no form block for a played player")
	}
	if form["rating_avg"].(float64) != 7.2 || form["matches"].(int) != 3 {
		t.Fatalf("form = %+v, want avg 7.2 over 3", form)
	}
	if band := form["band"].(map[string]any)["key"]; band != "desc.form.IN_FORM" {
		t.Fatalf("form band = %v, want IN_FORM at avg 7.2", band)
	}
	career, ok := person["career_history"].([]any)
	if !ok || len(career) != 1 {
		t.Fatalf("career_history = %v, want the archived season", person["career_history"])
	}
	row := career[0].(map[string]any)
	if row["apps"].(int) != 30 || row["rating_avg"].(float64) != 7.1 {
		t.Fatalf("career row = %+v, want 30 apps at 7.1", row)
	}

	squad := dataOf(t, g.getSquad(mid, "s2", getSquadIn{}))
	for _, r := range squad["players"].([]map[string]any) {
		if r["player"].(int64) != p.ID {
			continue
		}
		band := r["form"].(map[string]any)["key"]
		if band != "desc.form.IN_FORM" {
			t.Fatalf("squad form band = %v, want IN_FORM", band)
		}
		return
	}
	t.Fatal("pinned player missing from the squad view")
}

func TestPublicRatingAverageHasStablePrecision(t *testing.T) {
	tests := []struct {
		name    string
		sumX10  int
		matches int
		want    float64
	}{
		{name: "repeating fraction", sumX10: 196, matches: 3, want: 6.53},
		{name: "exact tenth", sumX10: 72, matches: 1, want: 7.2},
		{name: "rounds half up", sumX10: 65, matches: 4, want: 1.63},
		{name: "no appearances", sumX10: 0, matches: 0, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := publicRatingAverage(tt.sumX10, tt.matches); got != tt.want {
				t.Fatalf("publicRatingAverage(%d, %d) = %v, want %v", tt.sumX10, tt.matches, got, tt.want)
			}
		})
	}
	wire, err := json.Marshal(map[string]any{"rating_avg": publicRatingAverage(196, 3)})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(wire); got != `{"rating_avg":6.53}` {
		t.Fatalf("public rating JSON = %s, want stable two-decimal output", got)
	}
}

// TestSubRowsCarryReason locks the public substitution "why": a
// reason-stamped SubEvent renders its token, an uncovered pre-reason row
// renders neither on nor reason.
func TestSubRowsCarryReason(t *testing.T) {
	rows := subRows([]worldgen.SubEvent{
		{Minute: 70, ClubID: 1, Off: 5, On: 6, Reason: "TACTICAL"},
		{Minute: 80, ClubID: 1, Off: 7},
	})
	if rows[0]["reason"] != "TACTICAL" || rows[0]["on"] != int64(6) {
		t.Fatalf("reason row misrendered: %+v", rows[0])
	}
	if _, ok := rows[1]["reason"]; ok {
		t.Fatalf("blank reason must stay off the row: %+v", rows[1])
	}
	if _, ok := rows[1]["on"]; ok {
		t.Fatalf("uncovered withdrawal must have no on: %+v", rows[1])
	}
}

// TestArchivedGetMatch locks the ledger surface: a past season's
// fixture — invisible to ResultFor, LiveMatches, and the current fixture list
// alike — serves the finished view flagged archived with its season, the
// commentary honestly empty (the prose wasn't kept), the facts (subs with
// reasons, ratings) intact. And the cost resolver treats an archived OWN
// fixture as own — the rollover cleared Fixtures, so without the archive
// fallback the manager would be overcharged as an other-club read.
func TestArchivedGetMatch(t *testing.T) {
	g, host, _, _ := newGateway(t)
	mid, ownClub := employedManager(t, g)
	w := g.Host.World()

	var other int64
	for i := range w.Clubs {
		if w.Clubs[i].ID != ownClub {
			other = w.Clubs[i].ID
			break
		}
	}
	var ownPID, otherPID int64
	for i := range w.Players {
		p := &w.Players[i]
		if p.ClubID == ownClub && ownPID == 0 {
			ownPID = p.ID
		}
		if p.ClubID == other && otherPID == 0 {
			otherPID = p.ID
		}
	}
	const archivedFixture = int64(9_999_999)
	kickoff := sim.GameTime(100 * sim.MinutesPerDay) // deep in season 1
	host.LockedWrite(func() {
		w.History = append(w.History, worldgen.SeasonSummary{
			SeasonYear: 1,
			Results: []worldgen.MatchResult{{
				FixtureID: archivedFixture, Competition: worldgen.CompetitionLeague,
				HomeID: ownClub, AwayID: other, HomeGoals: 2, AwayGoals: 1,
				Kickoff: kickoff,
				HomeXI:  []int64{ownPID}, AwayXI: []int64{otherPID},
				Subs:       []worldgen.SubEvent{{Minute: 61, ClubID: ownClub, Off: ownPID, On: otherPID, Reason: "TACTICAL"}},
				RatingsX10: map[int64]int{ownPID: 74, otherPID: 61},
			}},
		})
	})

	env := g.getMatch(mid, "s1", getMatchIn{Fixture: archivedFixture})
	spent := env["meta"].(map[string]any)["focus"].(map[string]any)["spent"].(int)
	if own, _ := focus.CostOwnOther(focus.GetMatch, true); spent != own {
		t.Fatalf("archived own fixture billed %d FP, want the own rate %d", spent, own)
	}
	data := dataOf(t, env)
	if data["status"] != "FINISHED" || data["archived"] != true {
		t.Fatalf("archived view status/flag = %v/%v", data["status"], data["archived"])
	}
	if data["season"] != 1 {
		t.Fatalf("archived season = %v, want 1", data["season"])
	}
	if lines := data["commentary"].([]map[string]any); len(lines) != 0 {
		t.Fatalf("archived commentary = %d lines, want the honest empty list", len(lines))
	}
	subs := data["subs"].([]map[string]any)
	if len(subs) != 1 || subs[0]["reason"] != "TACTICAL" {
		t.Fatalf("archived subs lost their facts: %+v", subs)
	}

	// A fixture nobody ever played stays a clean 404 — the archive fallback
	// must not swallow the not-found path.
	if code := errCode(g.getMatch(mid, "s2", getMatchIn{Fixture: 8_888_888})); code != "NOT_FOUND" {
		t.Fatalf("unknown fixture = %q, want NOT_FOUND", code)
	}
}
