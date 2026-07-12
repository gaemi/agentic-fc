package engine

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/narrative"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// firstKickoff finds the earliest scheduled fixture, so match tests run a
// horizon that actually reaches the fixture list (kickoffs begin ~day 46).
func firstKickoff(e *Engine) sim.GameTime {
	var first sim.GameTime = 1 << 62
	for _, f := range e.world.Fixtures {
		if f.Kickoff < first {
			first = f.Kickoff
		}
	}
	return first
}

func TestChanceTypeCatalogLabels(t *testing.T) {
	for _, key := range []string{
		chanceCrossHeader,
		chanceCutback,
		chanceThroughBall,
		chanceLongShot,
		chanceSetPieceHeader,
		chanceScramble,
		chanceCounter,
	} {
		term := "term.chance_type." + key
		if got := narrative.Default.Render(narrative.LocaleEN, term, nil); got == term || got == "" {
			t.Fatalf("missing English chance type label for %s: %q", key, got)
		}
		if got := narrative.Default.Render(narrative.LocaleKO, term, nil); got == term || got == "" {
			t.Fatalf("missing Korean chance type label for %s: %q", key, got)
		}
	}
}

func TestExpandedCommentaryCatalogKeys(t *testing.T) {
	for _, chanceType := range []string{
		chanceCrossHeader,
		chanceCutback,
		chanceThroughBall,
		chanceLongShot,
		chanceSetPieceHeader,
		chanceScramble,
		chanceCounter,
	} {
		for _, key := range append(missCommentKeys(chanceType), goalCommentKeys(chanceType)...) {
			for _, loc := range narrative.Supported {
				if got := narrative.Default.Render(loc, key, map[string]any{
					"player": "P", "club": "C", "home_goals": 1, "away_goals": 0,
				}); got == key || got == "" {
					t.Fatalf("missing %s commentary key %s", loc, key)
				}
			}
		}
	}
	for i := 1; i <= 15; i++ {
		key := "comment.quiet." + strconv.Itoa(i)
		for _, loc := range narrative.Supported {
			if got := narrative.Default.Render(loc, key, nil); got == key || got == "" {
				t.Fatalf("missing %s quiet key %s", loc, key)
			}
		}
	}
}

func TestMatchdayNewsGroupsFixturesAndResults(t *testing.T) {
	e, _ := newEngine(t, 42)
	kickoff := firstKickoff(e)
	if _, err := e.RunUntil(kickoff + day(12)); err != nil {
		t.Fatal(err)
	}
	preview, roundUp := 0, 0
	roundUpKeys := map[string]bool{}
	for _, n := range e.world.News {
		switch n.Key {
		case FeedKickoff, FeedMatchResult, FeedCupResult:
			t.Fatalf("individual match news should be grouped, got %s: %+v", n.Key, n)
		case FeedMatchdayPreview:
			preview++
		case FeedMatchdayResults:
			roundUp++
			key := matchdayTestKey(n.Params)
			if roundUpKeys[key] {
				t.Fatalf("duplicate round-up for %s", key)
			}
			roundUpKeys[key] = true
			if len(n.ClubIDs) == 0 || len(mapsParam(t, n.Params["results"])) == 0 || n.Params["story"] == nil {
				t.Fatalf("round-up missing article params: %+v", n.Params)
			}
		}
	}
	if preview != 0 || roundUp == 0 {
		t.Fatalf("matchday news policy mismatch: preview=%d roundUp=%d", preview, roundUp)
	}
}

func matchdayTestKey(params map[string]any) string {
	return strconv.FormatInt(params["kickoff"].(int64), 10) + "/" +
		params["competition"].(string) + "/" + strconv.Itoa(params["division"].(int))
}

func mapsParam(t *testing.T, raw any) []map[string]any {
	t.Helper()
	rows, ok := raw.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", raw)
	}
	return rows
}

