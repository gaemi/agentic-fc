// Package engine implements the Simulation Core loop from
// docs/03-simulation-engine.md: the single-writer roll-and-reschedule
// executor that drains the event queue in total order, mutates world state,
// logs every roll to the audit trail (FR-29), and re-schedules each entity's
// next roll. Pacing (Adaptive Tempo) lives in tempo.go and never changes
// outcomes — only how fast the queue drains in real time (docs/03 §4).
package engine

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/gaemi/agentic-fc/internal/rng"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Engine is the single-writer Simulation Core (docs/05 A1). Not safe for
// concurrent use by design: all mutations flow through Step.
type Engine struct {
	world *worldgen.World
	queue *sim.Queue
	audit store.AuditLog
	now   sim.GameTime

	players   map[int64]*worldgen.Player
	clubs     map[int64]*worldgen.Club
	managers  map[int64]*worldgen.Manager
	fixtures  map[int64]*worldgen.Fixture
	kickoffs  []sim.GameTime // sorted unique kickoff times (match windows)
	sink      Sink           // optional feed observer (never affects outcomes)
	alertSink AlertSink
}

// New wires an engine over a generated world and its primed queue.
func New(w *worldgen.World, q *sim.Queue, audit store.AuditLog) *Engine {
	e := &Engine{
		world:    w,
		queue:    q,
		audit:    audit,
		players:  make(map[int64]*worldgen.Player, len(w.Players)),
		clubs:    make(map[int64]*worldgen.Club, len(w.Clubs)),
		managers: make(map[int64]*worldgen.Manager, len(w.Managers)),
	}
	for i := range w.Clubs {
		e.clubs[w.Clubs[i].ID] = &w.Clubs[i]
	}
	e.rebuildPlayerIndex()
	e.rebuildManagerIndex()
	e.buildFixtureIndex()
	return e
}

// rebuildPlayerIndex rebuilds the player id→pointer map from World.Players. New()
// builds it once; a runtime player spawn (a spring youth intake) calls
// it again — appending to World.Players may reallocate the slice and invalidate
// every held *Player, exactly as a manager spawn does for the manager index.
func (e *Engine) rebuildPlayerIndex() {
	e.players = make(map[int64]*worldgen.Player, len(e.world.Players))
	for i := range e.world.Players {
		e.players[e.world.Players[i].ID] = &e.world.Players[i]
	}
}

// rebuildManagerIndex rebuilds the manager id→pointer map from World.Managers.
// New() builds it once; a runtime manager spawn (a caretaker install or newgen
// backfill) calls it again — appending to World.Managers may
// reallocate the slice and invalidate every held *Manager, so the map must be
// rebuilt, exactly as the season rollover rebuilds the fixture index.
func (e *Engine) rebuildManagerIndex() {
	e.managers = make(map[int64]*worldgen.Manager, len(e.world.Managers))
	for i := range e.world.Managers {
		e.managers[e.world.Managers[i].ID] = &e.world.Managers[i]
	}
}

// buildFixtureIndex rebuilds the fixture map and the sorted kickoff list from
// World.Fixtures. New() calls it once; the season rollover calls it again after
// regenerating the schedule, so the live engine ends up in exactly the state
// New(post-rollover-world) would build — the resume-safety contract (NFR-2).
func (e *Engine) buildFixtureIndex() {
	e.fixtures = make(map[int64]*worldgen.Fixture, len(e.world.Fixtures))
	e.kickoffs = e.kickoffs[:0]
	last := sim.GameTime(-1)
	for i := range e.world.Fixtures {
		e.fixtures[e.world.Fixtures[i].ID] = &e.world.Fixtures[i]
		if k := e.world.Fixtures[i].Kickoff; k != last {
			e.kickoffs = append(e.kickoffs, k)
			last = k
		}
	}
	sortGameTimes(e.kickoffs)
}

// SetSink attaches the feed observer (nil detaches). Call before running.
func (e *Engine) SetSink(s Sink) { e.sink = s }

// ResumeAt restores the clock from a snapshot (FR-28). Roll streams are
// stateless (kind/id/payload@gametime labels), so a resumed run rolls the
// exact dice the uninterrupted run would have.
func (e *Engine) ResumeAt(t sim.GameTime) {
	if t > e.now {
		e.now = t
	}
}

// Now is the game time of the last drained event (0 before any).
func (e *Engine) Now() sim.GameTime { return e.now }

// World exposes the engine's authoritative state (read-only by convention;
// writes outside Step break the single-writer contract).
func (e *Engine) World() *worldgen.World { return e.world }

// Queue exposes the event queue for the pacer/runner.
func (e *Engine) Queue() *sim.Queue { return e.queue }

