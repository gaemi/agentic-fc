package engine

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// The match engine (docs/03 §3 "key-moment sampling"). A fixture is
// not resolved atomically at kickoff — it STREAMS: kickoff seeds a running
// LiveMatch and schedules the first moment; each moment rolls, mutates the
// tally, and self-schedules the next (the roll-and-reschedule DES pattern of
// docs/03 §1); the final moment finalizes standings, stats, and ratings. The
// running tally lives in World.LiveMatches so a mid-match snapshot→reload
// resumes identically, and every roll is stateless-by-label via rollStream, so
// tempo/replay/resume reproduce the exact same match (NFR-2).
//
// All numeric values here are tunable (docs/98). Ratings are ×10 integers so
// nothing reaching the world hash is a float.
const (
	matchFullTimeMinutes = 90
	matchMoments         = 18 // key moments sampled across a match
	lateDramaMinute      = 85 // goals from here narrate as late drama

	// Commentary context thresholds (presentation only, docs/12 §7: variety
	// must not perturb play — none of these gate an RNG draw).
	hatTrickGoals         = 3  // a scorer's third goal headlines the call
	comebackDeficitMin    = 2  // trailed by this many before leveling/leading
	responseWindowMinutes = 5  // re-taking the lead this fast reads as a response
	routGoalsMin          = 4  // a side's fourth goal...
	routMarginMin         = 3  // ...at this margin narrates as a rout
	tensionQuietMinute    = 75 // close games read tense from here
	cruiseQuietMinute     = 60 // big leads read as cruise control from here
	cruiseQuietMargin     = 3

	chanceBaseRate         = 0.55  // per-moment P(a clear chance falls to a side)
	conversionBase         = 0.20  // P(a chance converts), before skill skew
	conversionScoreDivisor = 140.0 // attack-defense delta scale for conversion
	conversionMin          = 0.03
	conversionMax          = 0.58
	volatilitySpread       = 0.40  // finishing wobble; narrows with Consistency
	cardRatePerMoment      = 0.06  // per-moment P(a card)
	redCardShare           = 0.10  // fraction of cards that are red
	injuryRatePerMoment    = 0.010 // per-moment P(an injury knock)
	injuryConditionHit     = 35    // condition lost to an in-match knock

	// Real injuries + substitutions (tunable docs/98). Lay-off =
	// base + IntN(span) days, shifted by hidden InjuryProne (longer) and
	// Recovery (shorter), floored at 1 — integer days only (NFR-2).
	injuryDaysBase = 3
	injuryDaysSpan = 14
	subsMax        = 3    // substitutions per side per match
	benchSize      = 5    // bench picked beside the XI at kickoff
	formWindow     = 5    // rolling match-rating window behind the form band
	homeAdvantage  = 1.15 // home attack weight in chance allocation

	// Discretionary substitutions (tunable docs/98): from
	// tacticalSubFromMinute a side may make at most one voluntary change per
	// moment — a fatigue withdrawal (dice-free) when a player's DERIVED
	// in-match condition falls under fatigueSubThreshold, else a fresh-legs
	// upgrade with probability tacticalSubProb when the bench holds a strictly
	// stronger outfielder. While only one substitution remains it is reserved
	// for injuries until subReserveUntil.
	tacticalSubFromMinute = 60
	fatigueSubThreshold   = 30
	tacticalSubProb       = 0.35
	subReserveUntil       = 75

	// SubEvent.Reason tokens — the public why of a substitution.
	subReasonInjury   = "INJURY"
	subReasonFatigue  = "FATIGUE"
	subReasonTactical = "TACTICAL"

	conditionDrainPlay = 22 // condition a full match costs a starter
	conditionFloorPlay = 5  // a match never drains below this
	sharpnessGainPlay  = 8  // sharpness a match builds

	// Rating band values live in worldgen (docs/98) — LiveRatingsX10 is shared
	// with the Console's live ratings pane; the full alias set keeps engine
	// call sites and tests reading naturally.
	ratingBaseX10   = worldgen.RatingBaseX10
	ratingMinX10    = worldgen.RatingMinX10
	ratingMaxX10    = worldgen.RatingMaxX10
	ratingGoalX10   = worldgen.RatingGoalX10
	ratingWinX10    = worldgen.RatingWinX10
	ratingLossX10   = worldgen.RatingLossX10
	ratingCleanX10  = worldgen.RatingCleanX10
	ratingYellowX10 = worldgen.RatingYellowX10
	ratingRedX10    = worldgen.RatingRedX10

	// In-match decisions: a chasing manager may push more attacking
	// in the closing stages, probability rising with their Risk Appetite.
	adjustFromMinute  = 55   // decisions only once the game state is clear
	maxMentalityShift = 2    // cap on the in-match attacking bias
	adjustBaseProb    = 0.15 // floor probability of a push when chasing
	adjustRiskWeight  = 0.55 // how far Risk Appetite raises that probability

	// Body-profile expression (tunable docs/98): public height and
	// weight do not replace attributes; they nudge how reach/Strength express
	// in match calculations. Zero body fields are treated as neutral so hand-built
	// tests and old dev snapshots degrade safely.
	bodyNeutralHeightCm = 180
	bodyNeutralWeightKg = 76
	bodyHeightStepCm    = 5
	bodyWeightStepKg    = 8
	bodyReachMinBonus   = -3
	bodyReachMaxBonus   = 4
	bodyMassMinBonus    = -2
	bodyMassMaxBonus    = 3
)

const (
	chanceCrossHeader    = "CROSS_HEADER"
	chanceCutback        = "CUTBACK"
	chanceThroughBall    = "THROUGH_BALL"
	chanceLongShot       = "LONG_SHOT"
	chanceSetPieceHeader = "SET_PIECE_HEADER"
	chanceScramble       = "SCRAMBLE"
	chanceCounter        = "COUNTER"

	shotQualityHighDelta = 25
	shotQualityLowDelta  = -15
)

// startMatch handles a kickoff: it selects both XIs, seeds the running match
// tally, announces the kick-off, and schedules the first key moment.
func (e *Engine) startMatch(ev *sim.Event) error {
	f, ok := e.fixtures[ev.EntityID]
	if !ok {
		return e.log(ev, "match", nil, "unknown_fixture", 0, 0)
	}
	e.emitKickoff(ev)
	e.issueMatchAlerts(ev.Due, f, "OWN_KICKOFF")

	homeXI, homeBench := e.selectSquad(f.HomeID, ev.Due, e.tacticalFor(f.HomeID))
	awayXI, awayBench := e.selectSquad(f.AwayID, ev.Due, e.tacticalFor(f.AwayID))
	lm := &worldgen.LiveMatch{
		FixtureID: f.ID, Competition: f.Competition, DivisionTier: f.DivisionTier,
		HomeID: f.HomeID, AwayID: f.AwayID, Kickoff: ev.Due,
		HomeXI: homeXI, AwayXI: awayXI,
		HomeBench: homeBench, AwayBench: awayBench,
	}
	if e.world.LiveMatches == nil {
		e.world.LiveMatches = make(map[int64]*worldgen.LiveMatch)
	}
	e.world.LiveMatches[f.ID] = lm
	e.comment(lm, ev.Due, kickoffCommentaryKey(f.ID), map[string]any{
		"home": e.clubName(f.HomeID), "away": e.clubName(f.AwayID),
	})

	e.scheduleHalftime(f.ID, ev.Due)
	e.scheduleMoment(f.ID, ev.Due, 0)
	return e.log(ev, "match", map[string]any{
		"home": f.HomeID, "away": f.AwayID,
	}, "kickoff", 0, 0)
}

func (e *Engine) scheduleHalftime(fixtureID int64, kickoff sim.GameTime) {
	e.queue.Schedule(&sim.Event{
		Due:      kickoff + sim.GameTime(matchFullTimeMinutes/2),
		Priority: sim.PriorityMatch,
		Kind:     sim.KindMatch,
		EntityID: fixtureID,
		Payload:  worldgen.PayloadMatchHalftime,
	})
}

func (e *Engine) handleMatchHalftime(ev *sim.Event) error {
	lm := e.world.LiveMatches[ev.EntityID]
	if lm == nil {
		return e.log(ev, "match", nil, "no_live_match", 0, 0)
	}
	lm.Clock = matchFullTimeMinutes / 2
	e.recordHalftimeCommentary(lm, ev.Due)
	return e.log(ev, "match", map[string]any{
		"score": []int{lm.HomeGoals, lm.AwayGoals}, "minute": lm.Clock,
	}, "halftime", 0, 0)
}

// momentClock returns the game-minute of the i-th moment: the midpoint of its
// even slice of the 90-minute match, so moments fall at ~2.5, 7.5 … 87.5.
func momentClock(i int) int {
	return (i*2 + 1) * matchFullTimeMinutes / (2 * matchMoments)
}

func (e *Engine) scheduleMoment(fixtureID int64, kickoff sim.GameTime, index int) {
	e.queue.Schedule(&sim.Event{
		Due:      kickoff + sim.GameTime(momentClock(index)),
		Priority: sim.PriorityMatch,
		Kind:     sim.KindMatch,
		EntityID: fixtureID,
		Payload:  worldgen.PayloadMatchMoment,
	})
}

