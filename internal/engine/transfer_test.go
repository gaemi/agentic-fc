package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// setupSigning deterministically picks the first free agent an employed manager
// can afford, gives that manager an ABSOLUTE SIGN directive on them, and returns
// the ids. Same seed ⇒ same pair, so two engines set up identically.
func setupSigning(t *testing.T, e *Engine) (faID, mgrID int64) {
	t.Helper()
	for fi := range e.world.Players {
		fa := &e.world.Players[fi]
		if fa.ClubID != 0 {
			continue
		}
		for mi := range e.world.Managers {
			m := &e.world.Managers[mi]
			if m.ClubID == 0 {
				continue
			}
			club := e.clubs[m.ClubID]
			demand := worldgen.WageDemandMinor(e.world.Config, e.world.Derived, club.DivisionTier, fa.AbilityPool, fa.Reputation)
			if club.WageBillWeeklyMinor+demand <= club.WageBudgetWeeklyMinor {
				m.Mindset.Directives = append(m.Mindset.Directives, mindset.Directive{
					ID: "sign1", Verb: mindset.VerbSign,
					Target: mindset.Target{Player: fa.ID}, Strength: mindset.StrengthAbsolute,
				})
				return fa.ID, m.ID
			}
		}
	}
	t.Fatal("no affording free-agent / manager pair in the generated world")
	return 0, 0
}

// TestFreeAgentSigning is the slice-1 end-to-end: a SIGN directive on a free
// agent, during the open (pre-season) window, moves the player to the club,
// writes a contract, grows the wage bill by exactly that wage, and files the
// completed-transfer news.
func TestFreeAgentSigning(t *testing.T) {
	e, _ := newEngine(t, 42)
	faID, mgrID := setupSigning(t, e)
	fa, m := e.players[faID], e.managers[mgrID]
	club := e.clubs[m.ClubID]
	billBefore := club.WageBillWeeklyMinor

	if _, err := e.RunUntil(day(20)); err != nil { // covers several decision rolls, window open
		t.Fatal(err)
	}

	if fa.ClubID != m.ClubID {
		t.Fatalf("free agent %d not signed: ClubID=%d, want %d", faID, fa.ClubID, m.ClubID)
	}
	if fa.Contract == nil {
		t.Fatal("signing wrote no contract")
	}
	// A season-1 signing of an N-season deal expires at end of season N
	// (inclusive: 1 + N − 1). Locks the expiry off-by-one.
	if fa.Contract.ExpirySeasonYear != contractYearsSigned {
		t.Fatalf("contract expiry season %d, want %d", fa.Contract.ExpirySeasonYear, contractYearsSigned)
	}
	if club.WageBillWeeklyMinor != billBefore+fa.Contract.WageWeeklyMinor {
		t.Fatalf("wage bill %d != before %d + wage %d", club.WageBillWeeklyMinor, billBefore, fa.Contract.WageWeeklyMinor)
	}
	found := false
	for _, n := range e.world.News {
		if n.Key == transferCompletedKey {
			found = true
		}
	}
	if !found {
		t.Fatal("no news.transfer.completed filed")
	}
}

// TestSigningWindowGated locks the derived window gate: a decision roll dated
// outside both windows never moves the player, and the gate is genuinely
// time-derived (open early season, shut mid-autumn).
func TestSigningWindowGated(t *testing.T) {
	e, _ := newEngine(t, 42)
	faID, mgrID := setupSigning(t, e)
	m := e.managers[mgrID]

	if worldgen.TransferWindowOpenAt(day(100)) { // ~October, between the two windows
		t.Fatal("expected the transfer window shut at day 100")
	}
	if !worldgen.TransferWindowOpenAt(day(10)) { // early season summer window
		t.Fatal("expected the summer window open at day 10")
	}

	ev := &sim.Event{Due: day(100), Kind: sim.KindManager, EntityID: m.ID, Payload: worldgen.PayloadDecisionRoll}
	if e.considerSignings(ev, m) {
		t.Fatal("signed while the window was closed")
	}
	if e.players[faID].ClubID != 0 {
		t.Fatalf("free agent moved while the window was closed (ClubID=%d)", e.players[faID].ClubID)
	}
}

