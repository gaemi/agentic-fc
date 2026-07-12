package consoleapi

import (
	"sort"

	"github.com/gaemi/agentic-fc/internal/narrative"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Post-match report prose (docs/07 §4.1, roadmap "public post-match
// explanatory summaries built from observed diagnostics"): a short verdict
// assembled from the already-public result ledger — result frame, one "how
// it was won" edge read from the diagnostics, one story beat read from the
// scorer ledger. Pure presentation: deterministic (variants rotate on the
// fixture ID), RNG-free, and structurally unable to leak hidden attributes
// because it only reads MatchResult fields the API already serves.

// Edge thresholds. Presentation-only judgment calls about when a diagnostic
// skew is big enough to narrate; they gate no simulation behavior.
const (
	reportPressEdgeMin    = 3  // leading side's pressing turnovers...
	reportPressEdgeRatio  = 2  // ...and at least this multiple of the other's
	reportAerialEdgeMin   = 3  // aerial wins needed to call the air
	reportSetPieceEdgeMin = 3  // set-piece threats needed to call a siege
	reportQualityEdgeMin  = 2  // extra HIGH-quality chances to call the looks
	reportPatternMin      = 3  // one chance pattern this often is an identity
	reportLateGoalMinute  = 85 // decisive goals from here read as late drama
)

// matchStoryLines renders the localized post-match verdict for a finished
// match: always a result frame, then at most one edge line and one story
// beat. Missing diagnostics (legacy archives) simply shorten the report.
func (s *Server) matchStoryLines(loc narrative.Locale, names map[int64]string, playerName func(int64) string, r *worldgen.MatchResult) []string {
	variant := func(base string) string {
		if r.FixtureID%2 == 0 {
			return base + ".2"
		}
		return base + ".1"
	}
	home, away := names[r.HomeID], names[r.AwayID]
	// winnerID is zero only for a genuine draw: on level cup ties the
	// shootout victor recorded in r.Winner takes it, keeping the story in
	// agreement with the winner field the API already serves.
	winnerID := int64(0)
	switch {
	case r.HomeGoals > r.AwayGoals:
		winnerID = r.HomeID
	case r.AwayGoals > r.HomeGoals:
		winnerID = r.AwayID
	default:
		winnerID = r.Winner
	}
	winner, loser := home, away
	if winnerID == r.AwayID {
		winner, loser = away, home
	}
	params := map[string]any{
		"home": home, "away": away, "winner": winner, "loser": loser,
		"home_goals": r.HomeGoals, "away_goals": r.AwayGoals,
	}
	margin := r.HomeGoals - r.AwayGoals
	if margin < 0 {
		margin = -margin
	}
	frame := ""
	switch {
	case margin == 0 && winnerID != 0:
		frame = variant("report.win.shootout")
	case margin == 0 && r.HomeGoals == 0:
		frame = variant("report.draw.blank")
	case margin == 0:
		frame = variant("report.draw.score")
	case margin >= 3:
		frame = variant("report.win.emphatic")
	case margin == 2:
		frame = variant("report.win.comfortable")
	default:
		frame = variant("report.win.narrow")
	}
	winnerSide := ""
	switch winnerID {
	case r.HomeID:
		winnerSide = matchSideHome
	case r.AwayID:
		winnerSide = matchSideAway
	}
	lines := []string{s.Catalogs.Render(loc, frame, params)}
	if key, edgeParams := matchEdge(r, home, away, winnerSide); key != "" {
		if edgeParams["pattern_key"] != nil {
			edgeParams["pattern"] = s.Catalogs.Render(loc, edgeParams["pattern_key"].(string), nil)
			delete(edgeParams, "pattern_key")
		}
		lines = append(lines, s.Catalogs.Render(loc, variant(key), edgeParams))
	}
	if key, beatParams := matchStoryBeat(r, winnerID, winner, loser, playerName); key != "" {
		lines = append(lines, s.Catalogs.Render(loc, variant(key), beatParams))
	}
	return lines
}

// matchEdge picks the loudest diagnostic skew — pressing, the air, set-piece
// pressure, chance quality, then a chance-pattern identity. Every edge is
// "how it was won", so outside a genuine draw only the winning side's
// dominance narrates: a side that pressed everyone off the park and still
// lost did not define the result. One line at most — the report is a
// verdict, not a stat dump.
func matchEdge(r *worldgen.MatchResult, home, away, winnerSide string) (string, map[string]any) {
	sideOf := func(side string) string {
		if side == matchSideHome {
			return home
		}
		return away
	}
	credits := func(side string) bool {
		return winnerSide == "" || side == winnerSide
	}
	d := r.Diagnostics
	if side, hi, lo := dominantSide(d.PressTurnovers); credits(side) && hi >= reportPressEdgeMin && hi >= lo*reportPressEdgeRatio {
		return "report.edge.press", map[string]any{"club": sideOf(side), "a": hi, "b": lo}
	}
	if side, hi, lo := dominantSide(d.AerialWins); credits(side) && hi >= reportAerialEdgeMin && hi >= lo*reportPressEdgeRatio {
		return "report.edge.aerial", map[string]any{"club": sideOf(side), "a": hi, "b": lo}
	}
	if side, hi, lo := dominantSide(d.SetPieceThreat); credits(side) && hi >= reportSetPieceEdgeMin && hi >= lo*reportPressEdgeRatio {
		return "report.edge.setpiece", map[string]any{"club": sideOf(side), "a": hi, "b": lo}
	}
	hiHome, hiAway := d.ShotQualityBySide["HOME_HIGH"], d.ShotQualityBySide["AWAY_HIGH"]
	if credits(matchSideHome) && hiHome-hiAway >= reportQualityEdgeMin {
		return "report.edge.quality", map[string]any{"club": home, "a": hiHome, "b": hiAway}
	}
	if credits(matchSideAway) && hiAway-hiHome >= reportQualityEdgeMin {
		return "report.edge.quality", map[string]any{"club": away, "a": hiAway, "b": hiHome}
	}
	// The pattern identity needs a winner (shootout victors included): a
	// draw has no side whose attacking signature "won it".
	if winnerSide != "" {
		// Sorted key walk: Go map order is random, and a tie between two
		// patterns must render the same line on every request.
		keys := make([]string, 0, len(r.ChanceTypesBySide))
		for chanceType := range r.ChanceTypesBySide {
			keys = append(keys, chanceType)
		}
		sort.Strings(keys)
		bestType, bestN := "", 0
		for _, chanceType := range keys {
			side, kind, ok := splitSideKey(chanceType)
			if ok && side == winnerSide && r.ChanceTypesBySide[chanceType] > bestN {
				bestType, bestN = kind, r.ChanceTypesBySide[chanceType]
			}
		}
		if bestN >= reportPatternMin {
			return "report.edge.pattern", map[string]any{
				"club": sideOf(winnerSide), "a": bestN, "pattern_key": "term.chance_type." + bestType,
			}
		}
	}
	return "", nil
}

// matchStoryBeat reads the scorer ledger for the retellable arc: a
// hat-trick, a two-goal comeback (win or salvaged draw), or a late winner.
func matchStoryBeat(r *worldgen.MatchResult, winnerID int64, winner, loser string, playerName func(int64) string) (string, map[string]any) {
	goalsByPlayer := map[int64]int{}
	bestScorer, bestGoals := int64(0), 0
	for _, e := range r.Scorers {
		goalsByPlayer[e.PlayerID]++
		if n := goalsByPlayer[e.PlayerID]; n > bestGoals {
			bestScorer, bestGoals = e.PlayerID, n
		}
	}
	if bestGoals >= 3 {
		if name := playerName(bestScorer); name != "" {
			return "report.beat.hattrick", map[string]any{"player": name, "count": bestGoals}
		}
	}
	if winnerID != 0 {
		// Covers shootout victors too: recovering from two down to force
		// penalties and winning them is still winning the hard way.
		if deficit := worstDeficit(r, winnerID); deficit >= 2 {
			return "report.beat.comeback_win", map[string]any{"club": winner, "deficit": deficit}
		}
	} else {
		for _, clubID := range []int64{r.HomeID, r.AwayID} {
			if deficit := worstDeficit(r, clubID); deficit >= 2 {
				name := loser
				if clubID == r.HomeID {
					name = winner // a draw's winner/loser default to home/away
				}
				return "report.beat.comeback_draw", map[string]any{"club": name, "deficit": deficit}
			}
		}
	}
	// Gate on the decisive goal, not the final margin: insurance goals in
	// stoppage time do not un-write a late winner.
	if winnerID != 0 {
		if minute, ok := decisiveGoalMinute(r, winnerID); ok && minute >= reportLateGoalMinute {
			return "report.beat.late_winner", map[string]any{"club": winner, "minute": minute}
		}
	}
	return "", nil
}

// dominantSide returns the stronger side of a HOME/AWAY counter with both
// values, HOME winning ties (a tie never passes the ratio gate anyway).
func dominantSide(counts map[string]int) (string, int, int) {
	h, a := counts[matchSideHome], counts[matchSideAway]
	if a > h {
		return matchSideAway, a, h
	}
	return matchSideHome, h, a
}

// splitSideKey parses "HOME_CUTBACK"-style by-side keys.
func splitSideKey(key string) (side, kind string, ok bool) {
	for _, s := range []string{matchSideHome, matchSideAway} {
		prefix := s + "_"
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			return s, key[len(prefix):], true
		}
	}
	return "", "", false
}

// worstDeficit replays the full ledger and returns the club's worst deficit.
func worstDeficit(r *worldgen.MatchResult, clubID int64) int {
	own, other, worst := 0, 0, 0
	for _, e := range r.Scorers {
		if e.ClubID == clubID {
			own++
		} else {
			other++
		}
		if d := other - own; d > worst {
			worst = d
		}
	}
	return worst
}

// decisiveGoalMinute finds the goal after which the winner never dropped
// back to level — the one the report calls the winner.
func decisiveGoalMinute(r *worldgen.MatchResult, winnerID int64) (int, bool) {
	own, other := 0, 0
	minute, found := 0, false
	for _, e := range r.Scorers {
		if e.ClubID == winnerID {
			own++
			if own-other == 1 {
				minute, found = e.Minute, true
			}
		} else {
			other++
			if own <= other {
				found = false
			}
		}
	}
	return minute, found
}
