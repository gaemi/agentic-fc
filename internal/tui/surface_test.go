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

func TestOverlayKeepsWidthWhenReplacingHalfWideCell(t *testing.T) {
	got := writeCells("abc중de", 3, "X")
	if got != "abcX de" {
		t.Fatalf("overlay wide boundary result = %q", got)
	}
	if w := lipgloss.Width(got); w != 7 {
		t.Fatalf("overlay width = %d, want 7: %q", w, got)
	}
}

func TestTextTableShrinksFlexColumnsBeforeClippingBorders(t *testing.T) {
	table := renderTextTable(52, []tableColumn{
		{Header: "", Width: 2, Align: alignLeft},
		{Header: "포지션", Width: 6, Align: alignLeft},
		{Header: "이름", MinWidth: 12, Flex: true, Align: alignLeft},
		{Header: "나이", Width: 4, Align: alignRight},
		{Header: "능력치", MinWidth: 18, Flex: true, Align: alignLeft},
		{Header: "계약", Width: 8, Align: alignRight},
	}, [][]string{{">", "GK", "Suzuki Hayato", "27", "민첩성 14 · 반사신경 13", "7"}})

	for i, line := range strings.Split(table, "\n") {
		if got := lipgloss.Width(line); got != 52 {
			t.Fatalf("table line %d width = %d, want 52: %q\n%s", i, got, line, table)
		}
		if strings.HasSuffix(line, "…") {
			t.Fatalf("table row ended with clipping ellipsis instead of a border:\n%s", table)
		}
		if strings.ContainsAny(line, "…") && strings.ContainsAny(line, "┌┬┐├┼┤└┴┘") {
			t.Fatalf("table border was clipped instead of shrinking flex columns:\n%s", table)
		}
	}
}
