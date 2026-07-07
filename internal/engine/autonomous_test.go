package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// setupDeficit puts the first employed club into a strict squad deficit by
// releasing 3 senior players to free agency, and gives it ample wage room so
// affordability never blocks convergence. Same seed ⇒ same club + same released
// players, so two engines set up identically. Returns the club id.
func setupDeficit(t *testing.T, e *Engine) int64 {
	t.Helper()
	var clubID int64
	for mi := range e.world.Managers {
		if e.world.Managers[mi].ClubID != 0 {
			clubID = e.world.Managers[mi].ClubID
			break
		}
	}
	if clubID == 0 {
		t.Fatal("no employed club")
	}
	club := e.clubs[clubID]
	club.WageBudgetWeeklyMinor = 1 << 50 // ample: affordability never the limiting factor
	released := 0
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.ClubID != clubID || p.Youth {
			continue
		}
		if p.Contract != nil {
			club.WageBillWeeklyMinor -= p.Contract.WageWeeklyMinor
		}
		p.ClubID = 0
		p.Contract = nil
		if released++; released == 3 {
			break
		}
	}
	return clubID
}

// totalDeficit is Φ = Σ max(0, target − size) over all clubs — the quantity the
// autonomous market drives monotonically to a fixed point.
func totalDeficit(e *Engine) int {
	target := e.world.Config.SquadSizeTarget
	sum := 0
	for i := range e.world.Clubs {
		if s := worldgen.SquadSize(e.world, e.world.Clubs[i].ID); s < target {
			sum += target - s
		}
	}
	return sum
}

// TestAutonomousBuyFillsDeficit is the slice-2b convergence invariant: a club in
// strict deficit autonomously signs free agents until it is back at target, the
// world's total deficit Φ is monotone non-increasing at every checkpoint, and the
// market settles at a fixed point (Φ = 0 here, with free agents available and
// wage room ample).
func TestAutonomousBuyFillsDeficit(t *testing.T) {
	e, _ := newEngine(t, 42)
	clubID := setupDeficit(t, e)
	target := e.world.Config.SquadSizeTarget
	if got := worldgen.SquadSize(e.world, clubID); got != target-3 {
		t.Fatalf("setup: squad %d, want %d", got, target-3)
	}
	if totalDeficit(e) != 3 {
		t.Fatalf("setup: Φ = %d, want 3", totalDeficit(e))
	}

	prev := totalDeficit(e)
	for _, d := range []int{5, 10, 20, 40, 55} { // all inside the summer window
		if _, err := e.RunUntil(day(d)); err != nil {
			t.Fatal(err)
		}
		phi := totalDeficit(e)
		if phi > prev {
			t.Fatalf("total squad deficit rose %d→%d by day %d — autonomous market is not convergent", prev, phi, d)
		}
		prev = phi
	}
	if got := worldgen.SquadSize(e.world, clubID); got != target {
		t.Fatalf("deficit club did not converge to target: squad %d, want %d", got, target)
	}
	if phi := totalDeficit(e); phi != 0 {
		t.Fatalf("market did not reach its fixed point: Φ = %d, want 0", phi)
	}
}

// TestAutonomousFeeMove locks the autonomous INTER-CLUB fee path (the fork's
// headline): a strict-deficit club buys a strict-surplus club's on-market castoff
// for a fee, deterministically, with money conserved at the transaction level.
// Free agents are made unattractive so the surplus castoff is the best buy.
func TestAutonomousFeeMove(t *testing.T) {
	e, _ := newEngine(t, 42)

	// Make every free agent unattractive (pool 1) so a real club's castoff wins.
	for i := range e.world.Players {
		if p := &e.world.Players[i]; p.ClubID == 0 && !p.Youth {
			p.AbilityPool = 1
		}
	}

	// Two distinct employed clubs; ample budgets so affordability never blocks.
	var a, b *worldgen.Club
	seen := map[int64]bool{}
	for mi := range e.world.Managers {
		cid := e.world.Managers[mi].ClubID
		if cid == 0 || seen[cid] {
			continue
		}
		seen[cid] = true
		if a == nil {
			a = e.clubs[cid]
		} else {
			b = e.clubs[cid]
			break
		}
	}
	if a == nil || b == nil {
		t.Fatal("need two employed clubs")
	}
	a.WageBudgetWeeklyMinor, a.TransferBudgetMinor = 1<<50, 1<<50
	b.WageBudgetWeeklyMinor, b.TransferBudgetMinor = 1<<50, 1<<50

	// Move one senior from b to a: a is surplus+1 (its lowest-pool senior goes
	// on-market), b is deficit-1 (and will want to buy).
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.ClubID == b.ID && !p.Youth {
			if p.Contract != nil {
				b.WageBillWeeklyMinor -= p.Contract.WageWeeklyMinor
				a.WageBillWeeklyMinor += p.Contract.WageWeeklyMinor
			}
			p.ClubID = a.ID
			break
		}
	}

	best := e.bestAffordableTarget(b, worldgen.SurplusListed(e.world))
	if best == nil || best.ClubID != a.ID {
		t.Fatalf("b's best target should be surplus club a's castoff, got %+v", best)
	}

	totalBefore := totalBalanceMinor(e)
	ev := &sim.Event{Due: day(10), Kind: sim.KindManager, EntityID: 0, Payload: worldgen.PayloadDecisionRoll}
	wage := worldgen.WageDemandMinor(e.world.Config, e.world.Derived, b.DivisionTier, best.AbilityPool, best.Reputation)
	if !e.acquire(ev, b, best, wage) {
		t.Fatal("autonomous fee acquisition failed")
	}
	if best.ClubID != b.ID {
		t.Fatalf("player did not move to the buyer: ClubID=%d, want %d", best.ClubID, b.ID)
	}
	if got := totalBalanceMinor(e); got != totalBefore {
		t.Fatalf("autonomous fee move broke money conservation: %d != %d", got, totalBefore)
	}
}

