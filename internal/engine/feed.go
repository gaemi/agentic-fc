package engine

import (
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// FeedEvent is one noteworthy world occurrence for spectator surfaces. It
// carries a narrative message key plus raw params — proper nouns are data,
// never translated; display layers render the key per locale (FR-35c).
// Emitting feed events never influences outcomes: sinks observe state the
// engine already committed.
type FeedEvent struct {
	GameTime sim.GameTime   `json:"game_time"`
	Key      string         `json:"key"`
	Params   map[string]any `json:"params,omitempty"`
}

// Sink receives feed events synchronously on the engine goroutine — keep
// implementations fast and non-blocking (fan-out belongs to the sink).
type Sink interface {
	OnFeedEvent(FeedEvent)
}

// Feed message keys (catalogs in internal/narrative, en+ko).
const (
	FeedDriftGrew     = "feed.drift.grew"
	FeedDriftDeclined = "feed.drift.declined"
	FeedWindowOpened  = "feed.window.opened"
	FeedWindowClosed  = "feed.window.closed"
	FeedSeasonEnded   = "feed.season.ended"
	FeedKickoff       = "feed.match.kickoff"
	FeedMatchResult   = "feed.match.result"
	FeedCupResult     = "feed.cup.result"
	FeedCupChampion   = "feed.cup.champion"
)

func (e *Engine) emit(t sim.GameTime, key string, params map[string]any) {
	if e.sink != nil {
		e.sink.OnFeedEvent(FeedEvent{GameTime: t, Key: key, Params: params})
	}
}

// clubName resolves a club for display; free agents read as club id 0.
func (e *Engine) clubName(id int64) string {
	if c, ok := e.clubs[id]; ok {
		return c.Name
	}
	return ""
}

// emitDrift reports an attribute change on the Console feed (the human
// god-view, which sees exact values). Drift is deliberately NOT filed as an
// agent news item: the MCP news ring must respect attribute masking, and
// broadcasting exact from→to values there would leak other-club visible
// attributes that get_squad/get_person mask into ranges (FR-22a). Agents
// track development through those masked reads and scouting instead.
func (e *Engine) emitDrift(t sim.GameTime, p *worldgen.Player, key string, a string, from, to int) {
	e.emit(t, key, map[string]any{
		"player":   p.Name,
		"club":     e.clubName(p.ClubID),
		"attr_key": a,
		"from":     from,
		"to":       to,
	})
}

// calendarKeyParams derives the message key + params for a window edge or
// season rollover; the window name comes from the calendar month the event
// lands in. Shared by the live feed and the news store.
func calendarKeyParams(t sim.GameTime, payload string) (string, map[string]any) {
	switch payload {
	case worldgen.PayloadWindowOpen, worldgen.PayloadWindowClose:
		window := "summer"
		if m := worldgen.DateOf(t).Month; m == 1 || m == 2 {
			window = "winter"
		}
		key := FeedWindowOpened
		if payload == worldgen.PayloadWindowClose {
			key = FeedWindowClosed
		}
		return key, map[string]any{"window_key": window}
	case worldgen.PayloadSeasonEnd:
		return FeedSeasonEnded, map[string]any{"season": worldgen.DateOf(t).Season - 1}
	}
	return "", nil
}

func (e *Engine) emitCalendar(t sim.GameTime, payload string) {
	if key, params := calendarKeyParams(t, payload); key != "" {
		e.emit(t, key, params)
	}
}

func (e *Engine) emitKickoff(ev *sim.Event) {
	f, ok := e.fixtures[ev.EntityID]
	if !ok {
		return
	}
	e.emit(ev.Due, FeedKickoff, map[string]any{
		"home":        e.clubName(f.HomeID),
		"away":        e.clubName(f.AwayID),
		"round":       f.Round,
		"competition": f.Competition,
	})
}
