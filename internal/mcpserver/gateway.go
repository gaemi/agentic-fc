// Package mcpserver is the Agent-facing MCP gateway (docs/11): the
// canonical tool surface, the Focus economy, and the input log that the
// replay contract depends on (NFR-2). The surface is English-only —
// envelopes keep message keys + params, so the contract stays
// language-neutral.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gaemi/agentic-fc/internal/engine"
	"github.com/gaemi/agentic-fc/internal/focus"
	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/narrative"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Host is the daemon-side world holder the gateway operates on. Every MCP
// call runs under the write lock: charged calls mutate Focus state, and the
// input log's (game_time, ingress_seq) ordering must be serialized (NFR-2).
// The Simulation Core stays the single writer of world systems; MCP writes
// touch only Mindset/Focus intent state (docs/05 A1, FR-15).
type Host interface {
	LockedWrite(fn func())
	Engine() *engine.Engine
	World() *worldgen.World
	// Paused reports the admin maintenance pause (or a never-started
	// world) — Focus regen is game-time-based so it halts automatically;
	// this drives meta.tempo and affordable_at (docs/11 §1.3).
	Paused() bool
	// RealUntil estimates wall-clock time until game time t (for
	// INSUFFICIENT_FOCUS affordable_at). Meaningless while paused.
	RealUntil(t sim.GameTime) time.Duration
}

// Gateway wires the MCP tool surface to the world.
type Gateway struct {
	Host     Host
	Inputs   store.InputLog
	Catalogs narrative.Catalogs

	// Locale is the spectator's display language for MCP UI widgets
	// empty follows the system language (FR-35c). WidgetMode selects
	// official MCP Apps vs compatibility result attachments. Both are human-facing
	// rendering only — the AI's structured JSON is never affected.
	Locale     narrative.Locale
	WidgetMode widgetMode

	// OnAccepted, when set, fires after every accepted (logged) call —
	// the daemon uses it to schedule a prompt snapshot, shrinking the
	// window in which a logged-but-unsnapshotted input could be lost to
	// a crash.
	OnAccepted func()

	// credMu guards the mutable credential set (tokens + creds). Runtime spawns
	// (caretakers, newgen — FR-34) mint tokens mid-run: VerifyToken reads off the
	// HTTP path while the daemon's reconciler registers new ones, so the two stores
	// need a lock. It is the single owner of the cred set — Credentials() delegates
	// here so there is exactly one copy (no drift between auth and the admin listing).
	credMu   sync.RWMutex
	tokens   map[string]int64             // Manager Token → manager id
	creds    []worldgen.ManagerCredential // the credential set (guarded by credMu)
	managers map[int64]*worldgen.Manager
	// cursors is per-session get_news state (docs/11 §4). Mutated only
	// under the write lock (every call runs through run→LockedWrite), so
	// no extra sync is needed. Transient: sessions don't survive restart,
	// so it isn't part of the snapshot.
	cursors map[string]int64
}

// New builds the gateway; creds provide the token → Manager binding
// (docs/04 §0 — binding follows the Manager, not the club).
func New(host Host, inputs store.InputLog, catalogs narrative.Catalogs,
	creds []worldgen.ManagerCredential) *Gateway {

	// Prune orphan credentials — a manifest cred whose manager_id has no manager in
	// the loaded world. The runtime reconciler mints + persists a token and only THEN
	// does the world snapshot catch up, so a crash in that window can leave the
	// durable manifest one caretaker ahead of the resumed snapshot; without this the
	// stale token would authenticate (bearer) yet resolve to no manager, and would
	// surface in the admin listing. The world is the source of truth and the manifest
	// conforms to it on load. **Load-bearing:** a RETIRED manager stays IN
	// world.Managers (entries are never removed), so its dead-but-present token
	// (FR-14e) is kept — only ids with no manager AT ALL are dropped, and those are
	// always orphans (fresh generation writes manifest ⊆ world; a normal resume
	// snapshots every manager). A manager the deterministic re-run re-creates is
	// re-minted by the reconciler.
	live := make(map[int64]bool, len(host.World().Managers))
	for i := range host.World().Managers {
		live[host.World().Managers[i].ID] = true
	}
	kept := make([]worldgen.ManagerCredential, 0, len(creds))
	for _, c := range creds {
		if live[c.ManagerID] {
			kept = append(kept, c)
		}
	}

	g := &Gateway{
		Host:     host,
		Inputs:   inputs,
		Catalogs: catalogs,
		tokens:   make(map[string]int64, len(kept)),
		creds:    kept,
		managers: map[int64]*worldgen.Manager{},
		cursors:  map[string]int64{},
	}
	for _, c := range kept {
		g.tokens[c.Token] = c.ManagerID
	}
	g.syncManagerIndex()
	return g
}

