package narrative

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestCatalogParity is the catalog completeness guardrail (FR-35c,
// CLAUDE.md): every message key ships in BOTH en and ko in the same change.
func TestCatalogParity(t *testing.T) {
	en, ko := Default[LocaleEN], Default[LocaleKO]
	if len(en) == 0 || len(ko) == 0 {
		t.Fatal("default catalogs must not be empty")
	}
	for key := range en {
		if _, ok := ko[key]; !ok {
			t.Errorf("key %q has no Korean template (FR-35c: en+ko together)", key)
		}
	}
	for key := range ko {
		if _, ok := en[key]; !ok {
			t.Errorf("key %q has no English template (English is the fallback)", key)
		}
	}
}

// TestKoreanPlaceholdersAvoidFixedParticles prevents generated Latin and
// mixed-script values from producing broken phrases such as "Kim Ha-jun가".
// Korean templates should add a stable noun or rewrite the sentence whenever
// the following particle depends on a final consonant. Checking every named
// placeholder keeps future scorer/coach/team parameters inside the guardrail.
func TestKoreanPlaceholdersAvoidFixedParticles(t *testing.T) {
	unsafe := regexp.MustCompile(`\{[a-z_]+\}(?:이라고|라고|으로|이라|이나|이랑|은|는|이|가|을|를|과|와|로|라|나|랑)(?:$|[\s.,!?—:;)])`)
	for key, tmpl := range Default[LocaleKO] {
		if match := unsafe.FindString(tmpl); match != "" {
			t.Errorf("Korean template %q attaches unsafe fixed particle %q", key, match)
		}
	}
}

func TestKoreanCommentaryRendersGeneratedNamesGrammatically(t *testing.T) {
	tests := []struct {
		key    string
		params map[string]any
		want   string
	}{
		{"comment.chance.long.2", map[string]any{"player": "Kim Ha-jun"}, "Kim Ha-jun 선수가"},
		{"comment.goal.setpiece.3", map[string]any{"player": "Tom Kennedy", "club": "AFC Moorfield", "home_goals": 1, "away_goals": 0}, "AFC Moorfield 팀이"},
		{"comment.sub.fatigue", map[string]any{"off": "Connor Allen", "on": "Jung Ji-ho", "club": "Deportivo Rosales"}, "Connor Allen 선수의 체력이"},
	}
	for _, tt := range tests {
		got := Default.Render(LocaleKO, tt.key, tt.params)
		if !strings.Contains(got, tt.want) {
			t.Errorf("%s rendered %q, want phrase %q", tt.key, got, tt.want)
		}
	}
}

func TestDefaultRendering(t *testing.T) {
	got := Default.Render(LocaleKO, "feed.window.opened", map[string]any{
		"window": Default.Render(LocaleKO, "term.window.summer", nil),
	})
	if got != "여름 이적시장이 열렸습니다." {
		t.Fatalf("ko render = %q", got)
	}
	// Unknown locale falls back to English; unknown key falls back to key.
	if got := Default.Render(Locale("fr"), "term.free_agent", nil); got != "Free agent" {
		t.Fatalf("fallback render = %q", got)
	}
	if got := Default.Render(LocaleEN, "no.such.key", nil); got != "no.such.key" {
		t.Fatalf("missing-key render = %q", got)
	}
}

// TestTransferFeeNewsHidesFee locks the FR-22 guard: the
// agent-facing fee-transfer headline names the clubs and the player but never the
// fee. The exact fee is pool²·k, so rendering it would invert to the hidden
// Ability Pool — the fee rides in the news Params for the human Console only, and
// the agent surface (renderNews) exposes just this rendered headline.
func TestTransferFeeNewsHidesFee(t *testing.T) {
	const fee = 12_345_000
	params := map[string]any{
		"club": "Athletic", "player": "R. Vega", "from": "United",
		"fee": fee, "wage": 40_000,
	}
	for _, loc := range []Locale{LocaleEN, LocaleKO} {
		got := Default.Render(loc, "news.transfer.fee_completed", params)
		if strings.Contains(got, strconv.Itoa(fee)) {
			t.Fatalf("%s headline leaks the fee (%d): %q", loc, fee, got)
		}
		if !strings.Contains(got, "R. Vega") || !strings.Contains(got, "United") {
			t.Fatalf("%s headline dropped the public names: %q", loc, got)
		}
	}
}