// handleMatchMoment rolls one key moment, folds it into the running tally, and
// either schedules the next moment or finalizes the match.
func (e *Engine) handleMatchMoment(ev *sim.Event) error {
	lm := e.world.LiveMatches[ev.EntityID]
	if lm == nil {
		// Already finalized (or never started) — drain harmlessly.
		return e.log(ev, "match", nil, "no_live_match", 0, 0)
	}
	r := e.rollStream(ev)
	i := lm.MomentIndex
	lm.Clock = momentClock(i)
	if i == matchMoments/2 && !hasHalftimeCommentary(lm) {
		// Upgrade fallback: a match loaded from an older snapshot has no queued
		// 45' event. Record the whistle once with its football minute, then let
		// this 47' moment continue normally.
		e.recordHalftimeCommentary(lm, ev.Due)
	}
	e.maybeAdjust(lm, ev.Due, r)
	e.rollMomentOutcome(lm, ev.Due, r)
	e.maybeDiscretionarySubs(lm, ev.Due, r)

	lm.MomentIndex++
	if lm.MomentIndex < matchMoments {
		e.scheduleMoment(lm.FixtureID, lm.Kickoff, lm.MomentIndex)
		return e.log(ev, "match", map[string]any{
			"score": []int{lm.HomeGoals, lm.AwayGoals}, "minute": lm.Clock,
		}, "moment", 0, 0)
	}
	return e.finalizeMatch(ev, lm)
}

func (e *Engine) recordHalftimeCommentary(lm *worldgen.LiveMatch, at sim.GameTime) {
	e.commentAtMinute(lm, at, matchFullTimeMinutes/2, halftimeCommentaryKey(lm.HomeGoals, lm.AwayGoals), map[string]any{
		"home": e.clubName(lm.HomeID), "away": e.clubName(lm.AwayID),
		"home_goals": lm.HomeGoals, "away_goals": lm.AwayGoals,
	})
}

func hasHalftimeCommentary(lm *worldgen.LiveMatch) bool {
	for _, line := range lm.Commentary {
		if strings.HasPrefix(line.Key, "comment.halftime") {
			return true
		}
	}
	return false
}

// rollMomentOutcome applies one moment's chance, card, and injury rolls to the
// running tally. Team strengths are recomputed from the CURRENT on-pitch sets
// each moment — substitutions change them mid-match — so the model reads from
// live player state; the outcome is otherwise pure over r.
func (e *Engine) rollMomentOutcome(lm *worldgen.LiveMatch, at sim.GameTime, r *rand.Rand) {
	homeAtk, homeDef := e.teamStrength(lm.OnPitch(lm.HomeID), e.tacticalFor(lm.HomeID), lm.HomeMentalityShift)
	awayAtk, awayDef := e.teamStrength(lm.OnPitch(lm.AwayID), e.tacticalFor(lm.AwayID), lm.AwayMentalityShift)

	chance := r.Float64() < chanceBaseRate
	if chance {
		// Allocate the chance by attack vs the opponent's defense; home edge.
		homeWeight := float64(homeAtk) * homeAdvantage / float64(awayDef+1)
		awayWeight := float64(awayAtk) / float64(homeDef+1)
		homeChance := homeWeight >= (homeWeight+awayWeight)*r.Float64()
		e.resolveChance(lm, at, r, homeChance)
	} else {
		// Quiet passage — connective tissue so the feed never goes dead.
		e.comment(lm, at, quietBeatKey(r, lm), nil)
	}
	if r.Float64() < cardRatePerMoment {
		e.bookOne(lm, at, r)
	}
	if r.Float64() < injuryRatePerMoment {
		e.injureOne(lm, at, r)
	}
}

var quietCommentaryKeys = []string{
	"comment.quiet.1", "comment.quiet.2", "comment.quiet.3", "comment.quiet.4",
	"comment.quiet.5", "comment.quiet.6", "comment.quiet.7", "comment.quiet.8",
	"comment.quiet.9", "comment.quiet.10", "comment.quiet.11", "comment.quiet.12",
	"comment.quiet.13", "comment.quiet.14", "comment.quiet.15",
	"comment.quiet.16", "comment.quiet.17", "comment.quiet.18",
	"comment.quiet.19", "comment.quiet.20", "comment.quiet.21",
}

// legacyQuietPoolSize anchors the quiet draw's RNG bound (see pickWidenedKey).
const legacyQuietPoolSize = 15

// stateQuietKeys narrates the shape of the match during quiet beats: a close
// game after the tension minute prefers nervy lines, a comfortable margin
// after the cruise minute prefers game-management lines. Nil means the broad
// quiet pool stands. Presentation only — the caller's RNG draw is identical
// either way.
func stateQuietKeys(lm *worldgen.LiveMatch) []string {
	margin := lm.HomeGoals - lm.AwayGoals
	if margin < 0 {
		margin = -margin
	}
	switch {
	case lm.Clock >= tensionQuietMinute && margin <= 1:
		return tensionQuietKeys
	case lm.Clock >= cruiseQuietMinute && margin >= cruiseQuietMargin:
		return cruiseQuietKeys
	default:
		return nil
	}
}

var tensionQuietKeys = []string{
	"comment.quiet.tension.1", "comment.quiet.tension.2",
	"comment.quiet.tension.3", "comment.quiet.tension.4",
}

var cruiseQuietKeys = []string{
	"comment.quiet.cruise.1", "comment.quiet.cruise.2", "comment.quiet.cruise.3",
}

// quietBeatKey makes the quiet draw state-aware without touching the RNG
// contract: exactly one IntN with the legacy bound, exactly as before. When
// the match state prefers a themed pool and it still has unused lines, the
// drawn value maps into that pool; otherwise it falls through to the broad
// quiet pool's unused-probe.
func quietBeatKey(r *rand.Rand, lm *worldgen.LiveMatch) string {
	legacyCount := legacyQuietPoolSize
	if legacyCount > len(quietCommentaryKeys) {
		legacyCount = len(quietCommentaryKeys)
	}
	drawn := r.IntN(legacyCount)
	used := usedCommentaryKeys(lm)
	if preferred := stateQuietKeys(lm); len(preferred) > 0 {
		if key := probeUnusedKey(drawn, lm, preferred, used); key != "" {
			return key
		}
	}
	if key := probeUnusedKey(drawn, lm, quietCommentaryKeys, used); key != "" {
		return key
	}
	return quietCommentaryKeys[(drawn+lm.Clock+len(lm.Commentary)*3)%len(quietCommentaryKeys)]
}

// pickUnusedCommentaryKey preserves the original single IntN draw and bound,
// then probes deterministically past keys already used in this match. Thus
// presentation avoids repeats until the pool is exhausted without moving the
// RNG stream seen by cards, injuries, or any other simulation outcome.
func pickUnusedCommentaryKey(r *rand.Rand, lm *worldgen.LiveMatch, keys []string) string {
	legacyCount := legacyQuietPoolSize
	if legacyCount > len(keys) {
		legacyCount = len(keys)
	}
	// The draw keeps its original bound; state rotation spreads it across
	// any later pool growth without moving the RNG stream (see pickWidenedKey).
	drawn := r.IntN(legacyCount)
	if key := probeUnusedKey(drawn, lm, keys, usedCommentaryKeys(lm)); key != "" {
		return key
	}
	return keys[(drawn+lm.Clock+len(lm.Commentary)*3)%len(keys)]
}

func usedCommentaryKeys(lm *worldgen.LiveMatch) map[string]bool {
	used := make(map[string]bool, len(lm.Commentary))
	for _, line := range lm.Commentary {
		used[line.Key] = true
	}
	return used
}

// probeUnusedKey maps an already-drawn value onto keys via the public-state
// rotation, then probes forward for a line this match has not spoken yet.
// Empty means the pool is exhausted.
func probeUnusedKey(drawn int, lm *worldgen.LiveMatch, keys []string, used map[string]bool) string {
	if len(keys) == 0 {
		return ""
	}
	start := (drawn + lm.Clock + len(lm.Commentary)*3) % len(keys)
	for offset := range keys {
		key := keys[(start+offset)%len(keys)]
		if !used[key] {
			return key
		}
	}
	return ""
}

// resolveChance chooses a chance type from the attacking side's tactical
// profile, then resolves an event-specific contest. This is the first slice of
// the match-model event grammar: tactics change event distribution, and each event
// reads different attribute mixes instead of one generic chance shortcut.
func (e *Engine) resolveChance(lm *worldgen.LiveMatch, at sim.GameTime, r *rand.Rand, home bool) {
	atkClub := lm.AwayID
	defClub := lm.HomeID
	if home {
		atkClub = lm.HomeID
		defClub = lm.AwayID
	}
	atkXI := lm.OnPitch(atkClub)
	defXI := lm.OnPitch(defClub)
	chanceType := chooseChanceType(e.tacticalFor(atkClub), r)
	scorer := e.pickScorerForChance(atkXI, chanceType, r)
	if scorer == 0 {
		return
	}
	if home {
		lm.HomeShots++
	} else {
		lm.AwayShots++
	}
	if lm.ChanceTypes == nil {
		lm.ChanceTypes = map[string]int{}
	}
	lm.ChanceTypes[chanceType]++
	if lm.ChanceTypesBySide == nil {
		lm.ChanceTypesBySide = map[string]int{}
	}
	side := "AWAY"
	if home {
		side = "HOME"
	}
	lm.ChanceTypesBySide[side+"_"+chanceType]++

	p := e.players[scorer]
	attackScore := e.chanceAttackScore(p, atkXI, chanceType)
	defenseScore := e.chanceDefenseScore(defXI, chanceType)
	e.recordMatchDiagnostics(lm, home, chanceType, attackScore, defenseScore)
	wobble := 1 + (r.Float64()-0.5)*volatilitySpread*(1-float64(consistency(p))/20)
	pGoal := clampFloat(conversionBase+float64(attackScore-defenseScore)/conversionScoreDivisor, conversionMin, conversionMax) * wobble
	name := ""
	if p != nil {
		name = p.Name
	}
	if r.Float64() >= pGoal {
		// A chance that came to nothing — saved or off target.
		e.comment(lm, at, pickWidenedKey(r, lm, legacyMissPoolSize, missCommentKeys(chanceType)),
			map[string]any{"player": name, "club": e.clubName(atkClub)})
		return
	}
	if home {
		lm.HomeGoals++
	} else {
		lm.AwayGoals++
	}
	lm.Scorers = append(lm.Scorers, worldgen.MatchEvent{
		Minute: lm.Clock, PlayerID: scorer, ClubID: atkClub,
	})
	// The pattern draw always happens and always uses the legacy pool bound,
	// so commentary variety never changes how much RNG the moment consumes
	// (docs/12: presentation must not perturb play).
	goalKey := pickWidenedKey(r, lm, legacyGoalPoolSize, goalCommentKeys(chanceType))
	if contextKey := goalContextCommentaryKey(lm, home, scorer); contextKey != "" {
		goalKey = contextKey
	}
	e.comment(lm, at, goalKey,
		map[string]any{
			"player": name, "club": e.clubName(atkClub),
			"home_goals": lm.HomeGoals, "away_goals": lm.AwayGoals,
		})
}