// TokenlessManagers returns partial credentials (Token unset) for every ACTIVE
// manager in the world that has no credential yet — the runtime spawns (caretakers,
// newgen backfills) that FR-34 requires be tokened at creation. RETIRED managers are
// skipped so a dead token is never minted (a displaced caretaker that retires before
// the next reconcile simply never receives one). The caller mints tokens into the
// returned creds, persists them to the Manifest, then calls AddCredentials — the
// world is read-only here (minting never touches the hash). Caller holds the world
// read lock; this takes credMu.RLock to snapshot the known-id set.
func (g *Gateway) TokenlessManagers(w *worldgen.World) []worldgen.ManagerCredential {
	g.credMu.RLock()
	known := make(map[int64]bool, len(g.creds))
	for _, c := range g.creds {
		known[c.ManagerID] = true
	}
	g.credMu.RUnlock()

	clubNames := make(map[int64]string, len(w.Clubs))
	for i := range w.Clubs {
		clubNames[w.Clubs[i].ID] = w.Clubs[i].Name
	}
	var pending []worldgen.ManagerCredential
	for i := range w.Managers {
		m := &w.Managers[i]
		if m.Status == worldgen.ManagerRetired || known[m.ID] {
			continue
		}
		pending = append(pending, worldgen.ManagerCredential{
			ManagerID:   m.ID,
			ManagerName: m.Name,
			ClubID:      m.ClubID,
			ClubName:    clubNames[m.ClubID],
			Archetype:   m.Archetype,
			Reputation:  m.Reputation,
			// Token is minted by the caller.
		})
	}
	return pending
}

// AddCredentials registers freshly minted runtime tokens so their managers can
// authenticate. The caller MUST persist them to the Manifest first (persist before
// expose — a token is never usable before it is durable).
func (g *Gateway) AddCredentials(creds []worldgen.ManagerCredential) {
	g.credMu.Lock()
	defer g.credMu.Unlock()
	for _, c := range creds {
		g.creds = append(g.creds, c)
		g.tokens[c.Token] = c.ManagerID
	}
}

// Credentials returns a snapshot copy of the current credential set (the Console's
// admin listing, FR-33). It is the single source of truth for the cred set.
func (g *Gateway) Credentials() []worldgen.ManagerCredential {
	g.credMu.RLock()
	defer g.credMu.RUnlock()
	return append([]worldgen.ManagerCredential(nil), g.creds...)
}

// VerifyToken is the auth.TokenVerifier for RequireBearerToken: it maps a
// Manager Token to its manager id (as TokenInfo.UserID). Unknown tokens
// reject at the HTTP layer — the INVALID_TOKEN semantics of docs/11 §1.2
// surface as 401s on this transport (sessions on a regenerated token fail
// their next request).
func (g *Gateway) VerifyToken(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
	g.credMu.RLock()
	id, ok := g.tokens[token]
	g.credMu.RUnlock()
	if !ok {
		return nil, auth.ErrInvalidToken
	}
	return &auth.TokenInfo{
		UserID:     strconv.FormatInt(id, 10),
		Expiration: time.Now().Add(24 * time.Hour), // sessions re-verify per request
	}, nil
}

// ---- Error model (docs/11 §1.2) ----

type apiError struct {
	Code       ErrorCode
	MessageKey string
	Params     map[string]any
	Details    map[string]any
}

