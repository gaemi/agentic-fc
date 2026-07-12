package narrative

import (
	"encoding/json"
	"strings"
	"testing"
)

func richStory(overrides map[string]any) map[string]any {
	story := map[string]any{
		"best_margin": 2, "draws": 1, "count": 8, "goals": 18,
		"home_wins": 4, "away_wins": 3, "scoreless": 0,
		"best_home": "Alpha", "best_away": "Beta", "home_goals": 3, "away_goals": 1,
		"top_total": 4, "top_margin": 0, "top_home": "Gamma", "top_away": "Delta",
		"top_home_goals": 2, "top_away_goals": 2,
	}
	for k, v := range overrides {
		story[k] = v
	}
	return story
}

// Payloads persisted before the round facts existed must render the legacy
// fragments byte for byte, so published articles do not silently rewrite.
func TestMatchdayStoryLineLegacyPayload(t *testing.T) {
	story := map[string]any{
		"best_margin": 1, "draws": 1,
		"best_home": "Alpha", "best_away": "Beta", "home_goals": 2, "away_goals": 1,
	}
	got := MatchdayStoryLine(Default, LocaleEN, story, 7)
	want := "Biggest margin: Alpha 2-1 Beta\nDraws: 1"
	if got != want {
		t.Fatalf("legacy story = %q, want %q", got, want)
	}
	if got := MatchdayStoryLine(Default, LocaleEN, map[string]any{"best_margin": 0, "draws": 0}, 7); got != "Storyline data was unavailable for this round-up" {
		t.Fatalf("malformed legacy story = %q", got)
	}
}

// Rich payloads lead with the widest win, graded by margin and framed for the
// side that won it, in both locales.
func TestMatchdayStoryLineLeads(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]any
		wants     map[Locale]string
	}{
		{"rout.home", map[string]any{"best_margin": 4, "home_goals": 5, "away_goals": 1},
			map[Locale]string{LocaleEN: "loudest statement", LocaleKO: "가장 큰 목소리"}},
		{"rout.away", map[string]any{"best_margin": 3, "home_goals": 0, "away_goals": 3},
			map[Locale]string{LocaleEN: "on the road at Alpha", LocaleKO: "Alpha 원정에서"}},
		{"clear.home", map[string]any{"best_margin": 2, "home_goals": 3, "away_goals": 1},
			map[Locale]string{LocaleEN: "clearest win", LocaleKO: "가장 선명한 승리"}},
		{"tight", map[string]any{"best_margin": 1, "home_goals": 2, "away_goals": 1},
			map[Locale]string{LocaleEN: "a single goal", LocaleKO: "한 골에 그쳤습니다"}},
		{"deadlock", map[string]any{"best_margin": 0, "draws": 8},
			map[Locale]string{LocaleEN: "Not one fixture found a winner", LocaleKO: "승자가 나온 경기가 하나도"}},
		// A cup round where every tie finished level: no draws recorded (all
		// carried a shootout winner), so the lead must speak penalties, not
		// "no winner".
		{"shootout_round", map[string]any{"best_margin": 0, "draws": 0, "home_wins": 5, "away_wins": 3},
			map[Locale]string{LocaleEN: "needed penalties", LocaleKO: "승부차기까지"}},
	}
	for _, tt := range tests {
		for loc, want := range tt.wants {
			got := MatchdayStoryLine(Default, loc, richStory(tt.overrides), 3)
			if !strings.Contains(got, want) {
				t.Errorf("%s %s: story %q missing %q", tt.name, loc, got, want)
			}
		}
	}
}

