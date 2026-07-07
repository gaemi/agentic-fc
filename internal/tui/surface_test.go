package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestAppFrameFullScreenTitleOverlay(t *testing.T) {
	frame := appFrame{
		Width:  40,
		Height: 10,
		Title:  "Agentic FC",
		Header: "World · Clock · [Tempo]",
		Tabs:   "1 Table  2 Fixtures",
		Body:   "body",
		Footer: "q quit",
	}.Render()

	lines := strings.Split(frame, "\n")
	if len(lines) != 10 {
		t.Fatalf("lines = %d, want 10:\n%s", len(lines), frame)
	}
	if !strings.Contains(lines[0], " Agentic FC ") {
		t.Fatalf("title overlay missing from top border: %q", lines[0])
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got != 40 {
			t.Fatalf("line %d width = %d, want 40: %q", i, got, line)
		}
	}
}

func TestOverlayZOrder(t *testing.T) {
	lines := []string{"0123456789"}
	got := applyOverlays(lines, []Overlay{
		textOverlay(2, 0, 2, "AA"),
		textOverlay(3, 0, 1, "b"),
	})
	if got[0] != "01AA456789" {
		t.Fatalf("overlay result = %q", got[0])
	}
}
