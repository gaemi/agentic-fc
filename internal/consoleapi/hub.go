package consoleapi

import (
	"encoding/json"
	"sync"

	"github.com/gaemi/agentic-fc/internal/engine"
	"github.com/gaemi/agentic-fc/internal/narrative"
)

// Hub fans engine feed events out to SSE subscribers, rendering per
// subscriber locale (server-side rendering, docs/07 §6). It implements
// engine.Sink; Publish runs on the engine goroutine, so delivery is
// non-blocking — a slow client loses lines, never stalls the world.
type Hub struct {
	Catalogs narrative.Catalogs

	mu   sync.Mutex
	subs map[chan []byte]narrative.Locale
}

func NewHub(c narrative.Catalogs) *Hub {
	return &Hub{Catalogs: c, subs: map[chan []byte]narrative.Locale{}}
}

// Subscribe registers a listener; the returned cancel must be called.
func (h *Hub) Subscribe(loc narrative.Locale) (<-chan []byte, func()) {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	h.subs[ch] = loc
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
		// Drain so a concurrent Publish never blocks on a dead channel.
		for {
			select {
			case <-ch:
			default:
				return
			}
		}
	}
}

// OnFeedEvent implements engine.Sink.
func (h *Hub) OnFeedEvent(ev engine.FeedEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch, loc := range h.subs {
		line := renderFeed(h.Catalogs, loc, ev)
		b, err := json.Marshal(line)
		if err != nil {
			continue
		}
		select {
		case ch <- b:
		default: // slow client: drop the line, never stall the engine
		}
	}
}

// System broadcasts a world lifecycle event (feed.world.started / .paused /
// .resumed — docs/05 A11) as a fully rendered Line per subscriber locale,
// exactly like any other feed line (FR-35c).
func (h *Hub) System(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch, loc := range h.subs {
		line := narrative.Line{
			Message: narrative.Message{
				Key:  key,
				Text: h.Catalogs.Render(loc, key, nil),
			},
			Cadence: narrative.Cadence{DisplayMillis: 4000, Density: "DRAMATIC"},
		}
		b, err := json.Marshal(line)
		if err != nil {
			continue
		}
		select {
		case ch <- b:
		default:
		}
	}
}
