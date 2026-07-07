package consoleapi

import (
	"fmt"

	"github.com/gaemi/agentic-fc/internal/engine"
	"github.com/gaemi/agentic-fc/internal/narrative"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// renderFeed turns a structured engine feed event into a rendered Line:
// term/attr sub-keys resolve first, then the event template (FR-35c).
func renderFeed(c narrative.Catalogs, loc narrative.Locale, ev engine.FeedEvent) narrative.Line {
	params := make(map[string]any, len(ev.Params))
	for k, v := range ev.Params {
		params[k] = v
	}
	if a, ok := params["attr_key"].(string); ok {
		params["attr"] = c.Render(loc, "attr."+a, nil)
		delete(params, "attr_key")
	}
	if w, ok := params["window_key"].(string); ok {
		params["window"] = c.Render(loc, "term.window."+w, nil)
		delete(params, "window_key")
	}
	if comp, ok := params["competition"].(string); ok {
		params["competition"] = c.Render(loc, "term.competition."+comp, nil)
	}
	if club, ok := params["club"].(string); ok && club == "" {
		params["club"] = c.Render(loc, "term.free_agent", nil)
	}

	// The wire message keeps the full resolved params (structured data for
	// clients that don't just print Text) plus the game time.
	params["game_time"] = int64(ev.GameTime)
	return narrative.Line{
		Message: narrative.Message{
			Key:    ev.Key,
			Params: params,
			Text:   c.Render(loc, ev.Key, params),
		},
		Cadence: cadenceFor(ev.Key),
	}
}

// cadenceFor assigns pacing hints per event family (FR-35a); values are
// initial and will be tuned with the narrative variety pass (NFR-8).
func cadenceFor(key string) narrative.Cadence {
	switch key {
	case engine.FeedKickoff:
		return narrative.Cadence{DisplayMillis: 2500, Density: "BUILDING"}
	case engine.FeedWindowOpened, engine.FeedWindowClosed, engine.FeedSeasonEnded:
		return narrative.Cadence{DisplayMillis: 4000, Density: "DRAMATIC"}
	default:
		return narrative.Cadence{DisplayMillis: 1500, Density: "ROUTINE"}
	}
}

// renderClock renders the header clock line for a locale (server-side —
// the Console stays catalog-free, docs/07 §6).
func renderClock(c narrative.Catalogs, loc narrative.Locale, t sim.GameTime) string {
	d := worldgen.DateOf(t)
	return c.Render(loc, "ui.clock", map[string]any{
		"season": d.Season,
		"month":  c.Render(loc, fmt.Sprintf("ui.month.%d", d.Month), nil),
		"day":    d.Day,
		"hh":     fmt.Sprintf("%02d", d.Hour),
		"mm":     fmt.Sprintf("%02d", d.Minute),
	})
}