// Step pops and handles the next event. Returns the handled event, or nil
// when the queue is empty.
func (e *Engine) Step() (*sim.Event, error) {
	ev := e.queue.Pop()
	if ev == nil {
		return nil, nil
	}
	if ev.Due > e.now {
		e.now = ev.Due
	}
	if err := e.handle(ev); err != nil {
		return ev, err
	}
	return ev, nil
}

// RunUntil drains every event due at or before t and advances the clock to
// t. Returns the number of events handled. Outcomes are independent of how
// a run is chunked — RunUntil(30d) equals thirty RunUntil(+1d) calls.
func (e *Engine) RunUntil(t sim.GameTime) (int, error) {
	n := 0
	for {
		next := e.queue.Peek()
		if next == nil || next.Due > t {
			break
		}
		if _, err := e.Step(); err != nil {
			return n, err
		}
		n++
	}
	if t > e.now {
		e.now = t
	}
	return n, nil
}

func (e *Engine) handle(ev *sim.Event) error {
	payload, ok := ev.Payload.(string)
	if !ok {
		return fmt.Errorf("event %+v has non-string payload", ev)
	}
	if strings.HasPrefix(payload, scoutPayloadPrefix) {
		return e.handleScoutReport(ev, payload)
	}
	if strings.HasPrefix(payload, focusAlertPayloadPrefix) {
		return e.handleFocusAlert(ev, payload)
	}
	if strings.HasPrefix(payload, calendarAlertPayloadPrefix) {
		return e.handleCalendarAlert(ev, payload)
	}
	switch payload {
	case worldgen.PayloadPlayerDrift:
		return e.handlePlayerDrift(ev)
	case worldgen.PayloadFinanceTick:
		return e.handleFinanceTick(ev)
	case worldgen.PayloadDecisionRoll:
		return e.handleDecisionRoll(ev)
	case worldgen.PayloadKickoff:
		return e.startMatch(ev)
	case worldgen.PayloadMatchMoment:
		return e.handleMatchMoment(ev)
	case worldgen.PayloadSeasonEnd:
		return e.handleSeasonEnd(ev)
	case worldgen.PayloadYouthIntake:
		return e.handleYouthIntake(ev)
	case worldgen.PayloadWindowOpen, worldgen.PayloadWindowClose:
		// The window edge is announced on the feed; the transfer gate itself is
		// derived from game time (worldgen.TransferWindowOpenAt), so no state to
		// toggle here.
		e.emitCalendar(ev.Due, payload)
		if payload == worldgen.PayloadWindowOpen {
			e.issueCalendarAlerts(ev.Due, "WINDOW_OPEN")
		} else {
			e.issueCalendarAlerts(ev.Due, "WINDOW_CLOSE")
		}
		if key, params := calendarKeyParams(ev.Due, payload); key != "" {
			e.addNews(worldgen.NewsItem{
				GameTime: ev.Due, Category: "transfer", Key: key, Params: params,
			})
		}
		return e.log(ev, "world", nil, "noted", 0, 0)
	default:
		return fmt.Errorf("unknown payload %q", payload)
	}
}

// rollStream derives the roll's RNG. The label embeds entity, payload, and
// due time, so streams are stateless: a resumed or replayed run derives the
// exact same dice for the exact same roll.
func (e *Engine) rollStream(ev *sim.Event) *rand.Rand {
	label := fmt.Sprintf("%s/%d/%s@%d",
		kindLabel(ev.Kind), ev.EntityID, ev.Payload, int64(ev.Due))
	return rng.Stream(e.world.Config.Seed, label)
}

func kindLabel(k sim.EntityKind) string {
	switch k {
	case sim.KindWorld:
		return "world"
	case sim.KindMatch:
		return "match"
	case sim.KindClub:
		return "club"
	case sim.KindManager:
		return "manager"
	default:
		return "player"
	}
}

// log appends one roll to the audit trail (FR-29): every factor named, the
// outcome, and the next-roll schedule.
func (e *Engine) log(ev *sim.Event, category string, factors map[string]any,
	outcome string, next sim.GameTime, mindsetVersion int) error {

	var fb []byte
	if factors != nil {
		b, err := json.Marshal(factors) // map keys sort: canonical JSON
		if err != nil {
			return err
		}
		fb = b
	}
	return e.audit.Append(store.RollEntry{
		GameTime:       ev.Due,
		EntityKind:     ev.Kind,
		EntityID:       ev.EntityID,
		Category:       category,
		Factors:        fb,
		Outcome:        outcome,
		NextRoll:       next,
		MindsetVersion: mindsetVersion,
	})
}

// reschedule enqueues the entity's next roll of the same kind.
func (e *Engine) reschedule(ev *sim.Event, due sim.GameTime) {
	e.queue.Schedule(&sim.Event{
		Due:      due,
		Priority: ev.Priority,
		Kind:     ev.Kind,
		EntityID: ev.EntityID,
		Payload:  ev.Payload,
	})
}

func sortGameTimes(s []sim.GameTime) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