// TestSigningDeterministicResume is the phase invariant: a run that snapshots to
// disk mid-window and resumes reaches the identical world hash as the
// uninterrupted run — the signing (player move + finances + contract, all in
// World) survives a restart and reproduces exactly (NFR-2, FR-28).
func TestSigningDeterministicResume(t *testing.T) {
	const seed = 42
	ea, _ := newEngine(t, seed)
	faID, _ := setupSigning(t, ea)
	horizon := day(20)
	if _, err := ea.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}
	if ea.players[faID].ClubID == 0 {
		t.Fatal("test vacuous: the uninterrupted run signed no one")
	}

	eb, _ := newEngine(t, seed)
	setupSigning(t, eb) // identical setup (same seed ⇒ same pair + directive)
	if _, err := eb.RunUntil(day(2)); err != nil {
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
	resumed := New(snap.World, sim.RestoreQueue(snap.Queue, snap.QueueNextSeq), &store.MemAuditLog{})
	resumed.ResumeAt(snap.Now)
	if _, err := resumed.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}

	ha, _ := ea.World().Hash()
	hb, _ := resumed.World().Hash()
	if ha != hb {
		t.Fatalf("signing resume diverged from the uninterrupted run:\nA %s\nB %s", ha, hb)
	}
}

// employedManagers returns n manager ids each bound to a DISTINCT club.
func employedManagers(t *testing.T, e *Engine, n int) []int64 {
	t.Helper()
	var out []int64
	seen := map[int64]bool{}
	for mi := range e.world.Managers {
		m := &e.world.Managers[mi]
		if m.ClubID != 0 && !seen[m.ClubID] {
			seen[m.ClubID] = true
			out = append(out, m.ID)
			if len(out) == n {
				return out
			}
		}
	}
	t.Fatalf("need %d employed clubs, found %d", n, len(out))
	return nil
}

// feeMovesFor counts completed fee transfers naming this player — used to prove a
// contested player is not re-bought (each re-buy would file another fee news).
func feeMovesFor(e *Engine, name string) int {
	n := 0
	for i := range e.world.News {
		if e.world.News[i].Key == transferFeeCompletedKey && e.world.News[i].Params["player"] == name {
			n++
		}
	}
	return n
}

// TestFreeAgentContestFirstRollWins locks "first roll wins" for the free-agent
// path: two clubs both SIGN the same free agent; whoever's roll
// fires first signs them, and the loser — for whom the player is now a contracted,
// UNLISTED signing — cannot fee-buy them, so there is no re-buy.
func TestFreeAgentContestFirstRollWins(t *testing.T) {
	e, _ := newEngine(t, 42)
	var fa int64
	for i := range e.world.Players {
		if e.world.Players[i].ClubID == 0 && !e.world.Players[i].Youth {
			fa = e.world.Players[i].ID
			break
		}
	}
	if fa == 0 {
		t.Fatal("no free agent")
	}
	mids := employedManagers(t, e, 2)
	for _, mid := range mids {
		mgr := e.managers[mid]
		e.clubs[mgr.ClubID].WageBudgetWeeklyMinor = 1 << 50
		e.clubs[mgr.ClubID].TransferBudgetMinor = 1 << 50
		mgr.Mindset.Directives = append(mgr.Mindset.Directives, mindset.Directive{
			ID: "buy", Verb: mindset.VerbSign, Target: mindset.Target{Player: fa}, Strength: mindset.StrengthAbsolute,
		})
	}
	c1, c2 := e.managers[mids[0]].ClubID, e.managers[mids[1]].ClubID

	if _, err := e.RunUntil(day(30)); err != nil {
		t.Fatal(err)
	}
	p := e.players[fa]
	if p.ClubID != c1 && p.ClubID != c2 {
		t.Fatalf("contested free agent not signed by either contender: ClubID=%d", p.ClubID)
	}
	if got := feeMovesFor(e, p.Name); got != 0 {
		t.Fatalf("contested free agent was re-bought via a fee %d time(s) — the poach reopened", got)
	}
}

