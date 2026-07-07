package engine

import "github.com/gaemi/agentic-fc/internal/worldgen"

// Board confidence. Each club carries a live integer
// Confidence (0–100) seeded to ConfidenceBaseline; it moves after every LEAGUE
// result by how the result compares to expectation — beating a stronger side
// lifts it, losing to a weaker one cuts it. Integer-only (no float on the world
// hash), moved in the match-result handler under the queue's total order, so it
// reproduces exactly on resume (NFR-2). The agent never sees the number, only the
// Descriptor (confidenceBand / securityBand). Cup results don't move it — the
// board's yardstick is the league objective. Sacking reads this signal.

// matchupExpectation is how a club was expected to fare against an opponent, by
// their predicted league finishes (lower = stronger).
type matchupExpectation int

const (
	expectFavorite matchupExpectation = iota // clearly the stronger side
	expectEven                               // no clear favorite
	expectUnderdog                           // clearly the weaker side
)

// confidenceFavoriteGap is how many predicted-finish places apart two clubs must
// be for one to be the clear favorite (tunable, docs/98).
const confidenceFavoriteGap = 4

// confidenceDelta is the confidence change by [win, draw, loss] for each matchup
// (tunable, docs/98): a favorite is punished for dropping points and barely
// rewarded for expected wins; an underdog is richly rewarded for a shock result.
var confidenceDelta = map[matchupExpectation][3]int{
	expectFavorite: {2, -2, -5},
	expectEven:     {4, 1, -4},
	expectUnderdog: {6, 3, -1},
}

// matchup classifies a club's expectation from the predicted finishes.
func matchup(myFinish, oppFinish int) matchupExpectation {
	switch d := oppFinish - myFinish; { // d > 0 ⇒ I'm predicted higher (stronger)
	case d >= confidenceFavoriteGap:
		return expectFavorite
	case d <= -confidenceFavoriteGap:
		return expectUnderdog
	default:
		return expectEven
	}
}

// clampConfidence keeps confidence in [1, 100]; the floor of 1 (not 0) keeps 0
// reserved as the "uninitialised" sentinel a snapshot load reseeds from baseline.
func clampConfidence(c int) int {
	if c < 1 {
		return 1
	}
	if c > 100 {
		return 100
	}
	return c
}

// moveConfidence shifts a club's confidence by its result against opp — gf/ga are
// the club's goals for/against.
func moveConfidence(club, opp *worldgen.Club, gf, ga int) {
	col := 2 // loss
	switch {
	case gf > ga:
		col = 0 // win
	case gf == ga:
		col = 1 // draw
	}
	club.Confidence = clampConfidence(club.Confidence + confidenceDelta[matchup(club.PredictedFinish, opp.PredictedFinish)][col])
}