func TestBodyProfileFeedsAerialAndStrength(t *testing.T) {
	baseVisible := map[attr.Visible]int{
		attr.JumpingReach: 10, attr.Strength: 10,
	}
	small := &worldgen.Player{
		Group: attr.FW, HeightCm: 170, WeightKg: 66, Visible: baseVisible,
		Condition: worldgen.ConditionMax, Sharpness: worldgen.ConditionMax,
	}
	large := &worldgen.Player{
		Group: attr.FW, HeightCm: 195, WeightKg: 95, Visible: baseVisible,
		Condition: worldgen.ConditionMax, Sharpness: worldgen.ConditionMax,
	}
	if bodyReach(large) <= bodyReach(small) {
		t.Fatalf("large player reach %d <= small %d", bodyReach(large), bodyReach(small))
	}
	if bodyStrength(large) <= bodyStrength(small) {
		t.Fatalf("large player strength %d <= small %d", bodyStrength(large), bodyStrength(small))
	}
	neutral := &worldgen.Player{Group: attr.FW, Visible: baseVisible, Condition: worldgen.ConditionMax, Sharpness: worldgen.ConditionMax}
	if bodyReach(neutral) != effective(neutral, attr.JumpingReach) || bodyStrength(neutral) != effective(neutral, attr.Strength) {
		t.Fatal("zero body profile should degrade to neutral modifiers")
	}
}

// TestMatchProducesResults exercises the streamed executor end to end: real
// scores, all starters rated inside the 6.0–8.0 band, scorers matching the
// scoreline, the live table updated, and season appearances accrued.
func TestMatchProducesResults(t *testing.T) {
	e, _ := newEngine(t, 42)
	if _, err := e.RunUntil(firstKickoff(e) + day(12)); err != nil {
		t.Fatal(err)
	}
	if len(e.world.Results) == 0 {
		t.Fatal("no matches produced results")
	}
	for _, r := range e.world.Results {
		if r.HomeGoals < 0 || r.AwayGoals < 0 {
			t.Fatalf("negative score: %+v", r)
		}
		if len(r.Scorers) != r.HomeGoals+r.AwayGoals {
			t.Fatalf("scorers %d != goals %d", len(r.Scorers), r.HomeGoals+r.AwayGoals)
		}
		chanceTypes := 0
		for _, n := range r.ChanceTypes {
			chanceTypes += n
		}
		if chanceTypes != r.HomeShots+r.AwayShots {
			t.Fatalf("chance type counts %d != shots %d", chanceTypes, r.HomeShots+r.AwayShots)
		}
		chanceTypesBySide, homeChanceTypes, awayChanceTypes := 0, 0, 0
		for key, n := range r.ChanceTypesBySide {
			chanceTypesBySide += n
			switch {
			case strings.HasPrefix(key, "HOME_"):
				homeChanceTypes += n
			case strings.HasPrefix(key, "AWAY_"):
				awayChanceTypes += n
			default:
				t.Fatalf("side-aware chance type has invalid key %q", key)
			}
		}
		if chanceTypesBySide != r.HomeShots+r.AwayShots {
			t.Fatalf("side-aware chance type counts %d != shots %d", chanceTypesBySide, r.HomeShots+r.AwayShots)
		}
		if homeChanceTypes != r.HomeShots || awayChanceTypes != r.AwayShots {
			t.Fatalf("side-aware chance types HOME %d/%d AWAY %d/%d", homeChanceTypes, r.HomeShots, awayChanceTypes, r.AwayShots)
		}
		shotQuality := 0
		for _, n := range r.Diagnostics.ShotQuality {
			shotQuality += n
		}
		if shotQuality != r.HomeShots+r.AwayShots {
			t.Fatalf("shot quality counts %d != shots %d", shotQuality, r.HomeShots+r.AwayShots)
		}
		if sumCounts(r.Diagnostics.AerialWins) > sumCounts(r.Diagnostics.AerialDuels) {
			t.Fatalf("aerial wins exceed attempts: %+v", r.Diagnostics)
		}
		var halftime, fulltime *worldgen.CommentaryLine
		for i := range r.Commentary {
			line := &r.Commentary[i]
			switch {
			case strings.HasPrefix(line.Key, "comment.halftime"):
				halftime = line
			case strings.HasPrefix(line.Key, "comment.fulltime"):
				fulltime = line
			}
		}
		halfHome, halfHomeOK := "", false
		halfAway, halfAwayOK := "", false
		if halftime != nil {
			halfHome, halfHomeOK = halftime.Params["home"].(string)
			halfAway, halfAwayOK = halftime.Params["away"].(string)
		}
		if halftime == nil || halftime.Key == "comment.halftime" ||
			!halfHomeOK || halfHome == "" || !halfAwayOK || halfAway == "" {
			t.Fatalf("fixture %d missing contextual half-time commentary: %+v", r.FixtureID, halftime)
		}
		if halftime.Minute != matchFullTimeMinutes/2 {
			t.Fatalf("fixture %d half-time minute = %d, want 45", r.FixtureID, halftime.Minute)
		}
		if fulltime == nil || fulltime.Key != fulltimeCommentaryKey(r.HomeGoals, r.AwayGoals) {
			t.Fatalf("fixture %d full-time commentary = %+v, want %q", r.FixtureID, fulltime, fulltimeCommentaryKey(r.HomeGoals, r.AwayGoals))
		}
		participants := len(r.HomeXI) + len(r.AwayXI)
		for _, s := range r.Subs {
			if s.On != 0 {
				participants++ // a sub-on takes the pitch and earns a rating too
			}
		}
		if len(r.RatingsX10) != participants {
			t.Fatalf("rated %d players, participants total %d", len(r.RatingsX10), participants)
		}
		for id, rt := range r.RatingsX10 {
			if rt < ratingMinX10 || rt > ratingMaxX10 {
				t.Fatalf("player %d rating %d outside band [%d,%d]", id, rt, ratingMinX10, ratingMaxX10)
			}
		}
	}
	played := 0
	for _, row := range e.world.Table[0] {
		played += row.Played
	}
	if played == 0 {
		t.Fatal("live league table not updated")
	}
	apps := 0
	for i := range e.world.Players {
		apps += e.world.Players[i].SeasonApps
	}
	if apps == 0 {
		t.Fatal("no season appearances recorded")
	}
}