func errFor(code ErrorCode, key string, params, details map[string]any) *apiError {
	return &apiError{Code: code, MessageKey: key, Params: params, Details: details}
}

// mapMindsetErr translates mindset package sentinels to wire codes.
func mapMindsetErr(err error) *apiError {
	var conflict *mindset.ConflictError
	switch {
	case errors.As(err, &conflict):
		return errFor(ErrConflict, "err.conflict",
			map[string]any{"with": conflict.With},
			map[string]any{"conflicts_with": conflict.With})
	case errors.Is(err, mindset.ErrDirectiveCap), errors.Is(err, mindset.ErrPriorityCap),
		errors.Is(err, mindset.ErrTooManyAxes):
		return errFor(ErrCapExceeded, "err.cap_exceeded", nil, nil)
	case errors.Is(err, mindset.ErrInvalidTarget):
		return errFor(ErrInvalidTarget, "err.invalid_target", nil, nil)
	default:
		return errFor(ErrValidation, "err.validation",
			map[string]any{"detail": err.Error()},
			map[string]any{"detail": err.Error()})
	}
}

// ---- Envelope (docs/11 §1.1) ----

// callCtx carries one resolved call through charge/apply/log.
type callCtx struct {
	manager *worldgen.Manager
	now     sim.GameTime
	session string
	tool    focus.Tool
	params  any
}

// run executes one MCP call end-to-end under the write lock: resolve
// manager → sync regen → afford check → apply → charge → input log →
// envelope. Failed calls cost nothing and are not logged (only accepted
// inputs enter the replay log — docs/03 §5).
// syncManagerIndex rebuilds the manager id→pointer cache when World.Managers has
// grown — a runtime spawn (caretaker install, newgen backfill) appends to that
// slice, which can reallocate the backing array and dangle every cached *Manager.
// Managers are only ever added (a retirement keeps the entry, flipping Status), so
// a length change is a reliable "something spawned" signal; when the length is
// unchanged the cached pointers still alias the live slice. Runs under the write
// lock at the top of run(), so every call resolves against current World state.
func (g *Gateway) syncManagerIndex() {
	w := g.Host.World()
	if len(g.managers) == len(w.Managers) {
		return
	}
	g.managers = make(map[int64]*worldgen.Manager, len(w.Managers))
	for i := range w.Managers {
		g.managers[w.Managers[i].ID] = &w.Managers[i]
	}
}

func (g *Gateway) run(managerID int64, sessionID string, tool focus.Tool,
	params any, cost func(*callCtx) int,
	apply func(*callCtx) (any, *apiError)) map[string]any {

	var out map[string]any
	g.Host.LockedWrite(func() {
		// A runtime manager spawn (caretaker, newgen) can reallocate World.Managers
		// and dangle our cached pointers; rebuild the cache when it has grown before
		// resolving anyone, so every call reads current World state.
		g.syncManagerIndex()
		m, ok := g.managers[managerID]
		if !ok {
			out = g.errEnvelope(nil, errFor(ErrInvalidToken, "err.invalid_token", nil, nil))
			return
		}
		// A retired manager's token is dead (FR-14e): every call fails, and because
		// Focus regen is lazy (synced only inside run), returning here also freezes
		// their balance — regen stops.
		if m.Status == worldgen.ManagerRetired {
			out = g.errEnvelope(nil, errFor(ErrManagerRetired, "err.manager_retired", nil, nil))
			return
		}
		cc := &callCtx{
			manager: m,
			now:     g.Host.Engine().Now(),
			session: sessionID,
			tool:    tool,
			params:  params,
		}
		syncFocus(m, cc.now)

		fp := cost(cc)
		if fp > m.FocusBalance {
			out = g.errEnvelope(cc, g.insufficientFocus(cc, fp))
			return
		}
		data, aerr := apply(cc)
		if aerr != nil {
			out = g.errEnvelope(cc, aerr)
			return
		}
		chargeFocus(m, tool, fp, cc.now)
		// Avatar-liveness stamp (FR-14e, manager careers): mark this token active at the
		// current GAME time so the engine's season-boundary retirement pass exempts a
		// live Avatar for the grace window (2 game-years). Stamped ONLY on the accepted
		// path — the same path logInput records — so a resumed run replays the identical
		// stamp; a rejected call isn't logged, so stamping it would not be reproducible
		// (NFR-2). Game time (not wall-clock) keeps it deterministic, and the field is
		// manager-internal — it never crosses the wire.
		m.LastActiveGameTime = cc.now
		// Crash consistency: the mutation and this append happen under the
		// same write lock that SaveSnapshot's read lock excludes, so no
		// snapshot can ever contain the mutation without the log entry.
		// If the append fails we panic while the state is memory-only —
		// the last snapshot + log stay consistent. The snapshot's
		// last_ingress_seq watermark covers the reverse case (logged,
		// then crash before any snapshot).
		g.logInput(cc, fp)
		out = map[string]any{"ok": true, "data": data, "meta": g.meta(cc, fp)}
	})
	if out["ok"] == true && g.OnAccepted != nil {
		g.OnAccepted()
	}
	return out
}

