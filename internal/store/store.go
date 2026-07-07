// Package store defines the persistence contracts: authoritative world state,
// the append-only feed, the roll audit trail, and the external-input log that
// the replay contract depends on (docs/03 §5, FR-28/28a/29, NFR-2).
package store

import (
	"encoding/json"

	"github.com/gaemi/agentic-fc/internal/sim"
)

// InputEntry records one accepted external input — every MCP tool call
// (including free/read-only calls: reads spend Focus and get_news advances
// per-session cursors) plus every admin operation.
type InputEntry struct {
	IngressSeq  uint64          `json:"ingress_seq"` // monotonic, assigned by the core
	GameTime    sim.GameTime    `json:"game_time"`
	SessionID   string          `json:"session_id"`
	ManagerID   int64           `json:"manager_id,omitempty"` // 0 for admin ops
	Tool        string          `json:"tool"`
	Params      json.RawMessage `json:"params"` // canonical JSON, embedded raw
	Result      string          `json:"result"` // ok | error code
	FocusCharge int             `json:"focus_charge"`
}

// InputLog is append-only; replay re-applies entries by (GameTime, IngressSeq).
type InputLog interface {
	Append(e InputEntry) (uint64, error) // assigns and returns IngressSeq
	// Seq is the last assigned IngressSeq — snapshots record it as their
	// watermark so replay knows which logged inputs the snapshot already
	// contains (WAL reconciliation, NFR-2).
	Seq() uint64
}

// RollEntry is one probability resolution in the audit trail (FR-29):
// inputs, weights, outcome, and the next-roll schedule, each factor
// individually explainable (docs/03 §2 constraint 4).
type RollEntry struct {
	GameTime       sim.GameTime    `json:"game_time"`
	EntityKind     sim.EntityKind  `json:"entity_kind"`
	EntityID       int64           `json:"entity_id"`
	Category       string          `json:"category"`
	Factors        json.RawMessage `json:"factors,omitempty"` // canonical JSON: named weights
	Outcome        string          `json:"outcome"`
	NextRoll       sim.GameTime    `json:"next_roll"`
	MindsetVersion int             `json:"mindset_version,omitempty"` // FR-16e, Manager decisions only
}

// AuditLog is append-only.
type AuditLog interface {
	Append(e RollEntry) error
}

// WorldStore persists the authoritative state; a crashed or stopped world
// resumes where it left off (FR-28). The world is append-only: no player-
// facing saves or rollback (FR-28a).
type WorldStore interface {
	// Implemented by FileStore (file.go): durable world+queue snapshots with
	// defensive state re-derivation on load.
}