// TestCupResultsDoNotTouchTable locks the match-scope edge: cup fixtures run
// through the same executor and land in Results, but drive no league standings
// (cup round 1 kicks off ~day 69; league from ~day 46).
func TestCupResultsDoNotTouchTable(t *testing.T) {
	e, _ := newEngine(t, 3)
	if _, err := e.RunUntil(firstKickoff(e) + day(30)); err != nil {
		t.Fatal(err)
	}
	league, cup := 0, 0
	for _, r := range e.world.Results {
		switch r.Competition {
		case worldgen.CompetitionLeague:
			league++
		case worldgen.CompetitionCup:
			cup++
		}
	}
	if cup == 0 {
		t.Fatal("test vacuous: no cup matches played in the horizon")
	}
	played := 0
	for _, table := range e.world.Table {
		for _, row := range table {
			played += row.Played
		}
	}
	if played != 2*league {
		t.Fatalf("aggregate table played=%d, want %d (2×league) — cup results leaked into the table", played, 2*league)
	}
}

// TestCommentaryParamsHaveNoFloats guards the one hash landmine the tempo /
// resume invariants can't catch: commentary params are persisted on LiveMatch,
// so a float among them would enter the world hash (NFR-2). Every param must be
// an int or string.
func TestCommentaryParamsHaveNoFloats(t *testing.T) {
	e, _ := newEngine(t, 5)
	if _, err := e.RunUntil(firstKickoff(e) + day(20)); err != nil {
		t.Fatal(err)
	}
	lines := 0
	for _, r := range e.world.Results {
		for _, line := range r.Commentary {
			lines++
			for k, v := range line.Params {
				switch v.(type) {
				case float32, float64:
					t.Fatalf("commentary %q param %q is a float (%v) — floats reach the world hash (NFR-2)", line.Key, k, v)
				}
			}
		}
	}
	if lines == 0 {
		t.Fatal("no commentary produced — test vacuous")
	}
}

// TestMatchDeterminismAcrossTempo locks the match invariant: the same seed
// run one drain vs many day-chunks reaches an identical world hash, even
// across played match days. Tempo/chunking never changes what happens (NFR-2).
func TestMatchDeterminismAcrossTempo(t *testing.T) {
	const seed = 7
	ea, _ := newEngine(t, seed)
	horizon := firstKickoff(ea) + day(20)
	if _, err := ea.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}
	if len(ea.world.Results) == 0 {
		t.Fatal("test vacuous: no matches in horizon")
	}

	eb, _ := newEngine(t, seed)
	for eb.Now() < horizon {
		to := eb.Now() + day(1)
		if to > horizon {
			to = horizon
		}
		if _, err := eb.RunUntil(to); err != nil {
			t.Fatal(err)
		}
	}
	ha, _ := ea.World().Hash()
	hb, _ := eb.World().Hash()
	if ha != hb {
		t.Fatalf("match sim not tempo-independent:\nA %s\nB %s", ha, hb)
	}
}

