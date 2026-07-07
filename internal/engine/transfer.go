package engine

import (
	"fmt"
	"sort"

	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/rng"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Transfers. A buyer's SIGN directive completes a move during
// an open transfer window, resolved INLINE in the manager's decision roll
// (handleDecisionRoll), which the queue totally orders by (game_time, priority,
// kind, id, seq). So two clubs chasing the same player settle by queue order for
// free — the first roll to fire signs them, later rolls find the target gone.
//
//   - slice 1 — free agents (ClubID 0): zero fee, zero selling club; the player's
//     willingness scales with the directive's strength.
//   - slice 2 — contracted players: an inter-club FEE move. The buyer pays the
//     valuation (worldgen.TransferValuationMinor) from its transfer budget; the
//     selling club banks it and sheds the outgoing wage; the deal completes only
//     if the selling club accepts (readily if it has SELL-listed the player).
//
// Every move is one atomic transaction within the single event, integer
// minor-units only (no float in the hash), and all state lives in World, so
// snapshot/resume reproduce it. Fee flows buyer→seller only (no bonuses yet), so
// the world's total BalanceMinor is conserved.

const (
	transferCompletedKey    = "news.transfer.completed"     // free agent
	transferFeeCompletedKey = "news.transfer.fee_completed" // inter-club fee move
)

// contractYearsSigned is the length (seasons) of a signing's contract.
const contractYearsSigned = 3

// signAcceptBase is a free agent's acceptance probability by the buyer's
// directive strength — a firmer SIGN pushes the manager's follow-through and the
// player's willingness (a free agent has no rival bid to lose to).
var signAcceptBase = map[mindset.Strength]float64{
	mindset.StrengthLean:     0.45,
	mindset.StrengthInsist:   0.75,
	mindset.StrengthAbsolute: 0.95,
}

// sellListedAccept is the selling club's acceptance when it has SELL-listed the
// player, by the SELL directive's strength — a club that wants to sell parts
// readily. In slice 2a a fee move requires a listing (only listed players are on
// the market), so this is the fee path's sole acceptance curve; unsolicited bids
// on unlisted players are not part of the current market model.
var sellListedAccept = map[mindset.Strength]float64{
	mindset.StrengthLean:     0.60,
	mindset.StrengthInsist:   0.85,
	mindset.StrengthAbsolute: 0.97,
}

// considerSignings acts on the manager's SIGN directives while the window is
// open, completing at most one signing per decision roll (buy OR — from slice 2 —
// a fee move). Contention resolves by queue order: the first roll to fire moves
// the player. It runs off its own per-(buyer,target) streams, so the decision
// roll's own dice (the reschedule jitter) are untouched — only club/player state
// moves.
func (e *Engine) considerSignings(ev *sim.Event, m *worldgen.Manager) bool {
	if m.ClubID == 0 || !worldgen.TransferWindowOpenAt(ev.Due) {
		return false
	}
	buyer := e.clubs[m.ClubID]
	if buyer == nil {
		return false
	}
	for _, d := range signDirectives(m) {
		if e.attemptSigning(ev, buyer, d) {
			return true // one transaction per roll
		}
	}
	return false
}

// signDirectives returns one SIGN directive per targeted player — the strongest
// (first-seen on a tie) — strongest overall first with an id tie-break. Deduping
// by player matters: two SIGN directives on the same target must not buy two
// acceptance rolls (that would inflate the intended probability). The result is a
// total order, so the pick is deterministic regardless of insertion order.
func signDirectives(m *worldgen.Manager) []mindset.Directive {
	best := map[int64]mindset.Directive{}
	for _, d := range m.Mindset.Directives {
		if d.Verb != mindset.VerbSign || d.Target.Player == 0 {
			continue
		}
		if cur, ok := best[d.Target.Player]; !ok || strengthRank(d.Strength) > strengthRank(cur.Strength) {
			best[d.Target.Player] = d
		}
	}
	out := make([]mindset.Directive, 0, len(best))
	for _, d := range best {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if ri, rj := strengthRank(out[i].Strength), strengthRank(out[j].Strength); ri != rj {
			return ri > rj
		}
		return out[i].Target.Player < out[j].Target.Player
	})
	return out
}

