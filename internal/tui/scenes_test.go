package tui

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/gaemi/agentic-fc/internal/narrative"
)

func sortedSceneKinds() []string {
	kinds := make([]string, 0, len(matchScenes))
	for kind := range matchScenes {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

// Every scene frame must occupy the exact shared canvas so modal art can
// never render ragged, regardless of how a scene was authored.
func TestSceneFramesShareCanvasDimensions(t *testing.T) {
	if len(matchScenes) == 0 {
		t.Fatal("no match scenes registered")
	}
	for _, kind := range sortedSceneKinds() {
		scene := matchScenes[kind]
		if scene.kind != kind {
			t.Fatalf("scene registered under %q reports kind %q", kind, scene.kind)
		}
		if scene.title == "" {
			t.Fatalf("scene %q has no title", kind)
		}
		if len(scene.frames) < 2 {
			t.Fatalf("scene %q has %d frames, want an animation of at least 2", kind, len(scene.frames))
		}
		for f, frame := range scene.frames {
			if len(frame) != sceneCanvasHeight {
				t.Fatalf("scene %q frame %d has %d rows, want %d", kind, f, len(frame), sceneCanvasHeight)
			}
			for y, line := range frame {
				if got := lipgloss.Width(line); got != sceneCanvasWidth {
					t.Fatalf("scene %q frame %d row %d width = %d, want %d: %q", kind, f, y, got, sceneCanvasWidth, line)
				}
				for _, r := range line {
					if lipgloss.Width(string(r)) != 1 {
						t.Fatalf("scene %q frame %d row %d contains non-single-width rune %q", kind, f, y, r)
					}
				}
			}
		}
	}
}

func TestSceneFramesAreDistinctWithinScene(t *testing.T) {
	for _, kind := range sortedSceneKinds() {
		scene := matchScenes[kind]
		seen := map[string]int{}
		for f, frame := range scene.frames {
			joined := strings.Join(frame, "\n")
			if prev, dup := seen[joined]; dup {
				t.Fatalf("scene %q frame %d duplicates frame %d", kind, f, prev)
			}
			seen[joined] = f
		}
	}
}

func TestSceneCanvasClipsAndRejectsWideRunes(t *testing.T) {
	c := newSceneCanvas()
	c.put(-1, 0, 'x')
	c.put(sceneCanvasWidth, 0, 'x')
	c.put(0, -1, 'x')
	c.put(0, sceneCanvasHeight, 'x')
	c.stamp(sceneCanvasWidth-2, 0, "abcdef")
	c.label(-3, 2, "clip")
	c.put(5, 5, '골')
	for y, line := range c.lines() {
		if got := lipgloss.Width(line); got != sceneCanvasWidth {
			t.Fatalf("row %d width = %d, want %d: %q", y, got, sceneCanvasWidth, line)
		}
	}
	if got := c.lines()[5][5]; got != ' ' {
		t.Fatalf("double-width rune written to canvas: %q", got)
	}
	if !strings.HasPrefix(c.lines()[0][sceneCanvasWidth-2:], "ab") {
		t.Fatalf("clipped stamp lost in-bounds cells: %q", c.lines()[0])
	}
	if !strings.HasPrefix(c.lines()[2], "p") {
		t.Fatalf("clipped label lost in-bounds cells: %q", c.lines()[2])
	}
}

func TestSceneByKindFallsBackToBuildUp(t *testing.T) {
	if got := sceneByKind("no-such-scene").kind; got != "build" {
		t.Fatalf("unknown scene kind fell back to %q, want build", got)
	}
}

func TestComposeSceneRequiresFrames(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("composeScene with no frames did not panic")
		}
	}()
	composeScene("empty", "EMPTY")
}