// The angle list is gated on the facts: impossible angles never appear, and
// the news-ID rotation only chooses among applicable ones.
func TestMatchdayStoryAngleGating(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]any
		want      string
		exclude   string
	}{
		{"goalfest", map[string]any{"goals": 30}, "term.matchday.story.angle.goalfest", "term.matchday.story.angle.gridlock"},
		{"gridlock", map[string]any{"goals": 6}, "term.matchday.story.angle.gridlock", "term.matchday.story.angle.goalfest"},
		{"awayday", map[string]any{"away_wins": 5, "home_wins": 2}, "term.matchday.story.angle.awayday", "term.matchday.story.angle.fortress"},
		{"fortress", map[string]any{"home_wins": 7, "away_wins": 0}, "term.matchday.story.angle.fortress", "term.matchday.story.angle.awayday"},
		{"thriller", map[string]any{"top_total": 6, "top_margin": 0}, "term.matchday.story.angle.thriller", ""},
		{"thriller.close", map[string]any{"top_total": 5, "top_margin": 1}, "term.matchday.story.angle.thriller", ""},
		// A 5-1 is high-scoring but one-sided: never a thriller.
		{"thriller.lopsided", map[string]any{"top_total": 6, "top_margin": 4}, "", "term.matchday.story.angle.thriller"},
		{"stalemates", map[string]any{"draws": 3, "scoreless": 2}, "term.matchday.story.angle.stalemates", "term.matchday.story.angle.level"},
		{"decisive", map[string]any{"draws": 0}, "term.matchday.story.angle.decisive", "term.matchday.story.angle.stalemates"},
	}
	for _, tt := range tests {
		angles := storyAngleKeys(richStory(tt.overrides))
		found := false
		for _, a := range angles {
			if tt.want != "" && a == tt.want {
				found = true
			}
			if tt.exclude != "" && a == tt.exclude {
				t.Errorf("%s: angle list %v contains excluded %s", tt.name, angles, tt.exclude)
			}
		}
		if tt.want != "" && !found {
			t.Errorf("%s: angle list %v missing %s", tt.name, angles, tt.want)
		}
	}
}

// A thriller that is the same fixture as the widest win must not repeat the
// lead's game as the secondary angle.
func TestMatchdayStoryThrillerSkipsLeadMatch(t *testing.T) {
	story := richStory(map[string]any{
		"top_total": 6, "top_home": "Alpha", "top_away": "Beta",
		"top_home_goals": 5, "top_away_goals": 1,
	})
	for _, a := range storyAngleKeys(story) {
		if a == "term.matchday.story.angle.thriller" {
			t.Fatalf("thriller angle offered for the lead fixture: %v", storyAngleKeys(story))
		}
	}
}

// Every lead and angle key must exist in both catalogs, and the rotation must
// reach every applicable angle across news IDs.
func TestMatchdayStoryKeysExistAndRotate(t *testing.T) {
	keys := []string{
		"term.matchday.story.lead.rout.home", "term.matchday.story.lead.rout.away",
		"term.matchday.story.lead.clear.home", "term.matchday.story.lead.clear.away",
		"term.matchday.story.lead.tight", "term.matchday.story.lead.deadlock",
		"term.matchday.story.lead.shootout_round",
		"term.matchday.story.angle.goalfest", "term.matchday.story.angle.gridlock",
		"term.matchday.story.angle.awayday", "term.matchday.story.angle.fortress",
		"term.matchday.story.angle.thriller", "term.matchday.story.angle.stalemates",
		"term.matchday.story.angle.level", "term.matchday.story.angle.decisive",
		"term.matchday.story.angle.balance",
	}
	for _, key := range keys {
		for _, loc := range Supported {
			if _, ok := Default[loc][key]; !ok {
				t.Fatalf("story key %q missing from %s catalog", key, loc)
			}
		}
	}
	story := richStory(map[string]any{"goals": 30, "away_wins": 5, "home_wins": 2, "top_total": 6})
	angles := storyAngleKeys(story)
	if len(angles) < 2 {
		t.Fatalf("expected multiple applicable angles, got %v", angles)
	}
	seen := map[string]bool{}
	for id := int64(1); id <= 64; id++ {
		line := MatchdayStoryLine(Default, LocaleEN, story, id)
		for _, a := range angles {
			if strings.Contains(line, Default.Render(LocaleEN, a, story)) {
				seen[a] = true
			}
		}
	}
	if len(seen) != len(angles) {
		t.Fatalf("rotation reached %d of %d angles: %v", len(seen), len(angles), seen)
	}
}

// Story params survive a JSON round-trip (json.Number decoding included), as
// happens when persisted worlds are reloaded.
func TestMatchdayStoryLineAfterJSONRoundTrip(t *testing.T) {
	raw, err := json.Marshal(richStory(map[string]any{"best_margin": 3, "home_goals": 4, "away_goals": 1}))
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var story map[string]any
	if err := dec.Decode(&story); err != nil {
		t.Fatal(err)
	}
	got := MatchdayStoryLine(Default, LocaleEN, story, 11)
	if !strings.Contains(got, "loudest statement") {
		t.Fatalf("round-tripped story = %q, want rout lead", got)
	}
	if !strings.Contains(got, ". ") {
		t.Fatalf("round-tripped story = %q, want a secondary angle", got)
	}
}