// TestAutonomousSkipsAtTargetExplicitSell locks finding 2: the
// autonomous buyer draws only from free agents + surplus castoffs, never an
// explicit SELL from an AT-TARGET club — buying that would pull the seller into a
// new deficit and break convergence. Free agents are made unattractive so, were
// the explicit SELL wrongly a candidate, it would be chosen.
func TestAutonomousSkipsAtTargetExplicitSell(t *testing.T) {
	e, _ := newEngine(t, 42)
	for i := range e.world.Players {
		if p := &e.world.Players[i]; p.ClubID == 0 && !p.Youth {
			p.AbilityPool = 1
		}
	}
	mids := employedManagers(t, e, 2)
	seller := e.managers[mids[0]] // an AT-TARGET club that lists a player
	var x int64
	for i := range e.world.Players {
		if e.world.Players[i].ClubID == seller.ClubID && !e.world.Players[i].Youth {
			x = e.world.Players[i].ID
			break
		}
	}
	seller.Mindset.Directives = append(seller.Mindset.Directives, mindset.Directive{
		ID: "sell", Verb: mindset.VerbSell, Target: mindset.Target{Player: x}, Strength: mindset.StrengthAbsolute,
	})

	// Buyer club: deficit + ample budget.
	buyer := e.clubs[e.managers[mids[1]].ClubID]
	buyer.WageBudgetWeeklyMinor, buyer.TransferBudgetMinor = 1<<50, 1<<50
	for i := range e.world.Players {
		if p := &e.world.Players[i]; p.ClubID == buyer.ID && !p.Youth {
			if p.Contract != nil {
				buyer.WageBillWeeklyMinor -= p.Contract.WageWeeklyMinor
			}
			p.ClubID = 0
			p.Contract = nil
			break
		}
	}

	best := e.bestAffordableTarget(buyer, worldgen.SurplusListed(e.world))
	if best != nil && best.ClubID != 0 {
		t.Fatalf("autonomous buy targeted contracted player %d at at-target club %d (an explicit SELL) — would create a new deficit", best.ID, best.ClubID)
	}
	if best != nil && best.ID == x {
		t.Fatal("autonomous buy targeted the at-target club's explicit SELL")
	}
}

// TestAutonomousMarketDormantAtTarget locks the "floor, not interference"
// property: in a fresh world where every club is exactly at target and no agent
// has issued a directive, the sim buys for nobody.
func TestAutonomousMarketDormantAtTarget(t *testing.T) {
	e, _ := newEngine(t, 42)
	target := e.world.Config.SquadSizeTarget
	before := map[int64]int{}
	for i := range e.world.Clubs {
		id := e.world.Clubs[i].ID
		before[id] = worldgen.SquadSize(e.world, id)
		if before[id] != target {
			t.Fatalf("club %d did not start at target: %d", id, before[id])
		}
	}
	if _, err := e.RunUntil(day(40)); err != nil {
		t.Fatal(err)
	}
	for i := range e.world.Clubs {
		id := e.world.Clubs[i].ID
		if got := worldgen.SquadSize(e.world, id); got != before[id] {
			t.Fatalf("club %d squad changed %d→%d with no perturbation — the sim bought over an agent's floor", id, before[id], got)
		}
	}
	for _, n := range e.world.News {
		if n.Category == "transfer" {
			t.Fatalf("an autonomous transfer fired in an at-target world: %s", n.Key)
		}
	}
}

// TestAutonomousResumeDeterminism is the slice-2b phase invariant: an autonomous
// market running (a deficit club buying) that snapshots mid-window and resumes
// reaches the identical world hash as the uninterrupted run — the autonomous
// path is fully deterministic (no dice) and all state lives in World.
func TestAutonomousResumeDeterminism(t *testing.T) {
	const seed = 42
	ea, _ := newEngine(t, seed)
	clubID := setupDeficit(t, ea)
	horizon := day(40)
	if _, err := ea.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}
	if worldgen.SquadSize(ea.world, clubID) == ea.world.Config.SquadSizeTarget-3 {
		t.Fatal("test vacuous: the uninterrupted run signed no one")
	}

	eb, _ := newEngine(t, seed)
	setupDeficit(t, eb) // identical setup (same seed ⇒ same club + released players)
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
		t.Fatalf("autonomous-market resume diverged from the uninterrupted run:\nA %s\nB %s", ha, hb)
	}
}
