package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gaemi/agentic-fc/internal/engine"
	"github.com/gaemi/agentic-fc/internal/focus"
	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/narrative"
	"github.com/gaemi/agentic-fc/internal/rng"
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

// testHost implements Host over a generated world.
type testHost struct {
	mu     sync.RWMutex
	world  *worldgen.World
	eng    *engine.Engine
	paused bool
}

func (h *testHost) LockedWrite(fn func())                { h.mu.Lock(); defer h.mu.Unlock(); fn() }
func (h *testHost) Engine() *engine.Engine               { return h.eng }
func (h *testHost) World() *worldgen.World               { return h.world }
func (h *testHost) Paused() bool                         { return h.paused }
func (h *testHost) RealUntil(sim.GameTime) time.Duration { return 42 * time.Second }

func newGateway(t *testing.T) (*Gateway, *testHost, *store.MemInputLog, *worldgen.Manifest) {
	t.Helper()
	return newGatewayCfg(t, worldgen.PresetCompact(31))
}

// newGatewayCfg builds a gateway over a chosen config — season tests need a
// multi-division world (PresetCompact is a single division).
func newGatewayCfg(t *testing.T, cfg worldgen.WorldConfig) (*Gateway, *testHost, *store.MemInputLog, *worldgen.Manifest) {
	t.Helper()
	res, err := worldgen.Generate(cfg, worldgen.WithTokenReader(&tokens{}))
	if err != nil {
		t.Fatal(err)
	}
	host := &testHost{
		world: res.World,
		eng:   engine.New(res.World, res.Queue, &store.MemAuditLog{}),
	}
	inputs := &store.MemInputLog{}
	g := New(host, inputs, narrative.Default, res.Manifest.Managers)
	return g, host, inputs, res.Manifest
}

func firstManagerID(m *worldgen.Manifest) int64 { return m.Managers[0].ManagerID }

func dataOf(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	if env["ok"] != true {
		t.Fatalf("call failed: %+v", env)
	}
	return env["data"].(map[string]any)
}

func errCode(env map[string]any) string {
	e, _ := env["error"].(map[string]any)
	if e == nil {
		return ""
	}
	code, _ := e["code"].(string)
	return code
}

// TestGatewayReindexAfterSpawn locks the gateway half of the spawn-pointer-safety
// contract: a runtime SpawnManager appends to World.Managers (which
// can reallocate and dangle the gateway's cached *Manager pointers), so run() must
// rebuild its index — after which BOTH a newly spawned manager and a pre-existing
// one resolve, neither returning INVALID_TOKEN.
func TestGatewayReindexAfterSpawn(t *testing.T) {
	g, host, _, manifest := newGateway(t)
	mid := firstManagerID(manifest)

	var newID int64
	host.LockedWrite(func() {
		w := g.Host.World()
		m := worldgen.SpawnManager(w, rng.Stream(w.Config.Seed, "career/spawn/test"), 0, 1, false)
		newID = m.ID
	})

	// The spawned manager is now resolvable (cache rebuilt on the length change);
	// an unemployed manager reaches the club-scope guard, never INVALID_TOKEN.
	if code := errCode(g.getSituation(newID, "s1", emptyIn{})); code == "INVALID_TOKEN" {
		t.Fatal("spawned manager not resolvable via the gateway — index not rebuilt")
	}
	// The pre-existing manager still resolves against the live slice, not a stale
	// pre-realloc pointer.
	if code := errCode(g.getSituation(mid, "s2", emptyIn{})); code == "INVALID_TOKEN" {
		t.Fatal("pre-existing manager unresolvable after a spawn realloc")
	}
}

// TestRetiredManagerRejected locks FR-14e: a RETIRED manager's token is dead —
// every tool call fails with MANAGER_RETIRED (and, since Focus regen is lazy,
// run returning early freezes their balance so regen stops).
func TestRetiredManagerRejected(t *testing.T) {
	g, host, _, manifest := newGateway(t)
	mid := firstManagerID(manifest)
	host.LockedWrite(func() { g.managers[mid].Status = worldgen.ManagerRetired })
	if code := errCode(g.getSituation(mid, "s1", emptyIn{})); code != "MANAGER_RETIRED" {
		t.Fatalf("retired manager call = %q, want MANAGER_RETIRED", code)
	}
}