// TestMatchResumeMidWindow is the decisive persistence test: it snapshots to
// disk WHILE a match is in progress (LiveMatches non-empty) and proves the
// resumed run reaches the exact same hash as the uninterrupted one — so the
// running tally in World and the pending moment events in the queue survive a
// mid-match restart (FR-28).
func TestMatchResumeMidWindow(t *testing.T) {
	const seed = 99
	ea, _ := newEngine(t, seed)
	kickoff := firstKickoff(ea)
	horizon := kickoff + day(10)
	if _, err := ea.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}

	eb, _ := newEngine(t, seed)
	if _, err := eb.RunUntil(kickoff + sim.GameTime(45)); err != nil { // 45' into the match
		t.Fatal(err)
	}
	if len(eb.world.LiveMatches) == 0 {
		t.Fatal("test vacuous: no live match at snapshot time")
	}

	fstore := &store.FileStore{Dir: t.TempDir()}
	events, nextSeq := eb.Queue().Snapshot()
	if err := fstore.SaveSnapshot(&store.Snapshot{
		Now: eb.Now(), World: eb.World(), Queue: events, QueueNextSeq: nextSeq,
	}); err != nil {
		t.Fatal(err)
	}
	snap, err := fstore.LoadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	resumed := New(snap.World, sim.RestoreQueue(snap.Queue, snap.QueueNextSeq), &store.MemAuditLog{})
	resumed.ResumeAt(snap.Now)
	if _, err := resumed.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}

	ha, _ := ea.World().Hash()
	hb, _ := resumed.World().Hash()
	if ha != hb {
		t.Fatalf("mid-match resume diverged from the uninterrupted run:\nA %s\nB %s", ha, hb)
	}
}

func TestAdjustmentCommentaryCyclesPerClubWithoutRNG(t *testing.T) {
	lm := &worldgen.LiveMatch{Adjustments: []worldgen.Adjustment{
		{ClubID: 1, Key: "adj.push"},
		{ClubID: 2, Key: "adj.push"},
		{ClubID: 1, Key: "adj.push"},
		{ClubID: 1, Key: "other"},
	}}
	if got := adjustmentCommentaryKey(lm, 1); got != "comment.adj.push.3" {
		t.Fatalf("club 1 adjustment key = %q, want third variant", got)
	}
	if got := adjustmentCommentaryKey(lm, 2); got != "comment.adj.push.2" {
		t.Fatalf("club 2 adjustment key = %q, want second variant", got)
	}
	if got := adjustmentCommentaryKey(lm, 3); got != "comment.adj.push.1" {
		t.Fatalf("new club adjustment key = %q, want first variant", got)
	}
	lm.Adjustments = append(lm.Adjustments,
		worldgen.Adjustment{ClubID: 1, Key: "adj.push"},
		worldgen.Adjustment{ClubID: 1, Key: "adj.push"},
	)
	if got := adjustmentCommentaryKey(lm, 1); got != "comment.adj.push.1" {
		t.Fatalf("club 1 wrapped adjustment key = %q, want first variant", got)
	}
}

func TestWhistleCommentaryKeysReflectScore(t *testing.T) {
	halftime := []struct {
		home, away int
		want       string
	}{
		{0, 0, "comment.halftime.goalless"},
		{1, 1, "comment.halftime.level"},
		{2, 1, "comment.halftime.home_lead"},
		{0, 2, "comment.halftime.away_lead"},
		{4, 1, "comment.halftime.home_big_lead"},
		{0, 3, "comment.halftime.away_big_lead"},
	}
	for _, tc := range halftime {
		if got := halftimeCommentaryKey(tc.home, tc.away); got != tc.want {
			t.Fatalf("halftime %d-%d key = %q, want %q", tc.home, tc.away, got, tc.want)
		}
	}
	fulltime := []struct {
		home, away int
		want       string
	}{
		{0, 0, "comment.fulltime.goalless"},
		{2, 2, "comment.fulltime.level"},
		{2, 1, "comment.fulltime.home_edge"},
		{0, 1, "comment.fulltime.away_edge"},
		{3, 1, "comment.fulltime.home_win"},
		{1, 3, "comment.fulltime.away_win"},
		{4, 1, "comment.fulltime.home_big"},
		{0, 3, "comment.fulltime.away_big"},
	}
	for _, tc := range fulltime {
		if got := fulltimeCommentaryKey(tc.home, tc.away); got != tc.want {
			t.Fatalf("fulltime %d-%d key = %q, want %q", tc.home, tc.away, got, tc.want)
		}
	}
}

