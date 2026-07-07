// agenticfc-console is the human-facing TUI client (docs/07-console-design.md):
// Viewer Mode over the Console API. It is a spectator console: media/news,
// league tables, club dossiers, fixtures, and live match views.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gaemi/agentic-fc/internal/narrative"
	"github.com/gaemi/agentic-fc/internal/tui"
)

func main() {
	server := flag.String("server", "http://127.0.0.1:7420", "Console API base URL")
	adminToken := flag.String("admin-token", "", "Admin Token (reserved for operator screens)")
	localeFlag := flag.String("locale", "", "display locale override (en|ko); default: system language")
	flag.Parse()
	_ = adminToken // reserved for Admin Mode

	// Locale follows the system language with English fallback (FR-35c);
	// the flag exists for testing and explicit override. The server renders
	// all text; the Console stays catalog-free (docs/07 §6).
	loc := narrative.FromEnv(os.Getenv)
	if *localeFlag != "" {
		loc = narrative.ResolveTag(*localeFlag)
	}

	client := tui.NewClient(*server, loc)
	p := tea.NewProgram(tui.NewModel(client), tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "console:", err)
		os.Exit(1)
	}
}