func TestFocusRegenTicks(t *testing.T) {
	m := &worldgen.Manager{FocusBalance: 10}
	syncFocus(m, 29) // under one tick
	if m.FocusBalance != 10 {
		t.Fatalf("balance = %d before first tick", m.FocusBalance)
	}
	syncFocus(m, 30) // exactly one tick (2 FP/hour = 1 FP / 30 min)
	if m.FocusBalance != 11 || m.FocusRegenMark != 30 {
		t.Fatalf("after 30min: balance %d mark %d", m.FocusBalance, m.FocusRegenMark)
	}
	syncFocus(m, 30+sim.MinutesPerDay) // a day: +48
	if m.FocusBalance != 59 {
		t.Fatalf("after a day: balance %d, want 59", m.FocusBalance)
	}
	syncFocus(m, 100*sim.MinutesPerDay) // way past cap
	if m.FocusBalance != focus.Cap {
		t.Fatalf("cap not enforced: %d", m.FocusBalance)
	}
}

// TestSyncFocusComposable: regen must be a pure function of game time —
// syncing along the way (e.g. during failed calls) can never change where
// the state ends up (replay determinism, NFR-2).
func TestSyncFocusComposable(t *testing.T) {
	a := &worldgen.Manager{FocusBalance: 90}
	b := &worldgen.Manager{FocusBalance: 90}

	// Path A syncs at many intermediate points (crossing the cap);
	// path B syncs once at the end.
	for _, at := range []sim.GameTime{17, 300, 301, 5000, 5001, 9999} {
		syncFocus(a, at)
	}
	syncFocus(b, 9999)
	if a.FocusBalance != b.FocusBalance || a.FocusRegenMark != b.FocusRegenMark {
		t.Fatalf("sync composition diverged:\nA %d@%d\nB %d@%d",
			a.FocusBalance, a.FocusRegenMark, b.FocusBalance, b.FocusRegenMark)
	}
	// The partial-tick remainder is preserved even across the cap.
	if a.FocusRegenMark%minutesPerFP != 0 || a.FocusRegenMark > 9999 {
		t.Fatalf("mark off the tick grid: %d", a.FocusRegenMark)
	}
}

func TestFreeToolsCostNothingButAreLogged(t *testing.T) {
	g, _, inputs, manifest := newGateway(t)
	mid := firstManagerID(manifest)

	for i, call := range []func() map[string]any{
		func() map[string]any { return g.getGuide(mid, "s1", emptyIn{}) },
		func() map[string]any { return g.getTime(mid, "s1", emptyIn{}) },
		func() map[string]any { return g.getSettings(mid, "s1", emptyIn{}) },
		func() map[string]any { return g.getFocus(mid, "s1", emptyIn{}) },
		func() map[string]any { return g.getMindset(mid, "s1", emptyIn{}) },
	} {
		env := call()
		dataOf(t, env)
		meta := env["meta"].(map[string]any)
		fp := meta["focus"].(map[string]any)
		if fp["balance"].(int) != focus.Cap {
			t.Fatalf("free call %d charged: %v", i, fp["balance"])
		}
	}
	if len(inputs.Entries) != 5 {
		t.Fatalf("input log has %d entries, want 5 (every accepted call logs)", len(inputs.Entries))
	}
	if inputs.Entries[0].Tool != "get_guide" || inputs.Entries[0].FocusCharge != 0 {
		t.Fatalf("first entry = %+v", inputs.Entries[0])
	}
}

func TestGuideIncludesPlayableVocabulary(t *testing.T) {
	g, _, _, manifest := newGateway(t)
	mid := firstManagerID(manifest)

	data := dataOf(t, g.getGuide(mid, "s1", emptyIn{}))
	vocab := data["vocabularies"].(map[string]any)
	goals := vocab["goals"].([]string)
	if !containsString(goals, "WIN_LEAGUE") || !containsString(goals, "FINISH_TOP_N") {
		t.Fatalf("guide goals missing core options: %v", goals)
	}
	verbs := vocab["directive_verbs"].([]string)
	if !containsString(verbs, "SIGN") || !containsString(verbs, "FORBID") {
		t.Fatalf("guide verbs missing core options: %v", verbs)
	}
	dials := vocab["tactical_dials"].(map[string][]string)
	if !containsString(dials["mentality"], "ATTACKING") || !containsString(dials["tempo"], "FAST") {
		t.Fatalf("guide tactical dials incomplete: %v", dials)
	}
	examples := data["examples"].(map[string]any)
	if examples["set_priorities"] == nil || examples["update_tactical_plan"] == nil {
		t.Fatalf("guide examples incomplete: %v", examples)
	}
}

