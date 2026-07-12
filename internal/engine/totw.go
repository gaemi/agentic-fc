package engine

import (
	"sort"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Team of the Week (docs/07 §4.2): once a league matchday window has every
// result in, the media desk files the round's best XI — a fixed 1-4-3-3
// sheet picked from the published ratings — with the top performer as the
// headline act. Pure presentation over public facts (ratings, goals,
// positions): deterministic ordering, no RNG, one article per window.

// totwShape is the fixed selection sheet: a classic 4-3-3 reads best as a
// "best XI" feature regardless of how the round's teams actually lined up.
var totwShape = []struct {
	group attr.PositionGroup
	count int
}{
	{attr.GK, 1}, {attr.DF, 4}, {attr.MF, 3}, {attr.FW, 3},
}

type totwCandidate struct {
	player    *worldgen.Player
	clubID    int64
	ratingX10 int
	goals     int
}

// addMatchdayTeamNews files the round's Team of the Week for a league
// division once every fixture of the window has a result. Ordering is
// deterministic: rating desc, goals desc, then player id.
func (e *Engine) addMatchdayTeamNews(at, kickoff sim.GameTime, competition string, division int) {
	if competition != worldgen.CompetitionLeague {
		return
	}
	if e.newsExists(FeedMatchdayTeam, kickoff, competition, division) {
		return
	}
	fixtures := e.fixturesAt(kickoff, competition, division)
	if len(fixtures) == 0 {
		return
	}
	results := e.resultsAt(kickoff, competition, division)
	if len(results) != len(fixtures) {
		return // the round-up defers on the same condition; so does the XI.
	}

	candidates := e.totwCandidates(results)
	if len(candidates) < 11 {
		return // defensive: a full round always rates 22+ players.
	}
	team := pickTeamOfTheWeek(candidates)
	if len(team) < 11 {
		return
	}

	params := e.matchdayBaseParams(kickoff, competition, division, len(results))
	rows := make([]map[string]any, 0, len(team))
	for _, c := range team {
		rows = append(rows, map[string]any{
			"name": c.player.Name, "club": e.clubName(c.clubID),
			"position": c.player.Position, "rating_x10": c.ratingX10, "goals": c.goals,
		})
	}
	star := team[0]
	for _, c := range team[1:] {
		if betterTotw(c, star) {
			star = c
		}
	}
	params["team"] = rows
	params["star"] = star.player.Name
	params["star_club"] = e.clubName(star.clubID)
	params["star_rating_x10"] = star.ratingX10
	params["star_goals"] = star.goals

	// The article belongs to the whole round: ClubIDs is the ownership
	// filter for get_news/get_situation and news alerts, so every club that
	// played the window sees it — not only the clubs that placed a player.
	e.addNews(worldgen.NewsItem{
		GameTime: at, Category: "match", Key: FeedMatchdayTeam,
		Params: params, ClubIDs: fixtureClubRefs(fixtures),
	})
}

// totwCandidates flattens a round's results into rated performances.
func (e *Engine) totwCandidates(results []worldgen.MatchResult) []totwCandidate {
	out := []totwCandidate{}
	for i := range results {
		r := &results[i]
		goals := map[int64]int{}
		for _, s := range r.Scorers {
			goals[s.PlayerID]++
		}
		clubOf := map[int64]int64{}
		for _, id := range r.HomeXI {
			clubOf[id] = r.HomeID
		}
		for _, id := range r.AwayXI {
			clubOf[id] = r.AwayID
		}
		for _, sub := range r.Subs {
			if sub.On != 0 {
				clubOf[sub.On] = sub.ClubID
			}
		}
		for pid, rating := range r.RatingsX10 {
			p := e.players[pid]
			if p == nil {
				continue
			}
			out = append(out, totwCandidate{
				player: p, clubID: clubOf[pid], ratingX10: rating, goals: goals[pid],
			})
		}
	}
	return out
}

// betterTotw is the deterministic candidate order: rating, then goals, then
// the stable id tie-break.
func betterTotw(a, b totwCandidate) bool {
	if a.ratingX10 != b.ratingX10 {
		return a.ratingX10 > b.ratingX10
	}
	if a.goals != b.goals {
		return a.goals > b.goals
	}
	return a.player.ID < b.player.ID
}

// pickTeamOfTheWeek fills the 1-4-3-3 sheet from the round's performances,
// backfilling from the best leftovers if a band somehow runs short. Rows
// come back in sheet order (GK first, forwards last).
func pickTeamOfTheWeek(candidates []totwCandidate) []totwCandidate {
	sort.Slice(candidates, func(i, j int) bool { return betterTotw(candidates[i], candidates[j]) })
	taken := map[int64]bool{}
	team := make([]totwCandidate, 0, 11)
	for _, band := range totwShape {
		need := band.count
		for _, c := range candidates {
			if need == 0 {
				break
			}
			if taken[c.player.ID] || c.player.Group != band.group {
				continue
			}
			taken[c.player.ID] = true
			team = append(team, c)
			need--
		}
	}
	for _, c := range candidates {
		if len(team) >= 11 {
			break
		}
		if !taken[c.player.ID] {
			taken[c.player.ID] = true
			team = append(team, c)
		}
	}
	return team
}