// Commentary pools were widened after launch, but the per-moment RNG
// argument sequence is part of the determinism contract: rand/v2's IntN can
// consume different amounts of the stream for different bounds. The draw
// therefore stays on the legacy pool size, and public match state rotates
// which slice of the widened pool that draw lands on.
const (
	legacyMissPoolSize = 4 // three chance lines + one save line per pattern
	legacyGoalPoolSize = 3 // three goal lines per pattern
)

func pickWidenedKey(r *rand.Rand, lm *worldgen.LiveMatch, legacyCount int, keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	if legacyCount > len(keys) {
		legacyCount = len(keys)
	}
	drawn := r.IntN(legacyCount)
	offset := (lm.Clock + lm.HomeGoals*3 + lm.AwayGoals) % len(keys)
	return keys[(drawn+offset)%len(keys)]
}

// kickoffCommentaryKey rotates the opening whistle line per fixture without
// touching the match RNG stream.
func kickoffCommentaryKey(fixtureID int64) string {
	keys := []string{"comment.kickoff", "comment.kickoff.2", "comment.kickoff.3"}
	idx := int(fixtureID % int64(len(keys)))
	if idx < 0 {
		idx += len(keys)
	}
	return keys[idx]
}

// goalContextCommentaryKey swaps a patterned goal call for one that speaks to
// the match situation: a hat-trick, late drama, a completed comeback, the
// opener, an equalizer, an instant response, or a rout. It returns "" when
// the pattern call should stand (ordinary goals that pad a lead). The variant
// rotates on public match state so no RNG is consumed — existing seeds replay
// the same football. It reads lm AFTER the goal is recorded (score bumped,
// scorer appended).
func goalContextCommentaryKey(lm *worldgen.LiveMatch, home bool, scorer int64) string {
	atk, def := lm.HomeGoals, lm.AwayGoals
	if !home {
		atk, def = def, atk
	}
	pick := func(keys ...string) string {
		return keys[(lm.Clock+lm.HomeGoals*3+lm.AwayGoals)%len(keys)]
	}
	late := lm.Clock >= lateDramaMinute
	// The scorer's personal milestone headlines the call whatever it does to
	// the scoreline — a hat-trick is the story of the day.
	if scorer != 0 && goalsBy(lm, scorer) == hatTrickGoals {
		if late {
			return pick("comment.goal.hattrick_late.1", "comment.goal.hattrick_late.2")
		}
		return pick("comment.goal.hattrick.1", "comment.goal.hattrick.2")
	}
	// The ladder runs specific-to-generic: a comeback or an instant response
	// is a sharper story than the clock, so both outrank the late-drama
	// calls; the late calls still take every ordinary closing-minutes
	// leveler or winner.
	deep, leveledSince, ledSince := comebackStanding(lm, home)
	switch {
	case deep && !leveledSince && atk == def:
		return pick("comment.goal.comeback_level.1", "comment.goal.comeback_level.2")
	case deep && !ledSince && atk == def+1:
		return pick("comment.goal.comeback_ahead.1", "comment.goal.comeback_ahead.2")
	case atk == def+1 && concededJustBefore(lm, home):
		return pick("comment.goal.response.1", "comment.goal.response.2")
	case late && atk == def:
		return pick("comment.goal.late_level.1", "comment.goal.late_level.2")
	case late && atk == def+1:
		return pick("comment.goal.late.1", "comment.goal.late.2")
	case lm.HomeGoals+lm.AwayGoals == 1:
		return pick("comment.goal.opener.1", "comment.goal.opener.2")
	case atk == def:
		return pick("comment.goal.equalizer.1", "comment.goal.equalizer.2")
	case atk >= routGoalsMin && atk-def >= routMarginMin:
		return pick("comment.goal.rout.1", "comment.goal.rout.2", "comment.goal.rout.3")
	default:
		return ""
	}
}

// goalsBy counts the scorer's goals in this match, including the goal just
// recorded.
func goalsBy(lm *worldgen.LiveMatch, scorer int64) int {
	n := 0
	for _, e := range lm.Scorers {
		if e.PlayerID == scorer {
			n++
		}
	}
	return n
}

// comebackStanding replays the scorer ledger up to (not including) the goal
// just recorded and reports whether the scoring side is still mid-fightback:
// deep is a two-goal-plus deficit suffered at some point, and
// leveledSince/ledSince say whether the side already drew level or led again
// after the LAST time the hole was that deep. Once a fightback has been
// completed, later goals are ordinary football again — a second deep deficit
// re-arms the story.
func comebackStanding(lm *worldgen.LiveMatch, home bool) (deep, leveledSince, ledSince bool) {
	if len(lm.Scorers) == 0 {
		return false, false, false
	}
	h, a := 0, 0
	for _, e := range lm.Scorers[:len(lm.Scorers)-1] {
		if e.ClubID == lm.HomeID {
			h++
		} else {
			a++
		}
		deficit := a - h
		if !home {
			deficit = h - a
		}
		if deficit >= comebackDeficitMin {
			deep, leveledSince, ledSince = true, false, false
			continue
		}
		if !deep {
			continue
		}
		if deficit <= 0 {
			leveledSince = true
		}
		if deficit < 0 {
			ledSince = true
		}
	}
	return deep, leveledSince, ledSince
}

// concededJustBefore reports whether the goal just recorded answered an
// opponent goal inside the response window — the "instant reply" story.
func concededJustBefore(lm *worldgen.LiveMatch, home bool) bool {
	if len(lm.Scorers) < 2 {
		return false
	}
	prev := lm.Scorers[len(lm.Scorers)-2]
	prevByOpponent := prev.ClubID != lm.HomeID
	if !home {
		prevByOpponent = prev.ClubID == lm.HomeID
	}
	return prevByOpponent && lm.Clock-prev.Minute <= responseWindowMinutes
}

func (e *Engine) bookOne(lm *worldgen.LiveMatch, at sim.GameTime, r *rand.Rand) {
	club := lm.HomeID
	if r.Float64() < 0.5 {
		club = lm.AwayID
	}
	side := lm.OnPitch(club)
	pid := e.pickByDiscipline(side, r)
	if pid == 0 {
		return
	}
	detail, key := cardVerdict(lm, pid, r.Float64() < redCardShare)
	lm.Cards = append(lm.Cards, worldgen.MatchEvent{
		Minute: lm.Clock, PlayerID: pid, ClubID: club, Detail: detail,
	})
	e.comment(lm, at, key, map[string]any{"player": e.playerName(pid), "club": e.clubName(club)})
}

// cardVerdict settles what a booking becomes: a straight red, or — when the
// player already sits on a yellow — a second yellow that upgrades to a RED in
// the ledger (so OnPitch ejects on it) under its own commentary key. A red,
// straight or second-yellow, sends the player off with no replacement; the
// side plays short (football rules — the ejected slot cannot be filled).
func cardVerdict(lm *worldgen.LiveMatch, pid int64, straightRed bool) (detail, key string) {
	if straightRed {
		return "RED", "comment.card.red"
	}
	for _, c := range lm.Cards {
		if c.PlayerID == pid && c.Detail == "YELLOW" {
			return "RED", "comment.card.secondyellow"
		}
	}
	return "YELLOW", "comment.card.yellow"
}

// injureOne turns a moment's injury roll into a real injury: the
// victim takes the condition hit, rolls a lay-off scaled by its hidden
// InjuryProne/Recovery on the same moment stream, and leaves the pitch — a
// forced substitution if the bench has a fit body and subs remain, otherwise
// the side plays short. Named news carries only the player, club, and a coarse
// severity band; the exact lay-off derives from hidden attributes and stays
// engine-internal (FR-22).
func (e *Engine) injureOne(lm *worldgen.LiveMatch, at sim.GameTime, r *rand.Rand) {
	club := lm.HomeID
	if r.Float64() < 0.5 {
		club = lm.AwayID
	}
	side := lm.OnPitch(club)
	if len(side) == 0 {
		return
	}
	pid := side[r.IntN(len(side))]
	p := e.players[pid]
	if p == nil {
		return
	}
	p.Condition = clampInt(p.Condition-injuryConditionHit, 0, worldgen.ConditionMax)

	days := injuryDaysBase + r.IntN(injuryDaysSpan)
	days += p.Hidden[attr.InjuryProne] - 10    // fragile players stay out longer
	days -= (p.Hidden[attr.Recovery] - 10) / 2 // strong healers return sooner
	if days < 1 {
		days = 1
	}
	p.InjuredUntil = at + sim.GameTime(int64(days)*sim.MinutesPerDay)
	band := injuryBand(days)
	p.Injuries = append(p.Injuries, worldgen.InjuryRecord{
		SeasonYear: worldgen.DateOf(at).Season, Band: band,
	})

	e.comment(lm, at, "comment.injury", map[string]any{"player": p.Name, "club": e.clubName(club)})
	params := map[string]any{"player": p.Name, "club": e.clubName(club)}
	e.addNews(worldgen.NewsItem{
		GameTime: at, Category: "injury", Key: injuryNewsKey(band), Params: params, ClubIDs: []int64{club},
	})
	e.emit(at, injuryNewsKey(band), cloneParams(params))

	e.withdrawInjured(lm, club, pid, at)
}