func TestGuideIsPlayerFacing(t *testing.T) {
	g, _, _, manifest := newGateway(t)
	mid := firstManagerID(manifest)

	data := dataOf(t, g.getGuide(mid, "s1", emptyIn{}))
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(b))
	for _, forbidden := range []string{"hidden", "formula", "internal", "seeded randomness"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("guide leaks implementation-facing term %q: %s", forbidden, text)
		}
	}
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func TestShapingChargesAndLogs(t *testing.T) {
	g, _, inputs, manifest := newGateway(t)
	mid := firstManagerID(manifest)

	env := g.addDirective(mid, "s1", addDirectiveIn{
		Verb: "KEEP", Target: mindset.Target{Player: 10001}, Strength: "INSIST",
	})
	data := dataOf(t, env)
	if data["active_directives"].(int) != 1 {
		t.Fatalf("directives = %v", data["active_directives"])
	}
	meta := env["meta"].(map[string]any)
	bal := meta["focus"].(map[string]any)["balance"].(int)
	if bal != focus.Cap-10 { // INSIST costs 10
		t.Fatalf("balance after INSIST = %d, want %d", bal, focus.Cap-10)
	}
	if spent := meta["focus"].(map[string]any)["spent"].(int); spent != 10 {
		t.Fatalf("meta.focus.spent = %d, want this call's charge 10 (docs/11 §1.1)", spent)
	}
	if len(inputs.Entries) != 1 || inputs.Entries[0].FocusCharge != 10 {
		t.Fatalf("input log = %+v", inputs.Entries)
	}

	// Conflicting directive: rejected, costs nothing, not logged.
	env = g.addDirective(mid, "s1", addDirectiveIn{
		Verb: "SELL", Target: mindset.Target{Player: 10001}, Strength: "LEAN",
	})
	if errCode(env) != "CONFLICT" {
		t.Fatalf("want CONFLICT, got %+v", env)
	}
	if len(inputs.Entries) != 1 {
		t.Fatal("failed call must not enter the input log")
	}
	meta = env["meta"].(map[string]any)
	if meta["focus"].(map[string]any)["balance"].(int) != focus.Cap-10 {
		t.Fatal("failed call charged Focus")
	}
}

func TestInsufficientFocus(t *testing.T) {
	g, host, _, manifest := newGateway(t)
	mid := firstManagerID(manifest)
	var m *worldgen.Manager
	host.LockedWrite(func() { m = g.managers[mid]; m.FocusBalance = 3 })

	env := g.updateDisposition(mid, "s1", updateDispositionIn{
		Targets: map[string]int{"D1": 5},
	})
	if errCode(env) != "INSUFFICIENT_FOCUS" {
		t.Fatalf("want INSUFFICIENT_FOCUS, got %+v", env)
	}
	details := env["error"].(map[string]any)["details"].(map[string]any)
	if details["required"].(int) != 25 || details["balance"].(int) != 3 {
		t.Fatalf("details = %+v", details)
	}
	at := details["affordable_at"].(map[string]any)
	if at["game"] == nil || at["real_seconds"] == nil {
		t.Fatalf("affordable_at = %+v", at)
	}

	host.paused = true
	env = g.updateDisposition(mid, "s1", updateDispositionIn{Targets: map[string]int{"D1": 5}})
	at = env["error"].(map[string]any)["details"].(map[string]any)["affordable_at"].(map[string]any)
	if at["paused"] != true {
		t.Fatalf("paused affordable_at = %+v (docs/11 §1.3)", at)
	}
}

func TestDispositionInstantVsDrift(t *testing.T) {
	g, _, _, manifest := newGateway(t)
	mid := firstManagerID(manifest)
	m := g.managers[mid]
	m.Mindset.Disposition.Current[mindset.AxisRiskAppetite] = 0

	env := g.updateDisposition(mid, "s1", updateDispositionIn{
		Targets: map[string]int{"D1": 2, "D2": 9},
	})
	data := dataOf(t, env)
	axes := data["axes"].(map[string]any)
	d1 := axes["D1"].(map[string]any)
	if d1["current"].(int) != 2 || d1["target"] != nil {
		t.Fatalf("D1 should apply instantly: %+v", d1)
	}
	d2 := axes["D2"].(map[string]any)
	if d2["target"].(int) != 9 || d2["eta"] == nil {
		t.Fatalf("D2 should drift: %+v", d2)
	}

	// 4 axes in one call → CAP_EXCEEDED (docs/11 §1.2).
	env = g.updateDisposition(mid, "s1", updateDispositionIn{
		Targets: map[string]int{"D1": 1, "D2": 1, "D3": 1, "D4": 1},
	})
	if errCode(env) != "CAP_EXCEEDED" {
		t.Fatalf("want CAP_EXCEEDED, got %+v", env)
	}
}

