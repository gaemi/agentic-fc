// Package sim implements the Simulation Core: the discrete-event,
// roll-and-reschedule engine specified in docs/03-simulation-engine.md.
package sim

import "fmt"

// GameTime is minutes since the world epoch (world creation, day 0 00:00).
// All scheduling and persistence use GameTime; wall-clock never enters the core.
type GameTime int64

const (
	MinutesPerHour = 60
	MinutesPerDay  = 24 * MinutesPerHour
)

func (t GameTime) Day() int64  { return int64(t) / MinutesPerDay }
func (t GameTime) Hour() int64 { return (int64(t) % MinutesPerDay) / MinutesPerHour }

func (t GameTime) String() string {
	return fmt.Sprintf("d%d %02d:%02d", t.Day(), t.Hour(), int64(t)%MinutesPerHour)
}

// Speed is the base real-to-game ratio chosen at world creation (FR-2).
type Speed int

// The fixed tier set (FR-2).
const (
	Speed5  Speed = 5
	Speed15 Speed = 15 // default: 1 game day = 96 real minutes
	Speed30 Speed = 30
	Speed60 Speed = 60
)

// Tempo is the current pacing mode (Adaptive Tempo, docs/02 §5.2).
type Tempo uint8

const (
	TempoMatch     Tempo = iota // match window: base Game Speed
	TempoIdle                   // idle window: base × IdleAcceleration
	TempoOffseason              // outside the fixture calendar: base × OffseasonAcceleration
	TempoPaused                 // admin maintenance pause (FR-34b): clock stopped
)

func (t Tempo) String() string {
	switch t {
	case TempoMatch:
		return "MATCH"
	case TempoIdle:
		return "IDLE"
	case TempoOffseason:
		return "OFFSEASON"
	case TempoPaused:
		return "PAUSED"
	}
	return "UNKNOWN"
}

// DefaultIdleAcceleration is the default in-season idle fast-forward factor.
const DefaultIdleAcceleration = 16

// DefaultOffseasonAcceleration is faster because no fixtures are pending.
const DefaultOffseasonAcceleration = 96
