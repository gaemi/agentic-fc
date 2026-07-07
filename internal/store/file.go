package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// SnapshotVersion is the current public on-disk format.
const SnapshotVersion = 1

// Snapshot is the persisted world state (FR-28): everything needed to
// resume exactly where a stopped or crashed world left off. Roll streams
// are stateless, so world + queue + clock is sufficient.
type Snapshot struct {
	Version      int             `json:"version"`
	Now          sim.GameTime    `json:"now"`
	Started      bool            `json:"started"` // resumes to its run state
	World        *worldgen.World `json:"world"`
	Queue        []sim.Event     `json:"queue"`
	QueueNextSeq uint64          `json:"queue_next_seq"`
	// LastIngressSeq is the input-log watermark at snapshot time: every
	// logged input with a higher seq is NOT yet reflected in this state
	// (replay applies them on top — WAL reconciliation, NFR-2).
	LastIngressSeq uint64 `json:"last_ingress_seq"`
}

// FileStore persists the snapshot and append-only logs in a directory.
// Snapshot writes are atomic AND durable: fresh 0600 temp, fsync, rename,
// directory fsync — a crash mid-save leaves the previous snapshot intact,
// and an acknowledged save survives power loss.
type FileStore struct {
	Dir string
}

func (f *FileStore) snapshotPath() string { return filepath.Join(f.Dir, "world.json") }

// SaveSnapshot atomically and durably replaces the on-disk snapshot.
func (f *FileStore) SaveSnapshot(s *Snapshot) error {
	s.Version = SnapshotVersion
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(f.Dir, ".world-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil { // durable before the rename
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, f.snapshotPath()); err != nil {
		os.Remove(name)
		return err
	}
	return syncDir(f.Dir) // the rename itself must survive power loss
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// LoadSnapshot returns (nil, nil) when no snapshot exists yet.
func (f *FileStore) LoadSnapshot() (*Snapshot, error) {
	b, err := os.ReadFile(f.snapshotPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("corrupt snapshot %s: %w", f.snapshotPath(), err)
	}
	s.Version = SnapshotVersion
	// JSON round-trips event payloads as plain values; ours are string
	// tags. Re-assert that so the executor's type switch stays exact.
	for i := range s.Queue {
		if p, ok := s.Queue[i].Payload.(string); ok {
			s.Queue[i].Payload = p
		}
	}
	// Unexported counters don't serialize: re-derive them.
	for i := range s.World.Managers {
		s.World.Managers[i].Mindset.SyncDirectiveCounter()
	}
	// Re-derive sparse counters from existing rows so runtime spawns stay
	// monotonic and never reuse an id.
	if s.World.NextManagerID == 0 {
		for i := range s.World.Managers {
			if id := s.World.Managers[i].ID; id > s.World.NextManagerID {
				s.World.NextManagerID = id
			}
		}
	}
	// Re-derive sparse player counters from existing rows for the same reason.
	if s.World.NextPlayerID == 0 {
		for i := range s.World.Players {
			if id := s.World.Players[i].ID; id > s.World.NextPlayerID {
				s.World.NextPlayerID = id
			}
		}
	}
	// A zero live confidence is uninitialised, not rock-bottom; use the
	// season baseline.
	for i := range s.World.Clubs {
		if c := &s.World.Clubs[i]; c.Confidence == 0 {
			c.Confidence = c.ConfidenceBaseline
		}
	}
	// Keep sparse configs runnable.
	if s.World.Config.IdleAcceleration == 0 {
		s.World.Config.IdleAcceleration = sim.DefaultIdleAcceleration
	}
	if s.World.Config.OffseasonAccel == 0 {
		s.World.Config.OffseasonAccel = sim.DefaultOffseasonAcceleration
	}
	return &s, nil
}

// jsonlAppender is a shared append-only JSONL file (0600, re-tightened on
// open). durable=true fsyncs every append.
type jsonlAppender struct {
	path    string
	durable bool
}

func (j *jsonlAppender) append(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	fh, err := os.OpenFile(j.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer fh.Close()
	if err := fh.Chmod(0o600); err != nil { // tighten an existing lax file
		return err
	}
	if _, err := fh.Write(append(b, '\n')); err != nil {
		return err
	}
	if j.durable {
		return fh.Sync()
	}
	return nil
}

// FileAuditLog appends roll audit entries to audit.jsonl (FR-29).
// Deliberately NOT fsynced per entry: the audit trail is derived,
// reproducible observability — replaying (seed, config, input log)
// regenerates it — so a lost tail after power loss costs nothing the
// replay contract needs.
type FileAuditLog struct{ j jsonlAppender }

func NewFileAuditLog(dir string) *FileAuditLog {
	return &FileAuditLog{j: jsonlAppender{path: filepath.Join(dir, "audit.jsonl")}}
}

func (f *FileAuditLog) Append(e RollEntry) error { return f.j.append(e) }

// FileInputLog appends external inputs to inputs.jsonl. This log IS the
// replay contract (NFR-2), so every append is fsynced before the sequence
// number is acknowledged. The counter resumes from the persisted count at
// construction.
type FileInputLog struct {
	j    jsonlAppender
	next uint64
}

func NewFileInputLog(dir string) (*FileInputLog, error) {
	f := &FileInputLog{j: jsonlAppender{
		path:    filepath.Join(dir, "inputs.jsonl"),
		durable: true,
	}}
	b, err := os.ReadFile(f.j.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, c := range b {
		if c == '\n' {
			f.next++
		}
	}
	return f, nil
}

func (f *FileInputLog) Append(e InputEntry) (uint64, error) {
	f.next++
	e.IngressSeq = f.next
	if err := f.j.append(e); err != nil {
		f.next--
		return 0, err
	}
	return e.IngressSeq, nil
}

func (f *FileInputLog) Seq() uint64 { return f.next }
