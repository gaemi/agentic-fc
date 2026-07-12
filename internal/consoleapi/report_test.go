package consoleapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gaemi/agentic-fc/internal/narrative"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// storyFixture builds a finished MatchResult between clubs 1 (Alpha) and 2
// (Beta) for direct matchStoryLines calls.
func storyFixture(fixtureID int64, homeGoals, awayGoals int) *worldgen.MatchResult {
	return &worldgen.MatchResult{
		FixtureID: fixtureID, HomeID: 1, AwayID: 2,
		HomeGoals: homeGoals, AwayGoals: awayGoals,
	}
}

func storyServer(t *testing.T) *Server {
	t.Helper()
	return &Server{Catalogs: narrative.Default}
}

func renderStory(t *testing.T, s *Server, r *worldgen.MatchResult) []string {
	t.Helper()
	names := map[int64]string{1: "Alpha", 2: "Beta"}
	playerName := func(id int64) string {
		return map[int64]string{9: "Hero", 21: "Villain"}[id]
	}
	return s.matchStoryLines(narrative.LocaleEN, names, playerName, r)
}

// The report is a verdict: result frame first, then at most one diagnostics
// edge and one ledger story beat — and every branch renders from real
// catalog keys in both locales (the Render fallback would leak raw keys).
func TestMatchStoryLinesFramesEdgesAndBeats(t *testing.T) {
	s := storyServer(t)

	r := storyFixture(101, 4, 0)
	r.Scorers = []worldgen.MatchEvent{
		{Minute: 10, PlayerID: 9, ClubID: 1}, {Minute: 30, PlayerID: 9, ClubID: 1},
		{Minute: 50, PlayerID: 9, ClubID: 1}, {Minute: 70, PlayerID: 5, ClubID: 1},
	}
	r.Diagnostics.PressTurnovers = map[string]int{"HOME": 5, "AWAY": 1}
	r.Diagnostics.ShotQualityBySide = map[string]int{"HOME_HIGH": 4, "AWAY_HIGH": 0}
	lines := renderStory(t, s, r)
	if len(lines) != 3 {
		t.Fatalf("story lines = %v, want frame+edge+beat", lines)
	}
	if !strings.Contains(lines[0], "Alpha") || !strings.Contains(lines[0], "Beta") {
		t.Fatalf("frame should name both sides: %q", lines[0])
	}
	if !strings.Contains(lines[1], "5") || !strings.Contains(lines[1], "1") ||
		!strings.Contains(lines[1], "Alpha") {
		t.Fatalf("press edge should outrank quality and carry the counts: %q", lines[1])
	}
	if !strings.Contains(lines[2], "Hero") || !strings.Contains(lines[2], "3") {
		t.Fatalf("hat-trick beat should name the scorer and count: %q", lines[2])
	}
	for _, line := range lines {
		if strings.Contains(line, "report.") {
			t.Fatalf("raw catalog key leaked into prose: %q", line)
		}
	}

	late := storyFixture(102, 2, 1)
	late.Scorers = []worldgen.MatchEvent{
		{Minute: 10, PlayerID: 9, ClubID: 1},
		{Minute: 50, PlayerID: 21, ClubID: 2},
		{Minute: 88, PlayerID: 5, ClubID: 1},
	}
	lines = renderStory(t, s, late)
	if len(lines) != 2 || !strings.Contains(lines[1], "88") {
		t.Fatalf("narrow win decided at 88' should read as a late winner: %v", lines)
	}

	salvage := storyFixture(103, 2, 2)
	salvage.Scorers = []worldgen.MatchEvent{
		{Minute: 10, PlayerID: 9, ClubID: 1}, {Minute: 20, PlayerID: 5, ClubID: 1},
		{Minute: 60, PlayerID: 21, ClubID: 2}, {Minute: 80, PlayerID: 22, ClubID: 2},
	}
	lines = renderStory(t, s, salvage)
	if len(lines) != 2 || !strings.Contains(lines[1], "Beta") {
		t.Fatalf("a draw salvaged from two down should credit the fighters: %v", lines)
	}

	blank := storyFixture(104, 0, 0)
	lines = renderStory(t, s, blank)
	if len(lines) != 1 {
		t.Fatalf("a goalless match without diagnostics is frame-only: %v", lines)
	}

	// Variant rotation: fixture parity flips the voice.
	a := renderStory(t, s, storyFixture(201, 3, 0))
	b := renderStory(t, s, storyFixture(202, 3, 0))
	if a[0] == b[0] {
		t.Fatalf("adjacent fixtures should rotate the frame voice: %q", a[0])
	}

	// Every report key renders in Korean too (FR-35c pairs are covered by
	// the catalog parity test; this guards against param typos).
	ko := s.matchStoryLines(narrative.LocaleKO, map[int64]string{1: "알파", 2: "베타"},
		func(int64) string { return "김철수" }, r)
	for _, line := range ko {
		if strings.Contains(line, "{") || strings.Contains(line, "report.") {
			t.Fatalf("Korean report line left params or keys unrendered: %q", line)
		}
	}
}

// The wire carries the rendered story on match detail (and archived detail),
// localized by the request.
func TestMatchDetailServesStory(t *testing.T) {
	s, host := newTestServer(t)
	w := host.world
	f := w.Fixtures[0]
	w.Results = append(w.Results, worldgen.MatchResult{
		FixtureID: f.ID, Competition: f.Competition, DivisionTier: f.DivisionTier,
		HomeID: f.HomeID, AwayID: f.AwayID, HomeGoals: 3, AwayGoals: 0,
		Kickoff: f.Kickoff,
	})
	code, body := get(t, s, fmt.Sprintf("/v1/matches/%d?locale=ko", f.ID))
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var detail matchDetailDTO
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Story) == 0 {
		t.Fatalf("match detail should carry the report prose: %+v", detail)
	}
	for _, line := range detail.Story {
		if strings.Contains(line, "report.") || strings.Contains(line, "{") {
			t.Fatalf("story line not rendered: %q", line)
		}
	}
}
