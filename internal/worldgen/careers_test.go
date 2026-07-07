package worldgen

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/rng"
)

// TestSpawnManagerDeterministic locks the lifecycle-infra invariant: a runtime
// manager spawn is fully reproducible — same seed + same spawn stream ⇒ identical
// manager (id, name, attributes) — so a resumed run spawns the same caretaker /
// newgen the uninterrupted run did (NFR-2). Also checks the monotonic id and the
// caretaker's modest, capped attributes.
func TestSpawnManagerDeterministic(t *testing.T) {
	wa := genWorld(t, 42)
	wb := genWorld(t, 42)
	if wa.NextManagerID == 0 || wa.NextManagerID != wb.NextManagerID {
		t.Fatalf("NextManagerID at generation = %d vs %d (want equal, non-zero)", wa.NextManagerID, wb.NextManagerID)
	}
	base := wa.NextManagerID

	ra := rng.Stream(wa.Config.Seed, "career/spawn/1")
	rb := rng.Stream(wb.Config.Seed, "career/spawn/1")
	ma := SpawnManager(wa, ra, wa.Clubs[0].ID, 1, true)
	mb := SpawnManager(wb, rb, wb.Clubs[0].ID, 1, true)

	if ma.ID != base+1 || mb.ID != ma.ID {
		t.Fatalf("spawned id = %d / %d, want %d (monotonic from NextManagerID)", ma.ID, mb.ID, base+1)
	}
	if ma.Name != mb.Name || ma.Reputation != mb.Reputation ||
		ma.Coaching != mb.Coaching || ma.ManManagement != mb.ManManagement || ma.Age != mb.Age {
		t.Fatalf("spawn not deterministic:\n a=%+v\n b=%+v", ma, mb)
	}
	if ma.Status != ManagerActive || !ma.Caretaker {
		t.Fatalf("caretaker lifecycle wrong: status=%q caretaker=%v", ma.Status, ma.Caretaker)
	}
	if ma.ClubID != wa.Clubs[0].ID {
		t.Fatalf("caretaker not installed at the club: ClubID=%d", ma.ClubID)
	}
	if ma.Coaching > caretakerAttrCap || ma.ManManagement > caretakerAttrCap {
		t.Fatalf("caretaker attrs not capped: coaching=%d man=%d cap=%d", ma.Coaching, ma.ManManagement, caretakerAttrCap)
	}
	if wa.NextManagerID != base+1 {
		t.Fatalf("NextManagerID = %d after one spawn, want %d", wa.NextManagerID, base+1)
	}
	// A second spawn takes the next id — no reuse.
	m2 := SpawnManager(wa, rng.Stream(wa.Config.Seed, "career/spawn/2"), 0, 2, false)
	if m2.ID != base+2 || m2.Caretaker {
		t.Fatalf("second (newgen) spawn id=%d caretaker=%v, want %d / false", m2.ID, m2.Caretaker, base+2)
	}
}
