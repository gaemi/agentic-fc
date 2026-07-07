package worldgen

import (
	"github.com/gaemi/agentic-fc/internal/focus"
	"github.com/gaemi/agentic-fc/internal/sim"
)

// FocusMinutesPerFP derives the integer regen tick from the registered
// economy constants: 2 FP/game-hour => exactly one FP per 30 game-minutes.
const FocusMinutesPerFP = sim.MinutesPerHour / focus.RegenPerGameHour

// SyncManagerFocus advances a manager's Focus balance to now using whole
// integer regen ticks. Callers must hold the world write lock or be running on
// the single-writer engine goroutine.
func SyncManagerFocus(m *Manager, now sim.GameTime) {
	if now <= m.FocusRegenMark {
		return
	}
	ticks := int64(now-m.FocusRegenMark) / FocusMinutesPerFP
	if ticks <= 0 {
		return
	}
	m.FocusRegenMark += sim.GameTime(ticks * FocusMinutesPerFP)
	m.FocusBalance += int(ticks)
	if m.FocusBalance > focus.Cap {
		m.FocusBalance = focus.Cap
	}
}