// injuryBand buckets a lay-off into the coarse severity band that is the only
// duration fact allowed on the wire (FR-22).
func injuryBand(days int) string {
	switch {
	case days < 7:
		return "DAYS"
	case days < 21:
		return "WEEKS"
	default:
		return "MONTH"
	}
}

func chooseChanceType(plan mindset.TacticalPlan, r *rand.Rand) string {
	types := []string{
		chanceCrossHeader, chanceCutback, chanceThroughBall, chanceLongShot,
		chanceSetPieceHeader, chanceScramble, chanceCounter,
	}
	weights := []int{12, 10, 11, 8, 7, 6, 8}
	switch plan.Width {
	case "WIDE":
		weights[0] += 9
		weights[1] += 6
	case "NARROW":
		weights[2] += 7
		weights[0] -= 4
	}
	switch plan.Directness {
	case "DIRECT":
		weights[0] += 6
		weights[4] += 3
		weights[6] += 4
	case "SHORT":
		weights[1] += 6
		weights[2] += 5
		weights[3] -= 3
	}
	switch plan.Tempo {
	case "FAST":
		weights[3] += 3
		weights[6] += 6
	case "SLOW":
		weights[1] += 4
		weights[2] += 3
		weights[6] -= 3
	}
	switch plan.Pressing {
	case "HIGH":
		weights[6] += 5
	case "LOW":
		weights[6] -= 2
	}
	if plan.Counter {
		weights[6] += 8
	}
	for i := range weights {
		if weights[i] < 1 {
			weights[i] = 1
		}
	}
	return types[pickWeightedIndex(weights, r)]
}

func injuryNewsKey(band string) string {
	switch band {
	case "DAYS":
		return "news.injury.days"
	case "WEEKS":
		return "news.injury.weeks"
	default:
		return "news.injury.month"
	}
}

func missCommentKeys(chanceType string) []string {
	switch chanceType {
	case chanceCrossHeader:
		return []string{"comment.chance.cross.1", "comment.chance.cross.2", "comment.chance.cross.3", "comment.chance.cross.4", "comment.save.cross.1", "comment.save.cross.2"}
	case chanceCutback:
		return []string{"comment.chance.cutback.1", "comment.chance.cutback.2", "comment.chance.cutback.3", "comment.chance.cutback.4", "comment.save.cutback.1", "comment.save.cutback.2"}
	case chanceThroughBall:
		return []string{"comment.chance.through.1", "comment.chance.through.2", "comment.chance.through.3", "comment.chance.through.4", "comment.save.through.1", "comment.save.through.2"}
	case chanceLongShot:
		return []string{"comment.chance.long.1", "comment.chance.long.2", "comment.chance.long.3", "comment.chance.long.4", "comment.save.long.1", "comment.save.long.2"}
	case chanceSetPieceHeader:
		return []string{"comment.chance.setpiece.1", "comment.chance.setpiece.2", "comment.chance.setpiece.3", "comment.chance.setpiece.4", "comment.save.setpiece.1", "comment.save.setpiece.2"}
	case chanceCounter:
		return []string{"comment.chance.counter.1", "comment.chance.counter.2", "comment.chance.counter.3", "comment.chance.counter.4", "comment.save.counter.1", "comment.save.counter.2"}
	case chanceScramble:
		return []string{"comment.chance.scramble.1", "comment.chance.scramble.2", "comment.chance.scramble.3", "comment.chance.scramble.4", "comment.save.scramble.1", "comment.save.scramble.2"}
	default:
		return []string{"comment.chance.scramble.1", "comment.chance.scramble.2", "comment.chance.scramble.3", "comment.chance.scramble.4", "comment.save.scramble.1", "comment.save.scramble.2"}
	}
}

func goalCommentKeys(chanceType string) []string {
	switch chanceType {
	case chanceCrossHeader:
		return []string{"comment.goal.cross.1", "comment.goal.cross.2", "comment.goal.cross.3", "comment.goal.cross.4"}
	case chanceCutback:
		return []string{"comment.goal.cutback.1", "comment.goal.cutback.2", "comment.goal.cutback.3", "comment.goal.cutback.4"}
	case chanceThroughBall:
		return []string{"comment.goal.through.1", "comment.goal.through.2", "comment.goal.through.3", "comment.goal.through.4"}
	case chanceLongShot:
		return []string{"comment.goal.long.1", "comment.goal.long.2", "comment.goal.long.3", "comment.goal.long.4"}
	case chanceSetPieceHeader:
		return []string{"comment.goal.setpiece.1", "comment.goal.setpiece.2", "comment.goal.setpiece.3", "comment.goal.setpiece.4"}
	case chanceCounter:
		return []string{"comment.goal.counter.1", "comment.goal.counter.2", "comment.goal.counter.3", "comment.goal.counter.4"}
	case chanceScramble:
		return []string{"comment.goal.scramble.1", "comment.goal.scramble.2", "comment.goal.scramble.3", "comment.goal.scramble.4"}
	default:
		return []string{"comment.goal.scramble.1", "comment.goal.scramble.2", "comment.goal.scramble.3", "comment.goal.scramble.4"}
	}
}

func halftimeCommentaryKey(homeGoals, awayGoals int) string {
	margin := homeGoals - awayGoals
	switch {
	case homeGoals == 0 && awayGoals == 0:
		return "comment.halftime.goalless"
	case homeGoals == awayGoals:
		return "comment.halftime.level"
	case margin >= 3:
		return "comment.halftime.home_big_lead"
	case margin <= -3:
		return "comment.halftime.away_big_lead"
	case homeGoals > awayGoals:
		return "comment.halftime.home_lead"
	default:
		return "comment.halftime.away_lead"
	}
}

func fulltimeCommentaryKey(homeGoals, awayGoals int) string {
	margin := homeGoals - awayGoals
	switch {
	case margin == 0 && homeGoals == 0:
		return "comment.fulltime.goalless"
	case margin == 0:
		return "comment.fulltime.level"
	case margin == 1:
		return "comment.fulltime.home_edge"
	case margin == -1:
		return "comment.fulltime.away_edge"
	case margin >= 3:
		return "comment.fulltime.home_big"
	case margin <= -3:
		return "comment.fulltime.away_big"
	case margin > 0:
		return "comment.fulltime.home_win"
	default:
		return "comment.fulltime.away_win"
	}
}

// withdrawInjured takes the injured player off: the strongest FIT bench player
// comes on when the side has substitutions left (deterministic — strength with
// an id tie-break, no dice), otherwise the withdrawal has no replacement and
// the side plays short from this minute. Either way the change is recorded in
// lm.Subs, which is what OnPitch/Participants derive from.
func (e *Engine) withdrawInjured(lm *worldgen.LiveMatch, club int64, pid int64, at sim.GameTime) {
	sub := worldgen.SubEvent{Minute: lm.Clock, ClubID: club, Off: pid, Reason: subReasonInjury}
	if lm.SubsUsed(club) < subsMax {
		if rep := e.bestFitOnBench(lm, club, at); rep != 0 {
			sub.On = rep
		}
	}
	lm.Subs = append(lm.Subs, sub)
	if sub.On != 0 {
		e.comment(lm, at, "comment.sub", map[string]any{
			"off": e.playerName(pid), "on": e.playerName(sub.On), "club": e.clubName(club),
		})
		return
	}
	e.comment(lm, at, "comment.sub.short", map[string]any{
		"player": e.playerName(pid), "club": e.clubName(club),
	})
}

// bestFitOnBench picks the strongest bench player who hasn't already come on,
// been withdrawn, or picked up this match's injury.
func (e *Engine) bestFitOnBench(lm *worldgen.LiveMatch, club int64, at sim.GameTime) int64 {
	bench := lm.HomeBench
	if club == lm.AwayID {
		bench = lm.AwayBench
	}
	used := map[int64]bool{}
	for _, s := range lm.Subs {
		if s.ClubID != club {
			continue
		}
		used[s.Off] = true
		if s.On != 0 {
			used[s.On] = true
		}
	}
	var best *worldgen.Player
	for _, id := range bench {
		p := e.players[id]
		if p == nil || used[id] || p.InjuredUntil > at {
			continue
		}
		if best == nil || p.AbilityPool > best.AbilityPool ||
			(p.AbilityPool == best.AbilityPool && p.ID < best.ID) {
			best = p
		}
	}
	if best == nil {
		return 0
	}
	return best.ID
}

