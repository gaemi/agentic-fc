package engine

import (
	"sort"
	"testing"

	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// TestResumeEquivalence locks the FR-28 invariant: a world stopped at any
// point and resumed from its snapshot follows the exact trajectory of an
// uninterrupted run — same hash, same clock, same pending queue. Roll
// streams are stateless, so no RNG state needs persisting.
func TestResumeEquivalence(t *testing.T) {
	const (
		seed    = 4242
		half    = 10
		horizon = 20
	)

	// Run A: uninterrupted.
	ea, _ := newEngine(t, seed)
	if _, err := ea.RunUntil(day(horizon)); err != nil {
		t.Fatal(err)
	}

	// Run B: stop at day 10, snapshot to disk, load, resume to day 20.
	eb, _ := newEngine(t, seed)
	if _, err := eb.RunUntil(day(half)); err != nil {
		t.Fatal(err)
	}
	fstore := &store.FileStore{Dir: t.TempDir()}
	events, nextSeq := eb.Queue().Snapshot()
	if err := fstore.SaveSnapshot(&store.Snapshot{
		Now: eb.Now(), World: eb.World(), Queue: events, QueueNextSeq: nextSeq,
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := fstore.LoadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap == nil {
		t.Fatal("snapshot missing after save")
	}
	resumed := New(snap.World, sim.RestoreQueue(snap.Queue, snap.QueueNextSeq), &store.MemAuditLog{})
	resumed.ResumeAt(snap.Now)
	if resumed.Now() != day(half) {
		t.Fatalf("resumed clock = %s, want %s", resumed.Now(), day(half))
	}
	if _, err := resumed.RunUntil(day(horizon)); err != nil {
		t.Fatal(err)
	}

	ha, _ := ea.World().Hash()
	hb, _ := resumed.World().Hash()
	if ha != hb {
		t.Fatalf("resume diverged from the uninterrupted run:\nA %s\nB %s", ha, hb)
	}
	compareQueues(t, ea.Queue(), resumed.Queue())
}

// compareQueues asserts two queues hold the same pending events.
func compareQueues(t *testing.T, a, b *sim.Queue) {
	t.Helper()
	ea, seqA := a.Snapshot()
	eb, seqB := b.Snapshot()
	if len(ea) != len(eb) || seqA != seqB {
		t.Fatalf("queues differ: %d/%d events, nextSeq %d/%d", len(ea), len(eb), seqA, seqB)
	}
	less := func(s []sim.Event) func(i, j int) bool {
		return func(i, j int) bool {
			if s[i].Due != s[j].Due {
				return s[i].Due < s[j].Due
			}
			return s[i].Seq < s[j].Seq
		}
	}
	sort.Slice(ea, less(ea))
	sort.Slice(eb, less(eb))
	for i := range ea {
		if ea[i] != eb[i] {
			t.Fatalf("queue event %d differs: %+v vs %+v", i, ea[i], eb[i])
		}
	}
}

// TestRestoredQueueDrainsIdentically: restoring a snapshot in any listed
// order preserves the NFR-2 total drain order.
func TestRestoredQueueDrainsIdentically(t *testing.T) {
	res, err := worldgen.Generate(worldgen.PresetCompact(9), worldgen.WithTokenReader(&tokens{}))
	if err != nil {
		t.Fatal(err)
	}
	events, nextSeq := res.Queue.Snapshot()
	// Reverse the listing to prove order-independence.
	rev := make([]sim.Event, len(events))
	for i, e := range events {
		rev[len(events)-1-i] = e
	}
	qa := sim.RestoreQueue(events, nextSeq)
	qb := sim.RestoreQueue(rev, nextSeq)
	for {
		x, y := qa.Pop(), qb.Pop()
		if x == nil && y == nil {
			return
		}
		if x == nil || y == nil || *x != *y {
			t.Fatalf("drain diverged: %+v vs %+v", x, y)
		}
	}
}
