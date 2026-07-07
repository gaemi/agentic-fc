package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// expiringSenior finds a contracted senior player at a real club whose deal
// ends with season 1, and pins the club pointer for assertions.
func expiringSenior(t *testing.T, e *Engine) *worldgen.Player {
	t.Helper()
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.ClubID != 0 && !p.Youth && p.Contract != nil && p.Contract.ExpirySeasonYear == 1 {
			return p
		}
	}
	t.Fatal("no season-1-expiring senior in the generated world")
	return nil
}

// billOf sums a club's real contract wages — the truth the cache must track.
func billOf(e *Engine, clubID int64) int64 {
	var bill int64
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.ClubID == clubID && p.Contract != nil {
			bill += p.Contract.WageWeeklyMinor
		}
	}
	return bill
}

// TestContractExpiriesSettleEveryDeal is the phase's completeness lock: after
// the boundary, NO senior non-retired player is left holding a contract that
// ended with the finished season — every expiring deal was renewed (expiry
// moved forward, wage repriced) or lapsed (free agency). The bill cache equals
// the contract sum at every club afterward, and both news keys appeared
// somewhere (churn: a default world renews and lapses at least one player
// each).
func TestContractExpiriesSettleEveryDeal(t *testing.T) {
	e := newEngineCfg(t, worldgen.DefaultConfig(31))
	if _, err := e.RunUntil(day(364)); err != nil {
		t.Fatal(err)
	}
	// Guarantee at least one lapse: zero one expiring player's club budget so
	// the autonomous default cannot afford the renewal (the fresh budget is
	// re-derived only AFTER the contract pass, so this sticks through it).
	e.clubs[expiringSenior(t, e).ClubID].WageBudgetWeeklyMinor = 0
	if _, err := e.RunUntil(day(366)); err != nil {
		t.Fatal(err)
	}
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.Retired || p.Youth || p.Contract == nil {
			continue
		}
		if p.Contract.ExpirySeasonYear <= 1 {
			t.Fatalf("player %d still holds a season-1 contract after the boundary", p.ID)
		}
	}
	for i := range e.world.Clubs {
		c := &e.world.Clubs[i]
		if got := billOf(e, c.ID); c.WageBillWeeklyMinor != got {
			t.Fatalf("club %d bill cache %d != contract sum %d after the contract pass", c.ID, c.WageBillWeeklyMinor, got)
		}
	}
	if !hasNews(e, newsContractRenewed) {
		t.Fatal("test vacuous: no renewal happened at the boundary")
	}
	if !hasNews(e, newsContractLapsed) {
		t.Fatal("test vacuous: no lapse happened at the boundary")
	}
}

// TestYouthContractAutoRenews locks the academy rule: an underage prospect's
// expiring deal quietly extends one season at the same wage — the academy
// never releases (youth intake), so a youth can never lapse into free agency.
func TestYouthContractAutoRenews(t *testing.T) {
	e := newEngineCfg(t, worldgen.DefaultConfig(31))
	if _, err := e.RunUntil(day(364)); err != nil {
		t.Fatal(err)
	}
	var yid int64
	var wage int64
	for i := range e.world.Players {
		p := &e.world.Players[i]
		// Will still be youth after the birthday: age stays under the
		// graduation age at the boundary.
		if p.Youth && p.ClubID != 0 && p.Age < youthGraduationAge-1 && p.Contract != nil {
			p.Contract.ExpirySeasonYear = 1 // force it to expire now
			yid, wage = p.ID, p.Contract.WageWeeklyMinor
			break
		}
	}
	if yid == 0 {
		t.Fatal("no underage academy prospect found")
	}
	if _, err := e.RunUntil(day(365)); err != nil {
		t.Fatal(err)
	}
	p := e.players[yid]
	if !p.Youth || p.ClubID == 0 || p.Contract == nil {
		t.Fatalf("academy prospect lapsed: youth=%v club=%d contract=%v", p.Youth, p.ClubID, p.Contract)
	}
	if p.Contract.ExpirySeasonYear != 2 || p.Contract.WageWeeklyMinor != wage {
		t.Fatalf("youth auto-renew = expiry %d wage %d, want expiry 2 wage %d (extend one season, no reprice)",
			p.Contract.ExpirySeasonYear, p.Contract.WageWeeklyMinor, wage)
	}
}

// TestReleaseDirectiveLapses locks the agent's RELEASE lever: the manager's
// standing word beats the autonomous keep-them default, and the player walks
// into free agency with the wage shed.
func TestReleaseDirectiveLapses(t *testing.T) {
	e := newEngineCfg(t, worldgen.DefaultConfig(31))
	if _, err := e.RunUntil(day(364)); err != nil {
		t.Fatal(err)
	}
	p := expiringSenior(t, e)
	clubID := p.ClubID
	m := e.clubManager(clubID)
	m.Mindset.Directives = append(m.Mindset.Directives, mindset.Directive{
		ID: "rel1", Verb: mindset.VerbRelease, Target: mindset.Target{Player: p.ID},
	})
	// Give the club room so the autonomous default WOULD have renewed —
	// proving the directive, not the budget, decided.
	e.clubs[clubID].WageBudgetWeeklyMinor = 1 << 50

	if _, err := e.RunUntil(day(365)); err != nil {
		t.Fatal(err)
	}
	if p.ClubID != 0 || p.Contract != nil {
		t.Fatalf("RELEASE ignored: club=%d contract=%v", p.ClubID, p.Contract)
	}
	if got := billOf(e, clubID); e.clubs[clubID].WageBillWeeklyMinor != got {
		t.Fatalf("bill cache %d != contract sum %d after RELEASE", e.clubs[clubID].WageBillWeeklyMinor, got)
	}
}

