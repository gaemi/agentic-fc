// Package mindset implements the typed Mindset & Tactical Plan schema from
// docs/10-mindset-schema.md — the Agent's entire write surface (FR-15/16).
package mindset

import (
	"errors"
	"fmt"
	"strings"
)

// ---- Disposition (10 bipolar axes, −10…+10) ----

type Axis string

const (
	AxisRiskAppetite        Axis = "D1"  // cautious ↔ daring
	AxisYouthPreference     Axis = "D2"  // proven veterans ↔ youth-first
	AxisPlayingIdentity     Axis = "D3"  // pragmatic ↔ expressive
	AxisFinancialPrudence   Axis = "D4"  // spendthrift ↔ frugal
	AxisLoyalty             Axis = "D5"  // ruthless ↔ loyal
	AxisDiscipline          Axis = "D6"  // laissez-faire ↔ authoritarian
	AxisHorizon             Axis = "D7"  // this weekend ↔ five-year project
	AxisMediaPosture        Axis = "D8"  // guarded ↔ provocative
	AxisTacticalFlexibility Axis = "D9"  // system purist ↔ chameleon
	AxisPersonalAmbition    Axis = "D10" // content ↔ ladder-climbing
)

var AllAxes = []Axis{
	AxisRiskAppetite, AxisYouthPreference, AxisPlayingIdentity,
	AxisFinancialPrudence, AxisLoyalty, AxisDiscipline, AxisHorizon,
	AxisMediaPosture, AxisTacticalFlexibility, AxisPersonalAmbition,
}

const (
	AxisMin = -10
	AxisMax = 10

	// InstantDelta: disposition deltas ≤ this apply immediately; larger
	// deltas drift at DriftPerGameWeek (FR-16b).
	InstantDelta     = 2
	DriftPerGameWeek = 2
)

// Disposition holds current values and drift targets. Decision rolls always
// consume Current; only Target updates instantly (FR-18 exception).
type Disposition struct {
	Current map[Axis]int `json:"current"`
	Target  map[Axis]int `json:"target,omitempty"`
}

// ---- Priorities (ranked goals, max 5) ----

type Goal string

const (
	GoalAvoidRelegation   Goal = "AVOID_RELEGATION"
	GoalWinLeague         Goal = "WIN_LEAGUE"
	GoalWinPromotion      Goal = "WIN_PROMOTION"
	GoalFinishTopN        Goal = "FINISH_TOP_N"
	GoalCupRun            Goal = "CUP_RUN"
	GoalDevelopYouth      Goal = "DEVELOP_YOUTH"
	GoalFinancialHealth   Goal = "FINANCIAL_HEALTH"
	GoalBuildSquadValue   Goal = "BUILD_SQUAD_VALUE"
	GoalEstablishIdentity Goal = "ESTABLISH_IDENTITY"
	GoalProtectJob        Goal = "PROTECT_JOB"
	GoalFindJob           Goal = "FIND_JOB"
)

const MaxPriorities = 5

// RankWeights: rank 1 → index 0 (docs/10 §3).
var RankWeights = [MaxPriorities]float64{1.0, 0.6, 0.4, 0.25, 0.15}

type Priority struct {
	Rank int  `json:"rank"` // 1-based
	Goal Goal `json:"goal"`
	// Params follow JSON number semantics: numeric values are float64,
	// always — they arrive from agents as JSON (MCP) and round-trip
	// through FR-28 snapshots, so any other numeric type would not
	// survive faithfully.
	Params map[string]any `json:"params,omitempty"`
}

// ---- Directives (typed standing orders, max 15) ----

type Verb string

const (
	// Selection & squad
	VerbStart       Verb = "START"
	VerbBench       Verb = "BENCH"
	VerbExclude     Verb = "EXCLUDE"
	VerbRotate      Verb = "ROTATE"
	VerbGiveMinutes Verb = "GIVE_MINUTES"
	VerbCaptain     Verb = "CAPTAIN"
	// Transfers
	VerbSign          Verb = "SIGN"
	VerbSell          Verb = "SELL"
	VerbKeep          Verb = "KEEP"
	VerbLoanOut       Verb = "LOAN_OUT"
	VerbTargetProfile Verb = "TARGET_PROFILE"
	// Contracts
	VerbRenew   Verb = "RENEW"
	VerbRelease Verb = "RELEASE"
	VerbWageCap Verb = "WAGE_CAP"
	// Development
	VerbDevelop         Verb = "DEVELOP"
	VerbRetrainPosition Verb = "RETRAIN_POSITION"
	// Tactics guard
	VerbForbid Verb = "FORBID"
	// Career & board
	VerbPursueJob Verb = "PURSUE_JOB"
	VerbRejectJob Verb = "REJECT_JOB"
	VerbPushBoard Verb = "PUSH_BOARD"
)