// TestFeeContestFirstRollWins locks "first roll wins" for the fee path: a player
// explicitly SELL-listed by their club is chased by two others; the first roll
// completes the fee move, and the loser cannot re-buy them (a buyer never
// re-lists its signing, so exactly one fee move happens).
func TestFeeContestFirstRollWins(t *testing.T) {
	e, _ := newEngine(t, 42)
	mids := employedManagers(t, e, 3)
	seller := e.managers[mids[0]]
	var x int64
	for i := range e.world.Players {
		if e.world.Players[i].ClubID == seller.ClubID && !e.world.Players[i].Youth {
			x = e.world.Players[i].ID
			break
		}
	}
	if x == 0 {
		t.Fatal("seller has no senior to list")
	}
	seller.Mindset.Directives = append(seller.Mindset.Directives, mindset.Directive{
		ID: "sell", Verb: mindset.VerbSell, Target: mindset.Target{Player: x}, Strength: mindset.StrengthAbsolute,
	})
	for _, mid := range mids[1:] {
		mgr := e.managers[mid]
		e.clubs[mgr.ClubID].WageBudgetWeeklyMinor = 1 << 50
		e.clubs[mgr.ClubID].TransferBudgetMinor = 1 << 50
		mgr.Mindset.Directives = append(mgr.Mindset.Directives, mindset.Directive{
			ID: "buy", Verb: mindset.VerbSign, Target: mindset.Target{Player: x}, Strength: mindset.StrengthAbsolute,
		})
	}
	c1, c2 := e.managers[mids[1]].ClubID, e.managers[mids[2]].ClubID

	if _, err := e.RunUntil(day(30)); err != nil {
		t.Fatal(err)
	}
	p := e.players[x]
	if p.ClubID != c1 && p.ClubID != c2 {
		t.Fatalf("listed player not bought by either contender: ClubID=%d", p.ClubID)
	}
	if got := feeMovesFor(e, p.Name); got != 1 {
		t.Fatalf("listed player moved via fee %d times, want exactly 1 (no re-buy)", got)
	}
}

// TestYouthNotTransferable locks the youth guard: even a SELL-listed
// youth cannot be signed away — the engine rejects the move regardless.
func TestYouthNotTransferable(t *testing.T) {
	e, _ := newEngine(t, 42)
	mids := employedManagers(t, e, 2)
	seller, buyerMgr := e.managers[mids[0]], e.managers[mids[1]]
	var y int64
	for i := range e.world.Players {
		if p := &e.world.Players[i]; p.ClubID == seller.ClubID && !p.Youth {
			p.Youth, y = true, p.ID // manufacture a youth
			break
		}
	}
	seller.Mindset.Directives = append(seller.Mindset.Directives, mindset.Directive{
		ID: "sell", Verb: mindset.VerbSell, Target: mindset.Target{Player: y}, Strength: mindset.StrengthAbsolute,
	})
	buyer := e.clubs[buyerMgr.ClubID]
	buyer.WageBudgetWeeklyMinor, buyer.TransferBudgetMinor = 1<<50, 1<<50
	ev := &sim.Event{Due: day(10), Kind: sim.KindManager, EntityID: buyerMgr.ID, Payload: worldgen.PayloadDecisionRoll}
	d := mindset.Directive{Verb: mindset.VerbSign, Target: mindset.Target{Player: y}, Strength: mindset.StrengthAbsolute}
	if e.attemptSigning(ev, buyer, d) {
		t.Fatal("signed a youth")
	}
	if e.players[y].ClubID != seller.ClubID {
		t.Fatal("youth was transferred despite the guard")
	}
}

// setupFeeTransfer deterministically picks the first contracted player that an
// employed manager of ANOTHER club can afford (fee + wage), gives that manager an
// ABSOLUTE SIGN on the player, and lists the player for sale (ABSOLUTE SELL by the
// selling club's manager) so acceptance is high. Same seed ⇒ same pair, so two
// engines set up identically.
func setupFeeTransfer(t *testing.T, e *Engine) (targetID, buyerMgrID int64) {
	t.Helper()
	for pi := range e.world.Players {
		p := &e.world.Players[pi]
		if p.ClubID == 0 {
			continue // need a contracted player, not a free agent
		}
		for mi := range e.world.Managers {
			m := &e.world.Managers[mi]
			if m.ClubID == 0 || m.ClubID == p.ClubID {
				continue // employed, and buying from a different club
			}
			buyer := e.clubs[m.ClubID]
			wage := worldgen.WageDemandMinor(e.world.Config, e.world.Derived, buyer.DivisionTier, p.AbilityPool, p.Reputation)
			if buyer.WageBillWeeklyMinor+wage > buyer.WageBudgetWeeklyMinor {
				continue
			}
			if worldgen.TransferValuationMinor(p.AbilityPool) > buyer.TransferBudgetMinor {
				continue
			}
			m.Mindset.Directives = append(m.Mindset.Directives, mindset.Directive{
				ID: "buy1", Verb: mindset.VerbSign,
				Target: mindset.Target{Player: p.ID}, Strength: mindset.StrengthAbsolute,
			})
			seller := e.clubManager(p.ClubID)
			seller.Mindset.Directives = append(seller.Mindset.Directives, mindset.Directive{
				ID: "sell1", Verb: mindset.VerbSell,
				Target: mindset.Target{Player: p.ID}, Strength: mindset.StrengthAbsolute,
			})
			return p.ID, m.ID
		}
	}
	t.Fatal("no affording buyer / contracted target pair in the generated world")
	return 0, 0
}

