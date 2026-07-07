package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

type tokens struct{ n uint32 }

func (t *tokens) Read(p []byte) (int, error) {
	for i := range p {
		t.n++
		p[i] = byte((t.n * 2654435761) >> 24)
	}
	return len(p), nil
}

func TestSnapshotRoundtrip(t *testing.T) {
	res, err := worldgen.Generate(worldgen.PresetCompact(5), worldgen.WithTokenReader(&tokens{}))
	if err != nil {
		t.Fatal(err)
	}
	f := &FileStore{Dir: t.TempDir()}

	if s, err := f.LoadSnapshot(); err != nil || s != nil {
		t.Fatalf("empty dir should load (nil, nil), got (%v, %v)", s, err)
	}

	events, nextSeq := res.Queue.Snapshot()
	want, _ := res.World.Hash()
	if err := f.SaveSnapshot(&Snapshot{
		Now: 123, World: res.World, Queue: events, QueueNextSeq: nextSeq,
	}); err != nil {
		t.Fatal(err)
	}

	s, err := f.LoadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if s.Now != 123 || s.QueueNextSeq != nextSeq || len(s.Queue) != len(events) {
		t.Fatalf("snapshot header mismatch: %+v", s)
	}
	got, err := s.World.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("world hash changed across the roundtrip:\n%s\n%s", want, got)
	}
	// Payloads must come back as the string tags the executor switches on.
	for _, e := range s.Queue {
		if _, ok := e.Payload.(string); !ok {
			t.Fatalf("payload type lost in roundtrip: %T", e.Payload)
		}
	}
	// Full in-memory fidelity, not just hash equality: numeric mindset
	// params follow JSON number semantics (float64) precisely so this
	// holds.
	if !reflect.DeepEqual(s.World, res.World) {
		t.Fatal("world not deep-equal after the snapshot roundtrip")
	}

	// Corrupt snapshots fail loudly, never silently regenerate (FR-28a).
	if err := os.WriteFile(filepath.Join(f.Dir, "world.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.LoadSnapshot(); err == nil {
		t.Fatal("corrupt snapshot must error")
	}
}

func TestFileAuditLog(t *testing.T) {
	dir := t.TempDir()
	a := NewFileAuditLog(dir)
	for i := 0; i < 3; i++ {
		if err := a.Append(RollEntry{GameTime: sim.GameTime(i), Category: "drift"}); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := 0
	for _, c := range b {
		if c == '\n' {
			lines++
		}
	}
	if lines != 3 {
		t.Fatalf("audit lines = %d, want 3", lines)
	}
}

func TestFileInputLogResumesSeq(t *testing.T) {
	dir := t.TempDir()
	l1, err := NewFileInputLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if seq, _ := l1.Append(InputEntry{Tool: "get_club"}); seq != 1 {
		t.Fatalf("first seq = %d", seq)
	}
	if seq, _ := l1.Append(InputEntry{Tool: "get_squad"}); seq != 2 {
		t.Fatalf("second seq = %d", seq)
	}
	// A reopened log continues the monotonic ingress sequence (NFR-2).
	l2, err := NewFileInputLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if seq, _ := l2.Append(InputEntry{Tool: "get_news"}); seq != 3 {
		t.Fatalf("resumed seq = %d", seq)
	}
}

// TestLoadDerivesNextManagerID locks sparse snapshot repair: when NextManagerID
// is absent, LoadSnapshot re-derives it from the max manager id so runtime
// spawns stay monotonic.
func TestLoadDerivesNextManagerID(t *testing.T) {
	fs := &FileStore{Dir: t.TempDir()}
	w := &worldgen.World{Managers: []worldgen.Manager{{ID: 1001}, {ID: 1005}, {ID: 1003}}}
	// NextManagerID deliberately left 0 to model sparse serialized state.
	if err := fs.SaveSnapshot(&Snapshot{World: w}); err != nil {
		t.Fatal(err)
	}
	snap, err := fs.LoadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.World.NextManagerID != 1005 {
		t.Fatalf("NextManagerID = %d after load, want 1005 (max manager id)", snap.World.NextManagerID)
	}
	// A snapshot that already carries the counter is left untouched.
	w2 := &worldgen.World{NextManagerID: 42, Managers: []worldgen.Manager{{ID: 1001}}}
	if err := fs.SaveSnapshot(&Snapshot{World: w2}); err != nil {
		t.Fatal(err)
	}
	snap2, err := fs.LoadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap2.World.NextManagerID != 42 {
		t.Fatalf("NextManagerID = %d, want 42 (preserved, not re-derived)", snap2.World.NextManagerID)
	}
}

// TestLoadDerivesNextPlayerID locks sparse snapshot repair: when NextPlayerID is
// absent, LoadSnapshot re-derives it from the max player id so runtime youth
// intake stays monotonic.
func TestLoadDerivesNextPlayerID(t *testing.T) {
	fs := &FileStore{Dir: t.TempDir()}
	w := &worldgen.World{Players: []worldgen.Player{{ID: 10001}, {ID: 10009}, {ID: 10004}}}
	// NextPlayerID deliberately left 0 to model sparse serialized state.
	if err := fs.SaveSnapshot(&Snapshot{World: w}); err != nil {
		t.Fatal(err)
	}
	snap, err := fs.LoadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.World.NextPlayerID != 10009 {
		t.Fatalf("NextPlayerID = %d after load, want 10009 (max player id)", snap.World.NextPlayerID)
	}
	// A snapshot that already carries the counter is left untouched.
	w2 := &worldgen.World{NextPlayerID: 42, Players: []worldgen.Player{{ID: 10001}}}
	if err := fs.SaveSnapshot(&Snapshot{World: w2}); err != nil {
		t.Fatal(err)
	}
	snap2, err := fs.LoadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap2.World.NextPlayerID != 42 {
		t.Fatalf("NextPlayerID = %d, want 42 (preserved, not re-derived)", snap2.World.NextPlayerID)
	}
}

// TestLoadReseedsConfidence locks sparse confidence repair: a club with an
// uninitialised (0) live Confidence is reseeded from its baseline on load; a
// club that already carries a value keeps it.
func TestLoadReseedsConfidence(t *testing.T) {
	fs := &FileStore{Dir: t.TempDir()}
	w := &worldgen.World{Clubs: []worldgen.Club{
		{ID: 1, ConfidenceBaseline: 60, Confidence: 0},
		{ID: 2, ConfidenceBaseline: 40, Confidence: 30},
	}}
	if err := fs.SaveSnapshot(&Snapshot{World: w}); err != nil {
		t.Fatal(err)
	}
	snap, err := fs.LoadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.World.Clubs[0].Confidence != 60 {
		t.Fatalf("club 0 confidence = %d, want 60 (reseeded from baseline)", snap.World.Clubs[0].Confidence)
	}
	if snap.World.Clubs[1].Confidence != 30 {
		t.Fatalf("club 1 confidence = %d, want 30 (preserved)", snap.World.Clubs[1].Confidence)
	}
}