// Every goal commentary line in the catalog must land on its glory scene: the
// generic templates on the goal scene itself, the patterned templates on
// their specific action scene. This ties the narrative catalog and the scene
// classifier together so new goal prose cannot quietly fall back to the
// build-up frame or lose its action animation.
func TestGoalCommentaryNeverFallsBackToBuildUp(t *testing.T) {
	params := map[string]any{
		"player": "Kim Min-jae", "club": "Alpha", "home_goals": 2, "away_goals": 1,
	}
	patternKinds := map[string]string{
		"cross": "cross", "cutback": "cutback", "through": "through",
		"long": "longshot", "setpiece": "setpiece", "scramble": "scramble",
		"counter": "counter",
		// Score-context calls have no action shape of their own; they play
		// the goal celebration.
		"opener": "goal", "equalizer": "goal", "late": "goal", "late_level": "goal",
		"hattrick": "goal", "hattrick_late": "goal",
		"comeback_level": "goal", "comeback_ahead": "goal",
		"response": "goal", "rout": "goal",
	}
	generic := regexp.MustCompile(`^comment\.goal\.\d+$`)
	patterned := regexp.MustCompile(`^comment\.goal\.([a-z_]+)\.\d+$`)
	for _, loc := range narrative.Supported {
		checked := 0
		for key := range narrative.Default[loc] {
			var want []string
			if generic.MatchString(key) {
				want = []string{"goal"}
			} else if m := patterned.FindStringSubmatch(key); m != nil {
				pattern, ok := patternKinds[m[1]]
				if !ok {
					t.Fatalf("goal pattern %q has no expected scene kind", m[1])
				}
				want = []string{pattern}
			} else {
				continue
			}
			checked++
			line := narrative.Default.Render(loc, key, params)
			got := matchSceneFromLine(line, nil).kind
			ok := false
			for _, w := range want {
				if got == w {
					ok = true
				}
			}
			if !ok {
				t.Errorf("%s %s: scene %q not in %v for line %q", loc, key, got, want, line)
			}
		}
		if checked == 0 {
			t.Fatalf("no goal commentary keys found for locale %s", loc)
		}
	}
}

// Chance and save templates must land on an action scene (their own pattern,
// the generic chance, or the keeper's save) — never the quiet build-up frame.
// Saves specifically must show the keeper.
func TestChanceAndSaveCommentaryLandOnActionScenes(t *testing.T) {
	params := map[string]any{
		"player": "Kim Min-jae", "club": "Alpha", "home_goals": 2, "away_goals": 1,
	}
	patternKinds := map[string]string{
		"cross": "cross", "cutback": "cutback", "through": "through",
		"long": "longshot", "setpiece": "setpiece", "scramble": "scramble",
		"counter": "counter",
	}
	chanceKey := regexp.MustCompile(`^comment\.chance\.([a-z]+)\.\d+$`)
	saveKey := regexp.MustCompile(`^comment\.save\.([a-z]+)\.\d+$`)
	for _, loc := range narrative.Supported {
		checked := 0
		for key := range narrative.Default[loc] {
			line := narrative.Default.Render(loc, key, params)
			got := matchSceneFromLine(line, nil).kind
			if m := saveKey.FindStringSubmatch(key); m != nil {
				checked++
				if got != "save" {
					t.Errorf("%s %s: scene %q, want save for line %q", loc, key, got, line)
				}
			} else if m := chanceKey.FindStringSubmatch(key); m != nil {
				checked++
				want := patternKinds[m[1]]
				if got != want && got != "chance" && got != "save" {
					t.Errorf("%s %s: scene %q, want %q/chance/save for line %q", loc, key, got, want, line)
				}
			}
		}
		if checked == 0 {
			t.Fatalf("no chance/save commentary keys found for locale %s", loc)
		}
	}
}

// The goal banners hang off goalProse, so every goal template must trip it
// and the interval/final/shootout bookkeeping (which also quotes the score)
// must not, or halftime beats would flash GOAL.
func TestGoalProseCoversGoalTemplatesOnly(t *testing.T) {
	params := map[string]any{
		"player": "Kim Min-jae", "club": "Alpha", "home": "Alpha", "away": "Beta",
		"home_goals": 2, "away_goals": 1, "home_pens": 4, "away_pens": 3,
		"winner": "Alpha",
	}
	goalKey := regexp.MustCompile(`^comment\.goal\.`)
	notGoalKey := regexp.MustCompile(`^comment\.(halftime|fulltime|shootout)`)
	for _, loc := range narrative.Supported {
		goals, others := 0, 0
		for key := range narrative.Default[loc] {
			line := narrative.Default.Render(loc, key, params)
			switch {
			case goalKey.MatchString(key):
				goals++
				if !goalProse(line) {
					t.Errorf("%s %s: goal template not detected as goal prose: %q", loc, key, line)
				}
			case notGoalKey.MatchString(key):
				others++
				if goalProse(line) {
					t.Errorf("%s %s: interval/final line wrongly detected as goal prose: %q", loc, key, line)
				}
			}
		}
		if goals == 0 || others == 0 {
			t.Fatalf("locale %s: goal templates %d, interval templates %d — expected both", loc, goals, others)
		}
	}
}