// attemptSigning completes one signing of directive d's target for the buyer, if
// the target is signable, the buyer can afford it, and the deal is accepted.
// Draws from its own per-(buyer,target) stream so adding or removing a candidate
// never shifts another candidate's acceptance dice. Returns whether it signed.
func (e *Engine) attemptSigning(ev *sim.Event, buyer *worldgen.Club, d mindset.Directive) bool {
	p := e.players[d.Target.Player]
	if p == nil || p.ClubID == buyer.ID || p.Youth || p.Retired {
		return false // gone, already ours, untransferable youth, or retired
	}
	// The incoming wage is repriced at the buyer's tier; the buyer must have room
	// on its wage budget either way (free or fee).
	wage := worldgen.WageDemandMinor(e.world.Config, e.world.Derived, buyer.DivisionTier, p.AbilityPool, p.Reputation)
	if buyer.WageBillWeeklyMinor+wage > buyer.WageBudgetWeeklyMinor {
		return false
	}
	r := rng.Stream(e.world.Config.Seed, fmt.Sprintf("transfer/bid/%d->%d@%d", buyer.ID, p.ID, int64(ev.Due)))

	if p.ClubID == 0 {
		if r.Float64() >= signAcceptBase[d.Strength] {
			return false // the deal falls through this time
		}
		e.executeTransfer(ev, buyer, nil, p, 0, wage)
		return true
	}

	// Contracted — an inter-club fee move needs the selling club to have EXPLICITLY
	// listed the player (a SELL directive). Surplus-derived availability feeds ONLY
	// the autonomous buyer, never an explicit SIGN — mixing it in here would let a
	// just-signed low-pool player become its buyer's castoff and be re-bought,
	// reopening the poach. Explicit-only keeps "first roll wins": once
	// bought the player sits at the buyer, unlisted, so later rival rolls skip.
	seller := e.clubs[p.ClubID]
	if seller == nil {
		return false
	}
	strength, listed := e.sellListed(seller.ID, p.ID)
	if !listed {
		return false
	}
	fee := worldgen.TransferValuationMinor(p.AbilityPool)
	if fee > buyer.TransferBudgetMinor {
		return false
	}
	if r.Float64() >= sellListedAccept[strength] {
		return false
	}
	e.executeTransfer(ev, buyer, seller, p, fee, wage)
	return true
}

// assessSquadNeeds is the autonomous market's sole action: a
// club in STRICT squad deficit signs the best player it can afford — the highest
// Ability Pool among free agents and SURPLUS-derived castoffs — deterministically,
// no dice. It fires only below target, so it is a FLOOR, never
// interference: an agent keeping its squad at or above target is never bought for
// by the sim. One acquisition per roll, mirroring the directive path.
//
// It buys ONLY free agents and surplus castoffs — never an explicit SELL listing,
// which can come from an at-target club and would pull that seller into deficit.
// So no autonomous move creates a new deficit: the world's total squad deficit
// Σ max(0, target − size) is monotone non-increasing and the market settles at a
// fixed point (bounded by free-agent supply and budgets). (An explicit SELL by an
// at-target club, taken by another agent's explicit SIGN, MAY create a deficit —
// that is an exogenous agent choice, which the autonomous market then fills.)
func (e *Engine) assessSquadNeeds(ev *sim.Event, m *worldgen.Manager) bool {
	if m.ClubID == 0 || !worldgen.TransferWindowOpenAt(ev.Due) {
		return false
	}
	buyer := e.clubs[m.ClubID]
	if buyer == nil || worldgen.SquadSize(e.world, buyer.ID) >= e.world.Config.SquadSizeTarget {
		return false // at or above target — never buy over an agent's own floor
	}
	best := e.bestAffordableTarget(buyer, worldgen.SurplusListed(e.world))
	if best == nil {
		return false
	}
	wage := worldgen.WageDemandMinor(e.world.Config, e.world.Derived, buyer.DivisionTier, best.AbilityPool, best.Reputation)
	return e.acquire(ev, buyer, best, wage)
}

