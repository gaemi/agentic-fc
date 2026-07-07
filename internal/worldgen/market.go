package worldgen

import (
	"sort"

	"github.com/gaemi/agentic-fc/internal/mindset"
)

// The transfer-market predicates. The two "on the market"
// notions are DELIBERATELY separate — they are not one rule:
//
//   - SurplusListed — a club's spare players. Convergence-safe: only the excess
//     beyond target is ever exposed, so a buyer taking one leaves the seller at or
//     above target. This is the ONLY contracted supply the autonomous market buys.
//   - ExplicitlyListed — players an agent has deliberately put up for sale. Can
//     come from a club of any size, so it must NOT feed the autonomous buyer (that
//     would let the sim pull an at-target seller into deficit and break the
//     convergence proof). It is the agent-facing market (explicit SIGN + search).
//
// Each rule lives in exactly one place so the callers can't drift; both exclude
// youth (never counted, sold, or exposed) and break ties by id, so both are fully
// deterministic (NFR-2).

// SquadSize counts a club's SENIOR (non-youth) players — the figure the transfer
// systems compare against SquadSizeTarget.
func SquadSize(w *World, clubID int64) int {
	if clubID == 0 {
		return 0
	}
	n := 0
	for i := range w.Players {
		if w.Players[i].ClubID == clubID && !w.Players[i].Youth {
			n++
		}
	}
	return n
}

// SurplusListed is the set of players a strict-surplus club has spare: its
// lowest-Ability-Pool senior EXCESS (squad − target). By construction the selling
// club stays at or above target, so an autonomous fee move buying one can never
// create a new deficit — the property the market's convergence rests on.
// Derived each call, never stored, so a club that falls back to target lists none.
func SurplusListed(w *World) map[int64]bool {
	byClub := map[int64][]*Player{}
	for i := range w.Players {
		p := &w.Players[i]
		if p.Youth || p.ClubID == 0 {
			continue
		}
		byClub[p.ClubID] = append(byClub[p.ClubID], p)
	}
	listed := map[int64]bool{}
	target := w.Config.SquadSizeTarget
	for _, squad := range byClub {
		if len(squad) <= target {
			continue // not strict surplus
		}
		sort.Slice(squad, func(a, b int) bool {
			if squad[a].AbilityPool != squad[b].AbilityPool {
				return squad[a].AbilityPool < squad[b].AbilityPool
			}
			return squad[a].ID < squad[b].ID
		})
		for k := 0; k < len(squad)-target; k++ {
			listed[squad[k].ID] = true
		}
	}
	return listed
}

// ExplicitlyListed is the set of players a club has deliberately put up for sale —
// a SELL directive by their OWN club's manager on a non-youth player. This is the
// agent-facing market (what an explicit SIGN completes and search_players
// surfaces), kept apart from SurplusListed so it never feeds the autonomous buyer.
func ExplicitlyListed(w *World) map[int64]bool {
	club := make(map[int64]int64, len(w.Players))
	youth := make(map[int64]bool, len(w.Players))
	for i := range w.Players {
		club[w.Players[i].ID] = w.Players[i].ClubID
		youth[w.Players[i].ID] = w.Players[i].Youth
	}
	listed := map[int64]bool{}
	for i := range w.Managers {
		m := &w.Managers[i]
		if m.ClubID == 0 {
			continue
		}
		for _, d := range m.Mindset.Directives {
			if d.Verb == mindset.VerbSell && d.Target.Player != 0 &&
				club[d.Target.Player] == m.ClubID && !youth[d.Target.Player] {
				listed[d.Target.Player] = true
			}
		}
	}
	return listed
}
