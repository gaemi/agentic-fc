package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gaemi/agentic-fc/internal/sim"
)

// Adaptive Tempo (docs/02 §5.2, docs/03 §4): match windows run at base Game
// Speed, in-season idle windows run at base × idle acceleration, and the
// fixtureless off-season runs at base × off-season acceleration. Tempo converts
// game time to real time and NOTHING else — simulation outcomes are independent
// of pacing.

// MatchWindowMinutes is the length of a match window in game minutes
// (~2 game-hours incl. half-time and stoppages — docs/02 §5.2, tunable).
const MatchWindowMinutes = 120

// TempoAt reports the world's tempo at game time t: TempoMatch while any
// fixture's window is open (league-wide match days, FR-6a), TempoIdle between
// fixtures once the season has begun, and TempoOffseason before the first
// fixture or after the final fixture. Pauses are a runner-level state
// (admin-only, FR-34b), not a property of game time.
func (e *Engine) TempoAt(t sim.GameTime) sim.Tempo {
	// Binary search the latest kickoff ≤ t.
	lo := e.kickoffUpperBound(t)
	if lo > 0 && t < e.kickoffs[lo-1]+MatchWindowMinutes {
		return sim.TempoMatch
	}
	if lo == 0 || lo == len(e.kickoffs) {
		return sim.TempoOffseason
	}
	return sim.TempoIdle
}

// Pacer converts game durations to real durations under Adaptive Tempo.
type Pacer struct {
	Speed                 sim.Speed
	IdleAcceleration      int
	OffseasonAcceleration int
}

// RealPerGameMinute is the wall-clock cost of one game minute at a tempo.
func (p Pacer) RealPerGameMinute(tempo sim.Tempo) time.Duration {
	switch tempo {
	case sim.TempoMatch:
		return time.Minute / time.Duration(p.speed())
	case sim.TempoIdle:
		return time.Minute / time.Duration(p.speed()*p.idleAcceleration())
	case sim.TempoOffseason:
		return time.Minute / time.Duration(p.speed()*p.offseasonAcceleration())
	default: // TempoPaused: the clock does not advance
		return 0
	}
}

func (p Pacer) speed() int {
	if p.Speed == 0 {
		return int(sim.Speed15)
	}
	return int(p.Speed)
}

func (p Pacer) idleAcceleration() int {
	if p.IdleAcceleration <= 0 {
		return sim.DefaultIdleAcceleration
	}
	return p.IdleAcceleration
}

func (p Pacer) offseasonAcceleration() int {
	if p.OffseasonAcceleration <= 0 {
		return p.idleAcceleration()
	}
	return p.OffseasonAcceleration
}

// RealDuration integrates wall-clock time for the game interval [from, to),
// walking the tempo boundaries (window opens at each kickoff, closes
// MatchWindowMinutes later).
func (e *Engine) RealDuration(p Pacer, from, to sim.GameTime) time.Duration {
	var total time.Duration
	t := from
	for t < to {
		tempo := e.TempoAt(t)
		next := to
		if b := e.nextBoundary(t); b < next {
			next = b
		}
		total += time.Duration(int64(next-t)) * p.RealPerGameMinute(tempo)
		t = next
	}
	return total
}

// nextBoundary returns the next game time at which tempo can change after t.
func (e *Engine) nextBoundary(t sim.GameTime) sim.GameTime {
	// Next window close after t.
	lo := e.kickoffUpperBound(t)
	if lo > 0 {
		if close := e.kickoffs[lo-1] + MatchWindowMinutes; close > t {
			return close
		}
	}
	if lo < len(e.kickoffs) {
		return e.kickoffs[lo]
	}
	return sim.GameTime(int64(1) << 62) // no more boundaries
}

func (e *Engine) kickoffUpperBound(t sim.GameTime) int {
	lo, hi := 0, len(e.kickoffs)
	for lo < hi {
		mid := (lo + hi) / 2
		if e.kickoffs[mid] <= t {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

// Runner drives the engine in real time: advance the game clock in short
// real-time quanta, draining events as they come due. The sleeper is
// injectable so tests run instantly; the daemon passes SleepReal.
//
// Advancing in quanta (instead of one long sleep to the next event) keeps
// the visible clock moving for spectators, makes pause responsive, and
// leaves room for externally injected events such as MCP calls. Outcomes are
// chunking-independent.
type Runner struct {
	Engine *Engine
	Pacer  Pacer
	Sleep  func(context.Context, time.Duration) error

	// Guard, when set, wraps every state mutation in a write lock so API
	// readers can snapshot between steps (single-writer stays intact:
	// only the runner mutates).
	Guard *sync.RWMutex

	paused atomic.Bool
}

// runnerQuantum is the real-time slice between clock advances.
const runnerQuantum = 200 * time.Millisecond

// SleepReal is the production sleeper.
func SleepReal(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// SetPaused pauses or resumes the runner (admin-only maintenance pause,
// FR-34b). While paused the game clock freezes; the loop keeps breathing so
// resume is immediate.
//
// Pausing is a barrier when Guard is set: SetPaused(true) returns only
// after any in-flight step has finished, and every later step re-checks
// the flag under the same lock — once the admin sees the acknowledgement,
// nothing advances (docs/05 A11).
func (r *Runner) SetPaused(v bool) {
	r.paused.Store(v)
	if v && r.Guard != nil {
		r.Guard.Lock()
		//lint:ignore SA2001 empty critical section is the barrier
		r.Guard.Unlock()
	}
}

// Paused reports the runner's pause state.
func (r *Runner) Paused() bool { return r.paused.Load() }

// Tempo reports the effective tempo right now: PAUSED when paused, else the
// engine's tempo at the current game time.
func (r *Runner) Tempo() sim.Tempo {
	if r.paused.Load() {
		return sim.TempoPaused
	}
	return r.Engine.TempoAt(r.Engine.Now())
}

// Run advances the world in real time until game time `until` (or ctx ends).
// Pacing decides only when RunUntil is called, never what it does.
func (r *Runner) Run(ctx context.Context, until sim.GameTime) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if r.paused.Load() {
			if err := r.Sleep(ctx, runnerQuantum); err != nil {
				return err
			}
			continue
		}
		now := r.Engine.Now()
		if now >= until {
			return nil
		}
		// One quantum of real time → game minutes at the current tempo,
		// clipped to the next tempo boundary and the horizon.
		tempo := r.Engine.TempoAt(now)
		perMinute := r.Pacer.RealPerGameMinute(tempo)
		step := sim.GameTime(int64(runnerQuantum / perMinute))
		if step < 1 {
			step = 1
		}
		target := now + step
		if b := r.Engine.nextBoundary(now); b < target {
			target = b
		}
		if target > until {
			target = until
		}
		if err := r.Sleep(ctx, r.Engine.RealDuration(r.Pacer, now, target)); err != nil {
			return err
		}
		if r.Guard != nil {
			r.Guard.Lock()
		}
		// Re-check under the lock: a pause may have landed during the
		// sleep, and its barrier synchronizes on this same lock.
		if r.paused.Load() {
			if r.Guard != nil {
				r.Guard.Unlock()
			}
			continue
		}
		_, err := r.Engine.RunUntil(target)
		if r.Guard != nil {
			r.Guard.Unlock()
		}
		if err != nil {
			return err
		}
	}
}
