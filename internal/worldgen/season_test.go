package worldgen

import "testing"

// TestPromotionRelegationSmallDivisions locks the clamp invariant the test suite flagged:
// with divisions smaller than 2×PromotionSlots, a middle division's top
// (promoted) and bottom (relegated) slots must not overlap, or a middle club is
// swapped twice and division sizes drift. Conservation (each tier keeps its
// club count) is the discriminating check — it fails without the half-size clamp.
func TestPromotionRelegationSmallDivisions(t *testing.T) {
	w := &World{}
	w.Config.Divisions = 3
	w.Derived.PromotionSlots = 2 // > half of a 3-club division: the trap value

	const perDiv = 3
	w.Table = make([][]Standing, w.Config.Divisions)
	for tier := 1; tier <= w.Config.Divisions; tier++ {
		for pos := 1; pos <= perDiv; pos++ {
			id := int64(tier*10 + pos)
			w.Clubs = append(w.Clubs, Club{ID: id, DivisionTier: tier})
			w.Table[tier-1] = append(w.Table[tier-1], Standing{Pos: pos, ClubID: id})
		}
	}

	w.applyPromotionRelegation()

	counts := map[int]int{}
	for _, c := range w.Clubs {
		counts[c.DivisionTier]++
	}
	for tier := 1; tier <= w.Config.Divisions; tier++ {
		if counts[tier] != perDiv {
			t.Fatalf("tier %d has %d clubs after pro/rel, want %d — a middle club was double-swapped", tier, counts[tier], perDiv)
		}
	}
}
