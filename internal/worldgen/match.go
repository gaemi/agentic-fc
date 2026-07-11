package worldgen

import (
	"sort"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/sim"
)

// MatchEvent is one timestamped in-match occurrence — a goal or a card. It is
// the minimal record get_match's finished view needs. Per-moment commentary
// prose is separate; these bare events carry the facts.
type MatchEvent struct {
	Minute   int    `json:"minute"`
	PlayerID int64  `json:"player_id"`
	ClubID   int64  `json:"club_id"`
	Detail   string `json:"detail,omitempty"` // cards: YELLOW | RED
}

// SubEvent records one substitution. On == 0 means the side had no fit
// replacement (or was out of subs) and played short from that minute — only
// the injury path may record that; a discretionary change always brings a
// replacement on. Reason is the public, TV-visible why: INJURY | FATIGUE |
// TACTICAL.
type SubEvent struct {
	Minute int    `json:"minute"`
	ClubID int64  `json:"club_id"`
	Off    int64  `json:"off"`
	On     int64  `json:"on,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// CommentaryLine is one rendered-later commentary beat: a message key plus
// params (docs/02 §4). Params carry ONLY ints and strings — never a float, the
// one hash landmine the match invariant tests don't catch (carry ×10 ints and
// divide at display if a number must render).
type CommentaryLine struct {
	Minute int            `json:"minute"`
	Key    string         `json:"key"`
	Params map[string]any `json:"params,omitempty"`
}

// Adjustment records a Manager's in-match decision. The *effect*
// lives on LiveMatch (e.g. MentalityShift), never on the Manager's Mindset —
// intent state belongs to the agent (single-writer rule); a sim-driven shift
// must reset at full time and persist on resume, which LiveMatch gives for free.
type Adjustment struct {
	Minute int    `json:"minute"`
	ClubID int64  `json:"club_id"`
	Key    string `json:"key"`
}

// MatchDiagnostics is the public tactical evidence accumulated from sampled
// key moments. It stores aggregate facts only — no private player traits or
// formula weights — and is safe for MCP/Console/TUI surfaces.
type MatchDiagnostics struct {
	ShotQuality       map[string]int `json:"shot_quality,omitempty"`         // LOW | MEDIUM | HIGH aggregate
	ShotQualityBySide map[string]int `json:"shot_quality_by_side,omitempty"` // HOME_HIGH, AWAY_MEDIUM, ...
	AerialDuels       map[string]int `json:"aerial_duels,omitempty"`         // HOME | AWAY attempts
	AerialWins        map[string]int `json:"aerial_wins,omitempty"`          // HOME | AWAY wins
	PressTurnovers    map[string]int `json:"press_turnovers,omitempty"`      // HOME | AWAY
	SetPieceThreat    map[string]int `json:"set_piece_threat,omitempty"`     // HOME | AWAY
	TacticalTilt      map[string]int `json:"tactical_tilt,omitempty"`        // HOME_WIDE, AWAY_CENTRAL, ...
}

func (d *MatchDiagnostics) AddShotQuality(side, band string) {
	if d.ShotQuality == nil {
		d.ShotQuality = map[string]int{}
	}
	d.ShotQuality[band]++
	if d.ShotQualityBySide == nil {
		d.ShotQualityBySide = map[string]int{}
	}
	d.ShotQualityBySide[side+"_"+band]++
}

func (d *MatchDiagnostics) AddSide(m *map[string]int, side string) {
	if *m == nil {
		*m = map[string]int{}
	}
	(*m)[side]++
}

func (d *MatchDiagnostics) AddTilt(side, family string) {
	if d.TacticalTilt == nil {
		d.TacticalTilt = map[string]int{}
	}
	d.TacticalTilt[side+"_"+family]++
}

// MatchResult is a finished fixture: score, scorers, cards, the two XIs, and
// per-player ratings. Ratings are ×10 integers (RatingsX10[id] = 72 ⇒ 7.2) so
// nothing reaching the world hash is a float (NFR-2).
type MatchResult struct {
	FixtureID    int64  `json:"fixture_id"`
	Competition  string `json:"competition"`
	DivisionTier int    `json:"division_tier,omitempty"`
	HomeID       int64  `json:"home_id"`
	AwayID       int64  `json:"away_id"`
	HomeGoals    int    `json:"home_goals"`
	AwayGoals    int    `json:"away_goals"`
	// Winner is the advancing club in a cup tie: the higher scorer, or
	// the shootout victor when the 90 minutes finished level. Zero for a league
	// result, where the score alone settles the points.
	Winner      int64            `json:"winner,omitempty"`
	Kickoff     sim.GameTime     `json:"kickoff"`
	HomeXI      []int64          `json:"home_xi"`
	AwayXI      []int64          `json:"away_xi"`
	Subs        []SubEvent       `json:"subs,omitempty"`
	Scorers     []MatchEvent     `json:"scorers,omitempty"`
	Cards       []MatchEvent     `json:"cards,omitempty"`
	RatingsX10  map[int64]int    `json:"ratings_x10,omitempty"`
	Commentary  []CommentaryLine `json:"commentary,omitempty"`
	Adjustments []Adjustment     `json:"adjustments,omitempty"`
	HomeShots   int              `json:"home_shots,omitempty"`
	AwayShots   int              `json:"away_shots,omitempty"`
	ChanceTypes map[string]int   `json:"chance_types,omitempty"`
	Diagnostics MatchDiagnostics `json:"diagnostics,omitempty"`
}

// LiveMatch is the running tally of an in-progress fixture, persisted so a
// mid-match snapshot→reload resumes identically. Rolls are stateless-by-label
// (match/<fixtureID>/moment/<MomentIndex>), so only the accumulated state
// needs a home here — the pending moment event lives in the queue.
type LiveMatch struct {
	FixtureID    int64        `json:"fixture_id"`
	Competition  string       `json:"competition"`
	DivisionTier int          `json:"division_tier,omitempty"`
	HomeID       int64        `json:"home_id"`
	AwayID       int64        `json:"away_id"`
	Kickoff      sim.GameTime `json:"kickoff"`
	HomeXI       []int64      `json:"home_xi"`
	AwayXI       []int64      `json:"away_xi"`
	// Benches and substitutions: the bench is picked at kickoff
	// beside the XI; Subs records every change in order. The current on-pitch
	// set and the participants are DERIVED (OnPitch/Participants) rather than
	// stored, so sparse mid-match state degrades to "XI unchanged" and resumes
	// exactly as it ran before.
	HomeBench   []int64      `json:"home_bench,omitempty"`
	AwayBench   []int64      `json:"away_bench,omitempty"`
	Subs        []SubEvent   `json:"subs,omitempty"`
	HomeGoals   int          `json:"home_goals"`
	AwayGoals   int          `json:"away_goals"`
	Clock       int          `json:"clock"`        // game-minutes elapsed
	MomentIndex int          `json:"moment_index"` // stateless-stream label
	Scorers     []MatchEvent `json:"scorers,omitempty"`
	Cards       []MatchEvent `json:"cards,omitempty"`

	// Narrative + in-match state: all persisted, so live get_match and a
	// mid-match resume both read the same running story.
	Commentary  []CommentaryLine `json:"commentary,omitempty"`
	Adjustments []Adjustment     `json:"adjustments,omitempty"`
	// MentalityShift is the manager's in-match attacking bias (−2…+2), added to
	// the base tactical mentality; it lives here so it resets at full time and
	// resumes with the match, never touching the agent's Mindset.
	HomeMentalityShift int              `json:"home_mentality_shift,omitempty"`
	AwayMentalityShift int              `json:"away_mentality_shift,omitempty"`
	HomeShots          int              `json:"home_shots,omitempty"`
	AwayShots          int              `json:"away_shots,omitempty"`
	ChanceTypes        map[string]int   `json:"chance_types,omitempty"`
	Diagnostics        MatchDiagnostics `json:"diagnostics,omitempty"`
}

// xiOf returns the side's starters for clubID (nil if the club isn't playing).
func (lm *LiveMatch) xiOf(clubID int64) []int64 {
	switch clubID {
	case lm.HomeID:
		return lm.HomeXI
	case lm.AwayID:
		return lm.AwayXI
	}
	return nil
}

// OnPitch derives a side's current on-pitch ids: starters minus everyone
// subbed off or SENT OFF, plus everyone subbed on, in stable order (XI order,
// then Subs order). A red card in the Cards ledger ejects its player with no
// replacement — the side plays short, which is the whole 10-man effect: team
// strengths sum over this set. Derived — never stored — so an old mid-match
// snapshot with no Subs reads as "starters unchanged" and resumes exactly as
// it ran (NFR-2).
func (lm *LiveMatch) OnPitch(clubID int64) []int64 {
	off := map[int64]bool{}
	for _, c := range lm.Cards {
		if c.ClubID == clubID && c.Detail == "RED" {
			off[c.PlayerID] = true
		}
	}
	var ons []int64
	for _, s := range lm.Subs {
		if s.ClubID != clubID {
			continue
		}
		off[s.Off] = true
		if s.On != 0 {
			ons = append(ons, s.On)
		}
	}
	var out []int64
	for _, id := range lm.xiOf(clubID) {
		if !off[id] {
			out = append(out, id)
		}
	}
	for _, id := range ons {
		if !off[id] { // a sub-on later withdrawn (injured again) or sent off stays off
			out = append(out, id)
		}
	}
	return out
}

// Participants derives everyone who took the pitch for a side — starters plus
// sub-ons — the set that earns appearances and ratings at full time.
func (lm *LiveMatch) Participants(clubID int64) []int64 {
	out := append([]int64{}, lm.xiOf(clubID)...)
	for _, s := range lm.Subs {
		if s.ClubID == clubID && s.On != 0 {
			out = append(out, s.On)
		}
	}
	return out
}

// SubsUsed counts a side's spent substitutions. A short-side withdrawal
// (On == 0) doesn't consume one — no replacement came on.
func (lm *LiveMatch) SubsUsed(clubID int64) int {
	n := 0
	for _, s := range lm.Subs {
		if s.ClubID == clubID && s.On != 0 {
			n++
		}
	}
	return n
}

// ConditionMax is the ceiling for Condition and Sharpness (0–100). Players
// start a fresh world fully rested and match-sharp.
const ConditionMax = 100

// Player rating band (×10 integers — nothing reaching the world hash is a
// float; docs/02 §2.3, tunable docs/98). The values live HERE, beside the
// LiveMatch they score, because two consumers share the one formula: the
// engine's full-time ratings and the Console's live "as it stands" pane.
const (
	RatingBaseX10   = 65 // 6.5
	RatingMinX10    = 60 // narrow practical band 6.0–8.0
	RatingMaxX10    = 80
	RatingGoalX10   = 8
	RatingWinX10    = 3
	RatingLossX10   = -3
	RatingCleanX10  = 5 // clean sheet, GK/DF only
	RatingYellowX10 = -3
	RatingRedX10    = -13
)

// LiveRatingsX10 scores every participant (starters + sub-ons) on the ×10
// band: base plus goals, result, clean sheet (GK/DF), and card penalties. A
// red — straight or a second yellow's upgrade — costs RatingRedX10 once and
// swallows any yellows (summing would double-penalize a second yellow, whose
// ledger keeps the earlier YELLOW entry). A pure function of the running
// tally: at full time it is THE rating; mid-match it reads "as if it ended
// now" (the Console's live pane). playerOf resolves ids — nil players score
// but never earn the positional clean-sheet bonus.
func LiveRatingsX10(lm *LiveMatch, playerOf func(int64) *Player) map[int64]int {
	goals := map[int64]int{}
	for _, s := range lm.Scorers {
		goals[s.PlayerID]++
	}
	cardAdj := map[int64]int{}
	sentOff := map[int64]bool{}
	for _, c := range lm.Cards {
		if c.Detail == "RED" {
			sentOff[c.PlayerID] = true
		}
	}
	for _, c := range lm.Cards {
		switch {
		case c.Detail == "RED":
			cardAdj[c.PlayerID] = RatingRedX10
		case !sentOff[c.PlayerID]:
			cardAdj[c.PlayerID] += RatingYellowX10
		}
	}
	out := make(map[int64]int, len(lm.HomeXI)+len(lm.AwayXI))
	rate := func(xi []int64, scored, conceded int) {
		result := RatingWinX10
		switch {
		case scored < conceded:
			result = RatingLossX10
		case scored == conceded:
			result = 0
		}
		for _, pid := range xi {
			v := RatingBaseX10 + result + goals[pid]*RatingGoalX10 + cardAdj[pid]
			if conceded == 0 {
				if p := playerOf(pid); p != nil && (p.Group == attr.GK || p.Group == attr.DF) {
					v += RatingCleanX10
				}
			}
			if v < RatingMinX10 {
				v = RatingMinX10
			}
			if v > RatingMaxX10 {
				v = RatingMaxX10
			}
			out[pid] = v
		}
	}
	rate(lm.Participants(lm.HomeID), lm.HomeGoals, lm.AwayGoals)
	rate(lm.Participants(lm.AwayID), lm.AwayGoals, lm.HomeGoals)
	return out
}

// EnsureMatchState seeds match state. At generation every player is fresh
// (zero condition), so they become fully rested and match-sharp and the live
// table is built. Idempotent: a populated world is left untouched.
func (w *World) EnsureMatchState() {
	for i := range w.Players {
		p := &w.Players[i]
		if p.Condition == 0 && p.Sharpness == 0 {
			p.Condition = ConditionMax
			p.Sharpness = ConditionMax
		}
	}
	if len(w.Table) == 0 {
		w.initTable()
	}
}

// ResultFor returns the stored result for a fixture, or nil if unplayed.
// Current season only — archived seasons live in History (ArchivedResultFor);
// the split is deliberate so form strings, dashboards, and cup progression
// never bleed a past season into the current one.
func (w *World) ResultFor(fixtureID int64) *MatchResult {
	for i := range w.Results {
		if w.Results[i].FixtureID == fixtureID {
			return &w.Results[i]
		}
	}
	return nil
}

// ArchivedResultFor returns a past season's archived result, newest season
// first, or nil. Linear — archive lookups are rare (a get_match on an old
// fixture id) and fixture ids are unique across seasons (monotonic).
func (w *World) ArchivedResultFor(fixtureID int64) *MatchResult {
	for s := len(w.History) - 1; s >= 0; s-- {
		results := w.History[s].Results
		for i := range results {
			if results[i].FixtureID == fixtureID {
				return &results[i]
			}
		}
	}
	return nil
}

// archiveCopy is the compact permanent form of a result: every FACT — score,
// winner, lineups, subs (with reasons), scorers, cards, ratings, adjustments,
// shots — deep-copied for isolation, with only the Commentary prose dropped
// (the bulk of the payload, and the one field holding map[string]any params).
func (r *MatchResult) archiveCopy() MatchResult {
	out := *r
	out.Commentary = nil
	out.HomeXI = append([]int64{}, r.HomeXI...)
	out.AwayXI = append([]int64{}, r.AwayXI...)
	out.Subs = append([]SubEvent{}, r.Subs...)
	out.Scorers = append([]MatchEvent{}, r.Scorers...)
	out.Cards = append([]MatchEvent{}, r.Cards...)
	out.Adjustments = append([]Adjustment{}, r.Adjustments...)
	if r.RatingsX10 != nil {
		out.RatingsX10 = make(map[int64]int, len(r.RatingsX10))
		for k, v := range r.RatingsX10 {
			out.RatingsX10[k] = v
		}
	}
	if r.ChanceTypes != nil {
		out.ChanceTypes = make(map[string]int, len(r.ChanceTypes))
		for k, v := range r.ChanceTypes {
			out.ChanceTypes[k] = v
		}
	}
	out.Diagnostics = r.Diagnostics.Clone()
	return out
}

func (d MatchDiagnostics) Clone() MatchDiagnostics {
	return MatchDiagnostics{
		ShotQuality:       cloneStringIntMap(d.ShotQuality),
		ShotQualityBySide: cloneStringIntMap(d.ShotQualityBySide),
		AerialDuels:       cloneStringIntMap(d.AerialDuels),
		AerialWins:        cloneStringIntMap(d.AerialWins),
		PressTurnovers:    cloneStringIntMap(d.PressTurnovers),
		SetPieceThreat:    cloneStringIntMap(d.SetPieceThreat),
		TacticalTilt:      cloneStringIntMap(d.TacticalTilt),
	}
}

func cloneStringIntMap(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// initTable seeds the live current-season standings: one table per division,
// every club at zero, ordered by club id (deterministic; re-sorts as results
// land). Called once at generation.
func (w *World) initTable() {
	w.Table = make([][]Standing, w.Config.Divisions)
	byTier := make([][]int64, w.Config.Divisions)
	for _, c := range w.Clubs {
		if c.DivisionTier >= 1 && c.DivisionTier <= w.Config.Divisions {
			byTier[c.DivisionTier-1] = append(byTier[c.DivisionTier-1], c.ID)
		}
	}
	for tier := range byTier {
		ids := byTier[tier]
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		rows := make([]Standing, len(ids))
		for i, id := range ids {
			rows[i] = Standing{Pos: i + 1, ClubID: id}
		}
		w.Table[tier] = rows
	}
}

// RecordLeagueResult folds a finished league fixture into the live table and
// re-sorts the division (points desc, goal difference desc, goals-for desc,
// club id asc — a total order, so the standings are replay-identical).
func (w *World) RecordLeagueResult(tier int, homeID, awayID int64, homeGoals, awayGoals int) {
	if tier < 1 || tier > len(w.Table) {
		return
	}
	table := w.Table[tier-1]
	for i := range table {
		row := &table[i]
		switch row.ClubID {
		case homeID:
			applyResult(row, homeGoals, awayGoals)
		case awayID:
			applyResult(row, awayGoals, homeGoals)
		}
	}
	sort.Slice(table, func(i, j int) bool {
		a, b := table[i], table[j]
		if a.Points != b.Points {
			return a.Points > b.Points
		}
		if gdA, gdB := a.GoalsFor-a.GoalsAgainst, b.GoalsFor-b.GoalsAgainst; gdA != gdB {
			return gdA > gdB
		}
		if a.GoalsFor != b.GoalsFor {
			return a.GoalsFor > b.GoalsFor
		}
		return a.ClubID < b.ClubID
	})
	for i := range table {
		table[i].Pos = i + 1
	}
}

func applyResult(row *Standing, scored, conceded int) {
	row.Played++
	row.GoalsFor += scored
	row.GoalsAgainst += conceded
	switch {
	case scored > conceded:
		row.Won++
		row.Points += 3
	case scored == conceded:
		row.Drawn++
		row.Points++
	default:
		row.Lost++
	}
}