// bestAffordableTarget picks the highest-Ability-Pool player the buyer can afford
// — wage now, plus the fee for a contracted surplus castoff — scanning free agents
// and surplus-derived players in id order. Ties break to the lowest id, so the
// choice is deterministic (NFR-2). Excludes youth, retired players (a retired
// free agent shares ClubID 0 with the signable pool — the flag is the only thing
// separating them), and the buyer's own players.
func (e *Engine) bestAffordableTarget(buyer *worldgen.Club, surplus map[int64]bool) *worldgen.Player {
	var best *worldgen.Player
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.Youth || p.Retired || p.ClubID == buyer.ID {
			continue
		}
		if p.ClubID != 0 && !surplus[p.ID] {
			continue // contracted but not a surplus castoff
		}
		wage := worldgen.WageDemandMinor(e.world.Config, e.world.Derived, buyer.DivisionTier, p.AbilityPool, p.Reputation)
		if buyer.WageBillWeeklyMinor+wage > buyer.WageBudgetWeeklyMinor {
			continue
		}
		if p.ClubID != 0 && worldgen.TransferValuationMinor(p.AbilityPool) > buyer.TransferBudgetMinor {
			continue
		}
		if best == nil || p.AbilityPool > best.AbilityPool ||
			(p.AbilityPool == best.AbilityPool && p.ID < best.ID) {
			best = p
		}
	}
	return best
}

// acquire performs the atomic move for a decided autonomous target: a free agent
// (fee 0) or an on-market contracted player (fee = valuation, paid to the selling
// club). The caller has already checked availability and affordability; a
// surplus/decided sale is accepted outright (the autonomous path takes no dice).
func (e *Engine) acquire(ev *sim.Event, buyer *worldgen.Club, p *worldgen.Player, wage int64) bool {
	if p.ClubID == 0 {
		e.executeTransfer(ev, buyer, nil, p, 0, wage)
		return true
	}
	seller := e.clubs[p.ClubID]
	if seller == nil {
		return false
	}
	e.executeTransfer(ev, buyer, seller, p, worldgen.TransferValuationMinor(p.AbilityPool), wage)
	return true
}

// executeTransfer atomically moves p to the buyer for fee (0 for a free agent),
// paying the selling club if any and repricing the player's wage. All state is in
// World, so snapshot/resume reproduce it; fee flows buyer→seller only, so total
// BalanceMinor is conserved.
func (e *Engine) executeTransfer(ev *sim.Event, buyer, seller *worldgen.Club, p *worldgen.Player, fee, wage int64) {
	oldWage := int64(0)
	if p.Contract != nil {
		oldWage = p.Contract.WageWeeklyMinor
	}
	if seller != nil {
		// The selling club banks the fee (balance and its re-spendable transfer
		// kitty) and sheds the outgoing wage — the seller-side updates slice 1
		// never needed. Symmetric with the buyer below, so a transfer conserves
		// both BalanceMinor and TransferBudgetMinor across the two clubs. (Only
		// per-transfer: the season rollover re-derives every club's budgets from
		// its balance, so TransferBudgetMinor is not conserved across seasons.)
		seller.BalanceMinor += fee
		seller.TransferBudgetMinor += fee
		seller.WageBillWeeklyMinor -= oldWage
	}
	if fee > 0 {
		buyer.BalanceMinor -= fee
		buyer.TransferBudgetMinor -= fee
	}
	p.ClubID = buyer.ID
	p.Contract = &worldgen.Contract{
		WageWeeklyMinor: wage,
		// Expiry is inclusive ("ends June 30 of that season"), so an N-season deal
		// signed in season S runs through S+N−1 (docs/09; the generator's semantics).
		ExpirySeasonYear: worldgen.DateOf(ev.Due).Season + contractYearsSigned - 1,
	}
	buyer.WageBillWeeklyMinor += wage
	// A just-acquired player can't stay on their new owner's sell list: clear any
	// SELL the buyer's manager holds on them, so a stale or pre-emptive listing
	// can't instantly re-list the new signing and let a later rival roll re-poach
	// them (defends "first roll wins"). The former owner's SELL is inert —
	// the player has left them — so it needs no cleanup.
	if bm := e.clubManager(buyer.ID); bm != nil {
		dropSellDirectives(bm, p.ID)
	}

	key := transferCompletedKey
	// World.News is agent-readable, so its params carry only PUBLIC values (names).
	// The pool-derived fee and wage would invert to the hidden Ability Pool, so they
	// ride ONLY on the human Console's emit feed below — never in the persisted news
	// ring. Keeping integer money out of the persisted map[string]any
	// also dodges the int→float64 rehydration a snapshot's json.Unmarshal would cause
	// (determinism: no float on the world-hash path).
	newsParams := map[string]any{"player": p.Name, "club": buyer.Name}
	clubIDs := []int64{buyer.ID}
	outcome := "signed"
	if seller != nil {
		key = transferFeeCompletedKey
		newsParams["from"] = seller.Name
		clubIDs = []int64{buyer.ID, seller.ID}
		outcome = "signed_fee"
	}
	e.world.AddNews(worldgen.NewsItem{
		GameTime: ev.Due, Category: "transfer", Key: key, Params: newsParams, ClubIDs: clubIDs,
	})
	// The human Console feed carries the money too (fee/wage) — it renders from this
	// emit event, a separate non-agent surface, not the news ring. Its own copy, so a
	// retaining observer can't reach persisted state (the determinism contract).
	feedParams := cloneParams(newsParams)
	feedParams["wage"] = wage
	if seller != nil {
		feedParams["fee"] = fee
	}
	e.emit(ev.Due, key, feedParams)
	_ = e.log(ev, "transfer", map[string]any{
		"player": p.ID, "club": buyer.ID, "fee": fee, "wage": wage,
	}, outcome, 0, 0)
}