// maybeDiscretionarySubs lets each side make voluntary changes:
// from tacticalSubFromMinute, home considered before away (fixed order), at
// most one change per side per moment. A fatigue withdrawal fires dice-free
// when the tiredest on-pitch outfielder's derived in-match condition drops
// under fatigueSubThreshold; otherwise a fresh-legs upgrade rolls
// tacticalSubProb on the moment stream when the bench holds a strictly
// stronger outfielder than the weakest on the pitch. Unlike the injury path a
// discretionary change NEVER goes short — no replacement, no substitution —
// and while only one sub remains it is held back for injuries until
// subReserveUntil. The conditional dice draw is the considerPush pattern:
// state at draw time is replay-identical, so consumption stays deterministic.
func (e *Engine) maybeDiscretionarySubs(lm *worldgen.LiveMatch, at sim.GameTime, r *rand.Rand) {
	if lm.Clock < tacticalSubFromMinute {
		return
	}
	e.considerDiscretionarySub(lm, at, r, lm.HomeID)
	e.considerDiscretionarySub(lm, at, r, lm.AwayID)
}

func (e *Engine) considerDiscretionarySub(lm *worldgen.LiveMatch, at sim.GameTime, r *rand.Rand, club int64) {
	used := lm.SubsUsed(club)
	if used >= subsMax || (used >= subsMax-1 && lm.Clock < subReserveUntil) {
		return
	}
	// Fatigue first — dice-free, like the injury withdrawal.
	if off := e.tiredestOutfielder(lm, club); off != 0 {
		if on := e.bestOutfieldOnBench(lm, club, at); on != 0 {
			e.recordDiscretionarySub(lm, at, club, off, on, subReasonFatigue, fatigueSubCommentaryKey(lm))
			return
		}
	}
	// Fresh legs: only rolled when a strictly stronger bench body exists, so
	// the changes stagger across moments rather than landing all at once.
	off, on := e.freshLegsUpgrade(lm, club, at)
	if off == 0 || on == 0 {
		return
	}
	if r.Float64() >= tacticalSubProb {
		return
	}
	e.recordDiscretionarySub(lm, at, club, off, on, subReasonTactical, "comment.sub.fresh")
}

var fatigueSubCommentaryKeys = []string{
	"comment.sub.fatigue.1",
	"comment.sub.fatigue.2",
	"comment.sub.fatigue.3",
	"comment.sub.fatigue.4",
	"comment.sub.fatigue.5",
	"comment.sub.fatigue.6",
}

// fatigueSubCommentaryKey advances the whole match through the presentation
// pool. With subsMax per side, the six-key pool covers today's subsMax*2 match
// capacity, so normal play never repeats a fatigue line. The choice reads
// persisted substitution facts and consumes no RNG, keeping richer prose
// separate from match results.
func fatigueSubCommentaryKey(lm *worldgen.LiveMatch) string {
	used := 0
	for _, sub := range lm.Subs {
		if sub.Reason == subReasonFatigue {
			used++
		}
	}
	return fatigueSubCommentaryKeys[used%len(fatigueSubCommentaryKeys)]
}

func (e *Engine) recordDiscretionarySub(lm *worldgen.LiveMatch, at sim.GameTime, club, off, on int64, reason, key string) {
	lm.Subs = append(lm.Subs, worldgen.SubEvent{
		Minute: lm.Clock, ClubID: club, Off: off, On: on, Reason: reason,
	})
	e.comment(lm, at, key, map[string]any{
		"off": e.playerName(off), "on": e.playerName(on), "club": e.clubName(club),
	})
}

// condAt is a player's DERIVED in-match condition: the pre-match value minus
// the pro-rated share of the full-time drain for the minutes played so far.
// Decision-only — play quality (effective/teamStrength) still reads the
// stored pre-match Condition, and the real drain lands once at full time in
// applyPostMatch; per-moment condition mutation is intentionally avoided.
func condAt(p *worldgen.Player, minutesOn int) int {
	return p.Condition - conditionDrainPlay*minutesOn/matchFullTimeMinutes
}

// LivePlayerCondition is the public 0..100 condition shown for a player who is
// currently on the pitch. Match simulation keeps the pre-match value stored
// until full time for deterministic single-write accounting, so observers must
// use this derived view instead of presenting that stale stored value. A
// substitute drains only for the minutes since coming on. Callers must use it
// only while the match is live, before applyPostMatch commits the full-time
// drain to the stored player state.
func LivePlayerCondition(p *worldgen.Player, lm *worldgen.LiveMatch) int {
	return clampInt(condAt(p, minutesOn(lm, p.ID)), 0, worldgen.ConditionMax)
}

// minutesOn is how long a player has been on the pitch: since kickoff for a
// starter, since their sub-on minute otherwise.
func minutesOn(lm *worldgen.LiveMatch, pid int64) int {
	for _, s := range lm.Subs {
		if s.On == pid {
			return lm.Clock - s.Minute
		}
	}
	return lm.Clock
}

// tiredestOutfielder returns the on-pitch outfielder whose derived condition
// sits under fatigueSubThreshold — the lowest first, id tie-break (dice-free).
// The keeper is never discretionarily withdrawn.
func (e *Engine) tiredestOutfielder(lm *worldgen.LiveMatch, club int64) int64 {
	var worst *worldgen.Player
	worstCond := fatigueSubThreshold
	for _, pid := range lm.OnPitch(club) {
		p := e.players[pid]
		if p == nil || p.Group == attr.GK {
			continue
		}
		c := condAt(p, minutesOn(lm, pid))
		if c < worstCond || (c == worstCond && worst != nil && p.ID < worst.ID) {
			worst, worstCond = p, c
		}
	}
	if worst == nil {
		return 0
	}
	return worst.ID
}

// bestOutfieldOnBench is bestFitOnBench restricted to outfielders — a
// voluntary change never brings a keeper on for an outfield slot.
func (e *Engine) bestOutfieldOnBench(lm *worldgen.LiveMatch, club int64, at sim.GameTime) int64 {
	bench := lm.HomeBench
	if club == lm.AwayID {
		bench = lm.AwayBench
	}
	used := map[int64]bool{}
	for _, s := range lm.Subs {
		if s.ClubID != club {
			continue
		}
		used[s.Off] = true
		if s.On != 0 {
			used[s.On] = true
		}
	}
	var best *worldgen.Player
	for _, id := range bench {
		p := e.players[id]
		if p == nil || used[id] || p.InjuredUntil > at || p.Group == attr.GK {
			continue
		}
		if best == nil || p.AbilityPool > best.AbilityPool ||
			(p.AbilityPool == best.AbilityPool && p.ID < best.ID) {
			best = p
		}
	}
	if best == nil {
		return 0
	}
	return best.ID
}

// freshLegsUpgrade finds a tactical swap: the weakest on-pitch outfielder and
// the strongest fit outfield bench body, returned only when the bench body is
// STRICTLY stronger (Ability Pool; id tie-breaks) — otherwise no change is
// worth a card.
func (e *Engine) freshLegsUpgrade(lm *worldgen.LiveMatch, club int64, at sim.GameTime) (off, on int64) {
	var weakest *worldgen.Player
	for _, pid := range lm.OnPitch(club) {
		p := e.players[pid]
		if p == nil || p.Group == attr.GK {
			continue
		}
		if weakest == nil || p.AbilityPool < weakest.AbilityPool ||
			(p.AbilityPool == weakest.AbilityPool && p.ID < weakest.ID) {
			weakest = p
		}
	}
	if weakest == nil {
		return 0, 0
	}
	onID := e.bestOutfieldOnBench(lm, club, at)
	if onID == 0 {
		return 0, 0
	}
	if b := e.players[onID]; b == nil || b.AbilityPool <= weakest.AbilityPool {
		return 0, 0
	}
	return weakest.ID, onID
}