type Strength string

const (
	StrengthLean     Strength = "LEAN"     // tilts close calls
	StrengthInsist   Strength = "INSIST"   // overrides most considerations
	StrengthAbsolute Strength = "ABSOLUTE" // ~95% compliance region — never certainty
)

// OddsMultiplier returns the initial weight effect (docs/10 §4.1, tunable).
func (s Strength) OddsMultiplier() float64 {
	switch s {
	case StrengthLean:
		return 2
	case StrengthInsist:
		return 6
	case StrengthAbsolute:
		return 20
	}
	return 1
}

// FocusCost in FP for add_directive (docs/11 §2, tunable).
func (s Strength) FocusCost() int {
	switch s {
	case StrengthLean:
		return 6
	case StrengthInsist:
		return 10
	case StrengthAbsolute:
		return 18
	}
	return 0
}

// Target is the typed reference a Directive points at. Exactly the fields
// relevant to the verb are set; ValidateTarget enforces the pairing.
type Target struct {
	Player        int64  `json:"player,omitempty"`
	Club          int64  `json:"club,omitempty"`
	PositionGroup string `json:"position_group,omitempty"` // GK/DF/MF/FW
	AgeGroup      string `json:"age_group,omitempty"`      // e.g. U21
	Formation     string `json:"formation,omitempty"`
	DivisionTier  int    `json:"division_tier,omitempty"`
	Scope         string `json:"scope,omitempty"` // WAGE_CAP scope, PUSH_BOARD request, ANY…
}

type Directive struct {
	ID       string   `json:"id"`
	Verb     Verb     `json:"verb"`
	Target   Target   `json:"target"`
	Strength Strength `json:"strength"`
	// Params follow JSON number semantics (numerics are float64) — see
	// Priority.Params.
	Params     map[string]any    `json:"params,omitempty"`
	Conditions map[string]string `json:"conditions,omitempty"`
	Expiry     string            `json:"expiry,omitempty"` // game date | END_OF_WINDOW | END_OF_SEASON
}

const MaxDirectives = 15 // FR-19, initial value

// AllVerbs and AllGoals exist for catalog drift tests (docs/10 vs code).
var AllVerbs = []Verb{
	VerbStart, VerbBench, VerbExclude, VerbRotate, VerbGiveMinutes, VerbCaptain,
	VerbSign, VerbSell, VerbKeep, VerbLoanOut, VerbTargetProfile,
	VerbRenew, VerbRelease, VerbWageCap,
	VerbDevelop, VerbRetrainPosition,
	VerbForbid,
	VerbPursueJob, VerbRejectJob, VerbPushBoard,
}

// AllStrengths mirrors AllVerbs/AllGoals for catalog drift tests.
var AllStrengths = []Strength{StrengthLean, StrengthInsist, StrengthAbsolute}

// DialValues exposes the tactical dial vocabularies for catalog drift tests
// (a new dial value must ship with its enum.* catalog entries). A deep copy —
// handing out the live validation map would be a mutation footgun.
func DialValues() map[string][]string {
	out := make(map[string][]string, len(tacticalDials))
	for dial, values := range tacticalDials {
		out[dial] = append([]string{}, values...)
	}
	return out
}

var AllGoals = []Goal{
	GoalAvoidRelegation, GoalWinLeague, GoalWinPromotion, GoalFinishTopN,
	GoalCupRun, GoalDevelopYouth, GoalFinancialHealth, GoalBuildSquadValue,
	GoalEstablishIdentity, GoalProtectJob, GoalFindJob,
}

// ErrInvalidTarget maps to the INVALID_TARGET wire error (docs/11 §1.2).
var ErrInvalidTarget = errors.New("target does not fit verb (INVALID_TARGET)")

