package worldgen

import (
	"fmt"
	"testing"

	"github.com/gaemi/agentic-fc/internal/rng"
)

// TestGenYouthIntakeCapThrottles locks the academy soft cap (youth intake): repeated
// intakes fill a club's academy up to youthAcademyCap and then stop adding, so the
// youth population is bounded without any deletion (deletion would dangle the player
// ids that news and results reference). Ids stay strictly monotonic throughout.
func TestGenYouthIntakeCapThrottles(t *testing.T) {
	res, err := Generate(PresetCompact(7), WithTokenReader(&counterReader{}))
	if err != nil {
		t.Fatal(err)
	}
	w := res.World
	club := &w.Clubs[0]

	countYouth := func() int {
		n := 0
		for i := range w.Players {
			if w.Players[i].ClubID == club.ID && w.Players[i].Youth {
				n++
			}
		}
		return n
	}

	prevMax := w.NextPlayerID
	sawFull := false
	// 12 batches is well past the cap for any starting academy; the academy must
	// saturate and then throttle to nothing.
	for i := 0; i < 12; i++ {
		r := rng.Stream(w.Config.Seed, fmt.Sprintf("test/youth/%d", i))
		ids := GenYouthIntake(w, r, club, 1)
		if got := countYouth(); got > youthAcademyCap {
			t.Fatalf("academy exceeded the cap after batch %d: %d > %d", i, got, youthAcademyCap)
		}
		for _, id := range ids {
			if id <= prevMax {
				t.Fatalf("intake reused or regressed a player id: %d <= %d", id, prevMax)
			}
			prevMax = id
		}
		if len(ids) == 0 {
			sawFull = true
		}
	}
	if !sawFull {
		t.Fatal("cap never throttled intake within 12 batches")
	}
	if got := countYouth(); got != youthAcademyCap {
		t.Fatalf("saturated academy = %d, want exactly the cap %d", got, youthAcademyCap)
	}
	if w.NextPlayerID != prevMax {
		t.Fatalf("NextPlayerID %d trails the last allocated id %d", w.NextPlayerID, prevMax)
	}
}

// TestGenYouthIntakeSeasonRelativeExpiry locks the regression fix: rollYouth
// stamps contract expiry in season-1-absolute years, so a later-season intake
// must shift it to the live season — a season-3 prospect signs a deal ending in
// season 3 or 4, never one that expired before it was born.
func TestGenYouthIntakeSeasonRelativeExpiry(t *testing.T) {
	res, err := Generate(PresetCompact(9), WithTokenReader(&counterReader{}))
	if err != nil {
		t.Fatal(err)
	}
	w := res.World
	r := rng.Stream(w.Config.Seed, "test/youth/season3")
	for _, id := range GenYouthIntake(w, r, &w.Clubs[0], 3) {
		for i := range w.Players {
			p := &w.Players[i]
			if p.ID != id {
				continue
			}
			if p.Contract == nil || p.Contract.ExpirySeasonYear < 3 || p.Contract.ExpirySeasonYear > 4 {
				t.Fatalf("season-3 intake expiry = %+v, want season 3-4", p.Contract)
			}
		}
	}
}
