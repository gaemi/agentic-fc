package focus

import "testing"

// Drift tests: lock the economy to docs/11 §2. If a value must change,
// change the doc, this test, and docs/98-tunables.md together.
func TestEconomyGolden(t *testing.T) {
	if Cap != 100 || RegenPerGameHour != 2 || StartingBalance != 100 {
		t.Fatal("economy constants drifted from docs/11 §2")
	}
}

func TestFlatCostsGolden(t *testing.T) {
	want := map[Tool]int{
		GetGuide: 0, GetTime: 0, GetFocus: 0, GetMindset: 0,
		GetSituation: 1, GetNews: 1, GetLeague: 2,
		GetPerson: 4, SearchPlayers: 4, Scout: 12,
		RemoveDirective: 2, SetPriorities: 12,
		UpdateTacticalPlan: 15, UpdateDisposition: 25,
	}
	for tool, cost := range want {
		got, ok := Cost(tool)
		if !ok || got != cost {
			t.Errorf("%s cost = %d (ok=%v), docs/11 §2 says %d", tool, got, ok, cost)
		}
	}
}

func TestOwnOtherCostsGolden(t *testing.T) {
	cases := []struct {
		tool       Tool
		own, other int
	}{
		{GetClub, 2, 4},
		{GetSquad, 3, 4},
		{GetMatch, 1, 3},
	}
	for _, c := range cases {
		if got, ok := CostOwnOther(c.tool, true); !ok || got != c.own {
			t.Errorf("%s own cost = %d, want %d", c.tool, got, c.own)
		}
		if got, ok := CostOwnOther(c.tool, false); !ok || got != c.other {
			t.Errorf("%s other cost = %d, want %d", c.tool, got, c.other)
		}
	}
}

// The canonical surface is 19 tools (docs/11): 5 free + 8 observation +
// 1 commission + 5 shaping.
func TestToolCount(t *testing.T) {
	all := []Tool{
		GetGuide, GetTime, GetSettings, GetFocus, GetMindset,
		GetSituation, GetNews, GetLeague, GetClub, GetSquad, GetPerson, GetMatch, SearchPlayers,
		Scout,
		UpdateDisposition, SetPriorities, AddDirective, RemoveDirective, UpdateTacticalPlan,
	}
	if len(all) != 19 {
		t.Fatalf("tool surface = %d, docs/11 says 19", len(all))
	}
	seen := map[Tool]bool{}
	for _, tool := range all {
		if seen[tool] {
			t.Fatalf("duplicate tool %s", tool)
		}
		seen[tool] = true
	}
}
