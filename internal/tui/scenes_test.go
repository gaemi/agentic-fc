package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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