// totalBalanceMinor sums every club's balance — the money-conservation probe.
func totalBalanceMinor(e *Engine) int64 {
	var sum int64
	for i := range e.world.Clubs {
		sum += e.world.Clubs[i].BalanceMinor
	}
	return sum
}

// TestFeeTransfer is the slice-2a end-to-end: a SIGN on a contracted, SELL-listed
// player moves them for a fee. The buyer pays the valuation from its balance and
// transfer budget, the selling club banks it and sheds the outgoing wage, the
// buyer takes on the repriced wage, and total money is conserved.
func TestFeeTransfer(t *testing.T) {
	e, _ := newEngine(t, 42)
	targetID, buyerMgrID := setupFeeTransfer(t, e)
	p := e.players[targetID]
	buyer := e.clubs[e.managers[buyerMgrID].ClubID]
	seller := e.clubs[p.ClubID]
	sellerID := seller.ID

	fee := worldgen.TransferValuationMinor(p.AbilityPool)
	wage := worldgen.WageDemandMinor(e.world.Config, e.world.Derived, buyer.DivisionTier, p.AbilityPool, p.Reputation)
	oldWage := p.Contract.WageWeeklyMinor
	buyerBal, buyerTB, buyerBill := buyer.BalanceMinor, buyer.TransferBudgetMinor, buyer.WageBillWeeklyMinor
	sellerBal, sellerTB, sellerBill := seller.BalanceMinor, seller.TransferBudgetMinor, seller.WageBillWeeklyMinor
	totalBefore := totalBalanceMinor(e)

	// Execute the move directly (no RunUntil) so the economy's finance ticks —
	// weekly revenue credited and wages debited over a multi-day run — don't
	// perturb the balances; the transaction's money mechanics are asserted in
	// isolation. The directive → decision-roll → move path through the loop is
	// covered end-to-end by TestFeeTransferResumeDeterminism below.
	sink := &captureSink{}
	e.SetSink(sink)
	ev := &sim.Event{Due: day(10), Kind: sim.KindManager, EntityID: buyerMgrID, Payload: worldgen.PayloadDecisionRoll}
	e.executeTransfer(ev, buyer, seller, p, fee, wage)

	if p.ClubID != buyer.ID {
		t.Fatalf("target %d not signed: ClubID=%d, want %d", targetID, p.ClubID, buyer.ID)
	}
	if p.Contract == nil || p.Contract.WageWeeklyMinor != wage {
		t.Fatalf("contract wage = %v, want %d", p.Contract, wage)
	}
	if p.Contract.ExpirySeasonYear != contractYearsSigned {
		t.Fatalf("contract expiry season %d, want %d", p.Contract.ExpirySeasonYear, contractYearsSigned)
	}
	// Buyer side.
	if buyer.BalanceMinor != buyerBal-fee || buyer.TransferBudgetMinor != buyerTB-fee {
		t.Fatalf("buyer money: bal %d (want %d), tb %d (want %d)",
			buyer.BalanceMinor, buyerBal-fee, buyer.TransferBudgetMinor, buyerTB-fee)
	}
	if buyer.WageBillWeeklyMinor != buyerBill+wage {
		t.Fatalf("buyer wage bill %d, want %d", buyer.WageBillWeeklyMinor, buyerBill+wage)
	}
	// Seller side — including the wage-bill shed slice 1 never exercised.
	if seller.BalanceMinor != sellerBal+fee || seller.TransferBudgetMinor != sellerTB+fee {
		t.Fatalf("seller money: bal %d (want %d), tb %d (want %d)",
			seller.BalanceMinor, sellerBal+fee, seller.TransferBudgetMinor, sellerTB+fee)
	}
	if seller.WageBillWeeklyMinor != sellerBill-oldWage {
		t.Fatalf("seller wage bill %d, want %d (shed old wage %d)", seller.WageBillWeeklyMinor, sellerBill-oldWage, oldWage)
	}
	// Money conservation: the fee only moved between the two clubs.
	if got := totalBalanceMinor(e); got != totalBefore {
		t.Fatalf("world balance not conserved: %d != %d", got, totalBefore)
	}
	found := false
	for i := range e.world.News {
		n := &e.world.News[i]
		if n.Key != transferFeeCompletedKey {
			continue
		}
		// The persisted, agent-readable news carries only PUBLIC params. The
		// pool-derived fee/wage must NOT sit here — they'd invert to the hidden
		// Ability Pool, and an int in a map[string]any rehydrates as a float on a
		// snapshot load (determinism). They ride on the human emit feed instead.
		if n.Params["fee"] != nil || n.Params["wage"] != nil {
			t.Fatalf("persisted agent news leaks money: %v", n.Params)
		}
		if n.Params["player"] != p.Name || n.Params["from"] != seller.Name {
			t.Fatalf("news dropped its public params: %v", n.Params)
		}
		if len(n.ClubIDs) != 2 || n.ClubIDs[0] != buyer.ID || n.ClubIDs[1] != sellerID {
			t.Fatalf("fee news refs = %v, want [%d %d]", n.ClubIDs, buyer.ID, sellerID)
		}
		found = true
	}
	if !found {
		t.Fatal("no news.transfer.fee_completed filed")
	}

	// The human Console feed DID carry the money (it renders from the emit event,
	// not the agent news ring) — humans see the fee, agents don't.
	var feedFee, feedWage any
	for _, fe := range sink.events {
		if fe.Key == transferFeeCompletedKey {
			feedFee, feedWage = fe.Params["fee"], fe.Params["wage"]
		}
	}
	if feedFee != fee || feedWage != wage {
		t.Fatalf("emit feed money = fee %v / wage %v, want %d / %d", feedFee, feedWage, fee, wage)
	}
}

