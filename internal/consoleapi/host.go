package consoleapi

import (
	"github.com/gaemi/agentic-fc/internal/engine"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Host is the daemon-side world holder the API serves from. Locked wraps
// every read in the same lock the runner writes under — the Simulation Core
// stays the single writer (docs/05 A1); the API only ever snapshots.
type Host interface {
	// Locked runs read under the world read-lock.
	Locked(read func())
	// World returns the world (access inside Locked only).
	World() *worldgen.World
	// Engine returns the engine (access inside Locked only).
	Engine() *engine.Engine

	// State is the lifecycle state: "ready" | "running" | "paused".
	State() string
	// Tempo is the effective tempo (PAUSED while paused).
	Tempo() sim.Tempo
	// Start moves ready → running (idempotent errors on re-start).
	Start() error
	// SetPaused toggles the admin maintenance pause (FR-34b).
	SetPaused(paused bool) error

	// Seed is admin-only information: publishing it would let anyone
	// regenerate the world and read every hidden attribute.
	Seed() uint64
	// Credentials lists the generated Manager Tokens (admin-only, FR-33).
	Credentials() []worldgen.ManagerCredential
}