// TestRejectedDispositionMutatesNothing: a rejected update_disposition —
// including cap and validation failures — must not move drift state,
// version, or anchors (docs/11 §1.2: failed calls mutate nothing).
func TestRejectedDispositionMutatesNothing(t *testing.T) {
	g, _, inputs, manifest := newGateway(t)
	mid := firstManagerID(manifest)
	m := g.managers[mid]
	m.Mindset.Disposition.Current[mindset.AxisRiskAppetite] = -8
	dataOf(t, g.updateDisposition(mid, "s1", updateDispositionIn{
		Targets: map[string]int{"D1": 8},
	}))
	m.DriftAnchor = 0
	m.DriftCreditMinutes = 123
	version := m.Mindset.Version
	logged := len(inputs.Entries)
	g.Host.Engine().ResumeAt(sim.GameTime(7 * sim.MinutesPerDay))

	for _, in := range []updateDispositionIn{
		{Targets: map[string]int{"D1": 1, "D2": 1, "D3": 1, "D4": 1}}, // cap
		{Targets: map[string]int{"D99": 1}},                           // unknown axis
		{Targets: map[string]int{"D1": 40}},                           // out of range
	} {
		env := g.updateDisposition(mid, "s1", in)
		if env["ok"] == true {
			t.Fatalf("call should fail: %+v", in)
		}
		if m.DriftAnchor != 0 || m.DriftCreditMinutes != 123 ||
			m.Mindset.Version != version ||
			m.Mindset.Disposition.Current[mindset.AxisRiskAppetite] != -8 {
			t.Fatalf("rejected call mutated drift state: %+v", in)
		}
	}
	if len(inputs.Entries) != logged {
		t.Fatal("rejected calls entered the input log")
	}
}

// TestRetargetPreservesAccruedDrift: calling update_disposition again must
// apply drift earned under the old targets before re-anchoring.
func TestRetargetPreservesAccruedDrift(t *testing.T) {
	g, _, _, manifest := newGateway(t)
	mid := firstManagerID(manifest)
	m := g.managers[mid]
	m.Mindset.Disposition.Current[mindset.AxisRiskAppetite] = -8
	dataOf(t, g.updateDisposition(mid, "s1", updateDispositionIn{
		Targets: map[string]int{"D1": 8},
	}))

	// A game-week passes with no decision roll landing in between.
	m.DriftAnchor = 0
	m.DriftCreditMinutes = 0
	weekLater := sim.GameTime(7 * sim.MinutesPerDay)
	g.Host.Engine().ResumeAt(weekLater)

	env := g.updateDisposition(mid, "s1", updateDispositionIn{
		Targets: map[string]int{"D2": 7},
	})
	dataOf(t, env)
	if cur := m.Mindset.Disposition.Current[mindset.AxisRiskAppetite]; cur != -6 {
		t.Fatalf("a week's accrued drift (2 pts) was lost on retarget: D1 = %d, want -6", cur)
	}
}

func TestForbidFencesTacticalPlan(t *testing.T) {
	g, _, _, manifest := newGateway(t)
	mid := firstManagerID(manifest)

	env := g.addDirective(mid, "s1", addDirectiveIn{
		Verb: "FORBID", Target: mindset.Target{Formation: "5-4-1"}, Strength: "ABSOLUTE",
	})
	dataOf(t, env)
	env = g.updateTacticalPlan(mid, "s1", updateTacticalPlanIn{Formation: "5-4-1"})
	if errCode(env) != "CONFLICT" {
		t.Fatalf("FORBID fence must reject the patch, got %+v", env)
	}
	// A different formation patches fine and keeps unset dials.
	env = g.updateTacticalPlan(mid, "s1", updateTacticalPlanIn{Formation: "4-4-2"})
	data := dataOf(t, env)
	plan := data["tactical_plan"].(mindset.TacticalPlan)
	if plan.Formation != "4-4-2" || plan.Mentality == "" {
		t.Fatalf("patch merged wrong: %+v", plan)
	}

	// Style fences: FORBID scope "pressing:HIGH" blocks that dial value
	// (docs/10 §4.2 dial:VALUE convention).
	dataOf(t, g.addDirective(mid, "s1", addDirectiveIn{
		Verb: "FORBID", Target: mindset.Target{Scope: "pressing:HIGH"}, Strength: "INSIST",
	}))
	if errCode(g.updateTacticalPlan(mid, "s1", updateTacticalPlanIn{Pressing: "HIGH"})) != "CONFLICT" {
		t.Fatal("style fence must reject the dial value")
	}
	dataOf(t, g.updateTacticalPlan(mid, "s1", updateTacticalPlanIn{Pressing: "LOW"}))
}

