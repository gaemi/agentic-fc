package engine

import (
	"math/rand/v2"
	"strconv"
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
