package engine

import (
	"reflect"
	"testing"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

func TestRunCalibrationDeterministic(t *testing.T) {
	a, err := RunCalibration([]uint64{1, 2}, 365)
	if err != nil {
		t.Fatal(err)
	}
	b, err := RunCalibration([]uint64{1, 2}, 365)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("calibration is not deterministic:\n%+v\n%+v", a, b)
	}
	if a.Matches == 0 || a.GoalsPerMatchX100 == 0 || a.ShotsPerMatchX100 == 0 {
		t.Fatalf("calibration did not observe matches: %+v", a)
	}
	if a.ConversionRateX100 <= 0 || a.ConversionRateX100 > 100 {
		t.Fatalf("conversion rate outside sane band: %+v", a)
	}
	if sumCounts(a.ShotQuality) != a.Shots {
		t.Fatalf("shot quality %d != shots %d", sumCounts(a.ShotQuality), a.Shots)
	}
}

func TestTacticalRoleFitBonus(t *testing.T) {
	target := &worldgen.Player{
		Group: attr.FW, HeightCm: 196, WeightKg: 92, AbilityPool: 50,
		Visible: map[attr.Visible]int{
			attr.Heading: 16, attr.JumpingReach: 16, attr.Strength: 16,
			attr.Balance: 14, attr.Bravery: 15, attr.Finishing: 11,
			attr.Pace: 7, attr.Acceleration: 7, attr.OffBall: 10,
		},
		Condition: worldgen.ConditionMax, Sharpness: worldgen.ConditionMax,
	}
	runner := &worldgen.Player{
		Group: attr.FW, HeightCm: 178, WeightKg: 72, AbilityPool: 50,
		Visible: map[attr.Visible]int{
			attr.Pace: 17, attr.Acceleration: 17, attr.Agility: 15,
			attr.OffBall: 15, attr.Finishing: 13, attr.FirstTouch: 12,
			attr.Heading: 7, attr.JumpingReach: 8, attr.Strength: 8,
		},
		Condition: worldgen.ConditionMax, Sharpness: worldgen.ConditionMax,
	}
	directWide := mindset.TacticalPlan{Width: "WIDE", Directness: "DIRECT"}
	counter := mindset.TacticalPlan{Pressing: "HIGH", Counter: true}
	if playerTacticalRole(target) != roleTargetForward {
		t.Fatalf("target role = %s", playerTacticalRole(target))
	}
	if playerTacticalRole(runner) != roleRunner {
		t.Fatalf("runner role = %s", playerTacticalRole(runner))
	}
	if selectionScore(target, directWide) <= selectionScore(runner, directWide) {
		t.Fatalf("direct wide plan should prefer target: target=%d runner=%d", selectionScore(target, directWide), selectionScore(runner, directWide))
	}
	if selectionScore(runner, counter) <= selectionScore(target, counter) {
		t.Fatalf("counter plan should prefer runner: runner=%d target=%d", selectionScore(runner, counter), selectionScore(target, counter))
	}
}