func TestLegacyInProgressMatchBackfillsExactHalftimeOnce(t *testing.T) {
	e, _ := newEngine(t, 42)
	kickoff := firstKickoff(e)
	if _, err := e.RunUntil(kickoff + 42); err != nil {
		t.Fatal(err)
	}
	events, nextSeq := e.queue.Snapshot()
	filtered := events[:0]
	for _, ev := range events {
		if ev.Payload != worldgen.PayloadMatchHalftime {
			filtered = append(filtered, ev)
		}
	}
	e.queue = sim.RestoreQueue(filtered, nextSeq)
	if _, err := e.RunUntil(kickoff + 47); err != nil {
		t.Fatal(err)
	}
	if len(e.world.LiveMatches) == 0 {
		t.Fatal("test horizon has no live matches")
	}
	for fixture, lm := range e.world.LiveMatches {
		count := 0
		for _, line := range lm.Commentary {
			if strings.HasPrefix(line.Key, "comment.halftime") {
				count++
				if line.Minute != matchFullTimeMinutes/2 {
					t.Fatalf("fixture %d fallback half-time minute = %d, want 45", fixture, line.Minute)
				}
			}
		}
		if count != 1 {
			t.Fatalf("fixture %d fallback half-time lines = %d, want 1", fixture, count)
		}
	}
}

func TestQuietCommentaryDoesNotRepeatBeforePoolExhaustion(t *testing.T) {
	lm := &worldgen.LiveMatch{Commentary: []worldgen.CommentaryLine{{Key: "comment.kickoff"}}}
	r := rand.New(rand.NewPCG(11, 29))
	seen := map[string]bool{}
	for range quietCommentaryKeys {
		key := pickUnusedCommentaryKey(r, lm, quietCommentaryKeys)
		if seen[key] {
			t.Fatalf("quiet key repeated before pool exhaustion: %q", key)
		}
		seen[key] = true
		lm.Commentary = append(lm.Commentary, worldgen.CommentaryLine{Key: key})
	}
	if len(seen) != len(quietCommentaryKeys) {
		t.Fatalf("quiet pool used %d keys, want %d", len(seen), len(quietCommentaryKeys))
	}
	if key := pickUnusedCommentaryKey(r, lm, quietCommentaryKeys); !seen[key] {
		t.Fatalf("exhausted pool returned unknown key %q", key)
	}
}

func TestQuietCommentarySelectionPreservesRNGPosition(t *testing.T) {
	selected := rand.New(rand.NewPCG(7, 31))
	baseline := rand.New(rand.NewPCG(7, 31))
	lm := &worldgen.LiveMatch{}
	for range 20 {
		key := pickUnusedCommentaryKey(selected, lm, quietCommentaryKeys)
		lm.Commentary = append(lm.Commentary, worldgen.CommentaryLine{Key: key})
		baseline.IntN(len(quietCommentaryKeys))
	}
	if got, want := selected.Uint64(), baseline.Uint64(); got != want {
		t.Fatalf("commentary selection moved RNG position: got %d want %d", got, want)
	}
}

