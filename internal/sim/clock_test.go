package sim

import "testing"

// Drift tests: lock time constants to docs (FR-2, docs/03 §5).
func TestSpeedTiersGolden(t *testing.T) {
	if Speed5 != 5 || Speed15 != 15 || Speed30 != 30 || Speed60 != 60 {
		t.Fatal("speed tiers drifted from FR-2 (5/15/30/60)")
	}
	if DefaultIdleAcceleration != 16 {
		t.Fatal("idle acceleration default drifted from the documented tunable (16x base)")
	}
	if DefaultOffseasonAcceleration != 96 {
		t.Fatal("off-season acceleration default drifted from the documented tunable (96x base)")
	}
}

// The priority-class drain order is part of the documented total order
// (docs/03 §5): World < Match < Decision < Condition < Drift.
func TestPriorityClassOrder(t *testing.T) {
	order := []PriorityClass{PriorityWorld, PriorityMatch, PriorityDecision, PriorityCondition, PriorityDrift}
	for i := 1; i < len(order); i++ {
		if order[i-1] >= order[i] {
			t.Fatalf("priority order drifted at index %d — docs/03 §5 defines World < Match < Decision < Condition < Drift", i)
		}
	}
}

func TestGameTimeFormatting(t *testing.T) {
	tt := GameTime(1*MinutesPerDay + 14*MinutesPerHour + 30)
	if tt.Day() != 1 || tt.Hour() != 14 {
		t.Fatalf("GameTime decomposition wrong: %v", tt)
	}
	if tt.String() != "d1 14:30" {
		t.Fatalf("String() = %q", tt.String())
	}
}
