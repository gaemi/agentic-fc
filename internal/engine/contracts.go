package engine

import (
	"fmt"
	"math/rand/v2"

	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/rng"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Contract expiries + renewals. Contracts have carried an
// ExpirySeasonYear since generation — inclusive, "ends June 30 of that season" —
// but nothing ever processed it. Now the season boundary settles every expiring
// deal: the club renews the players it wants (repriced at the current tier's
// market wage) and the rest lapse into free agency, feeding the market's
// pre-season scramble the moment the summer window opens.
//
// The pass runs in handleSeasonEnd AFTER processPlayerCareers (a same-boundary
// retiree's contract is already nil, so it never reaches here) and BEFORE
// RolloverSeason (renewal repricing and lapse-shedding must hit the wage-bill
// cache before careers-E budget derivation reads it; DivisionTier is still the
// just-finished season's tier at that point, which is the tier the market wage
// reprices at). Agent control rides the existing Mindset contract verbs —
// RENEW (with an optional wage_ceiling param) and RELEASE — which are accepted
// but inert until now; without a directive the club applies the autonomous
// default. Deterministic: decisions resolve in World.Players slice order (id
// order — the explicit policy for who claims budget room first), the only dice
// are the renewal length on a per-player labelled stream, and every wage delta
// updates the bill cache in place (NFR-2).

// Contract news keys — count-only per club, like the youth items: a boundary
// can lapse several players at one club, and named items would flood the ring.
// A lapsed player is discoverable on demand via search_players(free_agent);
// wage figures never ride along (FR-22 discipline from the fee-transfer work).
const (
	newsContractRenewed = "news.contract.renewed"
	newsContractLapsed  = "news.contract.lapsed"
)

// renewalYears rolls the new deal's length, mirroring generation's
// age-dependent contract curve (tunable, docs/98): young players sign longer.
func renewalYears(r *rand.Rand, age int) int {
	switch {
	case age <= 21:
		return 2 + r.IntN(2) // 2–3
	case age <= 29:
		return 1 + r.IntN(3) // 1–3
	default:
		return 1 + r.IntN(2) // 1–2
	}
}

// processContractExpiries settles every contract that ends with the finished
// season. endingSeason is the season that just completed (the boundary event's
// date is already in the new season).
func (e *Engine) processContractExpiries(ev *sim.Event, endingSeason int) {
	// Surplus is computed ONCE from pre-pass state: the predicate's convergence
	// proof rests on strict pre-existing excess, and a deficit opened by a lapse
	// is the autonomous buyer's job to fill next window — re-deriving mid-pass
	// would make one player's fate depend on another's in a way no one can read.
	surplus := worldgen.SurplusListed(e.world)

	renewed := map[int64]int{}
	lapsed := map[int64]int{}
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.Retired || p.Contract == nil || p.Contract.ExpirySeasonYear > endingSeason {
			continue
		}
		if p.Youth {
			// The academy never releases a prospect (youth intake): the deal quietly
			// extends a season at the academy wage — no reprice, no news. A
			// same-boundary graduate turned senior above and faces the senior
			// renew-or-lapse like anyone else: a club keeps the 18-year-olds it
			// rates (repriced to senior terms) and releases the rest — the
			// academy-to-free-agency pipeline is a real, intended outcome.
			p.Contract.ExpirySeasonYear = endingSeason + 1
			continue
		}
		clubID := p.ClubID // captured first: a lapse zeroes p.ClubID
		if e.clubs[clubID] == nil {
			// A contracted player with no resolvable club is corrupt
			// state — skip rather than half-process it (counting a lapse while
			// leaving the contract would re-count it every boundary).
			continue
		}
		if e.renewContractOrLapse(p, endingSeason, surplus) {
			renewed[clubID]++
		} else {
			lapsed[clubID]++
		}
	}

	for i := range e.world.Clubs {
		club := &e.world.Clubs[i]
		if n := renewed[club.ID]; n > 0 {
			e.contractNews(ev.Due, club, n, newsContractRenewed)
		}
		if n := lapsed[club.ID]; n > 0 {
			e.contractNews(ev.Due, club, n, newsContractLapsed)
		}
	}
}

