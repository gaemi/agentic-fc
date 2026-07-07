package engine

import (
	"fmt"

	"github.com/gaemi/agentic-fc/internal/rng"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Player lifecycle. At each season boundary every player has a
// birthday, academy youth who have come of age graduate into the senior squad,
// and veterans roll age-driven retirement — the player-side mirror of the
// manager careers pass (manager careers). Aging is what brings the whole drift
// trajectory to life: growth tiers and the physicals-first decline in
// drift.go were always age-driven, players just never aged before this.
//
// The pass runs inside handleSeasonEnd AFTER manager careers and BEFORE
// RolloverSeason, so wages shed by retirement have left the bill cache before
// the rollover derives the new season's budgets from it (careers E). Everything
// is deterministic: birthdays and graduation are dice-free slice-order passes,
// and each retirement draws its own id-keyed labelled stream, so the outcome is
// order-free and a resumed run reproduces the identical population (NFR-2).

// NewsYouthGraduated announces a club's academy graduates joining the senior
// squad — count only, like the intake (FR-22).
const NewsYouthGraduated = "news.youth.graduated"

// Retirement news keys: a contracted player retires from a club; a free agent
// simply calls time. Player names are public — no hidden-attribute data rides
// along (FR-22).
const (
	newsPlayerRetired     = "news.player.retired"
	newsPlayerRetiredFree = "news.player.retired_free"
)

// Player lifecycle ages (tunable, docs/98). A youth turns senior at the
// graduation age; below the retirement floor a player never retires, at the
// ceiling it is certain, linear between — integer-only, like the manager curve.
// Constraint: youthGraduationAge must stay below playerRetireAgeFloor — the
// pass runs graduation before retirement and skips Youth in the retirement
// loop, so crossing them would let a same-tick graduate retire.
const (
	youthGraduationAge   = 18
	playerRetireAgeFloor = 32
	playerRetireAgeCeil  = 40
)

// playerRetirementChance is the percentage chance a player of the given age
// retires this season.
func playerRetirementChance(age int) int {
	if age < playerRetireAgeFloor {
		return 0
	}
	if age >= playerRetireAgeCeil {
		return 100
	}
	return (age - playerRetireAgeFloor) * 100 / (playerRetireAgeCeil - playerRetireAgeFloor)
}

// processPlayerCareers advances the player population at the season boundary:
// birthdays first (an intake prospect turning youthGraduationAge graduates this
// same boundary), then graduation, then retirement. Graduation and retirement
// are disjoint by age — a youth is a teenager, the retirement floor is 32 — so
// no player is both promoted and retired in one tick. No World.Players append
// happens here, so the player index stays valid throughout.
func (e *Engine) processPlayerCareers(ev *sim.Event, season int) {
	for i := range e.world.Players {
		if !e.world.Players[i].Retired {
			e.world.Players[i].Age++
		}
	}
	e.graduateYouth(ev)
	e.retirePlayers(ev, season)
}

// graduateYouth flips every of-age academy prospect into the senior squad. The
// flag is the single gate every senior system filters on (selectXI, SquadSize,
// the market predicates), so graduation needs no further wiring — the graduate
// is selectable, counted, and tradable from the next read. News per club in
// slice order (count-only), never map order (NFR-2).
func (e *Engine) graduateYouth(ev *sim.Event) {
	counts := map[int64]int{}
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.Youth && !p.Retired && p.Age >= youthGraduationAge {
			p.Youth = false
			counts[p.ClubID]++
		}
	}
	for i := range e.world.Clubs {
		club := &e.world.Clubs[i]
		if n := counts[club.ID]; n > 0 {
			e.youthNews(ev.Due, club, n, NewsYouthGraduated)
		}
	}
}

// retirePlayers rolls age-driven retirement for every senior player on its own
// labelled stream (career/player_retire/<id>@<season>), independent of every
// other roll and of the boundary's other dice.
func (e *Engine) retirePlayers(ev *sim.Event, season int) {
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.Retired || p.Youth {
			continue
		}
		chance := playerRetirementChance(p.Age)
		if chance == 0 {
			continue
		}
		r := rng.Stream(e.world.Config.Seed, fmt.Sprintf("career/player_retire/%d@%d", p.ID, season))
		if r.IntN(100) < chance {
			e.retirePlayer(ev, p)
		}
	}
}

// retirePlayer ends p's career: the row stays in World.Players (news and results
// reference player ids — deletion would dangle them) but is flagged out of every
// selection, market, and scouting predicate. An employed retiree's wage leaves
// the club's bill cache — the same discipline as a transfer out — so the season
// rollover derives the new budgets from a truthful bill. The player's queued
// drift roll dies at the RETIRED guard in handlePlayerDrift, ending the chain.
func (e *Engine) retirePlayer(ev *sim.Event, p *worldgen.Player) {
	clubID := p.ClubID
	p.Retired = true
	p.ClubID = 0
	if p.Contract != nil {
		if club := e.clubs[clubID]; club != nil {
			club.WageBillWeeklyMinor -= p.Contract.WageWeeklyMinor
		}
		p.Contract = nil
	}

	if clubID != 0 {
		if club := e.clubs[clubID]; club != nil {
			params := map[string]any{"player": p.Name, "club": club.Name}
			e.addNews(worldgen.NewsItem{
				GameTime: ev.Due, Category: "career", Key: newsPlayerRetired,
				Params: params, ClubIDs: []int64{clubID},
			})
			e.emit(ev.Due, newsPlayerRetired, cloneParams(params))
			return
		}
	}
	params := map[string]any{"player": p.Name}
	e.addNews(worldgen.NewsItem{
		GameTime: ev.Due, Category: "career", Key: newsPlayerRetiredFree, Params: params,
	})
	e.emit(ev.Due, newsPlayerRetiredFree, cloneParams(params))
}