func (g *Gateway) errEnvelope(cc *callCtx, e *apiError) map[string]any {
	env := map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":        string(e.Code),
			"message_key": e.MessageKey,
			"message":     g.Catalogs.Render(narrative.LocaleEN, e.MessageKey, e.Params),
			"details":     e.Details,
		},
	}
	if cc != nil {
		env["meta"] = g.meta(cc, 0) // failed calls cost nothing
	}
	return env
}

func (g *Gateway) insufficientFocus(cc *callCtx, required int) *apiError {
	details := map[string]any{
		"required": required,
		"balance":  cc.manager.FocusBalance,
	}
	if g.Host.Paused() {
		details["affordable_at"] = map[string]any{"paused": true}
	} else {
		at := affordableAt(cc.manager, required, cc.now)
		details["affordable_at"] = map[string]any{
			"game":         gameTimeISO(at),
			"real_seconds": int(g.Host.RealUntil(at).Seconds()),
		}
	}
	return errFor(ErrInsufficientFocus, "err.focus.insufficient",
		map[string]any{"required": required, "balance": cc.manager.FocusBalance},
		details)
}

// meta builds the free rider block; spent is THIS call's charge
// (docs/11 §1.1 — zero for free tools and failed calls).
func (g *Gateway) meta(cc *callCtx, spent int) map[string]any {
	tempo := sim.TempoPaused
	if !g.Host.Paused() {
		tempo = g.Host.Engine().TempoAt(cc.now)
	}
	return map[string]any{
		"game_time": gameTimeISO(cc.now),
		"tempo":     tempo.String(),
		"tool":      string(cc.tool),
		"focus": map[string]any{
			"spent":               spent,
			"balance":             cc.manager.FocusBalance,
			"cap":                 focus.Cap,
			"regen_per_game_hour": focus.RegenPerGameHour,
		},
		"mindset_version": cc.manager.Mindset.Version,
	}
}

func (g *Gateway) logInput(cc *callCtx, fp int) {
	raw, err := json.Marshal(cc.params)
	if err != nil {
		raw = []byte("null")
	}
	if _, err := g.Inputs.Append(store.InputEntry{
		GameTime:    cc.now,
		SessionID:   cc.session,
		ManagerID:   cc.manager.ID,
		Tool:        string(cc.tool),
		Params:      raw,
		Result:      "ok",
		FocusCharge: fp,
	}); err != nil {
		// The input log IS the replay contract; a failed append must be
		// loud. The call already applied — surfacing the storage fault is
		// the daemon's concern (it fsyncs; failure here means the disk is
		// gone).
		panic(fmt.Sprintf("input log append failed: %v", err))
	}
}

// ---- Focus state (docs/11 §2; integer ticks — NFR-2) ----

// minutesPerFP derives the regen tick from the registered constant:
// 2 FP/game-hour ⇒ exactly one FP per 30 game-minutes.
const minutesPerFP = sim.MinutesPerHour / focus.RegenPerGameHour