// finalizeMatch closes the match: it computes ratings, records the result and
// standings, updates season stats and condition, clears the live tally, and
// announces the score.
func (e *Engine) finalizeMatch(ev *sim.Event, lm *worldgen.LiveMatch) error {
	lm.Clock = matchFullTimeMinutes
	e.comment(lm, ev.Due, fulltimeCommentaryKey(lm.HomeGoals, lm.AwayGoals), map[string]any{
		"home": e.clubName(lm.HomeID), "away": e.clubName(lm.AwayID),
		"home_goals": lm.HomeGoals, "away_goals": lm.AwayGoals,
	})

	// A cup tie needs a winner to advance: the higher scorer, or a
	// penalty shootout when level. The shootout line joins the running commentary
	// BEFORE the result is snapshotted below, so get_match shows who went through.
	var winner int64
	if lm.Competition == worldgen.CompetitionCup {
		homePens, awayPens, shootout := 0, 0, false
		winner, homePens, awayPens, shootout = e.resolveCupWinner(lm)
		if shootout {
			e.comment(lm, ev.Due, "comment.shootout", map[string]any{
				"winner": e.clubName(winner),
				"home":   e.clubName(lm.HomeID), "away": e.clubName(lm.AwayID),
				"home_pens": homePens, "away_pens": awayPens,
			})
		}
	}

	res := worldgen.MatchResult{
		FixtureID: lm.FixtureID, Competition: lm.Competition, DivisionTier: lm.DivisionTier,
		HomeID: lm.HomeID, AwayID: lm.AwayID,
		HomeGoals: lm.HomeGoals, AwayGoals: lm.AwayGoals, Winner: winner,
		Kickoff: lm.Kickoff, HomeXI: lm.HomeXI, AwayXI: lm.AwayXI, Subs: lm.Subs,
		Scorers: lm.Scorers, Cards: lm.Cards,
		RatingsX10:        e.ratings(lm),
		Commentary:        lm.Commentary,
		Adjustments:       lm.Adjustments,
		HomeShots:         lm.HomeShots,
		AwayShots:         lm.AwayShots,
		ChanceTypes:       cloneChanceTypes(lm.ChanceTypes),
		ChanceTypesBySide: cloneChanceTypes(lm.ChanceTypesBySide),
		Diagnostics:       lm.Diagnostics,
	}
	e.world.Results = append(e.world.Results, res)

	// League results feed the table; cup results advance the bracket.
	if lm.Competition == worldgen.CompetitionLeague {
		e.world.RecordLeagueResult(lm.DivisionTier, lm.HomeID, lm.AwayID, lm.HomeGoals, lm.AwayGoals)
		// Board confidence moves on league results vs expectation (board confidence),
		// then the board reviews the manager (board review) — both under queue order.
		if home, away := e.clubs[lm.HomeID], e.clubs[lm.AwayID]; home != nil && away != nil {
			moveConfidence(home, away, lm.HomeGoals, lm.AwayGoals)
			moveConfidence(away, home, lm.AwayGoals, lm.HomeGoals)
			e.evaluateSacking(ev, home)
			e.evaluateSacking(ev, away)
		}
	} else {
		e.advanceCup(ev.Due, lm)
	}
	e.applyPostMatch(lm, res.RatingsX10)
	delete(e.world.LiveMatches, lm.FixtureID)

	params := map[string]any{
		"home": e.clubName(lm.HomeID), "away": e.clubName(lm.AwayID),
		"home_goals": lm.HomeGoals, "away_goals": lm.AwayGoals,
		"competition": lm.Competition,
	}
	// Per-fixture result events still feed the realtime ticker. Persistent
	// Media news is grouped into matchday round-ups below.
	// A cup tie names the club that advanced (decisive or on penalties) in the
	// headline itself; the league line has no winner to name. Separate keys keep
	// the {winner} placeholder out of league renders (a missing param renders
	// literally).
	resultKey := FeedMatchResult
	if winner != 0 {
		params["winner"] = e.clubName(winner)
		resultKey = FeedCupResult
	}
	e.emit(ev.Due, resultKey, params)
	if f, ok := e.fixtures[lm.FixtureID]; ok {
		e.issueMatchAlerts(ev.Due, f, "OWN_FULL_TIME")
	}
	matchdayNews := e.addMatchdayResultsNews(ev.Due, lm.Kickoff, lm.Competition, lm.DivisionTier)
	factors := map[string]any{
		"result": []int{lm.HomeGoals, lm.AwayGoals}, "fixture": lm.FixtureID,
	}
	if matchdayNews != nil {
		factors["matchday_news"] = matchdayNews
	}
	return e.log(ev, "match", factors, "full_time", 0, 0)
}

func (e *Engine) addMatchdayResultsNews(at, kickoff sim.GameTime, competition string, division int) map[string]any {
	if e.newsExists(FeedMatchdayResults, kickoff, competition, division) {
		return nil
	}
	fixtures := e.fixturesAt(kickoff, competition, division)
	if len(fixtures) == 0 {
		return nil
	}
	results := e.resultsAt(kickoff, competition, division)
	// Generated fixtures currently always produce exactly one result. Until the
	// engine has terminal postponed/cancelled fixture states, matchday round-ups
	// are emitted only when every generated fixture in this window has a result;
	// deferrals are recorded in the full-time audit factors below.
	if len(results) != len(fixtures) {
		return map[string]any{
			"status":      "deferred",
			"reason":      "result_count_mismatch",
			"kickoff":     int64(kickoff),
			"competition": competition,
			"division":    division,
			"fixtures":    len(fixtures),
			"results":     len(results),
		}
	}
	params := e.matchdayBaseParams(kickoff, competition, division, len(results))
	params["results"] = e.resultPayloads(results)
	params["table"] = e.tableSnapshotPayloads(results)
	params["story"] = e.resultStoryPayload(results)
	e.addNews(worldgen.NewsItem{
		GameTime: at, Category: "match", Key: FeedMatchdayResults,
		Params: params, ClubIDs: fixtureClubRefs(fixtures),
	})
	return nil
}

func (e *Engine) newsExists(key string, kickoff sim.GameTime, competition string, division int) bool {
	// World.News is appended in monotonic GameTime order; the backward scan can
	// stop once it reaches news older than the target match window.
	for i := len(e.world.News) - 1; i >= 0; i-- {
		n := e.world.News[i]
		if n.GameTime < kickoff {
			break
		}
		if n.Key != key {
			continue
		}
		gotKickoff, ok := newsParamInt64(n.Params["kickoff"])
		gotDivision, okDivision := newsParamInt64(n.Params["division"])
		if ok && okDivision && gotKickoff == int64(kickoff) &&
			n.Params["competition"] == competition && gotDivision == int64(division) {
			return true
		}
	}
	return false
}

func newsParamInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), n == float64(int64(n))
	case json.Number:
		got, err := n.Int64()
		return got, err == nil
	default:
		return 0, false
	}
}

func (e *Engine) fixturesAt(kickoff sim.GameTime, competition string, division int) []*worldgen.Fixture {
	out := []*worldgen.Fixture{}
	for i := range e.world.Fixtures {
		f := &e.world.Fixtures[i]
		if f.Kickoff == kickoff && f.Competition == competition && f.DivisionTier == division {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Competition != b.Competition {
			return a.Competition < b.Competition
		}
		if a.DivisionTier != b.DivisionTier {
			return a.DivisionTier < b.DivisionTier
		}
		if a.Round != b.Round {
			return a.Round < b.Round
		}
		return a.ID < b.ID
	})
	return out
}

func (e *Engine) resultsAt(kickoff sim.GameTime, competition string, division int) []worldgen.MatchResult {
	out := []worldgen.MatchResult{}
	for _, r := range e.world.Results {
		if r.Kickoff == kickoff && r.Competition == competition && r.DivisionTier == division {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Competition != b.Competition {
			return a.Competition < b.Competition
		}
		if a.DivisionTier != b.DivisionTier {
			return a.DivisionTier < b.DivisionTier
		}
		if a.FixtureID != b.FixtureID {
			return a.FixtureID < b.FixtureID
		}
		return a.HomeID < b.HomeID
	})
	return out
}

func (e *Engine) matchdayBaseParams(kickoff sim.GameTime, competition string, division int, count int) map[string]any {
	d := worldgen.DateOf(kickoff)
	return map[string]any{
		"kickoff":      int64(kickoff),
		"competition":  competition,
		"division":     division,
		"count":        count,
		"season":       d.Season,
		"month":        d.Month,
		"day":          d.Day,
		"kickoff_time": fmt.Sprintf("%02d:%02d", d.Hour, d.Minute),
	}
}

func (e *Engine) resultPayloads(results []worldgen.MatchResult) []map[string]any {
	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		payload := map[string]any{
			"home":       e.clubName(r.HomeID),
			"away":       e.clubName(r.AwayID),
			"home_goals": r.HomeGoals,
			"away_goals": r.AwayGoals,
		}
		if r.Winner != 0 {
			payload["winner"] = e.clubName(r.Winner)
		}
		out = append(out, payload)
	}
	return out
}

func (e *Engine) tableSnapshotPayloads(results []worldgen.MatchResult) []map[string]any {
	seen := map[int]bool{}
	out := []map[string]any{}
	for _, r := range results {
		if r.Competition != worldgen.CompetitionLeague || seen[r.DivisionTier] ||
			r.DivisionTier < 1 || r.DivisionTier > len(e.world.Table) {
			continue
		}
		seen[r.DivisionTier] = true
		table := e.world.Table[r.DivisionTier-1]
		if len(table) == 0 {
			continue
		}
		leader := table[0]
		out = append(out, map[string]any{
			"division": r.DivisionTier,
			"club":     e.clubName(leader.ClubID),
			"points":   leader.Points,
		})
	}
	return out
}

func (e *Engine) resultStoryPayload(results []worldgen.MatchResult) map[string]any {
	if len(results) == 0 {
		return nil
	}
	var best worldgen.MatchResult
	bestMargin := 0
	draws := 0
	for _, r := range results {
		if r.HomeGoals == r.AwayGoals {
			draws++
		}
		if margin := absInt(r.HomeGoals - r.AwayGoals); margin > bestMargin {
			best, bestMargin = r, margin
		}
	}
	out := map[string]any{
		"best_margin": bestMargin,
		"draws":       draws,
	}
	if bestMargin > 0 {
		out["best_home"] = e.clubName(best.HomeID)
		out["best_away"] = e.clubName(best.AwayID)
		out["home_goals"] = best.HomeGoals
		out["away_goals"] = best.AwayGoals
	}
	return out
}