// State-aware quiet beats: nervy lines for close late games, cruise lines
// for one-sided ones — nil means the broad pool stands.
func TestStateQuietKeysSelectsThemedPools(t *testing.T) {
	cases := []struct {
		name   string
		clock  int
		hg, ag int
		want   string // "", "tension", "cruise"
	}{
		{"early goalless", 30, 0, 0, ""},
		{"late close game", 76, 1, 1, "tension"},
		{"late one-goal game", 88, 1, 2, "tension"},
		{"cruising margin", 65, 4, 0, "cruise"},
		{"late blowout stays cruise", 80, 0, 4, "cruise"},
		{"big margin before the hour", 55, 3, 0, ""},
		{"two-goal game is neither", 80, 2, 0, ""},
	}
	for _, tc := range cases {
		lm := &worldgen.LiveMatch{Clock: tc.clock, HomeGoals: tc.hg, AwayGoals: tc.ag}
		got := stateQuietKeys(lm)
		prefix := map[string]string{"tension": "comment.quiet.tension.", "cruise": "comment.quiet.cruise."}[tc.want]
		if prefix == "" {
			if got != nil {
				t.Fatalf("%s: themed pool %v, want broad quiet pool", tc.name, got)
			}
			continue
		}
		if len(got) == 0 || !strings.HasPrefix(got[0], prefix) {
			t.Fatalf("%s: pool %v, want prefix %q", tc.name, got, prefix)
		}
	}
	for _, key := range append(append([]string{}, tensionQuietKeys...), cruiseQuietKeys...) {
		for _, loc := range narrative.Supported {
			if _, ok := narrative.Default[loc][key]; !ok {
				t.Fatalf("themed quiet key %q missing from %s catalog", key, loc)
			}
		}
	}
}

// The themed quiet draw must consume exactly the RNG the broad draw consumes
// (docs/12: presentation must not perturb play), prefer unused themed lines
// while they last, and fall back to the broad pool afterwards.
func TestQuietBeatKeyNarratesStateWithoutMovingRNG(t *testing.T) {
	themed := rand.New(rand.NewPCG(3, 17))
	baseline := rand.New(rand.NewPCG(3, 17))
	tense := &worldgen.LiveMatch{Clock: 80, HomeGoals: 1, AwayGoals: 1}
	neutral := &worldgen.LiveMatch{Clock: 30}
	seen := map[string]bool{}
	for range tensionQuietKeys {
		key := quietBeatKey(themed, tense)
		if !strings.HasPrefix(key, "comment.quiet.tension.") {
			t.Fatalf("tense beat spoke %q, want a tension line", key)
		}
		if seen[key] {
			t.Fatalf("tension line %q repeated before the pool was exhausted", key)
		}
		seen[key] = true
		tense.Commentary = append(tense.Commentary, worldgen.CommentaryLine{Key: key})
		quietBeatKey(baseline, neutral)
		neutral.Commentary = append(neutral.Commentary, worldgen.CommentaryLine{Key: "comment.kickoff"})
	}
	if key := quietBeatKey(themed, tense); !strings.HasPrefix(key, "comment.quiet.") ||
		strings.HasPrefix(key, "comment.quiet.tension.") {
		t.Fatalf("exhausted tension pool should fall back to the broad quiet pool, got %q", key)
	}
	quietBeatKey(baseline, neutral)
	if got, want := themed.Uint64(), baseline.Uint64(); got != want {
		t.Fatalf("themed quiet selection moved the RNG position: got %d want %d", got, want)
	}
}

