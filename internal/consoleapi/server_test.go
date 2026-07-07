package consoleapi

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/engine"
	"github.com/gaemi/agentic-fc/internal/narrative"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

type tokens struct{ n uint32 }

func (t *tokens) Read(p []byte) (int, error) {
	for i := range p {
		t.n++
		p[i] = byte((t.n * 2654435761) >> 24)
	}
	return len(p), nil
}

// testHost is an in-memory Host over a freshly generated world.
type testHost struct {
	mu     sync.RWMutex
	world  *worldgen.World
	engine *engine.Engine
	seed   uint64
	creds  []worldgen.ManagerCredential
	state  string
}

func newTestHost(t *testing.T) *testHost {
	t.Helper()
	res, err := worldgen.Generate(worldgen.PresetCompact(21), worldgen.WithTokenReader(&tokens{}))
	if err != nil {
		t.Fatal(err)
	}
	return &testHost{
		world:  res.World,
		engine: engine.New(res.World, res.Queue, &store.MemAuditLog{}),
		seed:   res.World.Config.Seed,
		creds:  res.Manifest.Managers,
		state:  "ready",
	}
}

func (h *testHost) Locked(read func()) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	read()
}
func (h *testHost) World() *worldgen.World { return h.world }
func (h *testHost) Engine() *engine.Engine { return h.engine }
func (h *testHost) State() string          { return h.state }
func (h *testHost) Seed() uint64           { return h.seed }
func (h *testHost) Tempo() sim.Tempo {
	if h.state == "paused" {
		return sim.TempoPaused
	}
	return h.engine.TempoAt(h.engine.Now())
}
func (h *testHost) Credentials() []worldgen.ManagerCredential { return h.creds }
func (h *testHost) Start() error {
	if h.state != "ready" {
		return errors.New("already started")
	}
	h.state = "running"
	return nil
}
func (h *testHost) SetPaused(p bool) error {
	if h.state == "ready" {
		return errors.New("world not started")
	}
	if p {
		h.state = "paused"
	} else {
		h.state = "running"
	}
	return nil
}

func newTestServer(t *testing.T) (*Server, *testHost) {
	t.Helper()
	host := newTestHost(t)
	return &Server{
		AdminToken:        "sekrit",
		Host:              host,
		Feed:              NewHub(narrative.Default),
		Catalogs:          narrative.Default,
		HeartbeatInterval: 50 * time.Millisecond,
	}, host
}