// ValidateTarget enforces the verb → target-shape pairing (docs/10 §4.2/4.3).
func ValidateTarget(v Verb, t Target) error {
	fail := func(want string) error {
		return fmt.Errorf("%s requires %s: %w", v, want, ErrInvalidTarget)
	}
	switch v {
	case VerbStart, VerbBench, VerbExclude, VerbCaptain,
		VerbSign, VerbSell, VerbKeep, VerbLoanOut,
		VerbRenew, VerbRelease, VerbDevelop, VerbRetrainPosition:
		if t.Player == 0 {
			return fail("a player")
		}
	case VerbGiveMinutes:
		if t.Player == 0 && t.AgeGroup == "" {
			return fail("a player or an age group")
		}
	case VerbRotate, VerbTargetProfile:
		if t.PositionGroup == "" {
			return fail("a position group")
		}
	case VerbWageCap:
		switch t.Scope {
		case "new_signings", "renewals", "all":
		default:
			return fail(`scope "new_signings" | "renewals" | "all"`)
		}
	case VerbForbid:
		if t.Formation == "" && t.Scope == "" {
			return fail("a formation or a style element (scope)")
		}
		// Style fences use the dial:VALUE convention (docs/10 §4.2); a
		// malformed scope would be accepted-but-inert, so reject it here.
		if t.Scope != "" {
			if err := validateForbidScope(t.Scope); err != nil {
				return fmt.Errorf("%v: %w", err, ErrInvalidTarget)
			}
		}
	case VerbPursueJob:
		if t.Club == 0 && t.DivisionTier == 0 && t.Scope != "ANY" {
			return fail(`a club, a division tier, or scope "ANY"`)
		}
	case VerbRejectJob:
		if t.Club == 0 && t.DivisionTier == 0 {
			return fail("a club or a division tier")
		}
	case VerbPushBoard:
		switch t.Scope {
		case "budget", "training_facilities", "youth_facilities":
		default:
			return fail(`scope "budget" | "training_facilities" | "youth_facilities"`)
		}
	default:
		return fmt.Errorf("unknown verb %q: %w", v, ErrInvalidTarget)
	}
	return nil
}

// ---- Tactical Plan (v1: formation + six dials) ----

type TacticalPlan struct {
	Formation  string `json:"formation"`  // from FormationCatalog
	Mentality  string `json:"mentality"`  // VERY_DEFENSIVE…VERY_ATTACKING
	Pressing   string `json:"pressing"`   // LOW | MID | HIGH
	Tempo      string `json:"tempo"`      // SLOW | MIXED | FAST
	Width      string `json:"width"`      // NARROW | MIXED | WIDE
	Directness string `json:"directness"` // SHORT | MIXED | DIRECT
	Counter    bool   `json:"counter"`
}

// FormationCatalog is the v1 shape list (docs/10 §5, ~12 shapes).
var FormationCatalog = []string{
	"4-4-2", "4-3-3", "4-2-3-1", "4-1-4-1", "4-4-1-1", "4-3-1-2",
	"3-5-2", "3-4-3", "5-3-2", "5-4-1", "4-5-1", "4-2-2-2",
}

// validateForbidScope enforces the dial:VALUE style-fence convention
// (docs/10 §4.2): the dial must be a tactical dial and the value one of
// its allowed settings.
func validateForbidScope(scope string) error {
	dial, value, ok := strings.Cut(scope, ":")
	if !ok {
		return fmt.Errorf("style scope %q is not dial:VALUE", scope)
	}
	allowed, ok := tacticalDials[strings.ToLower(dial)]
	if !ok {
		return fmt.Errorf("unknown tactical dial %q", dial)
	}
	for _, v := range allowed {
		if strings.EqualFold(v, value) {
			return nil
		}
	}
	return fmt.Errorf("dial %s has no setting %q", dial, value)
}

var tacticalDials = map[string][]string{
	"mentality":  {"VERY_DEFENSIVE", "DEFENSIVE", "BALANCED", "ATTACKING", "VERY_ATTACKING"},
	"pressing":   {"LOW", "MID", "HIGH"},
	"tempo":      {"SLOW", "MIXED", "FAST"},
	"width":      {"NARROW", "MIXED", "WIDE"},
	"directness": {"SHORT", "MIXED", "DIRECT"},
}