// Score-context calls replace the pattern line only for the moments that
// matter (opener, equalizer, late drama) and never consume match RNG.
func TestGoalContextCommentaryKey(t *testing.T) {
	// Scorer ledgers for the story-aware cases; club 1 is home, club 2 away.
	goals := func(events ...[3]int64) []worldgen.MatchEvent {
		out := make([]worldgen.MatchEvent, 0, len(events))
		for _, e := range events {
			out = append(out, worldgen.MatchEvent{Minute: int(e[0]), PlayerID: e[1], ClubID: e[2]})
		}
		return out
	}
	cases := []struct {
		name       string
		clock      int
		homeGoals  int
		awayGoals  int
		home       bool
		scorer     int64
		scorers    []worldgen.MatchEvent
		wantPrefix string
	}{
		{"opener", 10, 1, 0, true, 0, nil, "comment.goal.opener."},
		{"away equalizer", 30, 1, 1, false, 0, nil, "comment.goal.equalizer."},
		{"late equalizer", 88, 2, 2, true, 0, nil, "comment.goal.late_level."},
		{"late winner", 87, 1, 0, true, 0, nil, "comment.goal.late."},
		{"late go-ahead", 86, 2, 1, true, 0, nil, "comment.goal.late."},
		{"padding the lead", 50, 2, 0, true, 0, nil, ""},
		{"late but comfortable", 88, 3, 1, true, 0, nil, ""},
		{"go-ahead keeps the pattern call", 40, 2, 1, true, 0, nil, ""},
		{"hat-trick outranks the scoreline", 50, 3, 0, true, 9,
			goals([3]int64{20, 9, 1}, [3]int64{35, 9, 1}, [3]int64{50, 9, 1}),
			"comment.goal.hattrick."},
		{"late hat-trick gets its own call", 88, 3, 3, true, 9,
			goals([3]int64{20, 9, 1}, [3]int64{40, 21, 2}, [3]int64{55, 21, 2}, [3]int64{60, 9, 1}, [3]int64{70, 22, 2}, [3]int64{88, 9, 1}),
			"comment.goal.hattrick_late."},
		{"comeback completed from two down", 70, 2, 2, true, 9,
			goals([3]int64{10, 21, 2}, [3]int64{25, 22, 2}, [3]int64{50, 8, 1}, [3]int64{70, 9, 1}),
			"comment.goal.comeback_level."},
		{"comeback turned into the lead", 75, 3, 2, true, 9,
			goals([3]int64{10, 21, 2}, [3]int64{25, 22, 2}, [3]int64{50, 8, 1}, [3]int64{60, 9, 1}, [3]int64{75, 10, 1}),
			"comment.goal.comeback_ahead."},
		{"instant response restores the lead", 63, 2, 1, true, 9,
			goals([3]int64{30, 8, 1}, [3]int64{60, 21, 2}, [3]int64{63, 9, 1}),
			"comment.goal.response."},
		{"slow reply keeps the pattern call", 63, 2, 1, true, 9,
			goals([3]int64{30, 8, 1}, [3]int64{50, 21, 2}, [3]int64{63, 9, 1}),
			""},
		{"late comeback still narrates the fightback", 88, 2, 2, true, 9,
			goals([3]int64{10, 21, 2}, [3]int64{25, 22, 2}, [3]int64{70, 8, 1}, [3]int64{88, 9, 1}),
			"comment.goal.comeback_level."},
		{"late instant response outranks the clock", 88, 3, 2, true, 9,
			goals([3]int64{30, 8, 1}, [3]int64{50, 21, 2}, [3]int64{60, 8, 1}, [3]int64{84, 22, 2}, [3]int64{88, 9, 1}),
			"comment.goal.response."},
		{"go-ahead after a completed comeback is ordinary again", 70, 4, 3, true, 9,
			goals([3]int64{5, 21, 2}, [3]int64{10, 22, 2}, [3]int64{25, 6, 1}, [3]int64{35, 7, 1},
				[3]int64{45, 8, 1}, [3]int64{55, 21, 2}, [3]int64{70, 9, 1}),
			""},
		{"a second deep deficit re-arms the comeback", 80, 4, 4, true, 9,
			goals([3]int64{5, 21, 2}, [3]int64{10, 22, 2}, [3]int64{20, 6, 1}, [3]int64{30, 7, 1},
				[3]int64{40, 21, 2}, [3]int64{50, 22, 2}, [3]int64{65, 8, 1}, [3]int64{80, 9, 1}),
			"comment.goal.comeback_level."},
		{"fourth goal at a rout margin", 60, 4, 0, true, 9,
			goals([3]int64{10, 6, 1}, [3]int64{20, 7, 1}, [3]int64{40, 8, 1}, [3]int64{60, 9, 1}),
			"comment.goal.rout."},
		{"four goals in a contest stay pattern", 60, 4, 2, true, 9,
			goals([3]int64{10, 6, 1}, [3]int64{20, 7, 1}, [3]int64{30, 21, 2}, [3]int64{40, 8, 1}, [3]int64{50, 22, 2}, [3]int64{60, 9, 1}),
			""},
	}
	for _, tc := range cases {
		lm := &worldgen.LiveMatch{
			HomeID: 1, AwayID: 2,
			Clock: tc.clock, HomeGoals: tc.homeGoals, AwayGoals: tc.awayGoals,
			Scorers: tc.scorers,
		}
		got := goalContextCommentaryKey(lm, tc.home, tc.scorer)
		if tc.wantPrefix == "" {
			if got != "" {
				t.Fatalf("%s: key %q, want pattern call", tc.name, got)
			}
			continue
		}
		if !strings.HasPrefix(got, tc.wantPrefix) {
			t.Fatalf("%s: key %q, want prefix %q", tc.name, got, tc.wantPrefix)
		}
		for _, loc := range narrative.Supported {
			if _, ok := narrative.Default[loc][got]; !ok {
				t.Fatalf("%s: key %q missing from %s catalog", tc.name, got, loc)
			}
		}
	}
}

