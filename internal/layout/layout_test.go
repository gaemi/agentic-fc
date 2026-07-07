package layout

import "testing"

// Golden thresholds per docs/07 §2 — the tier matrix acceptance spec.
func TestTierTable(t *testing.T) {
	cases := []struct {
		cols, rows int
		want       Tier
	}{
		{59, 24, TierXS},  // too narrow
		{200, 15, TierXS}, // too short is XS at any width
		{60, 16, TierS},
		{80, 24, TierS}, // the default terminal must land in S
		{99, 24, TierS},
		{100, 24, TierM},
		{139, 24, TierM},
		{140, 24, TierL},
		{179, 24, TierL},
		{180, 24, TierXL},
	}
	for _, c := range cases {
		if got := Compute(c.cols, c.rows); got != c.want {
			t.Errorf("Compute(%d, %d) = %s, want %s", c.cols, c.rows, got, c.want)
		}
	}
	if Rows(27) != RowsShort || Rows(28) != RowsTall || Rows(42) != RowsExtraTall {
		t.Error("row modifiers drifted from docs/07 §2 (16/28/42)")
	}
}