func get(t *testing.T, s *Server, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestWorldEndpoint(t *testing.T) {
	s, _ := newTestServer(t)
	code, body := get(t, s, "/v1/world?locale=ko")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	var dto map[string]any
	if err := json.Unmarshal([]byte(body), &dto); err != nil {
		t.Fatal(err)
	}
	if dto["state"] != "ready" || dto["tempo_label"] != "비시즌" {
		t.Fatalf("world dto = %v", dto)
	}
	if _, leaked := dto["seed"]; leaked {
		t.Fatal("seed leaked to the viewer surface")
	}
	if !strings.Contains(dto["clock_text"].(string), "시즌 1") {
		t.Fatalf("ko clock = %q", dto["clock_text"])
	}
}

func TestUIStringsLocaleAndFallback(t *testing.T) {
	s, _ := newTestServer(t)
	_, body := get(t, s, "/v1/ui?locale=ko")
	var out struct {
		Strings map[string]string `json:"strings"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Strings["ui.tab.table"] != "순위표" || out.Strings["attr.PACE"] != "스피드" ||
		out.Strings["term.chance_type.COUNTER"] != "역습" {
		t.Fatalf("ko ui strings wrong: %v", out.Strings["ui.tab.table"])
	}
	_, body = get(t, s, "/v1/ui?locale=fr") // unknown → English fallback
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Strings["ui.tab.table"] != "Table" {
		t.Fatalf("fallback ui strings wrong: %v", out.Strings["ui.tab.table"])
	}
}

func TestTables(t *testing.T) {
	s, host := newTestServer(t)
	code, body := get(t, s, "/v1/tables?tier=1")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	var out struct {
		Rows []tableRowDTO `json:"rows"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != host.world.Config.ClubsPerDivision {
		t.Fatalf("rows = %d", len(out.Rows))
	}
	if code, _ := get(t, s, "/v1/tables?tier=9"); code != http.StatusNotFound {
		t.Fatalf("bad tier status %d", code)
	}
	// Error bodies are localized (FR-35c covers failure states).
	if _, body := get(t, s, "/v1/tables?tier=9&locale=ko"); !strings.Contains(body, "존재하지 않는 디비전입니다.") ||
		!strings.Contains(body, "error.unknown_tier") {
		t.Fatalf("ko error body = %s", body)
	}
}

func TestClubDetail(t *testing.T) {
	s, host := newTestServer(t)
	id := host.world.Clubs[0].ID
	code, body := get(t, s, fmt.Sprintf("/v1/clubs/%d", id))
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	var out struct {
		Name     string            `json:"name"`
		Manager  string            `json:"manager"`
		Board    map[string]string `json:"board"`
		Finances map[string]string `json:"finances"`
		Squad    []playerDTO       `json:"squad"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	wantSquad := host.world.Config.SquadSizeTarget
	seniors := 0
	for _, p := range out.Squad {
		if !p.Youth {
			seniors++
		}
		if len(p.Attributes) != len(attr.PoolCosts[attr.PositionGroup(p.Group)]) {
			t.Fatalf("player %d has %d attributes", p.ID, len(p.Attributes))
		}
		if p.HeightCm < 160 || p.HeightCm > 205 || p.WeightKg < 58 || p.WeightKg > 108 {
			t.Fatalf("player %d body out of range: %dcm %dkg", p.ID, p.HeightCm, p.WeightKg)
		}
		if p.Familiarity == "" || p.FamiliarityLabel == "" {
			t.Fatalf("player %d missing familiarity descriptor/label", p.ID)
		}
	}
	if seniors != wantSquad {
		t.Fatalf("squad = %d, want %d", seniors, wantSquad)
	}
	if out.Manager == "" || out.Board["security"] == "" || out.Finances["cash"] == "" ||
		out.Finances["salary_bill"] == "" || out.Finances["market_funds"] == "" {
		t.Fatalf("club detail missing observer fields: %#v", out)
	}

	// Descriptor labels localize per request (FR-35c).
	_, body = get(t, s, fmt.Sprintf("/v1/clubs/%d?locale=ko", id))
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	koLabels := map[string]bool{"최적": true, "능숙": true, "준수": true, "어색": true}
	for _, p := range out.Squad {
		if !koLabels[p.FamiliarityLabel] {
			t.Fatalf("player %d ko familiarity label = %q", p.ID, p.FamiliarityLabel)
		}
	}
}

func TestNewsArticles(t *testing.T) {
	s, host := newTestServer(t)
	host.world.AddNews(worldgen.NewsItem{
		GameTime: 123,
		Category: "board",
		Key:      "news.board.appointed",
		Params:   map[string]any{"club": "Alderton Athletic", "manager": "Lee Carter"},
		ClubIDs:  []int64{host.world.Clubs[0].ID},
	})
	host.world.AddNews(worldgen.NewsItem{
		GameTime:  124,
		Category:  "scout",
		Key:       "news.scout.report",
		Params:    map[string]any{"player": "Private", "level": 2},
		ManagerID: host.world.Managers[0].ID,
	})

	code, body := get(t, s, "/v1/news?limit=10&locale=ko")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	var out struct {
		Items []newsArticleDTO `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("news items = %d, body = %s", len(out.Items), body)
	}
	item := out.Items[0]
	for _, want := range []string{"클럽 데스크", "Alderton Athletic, Lee Carter 신임 감독 선임", "보드룸 압박", "압박계", "경기력"} {
		if !strings.Contains(item.Source+item.Title+item.Deck+item.Body, want) {
			t.Fatalf("article missing %q: %#v", want, item)
		}
	}

	for i := 0; i < 150; i++ {
		host.world.AddNews(worldgen.NewsItem{
			GameTime: sim.GameTime(200 + i),
			Category: "board",
			Key:      "news.board.appointed",
			Params:   map[string]any{"club": fmt.Sprintf("Club %03d", i), "manager": "Archive Manager"},
		})
	}
	code, body = get(t, s, "/v1/news?limit=150")
	if code != http.StatusOK {
		t.Fatalf("history news status %d", code)
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 150 {
		t.Fatalf("history news items = %d, want 150", len(out.Items))
	}
}

// TestHiddenNeverLeaks is the FR-22 guardrail: no viewer endpoint may emit
// hidden attributes, trajectory values, reputation, raw tendencies, wages,
// balances, tokens, or the world seed.
func TestHiddenNeverLeaks(t *testing.T) {
	s, host := newTestServer(t)
	// Seed a live match so the live-match surface has real content to scan
	// (commentary params carry names/scores only — the pitch markers must
	// never grow anything maskable).
	host.world.LiveMatches = map[int64]*worldgen.LiveMatch{
		1: {
			FixtureID: 1, Competition: "LEAGUE",
			HomeID: host.world.Clubs[0].ID, AwayID: host.world.Clubs[1].ID,
			HomeGoals: 1, Clock: 30,
			Commentary: []worldgen.CommentaryLine{
				{Minute: 10, Key: "comment.goal.1", Params: map[string]any{
					"player": "P", "club": host.world.Clubs[0].Name,
					"home_goals": 1, "away_goals": 0}},
				{Minute: 20, Key: "comment.injury", Params: map[string]any{
					"player": "Q", "club": host.world.Clubs[1].Name}},
			},
		},
	}
	host.world.Results = append(host.world.Results, worldgen.MatchResult{
		FixtureID: 99, Competition: "LEAGUE", DivisionTier: 1,
		HomeID: host.world.Clubs[0].ID, AwayID: host.world.Clubs[1].ID,
		HomeGoals: 2, AwayGoals: 1,
		Scorers: []worldgen.MatchEvent{{Minute: 10, ClubID: host.world.Clubs[0].ID}},
	})
	paths := []string{
		"/v1/world", "/v1/ui?locale=ko", "/v1/news", "/v1/tables?tier=1", "/v1/clubs",
		fmt.Sprintf("/v1/clubs/%d", host.world.Clubs[0].ID), "/v1/fixtures",
		"/v1/matches/live", "/v1/matches/99",
	}
	forbidden := []string{
		// JSON keys that must never appear on the viewer surface
		`"seed"`, `"hidden"`, `"ability_pool"`, `"potential_cap"`,
		`"reputation"`, `"wealth"`, `"board_patience"`, `"board_ambition"`,
		`"fan_patience"`, `"fan_passion"`, `"youth_emphasis"`,
		`"training_facilities"`, `"youth_facilities"`, `"wage_`,
		`"balance_minor"`, `"transfer_budget`, `"token"`,
		// hidden attribute enum values
		"PROFESSIONALISM", "TEMPERAMENT", "ADAPTABILITY", "SOCIABILITY",
		"INFLUENCE", "CONSISTENCY", "BIG_MATCH_NERVE", "INJURY_PRONENESS",
		"DEVELOPMENT_SPEED", "DECLINE_ONSET", "DECLINE_SPEED", "VERSATILITY",
	}
	for _, path := range paths {
		code, body := get(t, s, path)
		if code != http.StatusOK {
			t.Fatalf("%s status %d", path, code)
		}
		for _, f := range forbidden {
			if strings.Contains(body, f) {
				t.Errorf("%s leaks %s", path, f)
			}
		}
	}

	// The SSE feed is a viewer surface too: render every feed event kind
	// (and the system lifecycle lines) through the same path the hub uses
	// and scan the wire payloads.
	events := []engine.FeedEvent{
		{GameTime: 1, Key: engine.FeedDriftGrew, Params: map[string]any{
			"player": "P", "club": "C", "attr_key": "PACE", "from": 9, "to": 10}},
		{GameTime: 2, Key: engine.FeedDriftDeclined, Params: map[string]any{
			"player": "P", "club": "", "attr_key": "STAMINA", "from": 9, "to": 8}},
		{GameTime: 3, Key: engine.FeedWindowOpened, Params: map[string]any{"window_key": "summer"}},
		{GameTime: 4, Key: engine.FeedWindowClosed, Params: map[string]any{"window_key": "winter"}},
		{GameTime: 5, Key: engine.FeedSeasonEnded, Params: map[string]any{"season": 1}},
		{GameTime: 6, Key: engine.FeedKickoff, Params: map[string]any{
			"home": "A", "away": "B", "round": 1, "competition": "LEAGUE"}},
	}
	for _, loc := range narrative.Supported {
		for _, ev := range events {
			b, err := json.Marshal(renderFeed(narrative.Default, loc, ev))
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range forbidden {
				if strings.Contains(string(b), f) {
					t.Errorf("feed event %s (%s) leaks %s", ev.Key, loc, f)
				}
			}
		}
	}

	// World lifecycle lines are serialized by Hub.System's own path — scan
	// those wire payloads too.
	for _, loc := range narrative.Supported {
		ch, cancel := s.Feed.Subscribe(loc)
		for _, key := range []string{
			"feed.world.started", "feed.world.paused", "feed.world.resumed",
		} {
			s.Feed.System(key)
			select {
			case b := <-ch:
				for _, f := range forbidden {
					if strings.Contains(string(b), f) {
						t.Errorf("system line %s (%s) leaks %s", key, loc, f)
					}
				}
			case <-time.After(time.Second):
				t.Fatalf("no system payload for %s", key)
			}
		}
		cancel()
	}
}

func TestAdminAuthAndLifecycle(t *testing.T) {
	s, _ := newTestServer(t)
	mux := s.Routes()

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated admin status = %d", rec.Code)
	}

	do := func(method, path string) (int, string) {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("X-Admin-Token", "sekrit")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	if code, body := do(http.MethodGet, "/v1/admin/status"); code != http.StatusOK ||
		!strings.Contains(body, `"seed"`) {
		t.Fatalf("admin status = %d %s", code, body)
	}
	if code, _ := do(http.MethodPost, "/v1/admin/pause"); code != http.StatusConflict {
		t.Fatal("pausing a ready world must conflict")
	}
	if code, body := do(http.MethodPost, "/v1/admin/start"); code != http.StatusOK ||
		!strings.Contains(body, "running") {
		t.Fatalf("start = %d %s", code, body)
	}
	if code, body := do(http.MethodPost, "/v1/admin/pause"); code != http.StatusOK ||
		!strings.Contains(body, "paused") {
		t.Fatalf("pause = %d %s", code, body)
	}
	if code, body := do(http.MethodPost, "/v1/admin/resume"); code != http.StatusOK ||
		!strings.Contains(body, "running") {
		t.Fatalf("resume = %d %s", code, body)
	}
	if code, body := do(http.MethodGet, "/v1/admin/managers"); code != http.StatusOK ||
		!strings.Contains(body, "mgr_") {
		t.Fatalf("managers = %d %s", code, body)
	}
}

func TestFeedSSE(t *testing.T) {
	s, _ := newTestServer(t)
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/feed?locale=ko")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}

	// Give the subscription a beat, then publish through the hub the same
	// way the engine sink does.
	time.Sleep(50 * time.Millisecond)
	s.Feed.OnFeedEvent(engine.FeedEvent{
		GameTime: 100,
		Key:      engine.FeedWindowOpened,
		Params:   map[string]any{"window_key": "summer"},
	})

	// System lifecycle lines render per locale like any feed line (A11 +
	// FR-35c) — publish one of each.
	s.Feed.System("feed.world.paused")

	deadline := time.After(3 * time.Second)
	lines := make(chan string, 8)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	var got []string
	for len(got) < 2 {
		select {
		case <-deadline:
			t.Fatalf("only %d feed lines within deadline: %v", len(got), got)
		case l := <-lines:
			if !strings.HasPrefix(l, "data: ") {
				continue // heartbeat or blank
			}
			var line narrative.Line
			if err := json.Unmarshal([]byte(strings.TrimPrefix(l, "data: ")), &line); err != nil {
				t.Fatal(err)
			}
			if line.Cadence.Density != "DRAMATIC" {
				t.Fatalf("cadence = %+v", line.Cadence)
			}
			got = append(got, line.Message.Text)
		}
	}
	if got[0] != "여름 이적시장이 열렸습니다." {
		t.Fatalf("rendered feed = %q", got[0])
	}
	if got[1] != "점검을 위해 월드가 일시정지되었습니다." {
		t.Fatalf("rendered system line = %q", got[1])
	}
}

var _ io.Reader = (*tokens)(nil)

// TestLiveMatches locks the Live Match endpoint (docs/07 §4.1): rendered
// commentary in the requested locale, abstract event markers derived from the
// commentary keys with the right side attribution, quiet passages skipped,
// and a stable fixture order.
func TestLiveMatches(t *testing.T) {
	s, host := newTestServer(t)
	home, away := &host.world.Clubs[0], &host.world.Clubs[1]
	host.world.LiveMatches = map[int64]*worldgen.LiveMatch{
		7: {
			FixtureID: 7, Competition: "LEAGUE", HomeID: home.ID, AwayID: away.ID,
			HomeGoals: 1, AwayGoals: 0, Clock: 40,
			Commentary: []worldgen.CommentaryLine{
				{Minute: 3, Key: "comment.quiet.1"},
				{Minute: 12, Key: "comment.goal.1", Params: map[string]any{
					"player": "A", "club": home.Name, "home_goals": 1, "away_goals": 0}},
				{Minute: 30, Key: "comment.card.yellow", Params: map[string]any{
					"player": "B", "club": away.Name}},
			},
		},
		3: {FixtureID: 3, Competition: "LEAGUE", HomeID: away.ID, AwayID: home.ID},
	}

	code, body := get(t, s, "/v1/matches/live?locale=ko")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	var out struct {
		Matches []liveMatchDTO `json:"matches"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Matches) != 2 || out.Matches[0].Fixture != 3 || out.Matches[1].Fixture != 7 {
		t.Fatalf("matches order/count wrong: %+v", out.Matches)
	}
	lm := out.Matches[1]
	if lm.Home != home.Name || lm.HomeGoals != 1 || lm.Minute != 40 {
		t.Fatalf("header wrong: %+v", lm)
	}
	if len(lm.Commentary) != 3 || !strings.Contains(strings.Join(lm.Commentary, " "), home.Name) {
		t.Fatalf("commentary not rendered: %+v", lm.Commentary)
	}
	if len(lm.Markers) != 2 {
		t.Fatalf("markers = %+v, want goal+card only (quiet skipped)", lm.Markers)
	}
	if lm.Markers[0].Kind != "GOAL" || lm.Markers[0].Side != "HOME" {
		t.Fatalf("goal marker = %+v, want HOME GOAL", lm.Markers[0])
	}
	if lm.Markers[1].Kind != "CARD" || lm.Markers[1].Side != "AWAY" {
		t.Fatalf("card marker = %+v, want AWAY CARD", lm.Markers[1])
	}
}

// TestTablesServeLiveStandings locks the live-standings fix: /v1/tables reads the
// LIVE current-season table.
func TestTablesServeLiveStandings(t *testing.T) {
	s, host := newTestServer(t)
	// Make the live table visibly different from the synthetic history.
	host.world.Table[0][0].Points = 4242
	winner := host.world.Table[0][0].ClubID
	_, body := get(t, s, "/v1/tables?tier=1")
	if !strings.Contains(body, "4242") {
		t.Fatalf("live table points not served; body = %.200s", body)
	}
	var out struct {
		Rows []tableRowDTO `json:"rows"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range out.Rows {
		if r.ClubID == winner && r.Points == 4242 {
			found = true
		}
	}
	if !found {
		t.Fatal("mutated live standing missing from the response")
	}
}

func TestFixturesAndMatchDetailServeResults(t *testing.T) {
	s, host := newTestServer(t)
	w := host.world
	f := w.Fixtures[0]
	var hp, ap *worldgen.Player
	for i := range w.Players {
		p := &w.Players[i]
		switch {
		case p.ClubID == f.HomeID && hp == nil && !p.Youth:
			hp = p
		case p.ClubID == f.AwayID && ap == nil && !p.Youth:
			ap = p
		}
	}
	if hp == nil || ap == nil {
		t.Fatal("test world has no players for fixture sides")
	}
	w.Results = append(w.Results, worldgen.MatchResult{
		FixtureID: f.ID, Competition: f.Competition, DivisionTier: f.DivisionTier,
		HomeID: f.HomeID, AwayID: f.AwayID, HomeGoals: 2, AwayGoals: 1,
		Kickoff: f.Kickoff, HomeShots: 9, AwayShots: 4,
		HomeXI: []int64{hp.ID}, AwayXI: []int64{ap.ID},
		Scorers: []worldgen.MatchEvent{{Minute: 20, PlayerID: hp.ID, ClubID: f.HomeID}},
		Cards:   []worldgen.MatchEvent{{Minute: 70, PlayerID: ap.ID, ClubID: f.AwayID, Detail: "YELLOW"}},
		Subs:    []worldgen.SubEvent{{Minute: 65, ClubID: f.HomeID, Off: hp.ID, On: hp.ID, Reason: "TACTICAL"}},
		RatingsX10: map[int64]int{
			hp.ID: 78,
			ap.ID: 62,
		},
		Commentary: []worldgen.CommentaryLine{
			{Minute: 20, Key: "comment.goal.1", Params: map[string]any{
				"player": hp.Name, "club": w.Clubs[0].Name, "home_goals": 1, "away_goals": 0}},
		},
	})

	code, body := get(t, s, "/v1/fixtures?tier=1&limit=1")
	if code != http.StatusOK {
		t.Fatalf("fixtures limited status %d", code)
	}
	var limited []fixtureDTO
	if err := json.Unmarshal([]byte(body), &limited); err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 {
		t.Fatalf("limited fixtures = %d, want 1", len(limited))
	}

	code, body = get(t, s, "/v1/fixtures?tier=1&limit=200")
	if code != http.StatusOK {
		t.Fatalf("fixtures status %d", code)
	}
	var fixtures []fixtureDTO
	if err := json.Unmarshal([]byte(body), &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 || fixtures[0].Status != "RESULT" || !fixtures[0].HasReplay || fixtures[0].HomeGoals != 2 {
		t.Fatalf("result fixture not surfaced first: %+v", fixtures[:min(1, len(fixtures))])
	}

	code, body = get(t, s, fmt.Sprintf("/v1/matches/%d?locale=ko", f.ID))
	if code != http.StatusOK {
		t.Fatalf("match detail status %d: %s", code, body)
	}
	var detail matchDetailDTO
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Fixture != f.ID || detail.HomeGoals != 2 || detail.HomeShots != 9 {
		t.Fatalf("detail score/stats wrong: %+v", detail)
	}
	if len(detail.Commentary) != 1 || !strings.Contains(detail.Commentary[0], hp.Name) {
		t.Fatalf("commentary replay not rendered: %+v", detail.Commentary)
	}
	if len(detail.Scorers) != 1 || detail.Scorers[0].Player != hp.Name ||
		len(detail.Cards) != 1 || detail.Cards[0].Detail != "YELLOW" ||
		len(detail.Subs) != 1 || detail.Subs[0].Reason != "TACTICAL" ||
		len(detail.Ratings) != 2 || detail.Ratings[0].Name != hp.Name {
		t.Fatalf("detail facts wrong: %+v", detail)
	}
}

// TestLiveMatchPanes locks the §4.1 side-pane data on the wire: the stats
// block counts shots/cards/subs from the persisted tally, the ratings rows
// come from the shared band formula ("as if it ended now" — a scorer on the
// winning side outrates a booked loser), and the momentum sparkline folds the
// FULL event stream (goals ×3, chances ×1, home positive) even where the
// windowed pitch markers have already scrolled away.
func TestLiveMatchPanes(t *testing.T) {
	s, host := newTestServer(t)
	w := host.world
	home, away := &w.Clubs[0], &w.Clubs[1]
	var hp, hp2, ap *worldgen.Player
	for i := range w.Players {
		p := &w.Players[i]
		switch {
		case p.ClubID == home.ID && hp == nil && !p.Youth:
			hp = p
		case p.ClubID == home.ID && hp2 == nil && !p.Youth:
			hp2 = p
		case p.ClubID == away.ID && ap == nil && !p.Youth:
			ap = p
		}
	}
	if hp == nil || hp2 == nil || ap == nil {
		t.Fatal("test worlds always staff both clubs")
	}
	w.LiveMatches = map[int64]*worldgen.LiveMatch{
		7: {
			FixtureID: 7, Competition: "LEAGUE", HomeID: home.ID, AwayID: away.ID,
			HomeGoals: 1, AwayGoals: 0, Clock: 70,
			HomeXI: []int64{hp.ID}, AwayXI: []int64{ap.ID},
			HomeShots: 6, AwayShots: 2,
			Subs:       []worldgen.SubEvent{{Minute: 60, ClubID: home.ID, Off: hp2.ID, On: hp2.ID, Reason: "TACTICAL"}},
			Scorers:    []worldgen.MatchEvent{{Minute: 12, PlayerID: hp.ID, ClubID: home.ID}},
			Cards:      []worldgen.MatchEvent{{Minute: 55, PlayerID: ap.ID, ClubID: away.ID, Detail: "YELLOW"}},
			Commentary: commentaryWithScrolledGoal(hp.Name, home.Name, ap.Name, away.Name),
		},
	}

	code, body := get(t, s, "/v1/matches/live")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	var out struct {
		Matches []liveMatchDTO `json:"matches"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	lm := out.Matches[0]

	st := lm.Stats
	if st.HomeShots != 6 || st.AwayShots != 2 || st.HomeCards != 0 || st.AwayCards != 1 ||
		st.HomeSubs != 1 || st.AwaySubs != 0 {
		t.Fatalf("stats pane wrong: %+v", st)
	}

	if len(lm.Ratings) < 2 {
		t.Fatalf("ratings rows = %+v, want both sides", lm.Ratings)
	}
	if lm.Ratings[0].Side != "HOME" || lm.Ratings[0].Name != hp.Name {
		t.Fatalf("first rating row = %+v, want the home scorer", lm.Ratings[0])
	}
	// Scorer on the winning side: base 65 + win 3 + goal 8 = 76, plus the
	// clean-sheet 5 when the scorer happens to be a GK/DF, clamped to 80.
	wantScorer := 76
	if hp.Group == attr.GK || hp.Group == attr.DF {
		wantScorer = 80
	}
	if lm.Ratings[0].RatingX10 != wantScorer {
		t.Fatalf("scorer rating = %d, want %d", lm.Ratings[0].RatingX10, wantScorer)
	}
	var awayRow *liveRatingDTO
	for i := range lm.Ratings {
		if lm.Ratings[i].Side == "AWAY" {
			awayRow = &lm.Ratings[i]
		}
	}
	// Booked on the losing side: 65 − 3 − 3 = 59, clamped to the band floor 60
	// (a GK/DF concession never earns a clean-sheet bonus while behind).
	if awayRow == nil || awayRow.RatingX10 != 60 {
		t.Fatalf("away rating row = %+v, want the band floor 60", awayRow)
	}

	if len(lm.Momentum) != momentumBuckets {
		t.Fatalf("momentum has %d buckets, want %d", len(lm.Momentum), momentumBuckets)
	}
	if lm.Momentum[1] != 3 || lm.Momentum[3] != -1 {
		t.Fatalf("momentum = %v, want +3 in bucket 1 (12' goal) and −1 in bucket 3 (33' chance)", lm.Momentum)
	}
	// The pitch markers stay windowed — and the 12' goal has scrolled OFF that
	// window (10 later chances follow it), yet its +3 is in the sparkline
	// above: momentum reads the FULL stream, not the capped markers.
	if len(lm.Markers) != liveMarkerWindow {
		t.Fatalf("markers = %d, want the window cap %d", len(lm.Markers), liveMarkerWindow)
	}
	for _, mk := range lm.Markers {
		if mk.Kind == "GOAL" {
			t.Fatal("the early goal should have scrolled off the marker window")
		}
	}
}

// commentaryWithScrolledGoal builds a stream long enough to push the opening
// goal off the pitch-marker window: the 12' goal, a 33' away chance, a 55'
// booking, then ten late home chances.
func commentaryWithScrolledGoal(scorer, homeName, awayPlayer, awayName string) []worldgen.CommentaryLine {
	lines := []worldgen.CommentaryLine{
		{Minute: 12, Key: "comment.goal.1", Params: map[string]any{
			"player": scorer, "club": homeName, "home_goals": 1, "away_goals": 0}},
		{Minute: 33, Key: "comment.chance.1", Params: map[string]any{
			"player": awayPlayer, "club": awayName}},
		{Minute: 55, Key: "comment.card.yellow", Params: map[string]any{
			"player": awayPlayer, "club": awayName}},
	}
	for minute := 60; minute < 70; minute++ {
		lines = append(lines, worldgen.CommentaryLine{
			Minute: minute, Key: "comment.chance.2", Params: map[string]any{
				"player": scorer, "club": homeName}})
	}
	return lines
}
