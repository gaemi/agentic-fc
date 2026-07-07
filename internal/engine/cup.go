package engine

import (
	"fmt"

	"github.com/gaemi/agentic-fc/internal/rng"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Cup within-season progression (docs/09 §3 single-elimination). The
// generator draws round 1; each later round is drawn here when its predecessor
// completes. A level tie is settled by a penalty shootout (no replays — decision
// model), then the round's winners are re-drawn into the next round. The
// draw mirrors the season-rollover schedule path exactly — append fixtures to
// World, rebuild the fixture index, schedule the new kickoffs — so it inherits
// that path's proven tempo/resume safety (NFR-2): the pure bracket state lives
// in World, and the kickoff events persist in the queue.

const (
	// shootoutKicks is the best-of-N penalty phase before sudden death.
	shootoutKicks = 5
	// shootoutConvert is the flat per-kick conversion probability (tunable,
	// docs/98). Shootout scores are cosmetic (the winner is what advances), so a
	// simple flat rate keeps the tie deterministic without a skill model.
	shootoutConvert = 0.75
	// shootoutSuddenDeathMax bounds sudden death so the tie-break is guaranteed
	// to terminate (each pair splits with p≈0.375, so reaching this many equal
	// pairs is astronomically unlikely — it is a safety guard, not a balance
	// knob). If it is ever hit, a decisive coin settles it.
	shootoutSuddenDeathMax = 50
)

// resolveCupWinner picks the advancing club: the higher scorer, or — when the 90
// minutes finished level — a penalty shootout on its own stateless stream
// (cup/shootout/<fixtureID>), independent of the match's moment dice so it never
// correlates with them yet still replays identically.
func (e *Engine) resolveCupWinner(lm *worldgen.LiveMatch) (winner int64, homePens, awayPens int, shootout bool) {
	switch {
	case lm.HomeGoals > lm.AwayGoals:
		return lm.HomeID, 0, 0, false
	case lm.AwayGoals > lm.HomeGoals:
		return lm.AwayID, 0, 0, false
	}
	r := rng.Stream(e.world.Config.Seed, fmt.Sprintf("cup/shootout/%d", lm.FixtureID))
	hp, ap := 0, 0
	for i := 0; i < shootoutKicks; i++ {
		if r.Float64() < shootoutConvert {
			hp++
		}
		if r.Float64() < shootoutConvert {
			ap++
		}
	}
	for i := 0; i < shootoutSuddenDeathMax && hp == ap; i++ { // sudden death, bounded
		if r.Float64() < shootoutConvert {
			hp++
		}
		if r.Float64() < shootoutConvert {
			ap++
		}
	}
	if hp == ap { // guard exhausted — a decisive coin always splits, so we terminate
		if r.Float64() < 0.5 {
			hp++
		} else {
			ap++
		}
	}
	if hp > ap {
		return lm.HomeID, hp, ap, true
	}
	return lm.AwayID, hp, ap, true
}

// advanceCup runs after a cup fixture is recorded: if it completed its round, it
// either crowns the champion (the final) or draws the next round and schedules
// its kickoffs. A no-op for a round with ties still to play.
func (e *Engine) advanceCup(at sim.GameTime, lm *worldgen.LiveMatch) {
	f, ok := e.fixtures[lm.FixtureID]
	if !ok {
		return
	}
	round := f.Round
	if !e.world.CupRoundComplete(round) {
		return
	}
	season := worldgen.DateOf(at).Season

	if round >= e.world.Derived.CupRounds { // the final just finished
		if champ := e.world.CupRoundWinners(round); len(champ) > 0 {
			e.announceCupChampion(at, champ[0], season)
		}
		return
	}

	next := round + 1
	if e.world.HasCupRound(next) {
		return // already drawn — defensive; round completion fires exactly once
	}
	stream := rng.Stream(e.world.Config.Seed, fmt.Sprintf("cup/s%d/r%d/draw", season, next))
	created := e.world.DrawCupRound(stream, next, season)
	e.buildFixtureIndex() // new fixtures enter the map + tempo match-windows
	for i := range created {
		e.queue.Schedule(&sim.Event{
			Due:      created[i].Kickoff,
			Priority: sim.PriorityMatch,
			Kind:     sim.KindMatch,
			EntityID: created[i].ID,
			Payload:  worldgen.PayloadKickoff,
		})
	}
}

// announceCupChampion files the cup win on the Console feed and the agent news
// ring — the payoff of the whole competition.
func (e *Engine) announceCupChampion(at sim.GameTime, clubID int64, season int) {
	// Recorded so the rollover can archive the season's winner;
	// the rollover clears it for the next campaign.
	e.world.CupChampionID = clubID
	params := map[string]any{"club": e.clubName(clubID), "season": season}
	e.emit(at, FeedCupChampion, params)
	e.addNews(worldgen.NewsItem{
		GameTime: at, Category: "match", Key: FeedCupChampion,
		Params: params, ClubIDs: []int64{clubID},
	})
}