// captureSink records feed events so a test can assert what the human Console
// surface receives (the agent surface is World.News, a separate path).
type captureSink struct{ events []FeedEvent }

func (c *captureSink) OnFeedEvent(e FeedEvent) { c.events = append(c.events, e) }

// TestFeeTransferBudgetGate locks the transfer-budget gate: a fee that exceeds
// the buyer's TransferBudgetMinor is rejected before any acceptance roll, and the
// player stays put — a club can't spend money it hasn't been given for transfers.
func TestFeeTransferBudgetGate(t *testing.T) {
	e, _ := newEngine(t, 42)
	targetID, buyerMgrID := setupFeeTransfer(t, e)
	p := e.players[targetID]
	sellerBefore := p.ClubID
	buyer := e.clubs[e.managers[buyerMgrID].ClubID]
	buyer.TransferBudgetMinor = worldgen.TransferValuationMinor(p.AbilityPool) - 1 // one Crown short

	ev := &sim.Event{Due: day(10), Kind: sim.KindManager, EntityID: buyerMgrID, Payload: worldgen.PayloadDecisionRoll}
	d := mindset.Directive{Verb: mindset.VerbSign, Target: mindset.Target{Player: targetID}, Strength: mindset.StrengthAbsolute}
	if e.attemptSigning(ev, buyer, d) {
		t.Fatal("signed despite the fee exceeding the transfer budget")
	}
	if p.ClubID != sellerBefore {
		t.Fatalf("player moved despite the budget gate: ClubID=%d, want %d", p.ClubID, sellerBefore)
	}
}