// Validate checks every set (non-empty) field against its allowed values.
// Empty fields are allowed — update_tactical_plan patches partially
// (docs/11 §6); completeness is enforced when a plan is first activated.
func (p TacticalPlan) Validate() error {
	check := func(dial, val string) error {
		if val == "" {
			return nil
		}
		for _, ok := range tacticalDials[dial] {
			if val == ok {
				return nil
			}
		}
		return fmt.Errorf("invalid %s %q (VALIDATION)", dial, val)
	}
	if p.Formation != "" {
		found := false
		for _, f := range FormationCatalog {
			if p.Formation == f {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown formation %q (VALIDATION)", p.Formation)
		}
	}
	for dial, val := range map[string]string{
		"mentality": p.Mentality, "pressing": p.Pressing, "tempo": p.Tempo,
		"width": p.Width, "directness": p.Directness,
	} {
		if err := check(dial, val); err != nil {
			return err
		}
	}
	return nil
}

// ---- Mindset ----

type Mindset struct {
	Version         int          `json:"version"` // FR-16e: rolls record this
	ArchetypeOrigin string       `json:"archetype_origin,omitempty"`
	Disposition     Disposition  `json:"disposition"`
	Priorities      []Priority   `json:"priorities"`
	Directives      []Directive  `json:"directives"`
	Tactical        TacticalPlan `json:"tactical_plan"`

	nextDirectiveID int
}

var (
	ErrDirectiveCap = errors.New("directive cap reached (CAP_EXCEEDED)")
	ErrPriorityCap  = errors.New("too many priorities (CAP_EXCEEDED)")
)

// ConflictError reports a direct contradiction with an active Directive
// (FR-16d): the add is rejected, never merged.
type ConflictError struct{ With string }

func (e *ConflictError) Error() string {
	return fmt.Sprintf("directive conflicts with %s (CONFLICT)", e.With)
}

// opposing verb pairs on the same target constitute a direct contradiction.
var opposing = map[Verb][]Verb{
	VerbStart:     {VerbBench, VerbExclude},
	VerbBench:     {VerbStart},
	VerbExclude:   {VerbStart},
	VerbSell:      {VerbKeep},
	VerbKeep:      {VerbSell},
	VerbRenew:     {VerbRelease},
	VerbRelease:   {VerbRenew},
	VerbPursueJob: {VerbRejectJob},
	VerbRejectJob: {VerbPursueJob},
}

// AddDirective validates target shape, cap, and conflicts, then assigns an
// ID and bumps Version.
func (m *Mindset) AddDirective(d Directive) (Directive, error) {
	if err := ValidateTarget(d.Verb, d.Target); err != nil {
		return Directive{}, err
	}
	if len(m.Directives) >= MaxDirectives {
		return Directive{}, ErrDirectiveCap
	}
	for _, ex := range m.Directives {
		if conflicts(d, ex) {
			return Directive{}, &ConflictError{With: ex.ID}
		}
	}
	m.nextDirectiveID++
	d.ID = fmt.Sprintf("dir_%04d", m.nextDirectiveID)
	m.Directives = append(m.Directives, d)
	m.Version++
	return d, nil
}

// SyncDirectiveCounter re-derives the internal ID counter from the highest
// existing directive ID. Call after deserializing a Mindset (the counter is
// unexported and not persisted — FR-28 snapshots restore through JSON), or
// freshly added directives would reuse IDs.
func (m *Mindset) SyncDirectiveCounter() {
	max := 0
	for _, d := range m.Directives {
		var n int
		if _, err := fmt.Sscanf(d.ID, "dir_%d", &n); err == nil && n > max {
			max = n
		}
	}
	if max > m.nextDirectiveID {
		m.nextDirectiveID = max
	}
}

// RemoveDirective removes by ID and bumps Version; false if not found.
func (m *Mindset) RemoveDirective(id string) bool {
	for i, d := range m.Directives {
		if d.ID == id {
			m.Directives = append(m.Directives[:i], m.Directives[i+1:]...)
			m.Version++
			return true
		}
	}
	return false
}

// MaxDispositionAxesPerCall bounds one update_disposition call (docs/11 §6).
const MaxDispositionAxesPerCall = 3

var (
	// ErrTooManyAxes maps to CAP_EXCEEDED ("4th axis in one call").
	ErrTooManyAxes = errors.New("too many axes in one call (CAP_EXCEEDED)")
	// ErrValidation marks malformed shaping input (VALIDATION).
	ErrValidation = errors.New("invalid value (VALIDATION)")
)

// ValidateDispositionTargets checks an update_disposition payload without
// touching state — callers that must sequence other mutations around the
// update (e.g. applying accrued drift first) validate up front so a
// rejected call provably mutates nothing.
func ValidateDispositionTargets(targets map[Axis]int) error {
	if len(targets) > MaxDispositionAxesPerCall {
		return ErrTooManyAxes
	}
	valid := map[Axis]bool{}
	for _, a := range AllAxes {
		valid[a] = true
	}
	for a, v := range targets {
		if !valid[a] {
			return fmt.Errorf("unknown axis %q: %w", a, ErrValidation)
		}
		if v < AxisMin || v > AxisMax {
			return fmt.Errorf("axis %s value %d out of %d…%d: %w",
				a, v, AxisMin, AxisMax, ErrValidation)
		}
	}
	return nil
}

// SetDispositionTargets applies the FR-16b drift model: deltas ≤ InstantDelta
// apply immediately; larger ones set a drift target (~DriftPerGameWeek
// pts/game-week, applied by the engine). Returns the axes applied instantly
// and the axes now drifting. Axes are processed in catalog order
// (determinism — never map order).
func (m *Mindset) SetDispositionTargets(targets map[Axis]int) (applied, drifting []Axis, err error) {
	if err := ValidateDispositionTargets(targets); err != nil {
		return nil, nil, err
	}
	if m.Disposition.Current == nil {
		m.Disposition.Current = map[Axis]int{}
	}
	for _, a := range AllAxes {
		v, ok := targets[a]
		if !ok {
			continue
		}
		cur := m.Disposition.Current[a]
		delta := v - cur
		if delta < 0 {
			delta = -delta
		}
		switch {
		case delta == 0:
			if m.Disposition.Target != nil {
				delete(m.Disposition.Target, a)
			}
			applied = append(applied, a)
		case delta <= InstantDelta:
			m.Disposition.Current[a] = v
			if m.Disposition.Target != nil {
				delete(m.Disposition.Target, a)
			}
			applied = append(applied, a)
		default:
			if m.Disposition.Target == nil {
				m.Disposition.Target = map[Axis]int{}
			}
			m.Disposition.Target[a] = v
			drifting = append(drifting, a)
		}
	}
	m.Version++
	return applied, drifting, nil
}

// DriftDispositionStep moves every drifting axis one point toward its
// target (the engine calls this on the drift cadence). Returns the number
// of axes moved; reached targets clear. Version bumps only on movement.
func (m *Mindset) DriftDispositionStep() int {
	if len(m.Disposition.Target) == 0 {
		return 0
	}
	moved := 0
	for _, a := range AllAxes {
		tgt, ok := m.Disposition.Target[a]
		if !ok {
			continue
		}
		cur := m.Disposition.Current[a]
		switch {
		case cur < tgt:
			m.Disposition.Current[a] = cur + 1
			moved++
		case cur > tgt:
			m.Disposition.Current[a] = cur - 1
			moved++
		}
		if m.Disposition.Current[a] == tgt {
			delete(m.Disposition.Target, a)
		}
	}
	if len(m.Disposition.Target) == 0 {
		m.Disposition.Target = nil
	}
	if moved > 0 {
		m.Version++
	}
	return moved
}

// ApplyTacticalPatch merges a partial plan (docs/11 §6): empty fields keep
// their current value; setCounter distinguishes "flip counter" from "not
// sent". The merged plan validates before anything mutates.
func (m *Mindset) ApplyTacticalPatch(p TacticalPlan, setCounter bool) (TacticalPlan, error) {
	if err := p.Validate(); err != nil {
		return m.Tactical, fmt.Errorf("%v: %w", err, ErrValidation)
	}
	merged := m.Tactical
	if p.Formation != "" {
		merged.Formation = p.Formation
	}
	if p.Mentality != "" {
		merged.Mentality = p.Mentality
	}
	if p.Pressing != "" {
		merged.Pressing = p.Pressing
	}
	if p.Tempo != "" {
		merged.Tempo = p.Tempo
	}
	if p.Width != "" {
		merged.Width = p.Width
	}
	if p.Directness != "" {
		merged.Directness = p.Directness
	}
	if setCounter {
		merged.Counter = p.Counter
	}
	m.Tactical = merged
	m.Version++
	return merged, nil
}

// SetPriorities replaces the ranked list (full replace — docs/11 §6).
// Ranks must read 1..N in list order — rank weights key off them (docs/10 §3).
func (m *Mindset) SetPriorities(ps []Priority) error {
	if len(ps) > MaxPriorities {
		return ErrPriorityCap
	}
	seen := map[Goal]bool{}
	for i, p := range ps {
		if p.Rank != i+1 {
			return fmt.Errorf("rank %d at position %d, want %d (VALIDATION): %w",
				p.Rank, i, i+1, ErrValidation)
		}
		if seen[p.Goal] {
			return fmt.Errorf("duplicate goal %s (VALIDATION): %w", p.Goal, ErrValidation)
		}
		seen[p.Goal] = true
	}
	m.Priorities = ps
	m.Version++
	return nil
}

func conflicts(a, b Directive) bool {
	for _, v := range opposing[a.Verb] {
		if b.Verb == v && sameTarget(a.Target, b.Target) {
			return true
		}
	}
	return false
}

func sameTarget(a, b Target) bool { return a == b }