// dropSellDirectives removes every SELL directive a manager holds on a player,
// through the Mindset's own removal API (which keeps its version and counters
// consistent). Called when the player joins the manager's club — you don't keep a
// standing order to sell someone you just signed.
func dropSellDirectives(m *worldgen.Manager, playerID int64) {
	var ids []string
	for _, d := range m.Mindset.Directives {
		if d.Verb == mindset.VerbSell && d.Target.Player == playerID {
			ids = append(ids, d.ID)
		}
	}
	for _, id := range ids {
		m.Mindset.RemoveDirective(id)
	}
}

// sellListed reports whether the club owning playerID has EXPLICITLY SELL-listed
// them (a SELL directive by its manager), and the strongest such strength. It is
// the gate AND the acceptance weight for an explicit-SIGN fee move (an explicit
// SIGN buys only an explicitly listed player; surplus castoffs go to the
// autonomous buyer alone — worldgen.SurplusListed).
func (e *Engine) sellListed(sellerClubID, playerID int64) (mindset.Strength, bool) {
	m := e.clubManager(sellerClubID)
	if m == nil {
		return "", false
	}
	var best mindset.Strength
	found := false
	for _, d := range m.Mindset.Directives {
		if d.Verb == mindset.VerbSell && d.Target.Player == playerID {
			if !found || strengthRank(d.Strength) > strengthRank(best) {
				best, found = d.Strength, true
			}
		}
	}
	return best, found
}

// clubManager returns the manager of a club (lowest id on the — currently
// impossible — tie), scanning World.Managers. Managers don't move between clubs
// until the careers phase, so a scan needs no maintained index and stays correct
// after a snapshot load.
func (e *Engine) clubManager(clubID int64) *worldgen.Manager {
	var out *worldgen.Manager
	for i := range e.world.Managers {
		m := &e.world.Managers[i]
		if m.ClubID == clubID && (out == nil || m.ID < out.ID) {
			out = m
		}
	}
	return out
}

// cloneParams shallow-copies a params map so an observer can't reach the copy
// persisted in World (transfer params hold only immutable scalars/strings).
func cloneParams(p map[string]any) map[string]any {
	c := make(map[string]any, len(p))
	for k, v := range p {
		c[k] = v
	}
	return c
}

func strengthRank(s mindset.Strength) int {
	switch s {
	case mindset.StrengthAbsolute:
		return 3
	case mindset.StrengthInsist:
		return 2
	case mindset.StrengthLean:
		return 1
	}
	return 0
}