// TestRenewDirectiveCeiling locks the RENEW lever both ways: with headroom the
// deal renews at the market wage; with a hard ceiling under the market ask the
// player walks — no clamp, the ask is the ask.
func TestRenewDirectiveCeiling(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ceiling func(market int64) float64
		renewed bool
	}{
		{"ceiling_above_market_renews", func(m int64) float64 { return float64(m + 1) }, true},
		{"ceiling_below_market_lapses", func(m int64) float64 { return float64(m - 1) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEngineCfg(t, worldgen.DefaultConfig(31))
			if _, err := e.RunUntil(day(364)); err != nil {
				t.Fatal(err)
			}
			p := expiringSenior(t, e)
			club := e.clubs[p.ClubID]
			market := worldgen.WageDemandMinor(e.world.Config, e.world.Derived, club.DivisionTier, p.AbilityPool, p.Reputation)
			m := e.clubManager(club.ID)
			m.Mindset.Directives = append(m.Mindset.Directives, mindset.Directive{
				ID: "ren1", Verb: mindset.VerbRenew, Target: mindset.Target{Player: p.ID},
				Params: map[string]any{"wage_ceiling": tc.ceiling(market)},
			})
			// Zero the budget so the autonomous default would ALWAYS lapse —
			// a renewal can only come from the directive.
			club.WageBudgetWeeklyMinor = 0

			if _, err := e.RunUntil(day(365)); err != nil {
				t.Fatal(err)
			}
			if tc.renewed {
				if p.Contract == nil || p.Contract.WageWeeklyMinor != market || p.Contract.ExpirySeasonYear < 2 {
					t.Fatalf("RENEW under ceiling failed: %+v (market %d)", p.Contract, market)
				}
			} else if p.Contract != nil || p.ClubID != 0 {
				t.Fatalf("RENEW over ceiling must lapse, got club=%d contract=%+v", p.ClubID, p.Contract)
			}
		})
	}
}

// TestLapsedPlayerIsSignable closes the loop with the market: a lapsed player
// is a real free agent — the autonomous deficit buyer's scan can pick them up.
func TestLapsedPlayerIsSignable(t *testing.T) {
	e := newEngineCfg(t, worldgen.DefaultConfig(31))
	if _, err := e.RunUntil(day(364)); err != nil {
		t.Fatal(err)
	}
	p := expiringSenior(t, e)
	m := e.clubManager(p.ClubID)
	m.Mindset.Directives = append(m.Mindset.Directives, mindset.Directive{
		ID: "rel2", Verb: mindset.VerbRelease, Target: mindset.Target{Player: p.ID},
	})
	if _, err := e.RunUntil(day(365)); err != nil {
		t.Fatal(err)
	}
	if p.ClubID != 0 {
		t.Fatal("setup: player did not lapse")
	}
	buyer := &e.world.Clubs[1]
	buyer.WageBudgetWeeklyMinor, buyer.TransferBudgetMinor = 1<<50, 1<<50
	best := e.bestAffordableTarget(buyer, map[int64]bool{})
	if best == nil {
		t.Fatal("no free agent visible to the autonomous scan")
	}
	found := false
	for i := range e.world.Players {
		q := &e.world.Players[i]
		if q.ID == p.ID && !q.Youth && !q.Retired && q.ClubID == 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("lapsed player not in the signable free-agent pool")
	}
}

// TestContractDirectiveLastRenewWins locks the duplicate-RENEW rule: the
// manager's LAST word replaces the whole verdict — a later
// unconditional RENEW clears the ceiling an earlier duplicate set, and a
// RELEASE anywhere ends the discussion.
func TestContractDirectiveLastRenewWins(t *testing.T) {
	e, _ := newEngine(t, 42)
	var m *worldgen.Manager
	for i := range e.world.Managers {
		if e.world.Managers[i].ClubID != 0 {
			m = &e.world.Managers[i]
			break
		}
	}
	m.Mindset.Directives = []mindset.Directive{
		{Verb: mindset.VerbRenew, Target: mindset.Target{Player: 77}, Params: map[string]any{"wage_ceiling": float64(1000)}},
		{Verb: mindset.VerbRenew, Target: mindset.Target{Player: 77}},
	}
	v := e.contractDirective(m.ClubID, 77)
	if v.verb != mindset.VerbRenew || v.wageCeiling != 0 {
		t.Fatalf("verdict = %+v, want unconditional RENEW (later directive clears the stale ceiling)", v)
	}
	m.Mindset.Directives = append(m.Mindset.Directives, mindset.Directive{
		Verb: mindset.VerbRelease, Target: mindset.Target{Player: 77},
	})
	if v := e.contractDirective(m.ClubID, 77); v.verb != mindset.VerbRelease {
		t.Fatalf("verdict = %+v, want RELEASE to outrank RENEW", v)
	}
}
