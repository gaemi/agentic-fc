package worldgen

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	mrand "math/rand/v2"

	"github.com/gaemi/agentic-fc/internal/rng"
	"github.com/gaemi/agentic-fc/internal/sim"
)

// Stage RNG streams: each stage consumes its own stream (docs/03 §5) so a
// change in one stage never perturbs another. Labels are part of the
// determinism contract — renaming one changes every world.
const (
	streamSkeleton = "gen/skeleton"
	streamClubs    = "gen/clubs"
	streamManagers = "gen/managers"
	streamPlayers  = "gen/players"
	streamHistory  = "gen/history"
	streamSchedule = "gen/schedule"
	streamEconomy  = "gen/economy"
	streamQueue    = "gen/queue"
)

// stageStreams exists for the drift test guarding label uniqueness.
var stageStreams = []string{
	streamSkeleton, streamClubs, streamManagers, streamPlayers,
	streamHistory, streamSchedule, streamEconomy, streamQueue,
}

type options struct {
	tokenReader io.Reader
}

type Option func(*options)

// WithTokenReader overrides the Manager Token entropy source (tests). The
// default is crypto/rand: tokens are credentials, never derived from the
// world seed, and never part of the world hash.
func WithTokenReader(r io.Reader) Option {
	return func(o *options) { o.tokenReader = r }
}

// Generate runs the full pipeline (docs/09 §4): validate & derive (stage 0),
// then the seeded stages 1–9. Same config + same seed ⇒ identical World and
// identical Hash (NFR-2); the Manifest's tokens are the only non-determined
// output.
func Generate(cfg WorldConfig, opts ...Option) (*Result, error) {
	o := options{tokenReader: rand.Reader}
	for _, opt := range opts {
		opt(&o)
	}
	cfg = cfg.Normalized()
	if err := cfg.Validate(); err != nil { // stage 0
		return nil, fmt.Errorf("world config: %w", err)
	}

	w := &World{Config: cfg, Derived: deriveStructure(cfg)}
	names := newNameRegistry()

	genSkeleton(w, rng.Stream(cfg.Seed, streamSkeleton), names) // stage 1
	genClubs(w, rng.Stream(cfg.Seed, streamClubs), names)       // stage 2
	genManagers(w, rng.Stream(cfg.Seed, streamManagers))        // stage 3
	genPlayers(w, rng.Stream(cfg.Seed, streamPlayers))          // stage 4
	genHistory(w, rng.Stream(cfg.Seed, streamHistory))          // stage 5
	genSchedule(w, rng.Stream(cfg.Seed, streamSchedule))        // stage 6
	genEconomy(w, rng.Stream(cfg.Seed, streamEconomy))          // stage 7
	w.EnsureMatchState()                                        // table + condition

	manifest, err := issueCredentials(w, o.tokenReader) // stage 8
	if err != nil {
		return nil, err
	}
	queue := primeQueue(w, rng.Stream(cfg.Seed, streamQueue)) // stage 9

	return &Result{World: w, Manifest: manifest, Queue: queue}, nil
}

// issueCredentials (stage 8) mints a Manager Token for every generated
// Manager — employed and pool — and assembles the handover manifest
// (docs/09 §5). The Admin Token already exists (daemon first launch, 05 A7).
func issueCredentials(w *World, entropy io.Reader) (*Manifest, error) {
	m := &Manifest{
		WorldName:  w.Config.Name,
		Seed:       w.Config.Seed,
		StartState: "ready",
	}
	if w.Config.StartRunning {
		m.StartState = "running"
	}
	clubNames := map[int64]string{}
	for i := range w.Clubs {
		clubNames[w.Clubs[i].ID] = w.Clubs[i].Name
	}
	for i := range w.Managers {
		mgr := &w.Managers[i]
		token, err := MintManagerToken(entropy)
		if err != nil {
			return nil, err
		}
		m.Managers = append(m.Managers, ManagerCredential{
			ManagerID:   mgr.ID,
			ManagerName: mgr.Name,
			ClubID:      mgr.ClubID,
			ClubName:    clubNames[mgr.ClubID],
			Archetype:   mgr.Archetype,
			Reputation:  mgr.Reputation,
			Token:       token,
		})
	}
	return m, nil
}