// DUMP_SCENES=1 go test ./internal/tui -run TestDumpScenes -v
// prints every frame of every scene for visual review.
func TestDumpScenes(t *testing.T) {
	if os.Getenv("DUMP_SCENES") == "" {
		t.Skip("set DUMP_SCENES=1 to print scene art")
	}
	for _, kind := range sortedSceneKinds() {
		scene := matchScenes[kind]
		for f, frame := range scene.frames {
			fmt.Printf("=== %s (%s) frame %d/%d ===\n", kind, scene.title, f+1, len(scene.frames))
			fmt.Println("+" + strings.Repeat("-", sceneCanvasWidth) + "+")
			for _, line := range frame {
				fmt.Println("|" + line + "|")
			}
			fmt.Println("+" + strings.Repeat("-", sceneCanvasWidth) + "+")
		}
	}
}

// Booking and injury prose must land on the referee's-book and stoppage
// scenes in both locales — a variant line that misses its signal words would
// silently demote a red card to the build-up frame.
func TestCardAndInjuryCommentaryPlayTheirScenes(t *testing.T) {
	params := map[string]any{"player": "Kim Min-jae", "club": "Alpha"}
	families := []struct {
		re   *regexp.Regexp
		want string
	}{
		{regexp.MustCompile(`^comment\.card\.`), "card"},
		{regexp.MustCompile(`^comment\.injury`), "injury"},
	}
	for _, loc := range narrative.Supported {
		for _, fam := range families {
			checked := 0
			for key := range narrative.Default[loc] {
				if !fam.re.MatchString(key) {
					continue
				}
				checked++
				line := narrative.Default.Render(loc, key, params)
				if got := matchSceneFromLine(line, nil).kind; got != fam.want {
					t.Errorf("%s %s: scene %q, want %q for line %q", loc, key, got, fam.want, line)
				}
			}
			if checked == 0 {
				t.Fatalf("no %s keys found for locale %s", fam.re, loc)
			}
		}
	}
}

// Ceremony prose (whistles and shootouts) must play its ceremony scene, not
// the quiet build-up frame, in both locales.
func TestCeremonyCommentaryPlaysCeremonyScenes(t *testing.T) {
	params := map[string]any{
		"player": "Kim Min-jae", "club": "Alpha", "home": "Alpha", "away": "Beta",
		"home_goals": 2, "away_goals": 1, "home_pens": 4, "away_pens": 3,
		"winner": "Alpha", "round": 3, "competition": "LEAGUE",
	}
	families := []struct {
		re   *regexp.Regexp
		want string
	}{
		{regexp.MustCompile(`^comment\.kickoff`), "kickoff"},
		{regexp.MustCompile(`^comment\.halftime`), "interval"},
		{regexp.MustCompile(`^comment\.fulltime`), "fulltime"},
		{regexp.MustCompile(`^comment\.shootout`), "shootout"},
	}
	for _, loc := range narrative.Supported {
		for _, fam := range families {
			checked := 0
			for key := range narrative.Default[loc] {
				if !fam.re.MatchString(key) {
					continue
				}
				checked++
				line := narrative.Default.Render(loc, key, params)
				if got := matchSceneFromLine(line, nil).kind; got != fam.want {
					t.Errorf("%s %s: scene %q, want %q for line %q", loc, key, got, fam.want, line)
				}
			}
			if checked == 0 {
				t.Fatalf("no %s keys found for locale %s", fam.re, loc)
			}
		}
	}
	if got := matchSceneFromLine("", &LiveMarker{Kind: "SHOOTOUT"}).kind; got != "shootout" {
		t.Fatalf("shootout marker scene = %q, want shootout", got)
	}
}