func TestKickoffCommentaryKeyRotatesAndExists(t *testing.T) {
	seen := map[string]bool{}
	for id := int64(100001); id < 100007; id++ {
		key := kickoffCommentaryKey(id)
		seen[key] = true
		for _, loc := range narrative.Supported {
			if _, ok := narrative.Default[loc][key]; !ok {
				t.Fatalf("kickoff key %q missing from %s catalog", key, loc)
			}
		}
	}
	if len(seen) != 3 {
		t.Fatalf("kickoff keys used = %v, want all 3 variants across fixtures", seen)
	}
}

// Every commentary key the engine can emit for chances and goals must exist
// in both catalogs, or the Console would render raw keys mid-match.
func TestMatchCommentaryKeyListsExistInCatalogs(t *testing.T) {
	types := []string{
		chanceCrossHeader, chanceCutback, chanceThroughBall, chanceLongShot,
		chanceSetPieceHeader, chanceCounter, chanceScramble,
	}
	keys := []string{}
	for _, ct := range types {
		keys = append(keys, missCommentKeys(ct)...)
		keys = append(keys, goalCommentKeys(ct)...)
	}
	for _, key := range keys {
		for _, loc := range narrative.Supported {
			if _, ok := narrative.Default[loc][key]; !ok {
				t.Fatalf("engine commentary key %q missing from %s catalog", key, loc)
			}
		}
	}
}

// The widened commentary pools must not change the RNG argument sequence:
// draws stay on the legacy bound while state rotation exposes every variant.
func TestPickWidenedKeyKeepsLegacyDrawAndCoversPool(t *testing.T) {
	keys := goalCommentKeys(chanceCrossHeader)
	if len(keys) != 4 {
		t.Fatalf("cross goal pool = %d keys, want 4", len(keys))
	}
	seen := map[string]bool{}
	for clock := 0; clock < 8; clock++ {
		lm := &worldgen.LiveMatch{Clock: clock, HomeGoals: 1}
		r1 := rand.New(rand.NewPCG(7, 9))
		r2 := rand.New(rand.NewPCG(7, 9))
		got := pickWidenedKey(r1, lm, legacyGoalPoolSize, keys)
		seen[got] = true
		// The reference draw with the legacy bound must leave both
		// generators in the same state.
		_ = r2.IntN(legacyGoalPoolSize)
		if a, b := r1.Uint64(), r2.Uint64(); a != b {
			t.Fatalf("widened draw perturbed the stream: %d vs %d", a, b)
		}
	}
	if len(seen) != len(keys) {
		t.Fatalf("state rotation exposed %d of %d variants: %v", len(seen), len(keys), seen)
	}
}

// Quiet commentary also keeps the legacy RNG bound while covering the wider
// pool through state rotation and the no-repeat probe.
func TestQuietDrawKeepsLegacyBoundAndCoversPool(t *testing.T) {
	for _, key := range quietCommentaryKeys {
		for _, loc := range narrative.Supported {
			if _, ok := narrative.Default[loc][key]; !ok {
				t.Fatalf("quiet key %q missing from %s catalog", key, loc)
			}
		}
	}
	seen := map[string]bool{}
	for clock := 0; clock < len(quietCommentaryKeys); clock++ {
		lm := &worldgen.LiveMatch{Clock: clock}
		r1 := rand.New(rand.NewPCG(3, 5))
		r2 := rand.New(rand.NewPCG(3, 5))
		seen[pickUnusedCommentaryKey(r1, lm, quietCommentaryKeys)] = true
		_ = r2.IntN(legacyQuietPoolSize)
		if a, b := r1.Uint64(), r2.Uint64(); a != b {
			t.Fatalf("quiet draw perturbed the stream at clock %d", clock)
		}
	}
	if len(seen) < len(quietCommentaryKeys)-2 {
		t.Fatalf("rotation covered only %d of %d quiet lines", len(seen), len(quietCommentaryKeys))
	}
}
