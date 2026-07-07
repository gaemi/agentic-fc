package worldgen

import (
	"math"
	"math/rand/v2"
	"sort"
)

// Stage 5 — history seeding, deliberately thin (docs/09 §4.4): a plausible
// last-season table per division, media predictions & board objectives
// (docs/09 §3, "after squads are generated"), and rivalries. No deeper fake
// history — the world's mythology is earned in play.

// squadStrength is the mean Ability Pool of a club's senior squad.
func squadStrength(w *World, clubID int64) float64 {
	sum, n := 0, 0
	for i := range w.Players {
		p := &w.Players[i]
		if p.ClubID == clubID && !p.Youth {
			sum += p.AbilityPool
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return float64(sum) / float64(n)
}

func genHistory(w *World, r *rand.Rand) {
	w.LastSeason = make([][]Standing, w.Config.Divisions)
	lastPos := map[int64]int{}

	for tier := 1; tier <= w.Config.Divisions; tier++ {
		clubs := clubsInTier(w, tier)
		n := len(clubs)
		games := 2 * (n - 1)

		// Last-season ranks: squad strength + noise gives the upsets.
		type ranked struct {
			id       int64
			strength float64
		}
		rows := make([]ranked, n)
		for i, c := range clubs {
			rows[i] = ranked{c.ID, squadStrength(w, c.ID) + r.NormFloat64()*8}
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].strength != rows[j].strength {
				return rows[i].strength > rows[j].strength
			}
			return rows[i].id < rows[j].id
		})

		table := make([]Standing, n)
		for rank, row := range rows {
			q := float64(n-1-rank) / float64(n-1) // 1.0 top … 0.0 bottom
			won := clamp(int(math.Round(float64(games)*(0.15+0.55*q)))+r.IntN(3)-1, 0, games)
			drawn := clamp(int(math.Round(float64(games)*0.22))+r.IntN(3)-1, 0, games-won)
			gf := max(0, int(math.Round(float64(games)*(0.8+1.2*q)))+r.IntN(7)-3)
			ga := max(0, int(math.Round(float64(games)*(2.0-1.2*q)))+r.IntN(7)-3)
			table[rank] = Standing{
				ClubID: row.id, Played: games,
				Won: won, Drawn: drawn, Lost: games - won - drawn,
				GoalsFor: gf, GoalsAgainst: ga,
				Points: 3*won + drawn,
			}
		}
		sort.Slice(table, func(i, j int) bool {
			a, b := table[i], table[j]
			if a.Points != b.Points {
				return a.Points > b.Points
			}
			gdA, gdB := a.GoalsFor-a.GoalsAgainst, b.GoalsFor-b.GoalsAgainst
			if gdA != gdB {
				return gdA > gdB
			}
			if a.GoalsFor != b.GoalsFor {
				return a.GoalsFor > b.GoalsFor
			}
			return a.ClubID < b.ClubID
		})
		for i := range table {
			table[i].Pos = i + 1
			lastPos[table[i].ClubID] = i + 1
		}
		w.LastSeason[tier-1] = table

		// Media prediction + board objective from current squad strength
		// (docs/09 §3), shared with the season rollover so the rule can't drift.
		derivePredictionsForTier(w, r, clubs, n)
		for _, c := range clubs {
			c.Confidence = c.ConfidenceBaseline // live confidence starts at the baseline
		}
	}

	genRivalries(w, r, lastPos)
}

// genRivalries gives every club 1–2 rivals from shared region + last-season
// proximity (docs/09 §4.4). Weight 3 = same region & division derby,
// 2 = same region across divisions, 1 = same-division proximity fallback.
func genRivalries(w *World, r *rand.Rand, lastPos map[int64]int) {
	// proximity metric: division distance dominates, then table distance.
	proximity := func(a, b *Club) int {
		d := (a.DivisionTier - b.DivisionTier) * 100
		if d < 0 {
			d = -d
		}
		p := lastPos[a.ID] - lastPos[b.ID]
		if p < 0 {
			p = -p
		}
		return d + p
	}
	seen := map[[2]int64]bool{}
	add := func(a, b *Club) {
		key := [2]int64{a.ID, b.ID}
		if key[0] > key[1] {
			key[0], key[1] = key[1], key[0]
		}
		if seen[key] {
			return
		}
		seen[key] = true
		weight := 1
		if a.RegionID == b.RegionID {
			weight = 2
			if a.DivisionTier == b.DivisionTier {
				weight = 3
			}
		}
		w.Rivalries = append(w.Rivalries, Rivalry{ClubA: key[0], ClubB: key[1], Weight: weight})
	}

	for i := range w.Clubs {
		club := &w.Clubs[i]
		var sameRegion, sameDiv []*Club
		for j := range w.Clubs {
			other := &w.Clubs[j]
			if other.ID == club.ID {
				continue
			}
			if other.RegionID == club.RegionID {
				sameRegion = append(sameRegion, other)
			} else if other.DivisionTier == club.DivisionTier {
				sameDiv = append(sameDiv, other)
			}
		}
		byProximity := func(s []*Club) {
			sort.Slice(s, func(x, y int) bool {
				px, py := proximity(club, s[x]), proximity(club, s[y])
				if px != py {
					return px < py
				}
				return s[x].ID < s[y].ID
			})
		}
		byProximity(sameRegion)
		byProximity(sameDiv)

		candidates := append(append([]*Club{}, sameRegion...), sameDiv...)
		if len(candidates) == 0 {
			continue
		}
		add(club, candidates[0])
		if len(candidates) > 1 && r.IntN(100) < 45 {
			add(club, candidates[1])
		}
	}
}

// predictionNoise scales the Gaussian jitter on squad strength for media
// predictions (tunable, docs/98).
const predictionNoise = 3.0

// derivePredictionsForTier assigns PredictedFinish / BoardObjectiveFinish /
// ConfidenceBaseline for one division's clubs, ranking current squad strength with
// a little noise (docs/09 §3). Shared by world generation and the season rollover
// so the prediction rule can't drift between them; it does NOT touch live
// Confidence — both callers seed it from the fresh baseline right after (careers E).
// n is len(clubs). The clubs come from clubsInTier, so they belong to this tier.
func derivePredictionsForTier(w *World, r *rand.Rand, clubs []*Club, n int) {
	type pred struct {
		id       int64
		strength float64
	}
	preds := make([]pred, n)
	for i, c := range clubs {
		preds[i] = pred{c.ID, squadStrength(w, c.ID) + r.NormFloat64()*predictionNoise}
	}
	sort.Slice(preds, func(i, j int) bool {
		if preds[i].strength != preds[j].strength {
			return preds[i].strength > preds[j].strength
		}
		return preds[i].id < preds[j].id
	})
	for rank, row := range preds {
		club := clubByID(w, row.id)
		club.PredictedFinish = rank + 1
		club.BoardObjectiveFinish = clamp(
			club.PredictedFinish-(club.Tendencies.BoardAmbition-10)/5, 1, n)
		club.ConfidenceBaseline = clamp(
			40+club.Tendencies.BoardPatience*2+
				(club.PredictedFinish-club.BoardObjectiveFinish)*4, 20, 95)
	}
}

func clubsInTier(w *World, tier int) []*Club {
	var out []*Club
	for i := range w.Clubs {
		if w.Clubs[i].DivisionTier == tier {
			out = append(out, &w.Clubs[i])
		}
	}
	return out
}

func clubByID(w *World, id int64) *Club {
	for i := range w.Clubs {
		if w.Clubs[i].ID == id {
			return &w.Clubs[i]
		}
	}
	return nil
}
