package mindset

import (
	"errors"
	"testing"
)

func TestAddDirectiveConflictRejected(t *testing.T) {
	m := &Mindset{}
	_, err := m.AddDirective(Directive{Verb: VerbKeep, Target: Target{Player: 4521}, Strength: StrengthAbsolute})
	if err != nil {
		t.Fatalf("KEEP should succeed: %v", err)
	}
	_, err = m.AddDirective(Directive{Verb: VerbSell, Target: Target{Player: 4521}, Strength: StrengthLean})
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("SELL same player must conflict, got %v", err)
	}
	// Different player: no conflict.
	if _, err := m.AddDirective(Directive{Verb: VerbSell, Target: Target{Player: 999}, Strength: StrengthLean}); err != nil {
		t.Fatalf("SELL other player should succeed: %v", err)
	}
}

func TestDirectiveCap(t *testing.T) {
	m := &Mindset{}
	for i := 0; i < MaxDirectives; i++ {
		_, err := m.AddDirective(Directive{Verb: VerbDevelop, Target: Target{Player: int64(i + 1)}, Strength: StrengthLean})
		if err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}
	if _, err := m.AddDirective(Directive{Verb: VerbDevelop, Target: Target{Player: 999}, Strength: StrengthLean}); !errors.Is(err, ErrDirectiveCap) {
		t.Fatalf("16th directive must hit cap, got %v", err)
	}
}

func TestSyncDirectiveCounter(t *testing.T) {
	m := &Mindset{}
	m.AddDirective(Directive{Verb: VerbKeep, Target: Target{Player: 1}, Strength: StrengthLean})
	m.AddDirective(Directive{Verb: VerbKeep, Target: Target{Player: 2}, Strength: StrengthLean})

	// Simulate an FR-28 JSON roundtrip: the unexported counter is lost.
	restored := &Mindset{Directives: append([]Directive{}, m.Directives...)}
	restored.SyncDirectiveCounter()
	d, err := restored.AddDirective(Directive{Verb: VerbKeep, Target: Target{Player: 3}, Strength: StrengthLean})
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "dir_0003" {
		t.Fatalf("post-restore ID = %s, want dir_0003 (no reuse)", d.ID)
	}
}

func TestVersionIncrements(t *testing.T) {
	m := &Mindset{}
	d, _ := m.AddDirective(Directive{Verb: VerbCaptain, Target: Target{Player: 1}, Strength: StrengthInsist})
	if m.Version != 1 {
		t.Fatalf("version after add = %d, want 1", m.Version)
	}
	if !m.RemoveDirective(d.ID) {
		t.Fatal("remove should find the directive")
	}
	if m.Version != 2 {
		t.Fatalf("version after remove = %d, want 2", m.Version)
	}
}

func TestSetPrioritiesValidation(t *testing.T) {
	m := &Mindset{}
	err := m.SetPriorities([]Priority{
		{Rank: 1, Goal: GoalAvoidRelegation},
		{Rank: 2, Goal: GoalDevelopYouth},
	})
	if err != nil {
		t.Fatalf("valid priorities rejected: %v", err)
	}
	err = m.SetPriorities([]Priority{
		{Rank: 1, Goal: GoalWinLeague},
		{Rank: 2, Goal: GoalWinLeague},
	})
	if err == nil {
		t.Fatal("duplicate goals must be rejected")
	}
	// Ranks must read 1..N in order — the rank weights key off them.
	err = m.SetPriorities([]Priority{
		{Rank: 2, Goal: GoalWinLeague},
		{Rank: 1, Goal: GoalCupRun},
	})
	if err == nil {
		t.Fatal("out-of-order ranks must be rejected")
	}
	six := make([]Priority, 6)
	goals := []Goal{GoalAvoidRelegation, GoalWinLeague, GoalCupRun, GoalDevelopYouth, GoalFinancialHealth, GoalProtectJob}
	for i := range six {
		six[i] = Priority{Rank: i + 1, Goal: goals[i]}
	}
	if err := m.SetPriorities(six); !errors.Is(err, ErrPriorityCap) {
		t.Fatalf("6 priorities must hit cap, got %v", err)
	}
}

