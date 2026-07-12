package narrative

import (
	"encoding/json"
	"strings"
)

// MatchdayStoryLine composes the "main thread" prose of a matchday round-up
// from the engine's public story payload. It is shared by the Console API and
// the MCP gateway so both surfaces tell the same story for the same article.
//
// Rich payloads (those carrying the round facts added alongside best_margin)
// yield a lead sentence about the day's defining result plus one secondary
// angle — goal tally, away day, thriller, stalemates — rotated by the
// persisted news ID, so the desk does not repeat one formula every round.
// Payloads persisted before the round facts existed render exactly the legacy
// fragments. Sentences carry no trailing period: the article body templates
// close the line.
func MatchdayStoryLine(c Catalogs, loc Locale, story map[string]any, newsID int64) string {
	if len(story) == 0 {
		return ""
	}
	if _, rich := storyInt(story, "goals"); rich {
		return richStoryLine(c, loc, story, newsID)
	}
	return legacyStoryLine(c, loc, story)
}

// legacyStoryLine reproduces the original fragment rendering byte for byte so
// articles persisted by older engines keep their published text.
func legacyStoryLine(c Catalogs, loc Locale, story map[string]any) string {
	lines := []string{}
	margin, hasMargin := storyInt(story, "best_margin")
	draws, hasDraws := storyInt(story, "draws")
	if hasMargin && margin > 0 {
		lines = append(lines, c.Render(loc, "term.matchday.story_margin", story))
	}
	if hasDraws && draws > 0 {
		lines = append(lines, c.Render(loc, "term.matchday.story_draws", story))
	}
	if hasDraws && draws == 0 && hasMargin && margin > 0 {
		lines = append(lines, c.Render(loc, "term.matchday.story_all_winners", nil))
	}
	if len(lines) == 0 {
		// Defensive fallback for malformed persisted params; engine payloads
		// always contain draws and best_margin.
		lines = append(lines, c.Render(loc, "term.matchday.story_unavailable", nil))
	}
	out := []string{}
	for _, line := range lines {
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// storyAngleSalt decorrelates the angle rotation from the body-variant
// rotation, which mixes the same news ID: without it, every editorial voice
// would always pair with the same secondary angle.
const storyAngleSalt = 0x51ed2701

func richStoryLine(c Catalogs, loc Locale, story map[string]any, newsID int64) string {
	lead := c.Render(loc, storyLeadKey(story), story)
	angles := storyAngleKeys(story)
	if len(angles) == 0 {
		return lead
	}
	angle := angles[int(mix64(uint64(newsID)+storyAngleSalt)%uint64(len(angles)))]
	return lead + ". " + c.Render(loc, angle, story)
}

// storyLeadKey grades the day's widest win: a rout, a clear win, a
// single-goal round, or — when every score finished level — a deadlock for a
// league round and a shootout round for a cup one (level cup ties carry a
// winner, so "no fixture found a winner" would contradict the results block).
func storyLeadKey(story map[string]any) string {
	margin, _ := storyInt(story, "best_margin")
	if margin <= 0 {
		if draws, ok := storyInt(story, "draws"); ok && draws == 0 {
			return "term.matchday.story.lead.shootout_round"
		}
		return "term.matchday.story.lead.deadlock"
	}
	homeGoals, _ := storyInt(story, "home_goals")
	awayGoals, _ := storyInt(story, "away_goals")
	side := ".home"
	if awayGoals > homeGoals {
		side = ".away"
	}
	switch {
	case margin >= routStoryMargin:
		return "term.matchday.story.lead.rout" + side
	case margin >= 2:
		return "term.matchday.story.lead.clear" + side
	default:
		return "term.matchday.story.lead.tight"
	}
}

// Story thresholds: a rout lead needs a three-goal gap, a thriller angle a
// five-goal aggregate, a goal-fest three goals per match on average.
const (
	routStoryMargin      = 3
	thrillerStoryTotal   = 5
	goalFestPerMatch     = 3
	stalemateStoryDraws  = 2
	awayDayStoryMinimum  = 2
	fortressStoryMinimum = 3
)

// storyAngleKeys lists every secondary angle the round supports, in a fixed
// order so the news-ID rotation is deterministic for a given payload.
func storyAngleKeys(story map[string]any) []string {
	count, _ := storyInt(story, "count")
	goals, _ := storyInt(story, "goals")
	homeWins, _ := storyInt(story, "home_wins")
	awayWins, _ := storyInt(story, "away_wins")
	draws, _ := storyInt(story, "draws")
	scoreless, _ := storyInt(story, "scoreless")
	topTotal, _ := storyInt(story, "top_total")

	angles := []string{}
	if count > 0 && goals >= goalFestPerMatch*count {
		angles = append(angles, "term.matchday.story.angle.goalfest")
	}
	if count > 1 && goals <= count {
		angles = append(angles, "term.matchday.story.angle.gridlock")
	}
	if awayWins > homeWins && awayWins >= awayDayStoryMinimum {
		angles = append(angles, "term.matchday.story.angle.awayday")
	}
	if homeWins >= fortressStoryMinimum && awayWins == 0 {
		angles = append(angles, "term.matchday.story.angle.fortress")
	}
	// A thriller needs goals AND a close finish: a 5-1 is high-scoring but
	// never "refused to settle". The margin gate requires the fact to be
	// present, so payloads without it never claim drama.
	if topMargin, ok := storyInt(story, "top_margin"); ok && topMargin <= 1 &&
		topTotal >= thrillerStoryTotal && !storyTopIsLead(story) {
		angles = append(angles, "term.matchday.story.angle.thriller")
	}
	if scoreless >= 1 && draws >= stalemateStoryDraws {
		angles = append(angles, "term.matchday.story.angle.stalemates")
	}
	if draws >= stalemateStoryDraws && scoreless == 0 && draws*3 >= count {
		angles = append(angles, "term.matchday.story.angle.level")
	}
	if draws == 0 && count > 1 {
		angles = append(angles, "term.matchday.story.angle.decisive")
	}
	if len(angles) == 0 {
		if count > 1 {
			// An unremarkable round still deserves a second sentence: fall
			// back to the day's win/draw balance.
			angles = append(angles, "term.matchday.story.angle.balance")
		} else if count == 1 {
			// A lone fixture (a cup final, a make-up match) cannot speak of
			// "the rest of the day" — its second sentence is the occasion.
			angles = append(angles, "term.matchday.story.angle.solo")
		}
	}
	return angles
}

// storyTopIsLead reports whether the highest-scoring match is the same
// fixture as the widest win, in which case the thriller angle would just
// repeat the lead sentence's game.
func storyTopIsLead(story map[string]any) bool {
	topHome, _ := story["top_home"].(string)
	bestHome, _ := story["best_home"].(string)
	topAway, _ := story["top_away"].(string)
	bestAway, _ := story["best_away"].(string)
	return topHome != "" && topHome == bestHome && topAway == bestAway
}

// storyInt reads a numeric story param that may arrive as int or, after a
// JSON round-trip, int64, float64, or json.Number.
func storyInt(story map[string]any, key string) (int, bool) {
	switch v := story[key].(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		if v == float64(int64(v)) {
			return int(v), true
		}
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n), true
		}
	}
	return 0, false
}