// TestFeeTransferResumeDeterminism extends the phase invariant to an inter-club
// fee move: a run that snapshots mid-window and resumes reaches the identical
// world hash as the uninterrupted run — the move (both clubs' money + wage bills,
// the player's club + contract) survives a restart and reproduces exactly.
func TestFeeTransferResumeDeterminism(t *testing.T) {
	const seed = 42
	ea, _ := newEngine(t, seed)
	targetID, buyerMgrID := setupFeeTransfer(t, ea)
	buyerClubID := ea.managers[buyerMgrID].ClubID
	horizon := day(20)
	if _, err := ea.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}
	if ea.players[targetID].ClubID != buyerClubID {
		t.Fatal("test vacuous: the uninterrupted run completed no fee move")
	}

	eb, _ := newEngine(t, seed)
	setupFeeTransfer(t, eb) // identical setup (same seed ⇒ same pair + directives)
	if _, err := eb.RunUntil(day(2)); err != nil {
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
	resumed := New(snap.World, sim.RestoreQueue(snap.Queue, snap.QueueNextSeq), &store.MemAuditLog{})
	resumed.ResumeAt(snap.Now)
	if _, err := resumed.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}

	ha, _ := ea.World().Hash()
	hb, _ := resumed.World().Hash()
	if ha != hb {
		t.Fatalf("fee-move resume diverged from the uninterrupted run:\nA %s\nB %s", ha, hb)
	}
}

// TestFeeTransferClearsStaleSell locks the poach defence: when a
// club signs a player it happens to hold a stale SELL on, that SELL is cleared —
// so the new signing is not instantly re-listed for a later rival roll to poach.
func TestFeeTransferClearsStaleSell(t *testing.T) {
	e, _ := newEngine(t, 42)
	targetID, buyerMgrID := setupFeeTransfer(t, e)
	p := e.players[targetID]
	buyerMgr := e.managers[buyerMgrID]
	buyer := e.clubs[buyerMgr.ClubID]
	seller := e.clubs[p.ClubID]
	// The buyer pre-holds a SELL on the very player it is about to sign (a stale
	// order — e.g. from a past spell owning them).
	buyerMgr.Mindset.Directives = append(buyerMgr.Mindset.Directives, mindset.Directive{
		ID: "stale-sell", Verb: mindset.VerbSell,
		Target: mindset.Target{Player: targetID}, Strength: mindset.StrengthLean,
	})
	fee := worldgen.TransferValuationMinor(p.AbilityPool)
	wage := worldgen.WageDemandMinor(e.world.Config, e.world.Derived, buyer.DivisionTier, p.AbilityPool, p.Reputation)
	ev := &sim.Event{Due: day(10), Kind: sim.KindManager, EntityID: buyerMgrID, Payload: worldgen.PayloadDecisionRoll}
	e.executeTransfer(ev, buyer, seller, p, fee, wage)

	if _, listed := e.sellListed(buyer.ID, targetID); listed {
		t.Fatal("the new signing is still SELL-listed by its buyer — poach defence failed")
	}
	for _, d := range buyerMgr.Mindset.Directives {
		if d.Verb == mindset.VerbSell && d.Target.Player == targetID {
			t.Fatal("stale SELL directive on the signing was not cleared")
		}
	}
}

// TestFeeTransferNewsSurvivesSnapshot locks the determinism side of keeping money
// off the persisted news ring: a completed fee transfer round-trips through a
// snapshot with an identical world hash — the news params are pure strings, so
// nothing rehydrates as a float via json.Unmarshal.
func TestFeeTransferNewsSurvivesSnapshot(t *testing.T) {
	e, _ := newEngine(t, 42)
	targetID, buyerMgrID := setupFeeTransfer(t, e)
	p := e.players[targetID]
	buyer := e.clubs[e.managers[buyerMgrID].ClubID]
	seller := e.clubs[p.ClubID]
	fee := worldgen.TransferValuationMinor(p.AbilityPool)
	wage := worldgen.WageDemandMinor(e.world.Config, e.world.Derived, buyer.DivisionTier, p.AbilityPool, p.Reputation)
	ev := &sim.Event{Due: day(10), Kind: sim.KindManager, EntityID: buyerMgrID, Payload: worldgen.PayloadDecisionRoll}
	e.executeTransfer(ev, buyer, seller, p, fee, wage)

	before, err := e.World().Hash()
	if err != nil {
		t.Fatal(err)
	}
	fstore := &store.FileStore{Dir: t.TempDir()}
	events, nextSeq := e.Queue().Snapshot()
	if err := fstore.SaveSnapshot(&store.Snapshot{
		Now: e.Now(), World: e.World(), Queue: events, QueueNextSeq: nextSeq,
	}); err != nil {
		t.Fatal(err)
	}
	snap, err := fstore.LoadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	after, err := snap.World.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("fee-transfer news did not survive snapshot round-trip:\n%s\n%s", before, after)
	}
}