func fixtureClubRefs(fixtures []*worldgen.Fixture) []int64 {
	seen := map[int64]bool{}
	out := []int64{}
	for _, f := range fixtures {
		for _, id := range []int64{f.HomeID, f.AwayID} {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

// applyPostMatch folds the finished match into each PARTICIPANT's season line
// (starters and sub-ons alike — anyone who took the pitch earns the
// appearance) and drains condition / builds sharpness.
func (e *Engine) applyPostMatch(lm *worldgen.LiveMatch, ratings map[int64]int) {
	goals := map[int64]int{}
	for _, s := range lm.Scorers {
		goals[s.PlayerID]++
	}
	for _, pid := range append(lm.Participants(lm.HomeID), lm.Participants(lm.AwayID)...) {
		p := e.players[pid]
		if p == nil {
			continue
		}
		p.SeasonApps++
		p.SeasonGoals += goals[pid]
		p.RatingSumX10 += ratings[pid]
		// Rolling form: the last formWindow ratings. It is observational only,
		// never a match-strength input, because a form multiplier would make
		// hot streaks self-reinforce.
		p.FormX10 = append(p.FormX10, ratings[pid])
		if len(p.FormX10) > formWindow {
			p.FormX10 = p.FormX10[len(p.FormX10)-formWindow:]
		}
		p.Condition = clampInt(p.Condition-conditionDrainPlay, conditionFloorPlay, worldgen.ConditionMax)
		p.Sharpness = clampInt(p.Sharpness+sharpnessGainPlay, 0, worldgen.ConditionMax)
	}
}

// ratings scores every participant at full time. The formula lives in
// worldgen.LiveRatingsX10 — one code location shared with the Console's live
// ratings pane (docs/98) — and is a pure function of the tally, so the
// full-time call and a mid-match "as if it ended now" render agree.
func (e *Engine) ratings(lm *worldgen.LiveMatch) map[int64]int {
	return worldgen.LiveRatingsX10(lm, func(id int64) *worldgen.Player { return e.players[id] })
}

// selectSquad picks a club's matchday squad at kickoff: the strongest FIT
// goalkeeper plus the ten strongest fit outfielders by Ability Pool (id
// tie-breaks — replay-identical) as the XI, and the next benchSize fit players
// by the same ranking as the bench. Fit = senior
// and not injured at kickoff — the InjuredUntil timestamp comparison IS the
// whole recovery model (no recovery event exists).
func (e *Engine) selectSquad(clubID int64, at sim.GameTime, plan mindset.TacticalPlan) (xi, bench []int64) {
	var gks, out []*worldgen.Player
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.ClubID != clubID || p.Youth || p.InjuredUntil > at {
			continue
		}
		if p.Group == attr.GK {
			gks = append(gks, p)
		} else {
			out = append(out, p)
		}
	}
	byStrength := func(s []*worldgen.Player) {
		sort.Slice(s, func(i, j int) bool {
			si := selectionScore(s[i], plan)
			sj := selectionScore(s[j], plan)
			if si != sj {
				return si > sj
			}
			return s[i].ID < s[j].ID
		})
	}
	byStrength(gks)
	byStrength(out)
	xi = make([]int64, 0, 11)
	if len(gks) > 0 {
		xi = append(xi, gks[0].ID)
	}
	n := 0
	for ; n < len(out) && len(xi) < 11; n++ {
		xi = append(xi, out[n].ID)
	}
	// Bench: the best of everyone left over (spare keepers included), re-ranked.
	var spare []*worldgen.Player
	spare = append(spare, out[n:]...)
	if len(gks) > 1 {
		spare = append(spare, gks[1:]...)
	}
	byStrength(spare)
	for i := 0; i < len(spare) && i < benchSize; i++ {
		bench = append(bench, spare[i].ID)
	}
	return xi, bench
}

// mentalityLevel maps the mentality dial to a −2…+2 attacking bias, so a
// manager's in-match shift can add to it numerically.
func mentalityLevel(m string) int {
	switch m {
	case "VERY_ATTACKING":
		return 2
	case "ATTACKING":
		return 1
	case "DEFENSIVE":
		return -1
	case "VERY_DEFENSIVE":
		return -2
	}
	return 0 // BALANCED or unset
}

// teamStrength aggregates an XI into integer attack and defense ratings,
// scaled by each player's condition/sharpness and shifted by the effective
// mentality (base dial + the in-match shift). Summation is over the XI slice
// (deterministic order, integer math).
func (e *Engine) teamStrength(xi []int64, plan mindset.TacticalPlan, shift int) (attack, defense int) {
	for _, pid := range xi {
		p := e.players[pid]
		if p == nil {
			continue
		}
		fit := (100 + p.Condition + p.Sharpness) // 100..300, integer scale /200 below
		a := (effective(p, attr.Finishing) + effective(p, attr.Dribbling) +
			effective(p, attr.Passing) + effective(p, attr.Acceleration) +
			effective(p, attr.Pace) + effective(p, attr.OffBall) +
			bodyReach(p)/2)
		d := (effective(p, attr.Tackling) + effective(p, attr.Marking) +
			effective(p, attr.Positioning) + effective(p, attr.Concentration) +
			bodyStrength(p) + bodyReach(p)/2)
		if p.Group == attr.GK {
			d += effective(p, attr.Reflexes) + effective(p, attr.Handling) + effective(p, attr.CommandOfArea)
		}
		attack += a * fit / 200
		defense += d * fit / 200
	}
	level := clampInt(mentalityLevel(plan.Mentality)+shift, -2, 2)
	attack += attack * level / 10   // +10% attack per attacking step
	defense -= defense * level / 10 // −10% defense per attacking step
	return attack, defense
}

// pickScorerForChance weights actors by the event they are actually receiving.
func (e *Engine) pickScorerForChance(xi []int64, chanceType string, r *rand.Rand) int64 {
	weights := make([]int, len(xi))
	for i, pid := range xi {
		p := e.players[pid]
		if p == nil {
			continue
		}
		w := e.chanceActorWeight(p, chanceType)
		switch p.Group {
		case attr.FW:
			w = w * 4 / 2
		case attr.MF:
			w = w * 3 / 2
		}
		weights[i] = w
	}
	return pickWeighted(xi, weights, r)
}

func (e *Engine) chanceActorWeight(p *worldgen.Player, chanceType string) int {
	roleBonus := tacticalRoleBonus(playerTacticalRole(p), chanceType)
	switch chanceType {
	case chanceCrossHeader, chanceSetPieceHeader:
		return factorHeaderQuality(p) + effective(p, attr.OffBall) + effective(p, attr.Bravery) + roleBonus
	case chanceCutback:
		return factorShotQuality(p, attr.Finishing) + effective(p, attr.OffBall) + roleBonus
	case chanceThroughBall:
		return factorSeparation(p) + effective(p, attr.FirstTouch) + effective(p, attr.Finishing) + roleBonus
	case chanceLongShot:
		return factorShotQuality(p, attr.LongShots) + roleBonus
	case chanceCounter:
		return factorSeparation(p) + effective(p, attr.Decisions) + effective(p, attr.Finishing) + roleBonus
	default:
		return effective(p, attr.Anticipation) + effective(p, attr.Bravery) + effective(p, attr.Finishing) + roleBonus
	}
}

func (e *Engine) chanceAttackScore(p *worldgen.Player, xi []int64, chanceType string) int {
	roleBonus := tacticalRoleBonus(playerTacticalRole(p), chanceType)
	switch chanceType {
	case chanceCrossHeader:
		return e.teamDelivery(xi, attr.Crossing) + factorHeaderQuality(p) + effective(p, attr.Bravery) + effective(p, attr.OffBall) + roleBonus
	case chanceSetPieceHeader:
		return e.teamDelivery(xi, attr.SetPieces) + factorHeaderQuality(p) + factorDuelPower(p)/2 + effective(p, attr.Bravery) + roleBonus
	case chanceCutback:
		return e.teamDelivery(xi, attr.Crossing) + factorShotQuality(p, attr.Finishing) + effective(p, attr.OffBall) + roleBonus
	case chanceThroughBall:
		return e.teamDelivery(xi, attr.Passing) + factorSeparation(p) + factorBallSecurity(p)/2 + factorShotQuality(p, attr.Finishing) + roleBonus
	case chanceLongShot:
		return e.teamDelivery(xi, attr.Passing)/2 + factorShotQuality(p, attr.LongShots)*2 + roleBonus
	case chanceCounter:
		return e.teamPressImpact(xi) + factorSeparation(p) + effective(p, attr.Decisions) + factorShotQuality(p, attr.Finishing) + roleBonus
	default:
		return effective(p, attr.Anticipation) + effective(p, attr.Bravery) + factorShotQuality(p, attr.Finishing) + e.teamDelivery(xi, attr.Passing)/2 + roleBonus
	}
}

func (e *Engine) chanceDefenseScore(xi []int64, chanceType string) int {
	bestDef, keeper := 0, 0
	for _, pid := range xi {
		p := e.players[pid]
		if p == nil {
			continue
		}
		if p.Group == attr.GK {
			keeper = max(keeper, e.keeperResponse(p, chanceType))
			continue
		}
		bestDef = max(bestDef, defensiveRead(p)+factorDuelPower(p)/3)
	}
	return bestDef + keeper
}

func (e *Engine) teamDelivery(xi []int64, delivery attr.Visible) int {
	best := 0
	for _, pid := range xi {
		if p := e.players[pid]; p != nil && p.Group != attr.GK {
			best = max(best, factorDeliveryQuality(p, delivery))
		}
	}
	return best
}

func (e *Engine) teamPressImpact(xi []int64) int {
	best := 0
	for _, pid := range xi {
		if p := e.players[pid]; p != nil && p.Group != attr.GK {
			best = max(best, factorPressImpact(p))
		}
	}
	return best
}

func defensiveRead(p *worldgen.Player) int {
	return factorDefensiveRead(p)
}

func (e *Engine) keeperResponse(p *worldgen.Player, chanceType string) int {
	switch chanceType {
	case chanceCrossHeader, chanceSetPieceHeader:
		return effective(p, attr.AerialReach) + effective(p, attr.CommandOfArea) + effective(p, attr.Handling)
	case chanceThroughBall, chanceCounter:
		return effective(p, attr.Sweeping) + effective(p, attr.OneOnOnes) + effective(p, attr.Reflexes)
	default:
		return effective(p, attr.Reflexes) + effective(p, attr.Handling) + effective(p, attr.Concentration)
	}
}

// pickByDiscipline targets the least disciplined player for a card.
func (e *Engine) pickByDiscipline(xi []int64, r *rand.Rand) int64 {
	weights := make([]int, len(xi))
	for i, pid := range xi {
		p := e.players[pid]
		if p == nil {
			continue
		}
		// Contact load comes from visible Aggression; bad Temperament and low
		// Sportsmanship turn that contact into cards.
		weights[i] = effective(p, attr.Aggression) + (21 - clampInt(p.Hidden[attr.Temperament], 1, 20)) + (21 - clampInt(p.Hidden[attr.Sportsmanship], 1, 20))
	}
	return pickWeighted(xi, weights, r)
}

func pickWeightedIndex(weights []int, r *rand.Rand) int {
	total := 0
	for _, w := range weights {
		if w > 0 {
			total += w
		}
	}
	if total == 0 {
		return 0
	}
	roll := r.IntN(total)
	for i, w := range weights {
		if w <= 0 {
			continue
		}
		roll -= w
		if roll < 0 {
			return i
		}
	}
	return len(weights) - 1
}

// pickWeighted draws one id from ids with the given weights using r, iterating
// in slice order (deterministic). Returns 0 if every weight is non-positive.
func pickWeighted(ids []int64, weights []int, r *rand.Rand) int64 {
	total := 0
	for _, w := range weights {
		if w > 0 {
			total += w
		}
	}
	if total == 0 {
		return 0
	}
	roll := r.IntN(total)
	for i, id := range ids {
		if weights[i] <= 0 {
			continue
		}
		roll -= weights[i]
		if roll < 0 {
			return id
		}
	}
	return 0
}

// managerForClub returns a club's employed manager (exactly one per club, so
// the map scan is order-independent — the returned value is deterministic).
func (e *Engine) managerForClub(clubID int64) *worldgen.Manager {
	for id := range e.managers {
		if m := e.managers[id]; m.ClubID == clubID {
			return m
		}
	}
	return nil
}

// tacticalFor returns the tactical plan of a club's manager, or the zero plan
// when a club has no employed manager (defensive; league clubs always do).
func (e *Engine) tacticalFor(clubID int64) mindset.TacticalPlan {
	if m := e.managerForClub(clubID); m != nil {
		return m.Mindset.Tactical
	}
	return mindset.TacticalPlan{}
}

// maybeAdjust lets a chasing manager gamble in the closing stages: it reads
// live Mindset (Risk Appetite) — a read, so no single-writer violation — and,
// if it fires, bumps the in-match MentalityShift on LiveMatch (never the
// agent's Mindset). Home is considered before away, a fixed order, and the
// probability roll is drawn only when a side is actually chasing, so the
// stream consumption stays deterministic.
func (e *Engine) maybeAdjust(lm *worldgen.LiveMatch, at sim.GameTime, r *rand.Rand) {
	if lm.Clock < adjustFromMinute {
		return
	}
	e.considerPush(lm, at, r, true)
	e.considerPush(lm, at, r, false)
}

func (e *Engine) considerPush(lm *worldgen.LiveMatch, at sim.GameTime, r *rand.Rand, home bool) {
	deficit, club, shift := lm.HomeGoals-lm.AwayGoals, lm.AwayID, &lm.AwayMentalityShift
	if home {
		deficit, club, shift = lm.AwayGoals-lm.HomeGoals, lm.HomeID, &lm.HomeMentalityShift
	}
	if deficit <= 0 || *shift >= maxMentalityShift {
		return // not chasing, or already all-in
	}
	risk := 0
	if m := e.managerForClub(club); m != nil {
		risk = m.Mindset.Disposition.Current[mindset.AxisRiskAppetite]
	}
	pAdjust := adjustBaseProb + float64(risk+10)/20*adjustRiskWeight // risk −10..+10 → 0…1
	if r.Float64() >= pAdjust {
		return
	}
	*shift++
	commentaryKey := adjustmentCommentaryKey(lm, club)
	lm.Adjustments = append(lm.Adjustments, worldgen.Adjustment{
		Minute: lm.Clock, ClubID: club, Key: "adj.push",
	})
	e.comment(lm, at, commentaryKey, map[string]any{"club": e.clubName(club)})
}

var adjustmentCommentaryKeys = []string{
	"comment.adj.push.1",
	"comment.adj.push.2",
	"comment.adj.push.3",
	"comment.adj.push.4",
}

// adjustmentCommentaryKey advances each club through the presentation pool
// independently. It consumes no RNG, so richer prose cannot perturb match
// simulation, and a club never repeats the same adjustment line back-to-back.
func adjustmentCommentaryKey(lm *worldgen.LiveMatch, clubID int64) string {
	used := 0
	for _, adj := range lm.Adjustments {
		if adj.ClubID == clubID && adj.Key == "adj.push" {
			used++
		}
	}
	return adjustmentCommentaryKeys[used%len(adjustmentCommentaryKeys)]
}

// effective is a player's effective value for a visible attribute: the base,
// scaled by condition and sharpness (context). Per-event volatility is applied
// at the point of use (e.g. finishing wobble in resolveChance). Integer math
// keeps the result reproducible.
func effective(p *worldgen.Player, a attr.Visible) int {
	base := p.Visible[a]
	fit := 100 + p.Condition + p.Sharpness // 100..300
	return base * fit / 300
}

func consistency(p *worldgen.Player) int {
	return clampInt(p.Hidden[attr.Consistency], 1, 20)
}

func bodyReach(p *worldgen.Player) int {
	a := attr.JumpingReach
	if p.Group == attr.GK {
		a = attr.AerialReach
	}
	return clampInt(effective(p, a)+heightBonus(p), 1, attr.ScaleMax+bodyReachMaxBonus)
}

func bodyStrength(p *worldgen.Player) int {
	return clampInt(effective(p, attr.Strength)+massBonus(p), 1, attr.ScaleMax+bodyMassMaxBonus)
}

func heightBonus(p *worldgen.Player) int {
	height := p.HeightCm
	if height == 0 {
		height = bodyNeutralHeightCm
	}
	return clampInt((height-bodyNeutralHeightCm)/bodyHeightStepCm, bodyReachMinBonus, bodyReachMaxBonus)
}

func massBonus(p *worldgen.Player) int {
	weight := p.WeightKg
	if weight == 0 {
		weight = bodyNeutralWeightKg
	}
	return clampInt((weight-bodyNeutralWeightKg)/bodyWeightStepKg, bodyMassMinBonus, bodyMassMaxBonus)
}

func weakFootExpression(p *worldgen.Player) int {
	return clampInt(p.WeakFoot, attr.ScaleMin, attr.ScaleMax)
}

func (e *Engine) recordMatchDiagnostics(lm *worldgen.LiveMatch, home bool, chanceType string, attackScore, defenseScore int) {
	side := "AWAY"
	if home {
		side = "HOME"
	}
	lm.Diagnostics.AddShotQuality(side, shotQualityBand(attackScore-defenseScore))
	lm.Diagnostics.AddTilt(side, chanceFamily(chanceType))
	if chanceType == chanceCrossHeader || chanceType == chanceSetPieceHeader {
		lm.Diagnostics.AddSide(&lm.Diagnostics.AerialDuels, side)
		if attackScore >= defenseScore {
			lm.Diagnostics.AddSide(&lm.Diagnostics.AerialWins, side)
		}
	}
	if chanceType == chanceSetPieceHeader {
		lm.Diagnostics.AddSide(&lm.Diagnostics.SetPieceThreat, side)
	}
	if chanceType == chanceCounter {
		lm.Diagnostics.AddSide(&lm.Diagnostics.PressTurnovers, side)
	}
}

func shotQualityBand(delta int) string {
	switch {
	case delta >= shotQualityHighDelta:
		return "HIGH"
	case delta <= shotQualityLowDelta:
		return "LOW"
	default:
		return "MEDIUM"
	}
}

func chanceFamily(chanceType string) string {
	switch chanceType {
	case chanceCrossHeader, chanceCutback:
		return "WIDE"
	case chanceThroughBall:
		return "CENTRAL"
	case chanceLongShot:
		return "DISTANCE"
	case chanceSetPieceHeader:
		return "SET_PIECE"
	case chanceCounter:
		return "TRANSITION"
	default:
		return "SCRAMBLE"
	}
}

func cloneChanceTypes(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// comment records one commentary beat on the running match and streams it to
// the Console feed (the match-day broadcast). Params must be ints/strings only
// — a float here would reach the world hash via LiveMatch.Commentary (NFR-2).
func (e *Engine) comment(lm *worldgen.LiveMatch, at sim.GameTime, key string, params map[string]any) {
	e.commentAtMinute(lm, at, lm.Clock, key, params)
}

func (e *Engine) commentAtMinute(lm *worldgen.LiveMatch, at sim.GameTime, minute int, key string, params map[string]any) {
	lm.Commentary = append(lm.Commentary, worldgen.CommentaryLine{
		Minute: minute, Key: key, Params: params,
	})
	e.emit(at, key, params)
}

// pickKey selects one message-key variant deterministically from r — the
// budgeted-variety mechanism (docs/02 §4 rule 3), stateless via the moment's
// stream.
func pickKey(r *rand.Rand, keys ...string) string {
	return keys[r.IntN(len(keys))]
}

func pickKeyFrom(r *rand.Rand, keys []string) string {
	return keys[r.IntN(len(keys))]
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (e *Engine) playerName(id int64) string {
	if p := e.players[id]; p != nil {
		return p.Name
	}
	return ""
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