// syncFocus is COMPOSABLE: the mark only ever advances by whole ticks, so
// sync(t₁) then sync(t₂) leaves exactly the state of sync(t₂) alone. That
// makes regen a pure function of game time — a failed call that synced
// along the way cannot alter the trajectory (replay determinism, NFR-2).
// At cap the balance clamps per batch (excess regen is lost) but the mark
// still tracks tick boundaries, preserving the partial-tick remainder.
func syncFocus(m *worldgen.Manager, now sim.GameTime) {
	if now <= m.FocusRegenMark {
		return
	}
	ticks := int64(now-m.FocusRegenMark) / minutesPerFP
	if ticks <= 0 {
		return
	}
	m.FocusRegenMark += sim.GameTime(ticks * minutesPerFP)
	m.FocusBalance += int(ticks)
	if m.FocusBalance > focus.Cap {
		m.FocusBalance = focus.Cap
	}
}

func chargeFocus(m *worldgen.Manager, tool focus.Tool, fp int, now sim.GameTime) {
	if fp == 0 {
		return
	}
	m.FocusBalance -= fp
	m.FocusSpends = append(m.FocusSpends, worldgen.FocusSpend{
		Tool: string(tool), Cost: fp, GameTime: now,
	})
	if len(m.FocusSpends) > 20 {
		m.FocusSpends = m.FocusSpends[len(m.FocusSpends)-20:]
	}
}

// affordableAt returns the game time at which the balance reaches required.
func affordableAt(m *worldgen.Manager, required int, now sim.GameTime) sim.GameTime {
	deficit := required - m.FocusBalance
	if deficit <= 0 {
		return now
	}
	// Next tick lands at FocusRegenMark + minutesPerFP, then every tick.
	return m.FocusRegenMark + sim.GameTime(int64(deficit)*minutesPerFP)
}

// ---- Game-time display (docs/11 §1.4: ISO-like game timestamps) ----

// displayBaseYear anchors season 1 at July 1925 — fictional but familiar
// (the docs examples read 1926). Content, not a tunable.
const displayBaseYear = 1925

func gameTimeISO(t sim.GameTime) string {
	d := worldgen.DateOf(t)
	year := displayBaseYear + d.Season - 1
	if d.Month < 7 { // Jan–Jun belong to the season's second calendar year
		year++
	}
	return fmt.Sprintf("%04d-%02d-%02dT%02d:%02d", year, d.Month, d.Day, d.Hour, d.Minute)
}

// managerIDFromCtx resolves the authenticated manager (RequireBearerToken
// stores TokenInfo in the request context; UserID is the manager id).
func managerIDFromCtx(ctx context.Context) (int64, *apiError) {
	info := auth.TokenInfoFromContext(ctx)
	if info == nil {
		return 0, errFor(ErrAuthRequired, "err.auth_required", nil, nil)
	}
	id, err := strconv.ParseInt(info.UserID, 10, 64)
	if err != nil {
		return 0, errFor(ErrInvalidToken, "err.invalid_token", nil, nil)
	}
	return id, nil
}

const mcpInstructions = "Agentic FC is an autonomous football-management simulation. Start every unfamiliar session by calling get_guide, then observe with get_time, get_focus, get_situation, and get_mindset before spending Focus. Do not invent enum values; use the goals, directive verbs, strengths, tactical dials, and target-shape examples returned by get_guide."

// MCPServer builds the SDK server with the v1 tool surface registered.
func (g *Gateway) MCPServer() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "agentic-fc",
		Version: "dev",
	}, &mcp.ServerOptions{Instructions: mcpInstructions})
	g.registerUIResources(s)
	g.registerTools(s)
	return s
}

// handle adapts a gateway tool method to an SDK handler: resolve auth,
// delegate, and return the envelope as structured output.
func handle[In any](g *Gateway, fn func(managerID int64, sessionID string, in In) map[string]any) mcp.ToolHandlerFor[In, map[string]any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, map[string]any, error) {
		id, aerr := managerIDFromCtx(ctx)
		if aerr != nil {
			return nil, g.errEnvelope(nil, aerr), nil
		}
		session := ""
		if req != nil && req.Session != nil {
			session = req.Session.ID()
		}
		return nil, fn(id, session, in), nil
	}
}
