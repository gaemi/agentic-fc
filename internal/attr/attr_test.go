package attr

import "testing"

// Drift tests: lock the taxonomy to docs/08. GKs carry the goalkeeping surface
// instead of the outfield technical surface; everyone shares mental+physical.
func TestPoolCostTableShape(t *testing.T) {
	wantRows := map[PositionGroup]int{GK: 32, DF: 33, MF: 33, FW: 33}
	for _, g := range []PositionGroup{GK, DF, MF, FW} {
		if got := len(PoolCosts[g]); got != wantRows[g] {
			t.Errorf("%s cost rows = %d, want %d (docs/08 §2)", g, got, wantRows[g])
		}
	}
	// GK must not price outfield technicals; outfield must not price GK skills.
	for _, a := range []Visible{Finishing, LongShots, FirstTouch, Passing, Crossing, Dribbling, Technique, Heading, Tackling, Marking, SetPieces} {
		if _, ok := PoolCosts[GK][a]; ok {
			t.Errorf("GK table must not contain outfield technical %s (docs/08 §4)", a)
		}
	}
	for _, g := range []PositionGroup{DF, MF, FW} {
		for _, a := range []Visible{Reflexes, OneOnOnes, Handling, AerialReach, CommandOfArea, Communication, Distribution, Sweeping, Eccentricity, Punching} {
			if _, ok := PoolCosts[g][a]; ok {
				t.Errorf("%s table must not contain goalkeeping %s", g, a)
			}
		}
	}
}

func TestPoolCostGoldenValues(t *testing.T) {
	golden := []struct {
		g    PositionGroup
		a    Visible
		want float64
	}{
		{FW, Pace, 2.2},
		{DF, Tackling, 1.8},
		{MF, Passing, 1.5},
		{FW, Finishing, 1.8},
		{GK, Reflexes, 2.0},
		{DF, SetPieces, 0.3},
	}
	for _, c := range golden {
		if got := PoolCosts[c.g][c.a]; got != c.want {
			t.Errorf("cost[%s][%s] = %v, docs/08 §4 says %v", c.g, c.a, got, c.want)
		}
	}
}

func TestWeakFootCost(t *testing.T) {
	if got := WeakFootCost(FW, 11); got != 10 {
		t.Fatalf("FW weak-foot 11 cost = %v, want 10", got)
	}
	if got := WeakFootCost(GK, 20); got != 3.8 {
		t.Fatalf("GK weak-foot 20 cost = %v, want 3.8", got)
	}
	if got := ProfilePoolCost(FW, map[Visible]int{Pace: 11}, 11); got != 32 {
		t.Fatalf("ProfilePoolCost = %v, want 32", got)
	}
}

func TestPoolCost(t *testing.T) {
	// A single attribute at 11 with weight 2.2: (11-1) × 2.2 = 22.
	got := PoolCost(FW, map[Visible]int{Pace: 11})
	if got != 22 {
		t.Fatalf("PoolCost = %v, want 22", got)
	}
	// Value at scale minimum consumes nothing.
	if got := PoolCost(FW, map[Visible]int{Pace: 1}); got != 0 {
		t.Fatalf("min value must cost 0, got %v", got)
	}
}

func TestFamiliarityDescriptor(t *testing.T) {
	cases := map[int]string{20: "Natural", 18: "Natural", 15: "Accomplished", 8: "Competent", 3: "Awkward"}
	for v, want := range cases {
		if got := FamiliarityDescriptor(v); got != want {
			t.Errorf("FamiliarityDescriptor(%d) = %s, want %s", v, got, want)
		}
	}
}