// renewContractOrLapse decides one expiring senior deal and applies it,
// reporting whether the player was renewed. A lapse zeroes p.ClubID, so the
// caller must capture the club id BEFORE calling.
func (e *Engine) renewContractOrLapse(p *worldgen.Player, endingSeason int, surplus map[int64]bool) bool {
	club := e.clubs[p.ClubID] // non-nil: the caller skips unresolvable clubs
	verdict := e.contractDirective(club.ID, p.ID)
	newWage := worldgen.WageDemandMinor(e.world.Config, e.world.Derived, club.DivisionTier, p.AbilityPool, p.Reputation)

	renew := false
	switch verdict.verb {
	case mindset.VerbRelease:
		// The manager has decided: the player goes.
	case mindset.VerbRenew:
		// The manager wants them — at market wage, unless the deal's hard
		// ceiling is beaten, in which case the player walks (no clamp: the ask
		// is the ask; a ceiling is a limit, not a negotiating trick).
		renew = verdict.wageCeiling == 0 || newWage <= verdict.wageCeiling
	default:
		// Autonomous default: keep anyone the club isn't already shedding —
		// not SELL-listed, not a surplus castoff — if the repriced wage fits
		// the budget. Budget room is claimed in player-id order (the pass's
		// iteration order): the bill cache moves with every decision, so a
		// later renewal sees the room its predecessors left.
		renew = !surplus[p.ID] && !e.sellListedAny(club.ID, p.ID) &&
			club.WageBillWeeklyMinor-p.Contract.WageWeeklyMinor+newWage <= club.WageBudgetWeeklyMinor
	}

	if !renew {
		club.WageBillWeeklyMinor -= p.Contract.WageWeeklyMinor
		p.ClubID = 0
		p.Contract = nil
		return false
	}
	r := rng.Stream(e.world.Config.Seed, fmt.Sprintf("contract/renew/%d@%d", p.ID, endingSeason))
	club.WageBillWeeklyMinor += newWage - p.Contract.WageWeeklyMinor
	p.Contract.WageWeeklyMinor = newWage
	p.Contract.ExpirySeasonYear = endingSeason + renewalYears(r, p.Age)
	return true
}

// contractVerdict is the manager's standing word on an expiring deal.
type contractVerdict struct {
	verb        mindset.Verb
	wageCeiling int64
}

// contractDirective finds the strongest RENEW/RELEASE the player's CURRENT
// club's manager holds on them. RELEASE outranks RENEW on a tie (the schema
// marks them mutually exclusive, but directives are free text intent — resolve
// the conflict conservatively). A directive from a former club's manager is
// inert: ownership is checked here, so a lapse or transfer strands the old
// directive harmlessly until it expires. A boundary-installed caretaker has a
// fresh Mindset — no directives — so the autonomous default governs its club.
func (e *Engine) contractDirective(clubID, playerID int64) contractVerdict {
	m := e.clubManager(clubID)
	if m == nil {
		return contractVerdict{}
	}
	out := contractVerdict{}
	for _, d := range m.Mindset.Directives {
		if d.Target.Player != playerID {
			continue
		}
		switch d.Verb {
		case mindset.VerbRelease:
			return contractVerdict{verb: mindset.VerbRelease}
		case mindset.VerbRenew:
			// The LAST RENEW is the manager's current word: it replaces the
			// whole verdict, including CLEARING a ceiling an earlier duplicate
			// set (regression review — assigning only on a present param would let a
			// stale ceiling shadow a later unconditional RENEW). Params carry
			// JSON numbers as float64; the ceiling is integer minor units —
			// convert exactly, never letting the float reach any stored value
			// (NFR-2). Non-positive or malformed → no ceiling.
			out = contractVerdict{verb: mindset.VerbRenew}
			if v, ok := d.Params["wage_ceiling"].(float64); ok && v > 0 {
				out.wageCeiling = int64(v)
			}
		}
	}
	return out
}

// sellListedAny reports whether the club's manager has ANY standing SELL on the
// player (strength irrelevant here — a club shedding a player doesn't renew
// them; the fee path's acceptance curve still reads the strength).
func (e *Engine) sellListedAny(clubID, playerID int64) bool {
	_, listed := e.sellListed(clubID, playerID)
	return listed
}

// contractNews files a count-only contract item (renewals or lapses) for a club.
func (e *Engine) contractNews(t sim.GameTime, club *worldgen.Club, count int, key string) {
	params := map[string]any{"club": club.Name, "count": count}
	e.addNews(worldgen.NewsItem{
		GameTime: t, Category: "contract", Key: key, Params: params, ClubIDs: []int64{club.ID},
	})
	e.emit(t, key, cloneParams(params))
}
