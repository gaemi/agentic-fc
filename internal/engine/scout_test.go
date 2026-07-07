package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// TestDriftIsNotAgentNews locks the FR-22a fix: attribute drift is a
// Console-feed (human god-view) event only. It must never enter the
// agent-facing news ring, where broadcasting exact from→to values would leak
// other-club visible attributes that get_squad/get_person mask into ranges.
func TestDriftIsNotAgentNews(t *testing.T) {
	e, _ := newEngine(t, 7)
	if _, err := e.RunUntil(day(180)); err != nil {
		t.Fatal(err)
	}
	w := e.World()
	if len(w.News) == 0 {
		t.Fatal("expected some agent news over 180 days (e.g. decision shifts); test would be vacuous")
	}
	for _, n := range w.News {
		if n.Key == FeedDriftGrew || n.Key == FeedDriftDeclined {
			t.Fatalf("attribute drift leaked into the agent news ring: %s", n.Key)
		}
	}
}

// TestMergeEvidenceDeepensWithoutDuplicating locks the re-scouting contract
// (docs/11 §5): a repeated impression refreshes its confidence and date in
// place, while a genuinely new impression is appended. Re-scouting must never
// stack duplicate prose lines.
func TestMergeEvidenceDeepensWithoutDuplicating(t *testing.T) {
	k := &worldgen.Knowledge{}

	mergeEvidence(k, []worldgen.Evidence{
		{Key: "evidence.pressure.high", Confidence: "LOW", GameTime: 10},
		{Key: "evidence.temperament.low", Confidence: "LOW", GameTime: 10},
	})
	if len(k.Evidence) != 2 {
		t.Fatalf("first report: want 2 evidence lines, got %d", len(k.Evidence))
	}

	// Re-scout: same two impressions at higher confidence, plus a new one.
	mergeEvidence(k, []worldgen.Evidence{
		{Key: "evidence.pressure.high", Confidence: "HIGH", GameTime: sim.GameTime(20)},
		{Key: "evidence.temperament.low", Confidence: "HIGH", GameTime: sim.GameTime(20)},
		{Key: "evidence.loyalty.high", Confidence: "MEDIUM", GameTime: sim.GameTime(20)},
	})
	if len(k.Evidence) != 3 {
		t.Fatalf("re-scout: want 3 distinct evidence lines, got %d", len(k.Evidence))
	}

	seen := map[string]worldgen.Evidence{}
	for _, e := range k.Evidence {
		if prev, dup := seen[e.Key]; dup {
			t.Fatalf("duplicate evidence key %q (%v and %v)", e.Key, prev, e)
		}
		seen[e.Key] = e
	}
	if got := seen["evidence.pressure.high"]; got.Confidence != "HIGH" || got.GameTime != 20 {
		t.Fatalf("re-scout did not refresh confidence/date in place: %+v", got)
	}
}