func TestSetPrioritiesAndRemoveDirective(t *testing.T) {
	g, _, _, manifest := newGateway(t)
	mid := firstManagerID(manifest)

	env := g.setPriorities(mid, "s1", setPrioritiesIn{Priorities: []priorityIn{
		{Rank: 1, Goal: "AVOID_RELEGATION"},
		{Rank: 2, Goal: "DEVELOP_YOUTH"},
	}})
	dataOf(t, env)
	if errCode(g.setPriorities(mid, "s1", setPrioritiesIn{Priorities: []priorityIn{
		{Rank: 1, Goal: "BECOME_IMMORTAL"},
	}})) != "VALIDATION" {
		t.Fatal("unknown goal must fail VALIDATION")
	}

	env = g.addDirective(mid, "s1", addDirectiveIn{
		Verb: "DEVELOP", Target: mindset.Target{Player: 10002}, Strength: "LEAN",
	})
	id := dataOf(t, env)["directive"].(mindset.Directive).ID
	if errCode(g.removeDirective(mid, "s1", removeDirectiveIn{ID: "dir_9999"})) != "NOT_FOUND" {
		t.Fatal("unknown directive id must be NOT_FOUND")
	}
	env = g.removeDirective(mid, "s1", removeDirectiveIn{ID: id})
	if dataOf(t, env)["active_directives"].(int) != 0 {
		t.Fatal("directive not removed")
	}
}

// TestDriftAppliesOnDecisionCadence: after update_disposition, the engine's
// decision rolls move Current toward Target at ~2 pts/game-week (FR-16b).
func TestDriftAppliesOnDecisionCadence(t *testing.T) {
	g, host, _, manifest := newGateway(t)
	mid := firstManagerID(manifest)
	m := g.managers[mid]
	m.Mindset.Disposition.Current[mindset.AxisRiskAppetite] = -8

	env := g.updateDisposition(mid, "s1", updateDispositionIn{Targets: map[string]int{"D1": 8}})
	dataOf(t, env)

	if _, err := host.eng.RunUntil(sim.GameTime(28 * sim.MinutesPerDay)); err != nil {
		t.Fatal(err)
	}
	cur := m.Mindset.Disposition.Current[mindset.AxisRiskAppetite]
	moved := cur - (-8)
	// 4 game-weeks at ~2/week ⇒ expect ~8 points, allow cadence jitter.
	if moved < 5 || moved > 11 {
		t.Fatalf("drift moved %d points in 4 weeks, want ≈8", moved)
	}
	if cur > 8 {
		t.Fatalf("drift overshot the target: %d", cur)
	}
}