// MintManagerToken mints one Manager Token from the given entropy source. Tokens
// are credentials — never derived from the world seed and never part of the world
// hash, so they live in the Manifest, not the World. Shared by
// world-creation credentialing (stage 8) and the daemon's runtime reconciler that
// backfills tokens for managers spawned mid-run (caretakers, newgen — FR-34).
func MintManagerToken(entropy io.Reader) (string, error) {
	var raw [16]byte
	if _, err := io.ReadFull(entropy, raw[:]); err != nil {
		return "", fmt.Errorf("minting manager token: %w", err)
	}
	return "mgr_" + hex.EncodeToString(raw[:]), nil
}

// Queue-priming payloads (stage 9). Typed payloads arrive with the sim core
// The executor contract is the string tag.
const (
	PayloadKickoff       = "kickoff"
	PayloadMatchMoment   = "match_moment" // one sampled key moment
	PayloadFinanceTick   = "finance_tick"
	PayloadDecisionRoll  = "decision_roll"
	PayloadConditionTick = "condition_recovery"
	PayloadPlayerDrift   = "player_drift"
	PayloadWindowClose   = "window_close"
	PayloadWindowOpen    = "window_open"
	PayloadSeasonEnd     = "season_rollover"
	PayloadYouthIntake   = "youth_intake" // youth intake: a club's spring academy intake
	PayloadFocusAlert    = "focus_alert"
	PayloadCalendarAlert = "calendar_alert"
)

// primeQueue (stage 9) stages the first roll for every entity — staggered so
// day one isn't a thundering herd — plus every known kickoff and the world
// calendar events. Insertion order is fixed (world, clubs, managers,
// players, fixtures): queue Seq is part of the total order (NFR-2).
func primeQueue(w *World, r *mrand.Rand) *sim.Queue {
	q := sim.NewQueue()
	minutesPerWeek := int64(7 * sim.MinutesPerDay)

	// World calendar events (docs/09 §3): window edges and season rollover.
	for _, e := range SeasonCalendarEvents(1) {
		q.Schedule(&sim.Event{Due: e.Due, Priority: sim.PriorityWorld,
			Kind: sim.KindWorld, Payload: e.Payload})
	}
	q.Schedule(&sim.Event{
		Due:      sim.MinutesPerDay,
		Priority: sim.PriorityCondition,
		Kind:     sim.KindWorld,
		Payload:  PayloadConditionTick,
	})

	for i := range w.Clubs {
		q.Schedule(&sim.Event{
			Due:      sim.GameTime(r.Int64N(minutesPerWeek)),
			Priority: sim.PriorityDrift,
			Kind:     sim.KindClub,
			EntityID: w.Clubs[i].ID,
			Payload:  PayloadFinanceTick,
		})
	}
	for i := range w.Managers {
		q.Schedule(&sim.Event{
			Due:      sim.GameTime(r.Int64N(3 * sim.MinutesPerDay)),
			Priority: sim.PriorityDecision,
			Kind:     sim.KindManager,
			EntityID: w.Managers[i].ID,
			Payload:  PayloadDecisionRoll,
		})
	}
	for i := range w.Players {
		q.Schedule(&sim.Event{
			Due:      sim.GameTime(r.Int64N(minutesPerWeek)),
			Priority: sim.PriorityDrift,
			Kind:     sim.KindPlayer,
			EntityID: w.Players[i].ID,
			Payload:  PayloadPlayerDrift,
		})
	}
	for i := range w.Fixtures {
		q.Schedule(&sim.Event{
			Due:      w.Fixtures[i].Kickoff,
			Priority: sim.PriorityMatch,
			Kind:     sim.KindMatch,
			EntityID: w.Fixtures[i].ID,
			Payload:  PayloadKickoff,
		})
	}
	// Youth intake: one per club at its spring intake day (youth intake). Clubs in
	// slice order so two clubs sharing an intake day tie-break on queue Seq (NFR-2);
	// re-primed each season by the engine's primeSeasonEvents, exactly like kickoffs.
	for i := range w.Clubs {
		q.Schedule(&sim.Event{
			Due:      YouthIntakeDue(&w.Clubs[i], 1),
			Priority: sim.PriorityDrift,
			Kind:     sim.KindClub,
			EntityID: w.Clubs[i].ID,
			Payload:  PayloadYouthIntake,
		})
	}
	return q
}