// Mirrored frames must share the canvas contract with their originals, keep
// their labels readable, and actually flip the action.
func TestMirroredScenesFlipArtButNotLabels(t *testing.T) {
	for _, kind := range sortedSceneKinds() {
		scene := matchScenes[kind]
		if len(scene.mirrored) != len(scene.frames) {
			t.Fatalf("scene %q mirrored frames = %d, want %d", kind, len(scene.mirrored), len(scene.frames))
		}
		for f, frame := range scene.mirrored {
			if len(frame) != sceneCanvasHeight {
				t.Fatalf("scene %q mirrored frame %d rows = %d", kind, f, len(frame))
			}
			for y, line := range frame {
				if got := lipgloss.Width(line); got != sceneCanvasWidth {
					t.Fatalf("scene %q mirrored frame %d row %d width = %d: %q", kind, f, y, got, line)
				}
			}
		}
	}

	goal := matchScenes["goal"]
	last := goal.mirrored[len(goal.mirrored)-1]
	if !strings.Contains(strings.Join(last, "\n"), "G O A L !") {
		t.Fatalf("mirrored goal frame lost its readable banner:\n%s", strings.Join(last, "\n"))
	}
	// Directional strokes are art, not banners: the flight trail must point
	// leftward once mirrored.
	flight := strings.Join(goal.mirrored[1], "\n")
	if !strings.Contains(flight, "*---") || strings.Contains(flight, "---*") {
		t.Fatalf("mirrored flight trail still points rightward:\n%s", flight)
	}
	// The goal furniture must sit on the left in the mirrored frame and on
	// the right in the original.
	barCol := func(frame []string) int {
		for _, line := range frame {
			if idx := strings.Index(line, "_________"); idx >= 0 {
				return idx
			}
		}
		return -1
	}
	if col := barCol(last); col < 0 || col > sceneCanvasWidth/2 {
		t.Fatalf("mirrored goal frame keeps the goal on the right (bar col %d):\n%s", col, strings.Join(last, "\n"))
	}
	orig := goal.frames[len(goal.frames)-1]
	if col := barCol(orig); col < sceneCanvasWidth/2 {
		t.Fatalf("original goal frame moved off the right (bar col %d):\n%s", col, strings.Join(orig, "\n"))
	}
}

func TestLiveModalMirrorsAwayAttacks(t *testing.T) {
	m := liveModel(140, 36)
	m.Matches[0].Minute = 30
	m.Matches[0].Commentary = []string{"Goal! Rao finds the net for Beta — it's 0–1."}
	m.Matches[0].Markers = []LiveMarker{{Minute: 30, Kind: "GOAL", Side: "AWAY"}}
	v := m.View()
	if !strings.Contains(v, "|: :") {
		t.Fatalf("away goal beat should mirror the goal mouth to the left:\n%s", v)
	}

	m.Matches[0].Markers = []LiveMarker{{Minute: 30, Kind: "GOAL", Side: "HOME"}}
	m.Matches[0].Commentary = []string{"Goal! Rao finds the net for Alpha — it's 1–0."}
	v = m.View()
	if strings.Contains(v, "|: :") {
		t.Fatalf("home goal beat should keep the goal on the right:\n%s", v)
	}
	if !strings.Contains(v, ": :|") {
		t.Fatalf("home goal beat lost the goal mouth:\n%s", v)
	}

	// The mirror decision follows the beat, not the clock: an away action
	// that stays the latest line through a quiet stretch keeps its direction.
	m.Matches[0].Minute = 80
	m.Matches[0].Markers = []LiveMarker{{Minute: 30, Kind: "GOAL", Side: "AWAY"}}
	m.Matches[0].Commentary = []string{"Goal! Rao finds the net for Beta — it's 0–1."}
	if v := m.View(); !strings.Contains(v, "|: :") {
		t.Fatalf("away scene flipped home-facing after a quiet stretch:\n%s", v)
	}

	// Ceremony and neutral scenes never mirror.
	if mirrorableScenes["kickoff"] || mirrorableScenes["build"] || mirrorableScenes["sub"] {
		t.Fatal("ceremony/neutral scenes must not be mirrorable")
	}
}

// Quiet commentary must stay on the build-up frame: crowd flavor may not
// hijack an action scene.
func TestQuietCommentaryStaysOnBuildUp(t *testing.T) {
	quiet := regexp.MustCompile(`^comment\.quiet\.\d+$`)
	for _, loc := range narrative.Supported {
		checked := 0
		for key := range narrative.Default[loc] {
			if !quiet.MatchString(key) {
				continue
			}
			checked++
			line := narrative.Default.Render(loc, key, nil)
			if got := matchSceneFromLine(line, nil).kind; got != "build" {
				t.Errorf("%s %s: scene %q, want build for line %q", loc, key, got, line)
			}
		}
		if checked < 15 {
			t.Fatalf("locale %s: only %d quiet keys found", loc, checked)
		}
	}
}