// Catalog drift tests: lock code catalogs to docs/10 counts and values.
func TestCatalogGolden(t *testing.T) {
	if len(AllAxes) != 10 {
		t.Fatalf("Disposition axes = %d, docs/10 §2 says 10", len(AllAxes))
	}
	if len(AllGoals) != 11 {
		t.Fatalf("Priority goals = %d, docs/10 §3 says 11", len(AllGoals))
	}
	if len(AllVerbs) != 20 {
		t.Fatalf("Directive verbs = %d, docs/10 §4.2 says 20", len(AllVerbs))
	}
	if MaxDirectives != 15 || MaxPriorities != 5 {
		t.Fatal("caps drifted from FR-19 / FR-16c")
	}
	if RankWeights != [5]float64{1.0, 0.6, 0.4, 0.25, 0.15} {
		t.Fatal("rank weights drifted from docs/10 §3")
	}
	wantCosts := map[Strength]int{StrengthLean: 6, StrengthInsist: 10, StrengthAbsolute: 18}
	wantOdds := map[Strength]float64{StrengthLean: 2, StrengthInsist: 6, StrengthAbsolute: 20}
	for s, c := range wantCosts {
		if s.FocusCost() != c {
			t.Fatalf("%s focus cost drifted from docs/11 §2", s)
		}
		if s.OddsMultiplier() != wantOdds[s] {
			t.Fatalf("%s odds multiplier drifted from docs/10 §4.1", s)
		}
	}
	if len(FormationCatalog) != 12 {
		t.Fatalf("formation catalog = %d, docs/10 §5 says ~12 (update both together)", len(FormationCatalog))
	}
}

func TestValidateTarget(t *testing.T) {
	valid := []Directive{
		{Verb: VerbStart, Target: Target{Player: 1}},
		{Verb: VerbGiveMinutes, Target: Target{AgeGroup: "U21"}},
		{Verb: VerbRotate, Target: Target{PositionGroup: "GK"}},
		{Verb: VerbWageCap, Target: Target{Scope: "renewals"}},
		{Verb: VerbForbid, Target: Target{Formation: "3-5-2"}},
		{Verb: VerbPursueJob, Target: Target{Scope: "ANY"}},
		{Verb: VerbPushBoard, Target: Target{Scope: "youth_facilities"}},
	}
	for _, d := range valid {
		if err := ValidateTarget(d.Verb, d.Target); err != nil {
			t.Errorf("%s %+v should be valid: %v", d.Verb, d.Target, err)
		}
	}
	// FORBID style fences follow the dial:VALUE convention (docs/10 §4.2).
	if err := ValidateTarget(VerbForbid, Target{Scope: "pressing:HIGH"}); err != nil {
		t.Errorf("dial:VALUE scope should be valid: %v", err)
	}
	invalid := []Directive{
		{Verb: VerbStart, Target: Target{}},                       // player missing
		{Verb: VerbWageCap, Target: Target{Scope: "everything"}},  // bad scope
		{Verb: VerbForbid, Target: Target{}},                      // nothing forbidden
		{Verb: VerbForbid, Target: Target{Scope: "handbrake"}},    // not dial:VALUE
		{Verb: VerbForbid, Target: Target{Scope: "pressing:MAX"}}, // no such setting
		{Verb: VerbPursueJob, Target: Target{}},                   // no destination
	}
	for _, d := range invalid {
		if err := ValidateTarget(d.Verb, d.Target); !errors.Is(err, ErrInvalidTarget) {
			t.Errorf("%s %+v should fail with ErrInvalidTarget, got %v", d.Verb, d.Target, err)
		}
	}
}

func TestTacticalPlanValidate(t *testing.T) {
	ok := TacticalPlan{Formation: "4-4-2", Mentality: "DEFENSIVE", Pressing: "MID",
		Tempo: "FAST", Width: "MIXED", Directness: "DIRECT", Counter: true}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}
	if err := (TacticalPlan{Formation: "2-3-5"}).Validate(); err == nil {
		t.Fatal("unknown formation must be rejected")
	}
	if err := (TacticalPlan{Mentality: "YOLO"}).Validate(); err == nil {
		t.Fatal("unknown mentality must be rejected")
	}
	// Partial patches (empty fields) are fine.
	if err := (TacticalPlan{Pressing: "HIGH"}).Validate(); err != nil {
		t.Fatalf("partial plan should validate: %v", err)
	}
}
