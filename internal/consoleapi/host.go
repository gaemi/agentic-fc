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
	// RuntimeSettings returns mutable operator settings that affect pacing
	// but not deterministic simulation outcomes.
	RuntimeSettings() RuntimeSettings
	// UpdateRuntimeSettings applies mutable operator settings from a stable
	// host-owned baseline and returns the persisted result.
	UpdateRuntimeSettings(RuntimeSettingsUpdater) (RuntimeSettings, error)

	// Seed is admin-only information: publishing it would let anyone
	// regenerate the world and read every hidden attribute.
	Seed() uint64
	// Credentials lists the generated Manager Tokens (admin-only, FR-33).
	Credentials() []worldgen.ManagerCredential
}

// RuntimeSettings are admin-editable settings that can change after world
// creation. They are deliberately limited to runtime pacing knobs; generation
// settings such as seed, league shape, economy, and quality stay immutable.
type RuntimeSettings struct {
	GameSpeed             sim.Speed `json:"game_speed"`
	IdleAcceleration      int       `json:"idle_acceleration"`
	OffseasonAcceleration int       `json:"offseason_acceleration"`
}

// RuntimeSettingsUpdater merges a partial operator request into the current
// settings. The Host runs it inside its own serialized update path.
type RuntimeSettingsUpdater func(RuntimeSettings) (RuntimeSettings, error)