// TestMCPOverHTTP is the end-to-end transport test: bearer auth in front of
// the streamable handler, TokenInfo propagating into tool handlers.
func TestMCPOverHTTP(t *testing.T) {
	g, _, _, manifest := newGateway(t)
	srv := g.MCPServer()
	handler := auth.RequireBearerToken(g.VerifyToken, nil)(
		mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	ts := httptest.NewServer(handler)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Wrong token → 401 at the HTTP layer (INVALID_TOKEN semantics).
	client := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "0"}, nil)
	if _, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: authedClient("mgr_bogus"),
	}, nil); err == nil {
		t.Fatal("bogus token must fail to connect")
	}

	// Real token: full round-trip.
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: authedClient(manifest.Managers[0].Token),
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()
	init := session.InitializeResult()
	if init == nil || init.ServerInfo == nil || init.ServerInfo.Version != "dev" {
		t.Fatalf("initialize server info = %#v", init)
	}
	if !strings.Contains(init.Instructions, "get_guide") {
		t.Fatalf("initialize instructions = %q", init.Instructions)
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var directiveTool, settingsTool *mcp.Tool
	for _, tool := range tools.Tools {
		switch tool.Name {
		case string(focus.AddDirective):
			directiveTool = tool
		case string(focus.GetSettings):
			settingsTool = tool
		}
	}
	if directiveTool == nil {
		t.Fatal("add_directive missing from tool list")
	}
	uiMeta, ok := directiveTool.Meta["ui"].(map[string]any)
	if !ok || uiMeta["resourceUri"] != widgetURI {
		t.Fatalf("tool UI metadata = %#v", directiveTool.Meta)
	}
	if settingsTool == nil {
		t.Fatal("get_settings missing from tool list")
	}
	uiMeta, ok = settingsTool.Meta["ui"].(map[string]any)
	if !ok || uiMeta["resourceUri"] != widgetURI {
		t.Fatalf("settings UI metadata = %#v", settingsTool.Meta)
	}
	resources, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	if len(resources.Resources) == 0 || resources.Resources[0].URI != widgetURI || resources.Resources[0].MIMEType != widgetMIME {
		t.Fatalf("resources = %#v", resources.Resources)
	}
	uiRes, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: widgetURI})
	if err != nil {
		t.Fatalf("read widget resource: %v", err)
	}
	if len(uiRes.Contents) != 1 || uiRes.Contents[0].MIMEType != widgetMIME ||
		!strings.Contains(uiRes.Contents[0].Text, "Agentic FC") {
		t.Fatalf("widget resource contents = %#v", uiRes.Contents)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_time"})
	if err != nil {
		t.Fatalf("get_time: %v", err)
	}
	env, ok := res.StructuredContent.(map[string]any)
	if !ok || env["ok"] != true {
		t.Fatalf("envelope = %#v", res.StructuredContent)
	}
	meta := env["meta"].(map[string]any)
	if gt, _ := meta["game_time"].(string); !strings.HasPrefix(gt, "1925-07-01") {
		t.Fatalf("game_time = %v", meta["game_time"])
	}
	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "get_settings"})
	if err != nil {
		t.Fatalf("get_settings: %v", err)
	}
	settings := res.StructuredContent.(map[string]any)
	data := settings["data"].(map[string]any)
	world := data["world"].(map[string]any)
	if world["seed"] != "redacted" {
		t.Fatalf("settings seed marker = %#v, want redacted", world["seed"])
	}
	if world["run_profile"] == "" {
		t.Fatalf("settings missing run_profile: %#v", world)
	}
	pacing := data["pacing"].(map[string]any)
	if pacing["base_game_speed"].(float64) == 0 || pacing["tempo_rates"] == nil {
		t.Fatalf("settings pacing incomplete: %#v", pacing)
	}
	if pacing["run_profile"] != world["run_profile"] {
		t.Fatalf("settings profile mismatch: world=%v pacing=%v", world["run_profile"], pacing["run_profile"])
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "add_directive",
		Arguments: map[string]any{
			"verb": "KEEP", "target": map[string]any{"player": 10001}, "strength": "LEAN",
		},
	})
	if err != nil {
		t.Fatalf("add_directive: %v", err)
	}
	env = res.StructuredContent.(map[string]any)
	if env["ok"] != true {
		t.Fatalf("add_directive envelope = %#v", env)
	}
	balance := env["meta"].(map[string]any)["focus"].(map[string]any)["balance"].(float64)
	if balance != float64(focus.Cap-6) { // LEAN costs 6
		t.Fatalf("balance over the wire = %v", balance)
	}
	widget, ok := res.Meta[widgetMetaKey].(map[string]any)
	if !ok || widget["mimeType"] != widgetMIME || !strings.Contains(fmt.Sprint(widget["html"]), "Added a directive") {
		t.Fatalf("default apps mode widget meta = %#v", res.Meta)
	}
	for _, c := range res.Content {
		if _, ok := c.(*mcp.EmbeddedResource); ok {
			t.Fatalf("default widget mode must not expose HTML EmbeddedResource content: %#v", res.Content)
		}
	}
}

type authRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (a authRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+a.token)
	return a.base.RoundTrip(r)
}

func authedClient(token string) *http.Client {
	return &http.Client{Transport: authRoundTripper{token: token, base: http.DefaultTransport}}
}
