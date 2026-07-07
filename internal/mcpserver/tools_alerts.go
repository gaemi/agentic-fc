package mcpserver

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gaemi/agentic-fc/internal/focus"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

type configureAlertsIn struct {
	Enabled *bool                 `json:"enabled,omitempty"`
	Watches []configureAlertWatch `json:"watches,omitempty"`
}

type configureAlertWatch struct {
	Kind        string   `json:"kind"`
	Categories  []string `json:"categories,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	When        string   `json:"when,omitempty"`
	LeadMinutes int      `json:"lead_minutes,omitempty"`
	Threshold   int      `json:"threshold,omitempty"`
	Edge        string   `json:"edge,omitempty"`
}

type getAlertsIn struct {
	Limit int `json:"limit,omitempty"`
}

type ackAlertsIn struct {
	Through int64 `json:"through"`
}

var validAlertNewsCategories = map[string]bool{
	"transfer": true,
	"match":    true,
	"injury":   true,
	"board":    true,
	"media":    true,
	"decision": true,
	"career":   true,
	"youth":    true,
	"contract": true,
}

func (g *Gateway) registerAlertTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        string(focus.ConfigureAlerts),
		Description: "0 FP. Configure manager-scoped Agent Alert watches for news, match, calendar, and Focus wake signals.",
	}, handle(g, g.configureAlerts))
	mcp.AddTool(s, &mcp.Tool{
		Name:        string(focus.GetAlerts),
		Description: "1 FP. Read pending Agent Alert summaries; subscribe to the alert resource for push wake signals.",
	}, handle(g, g.getAlerts))
	mcp.AddTool(s, &mcp.Tool{
		Name:        string(focus.AckAlerts),
		Description: "0 FP. Acknowledge pending Agent Alerts through an inclusive numeric cursor.",
	}, handle(g, g.ackAlerts))
}

func (g *Gateway) configureAlerts(mid int64, sid string, in configureAlertsIn) map[string]any {
	return g.run(mid, sid, focus.ConfigureAlerts, in, flatCost(focus.ConfigureAlerts),
		func(cc *callCtx) (any, *apiError) {
			st := cc.manager.AlertsState()
			if in.Enabled != nil {
				st.Enabled = *in.Enabled
			}
			if in.Watches != nil {
				if len(in.Watches) > worldgen.AlertWatchCap {
					return nil, errFor(ErrCapExceeded, "err.cap_exceeded",
						map[string]any{"max": worldgen.AlertWatchCap}, nil)
				}
				watches := make([]worldgen.AlertWatch, 0, len(in.Watches))
				for _, raw := range in.Watches {
					w, aerr := g.validateAlertWatch(raw)
					if aerr != nil {
						return nil, aerr
					}
					st.NextWatchID++
					w.ID = st.NextWatchID
					g.initializeFocusWatch(cc.manager, &w)
					watches = append(watches, w)
				}
				st.Watches = watches
				st.Version++
			}
			return g.alertsData(cc.manager, st, 50), nil
		})
}

func (g *Gateway) getAlerts(mid int64, sid string, in getAlertsIn) map[string]any {
	return g.run(mid, sid, focus.GetAlerts, in, flatCost(focus.GetAlerts),
		func(cc *callCtx) (any, *apiError) {
			limit := in.Limit
			if limit <= 0 {
				limit = 50
			}
			if limit > 100 {
				limit = 100
			}
			return g.alertsData(cc.manager, cc.manager.AlertsState(), limit), nil
		})
}

func (g *Gateway) ackAlerts(mid int64, sid string, in ackAlertsIn) map[string]any {
	return g.run(mid, sid, focus.AckAlerts, in, flatCost(focus.AckAlerts),
		func(cc *callCtx) (any, *apiError) {
			st := cc.manager.AlertsState()
			if in.Through > st.NextID {
				return nil, errFor(ErrValidation, "err.validation",
					map[string]any{"detail": "through is greater than highest issued alert id"},
					map[string]any{"highest_issued_id": st.NextID})
			}
			clearedFocusWatches := map[int64]bool{}
			for i := range st.Watches {
				w := &st.Watches[i]
				if w.Kind == worldgen.AlertKindFocus && w.FiredAlertID != 0 && in.Through >= w.FiredAlertID {
					clearedFocusWatches[w.ID] = true
				}
			}
			applied := st.Ack(in.Through)
			for i := range st.Watches {
				if clearedFocusWatches[st.Watches[i].ID] {
					g.initializeFocusWatch(cc.manager, &st.Watches[i])
				}
			}
			return map[string]any{
				"acked_through":     applied,
				"highest_issued_id": st.NextID,
				"pending_count":     len(st.Pending),
			}, nil
		})
}

func (g *Gateway) validateAlertWatch(raw configureAlertWatch) (worldgen.AlertWatch, *apiError) {
	w := worldgen.AlertWatch{
		Kind:        strings.ToUpper(raw.Kind),
		Categories:  append([]string{}, raw.Categories...),
		Scope:       raw.Scope,
		When:        strings.ToUpper(raw.When),
		LeadMinutes: raw.LeadMinutes,
		Threshold:   raw.Threshold,
		Edge:        raw.Edge,
	}
	if w.Scope == "" {
		w.Scope = "own"
	}
	switch w.Scope {
	case "own", "league", "world":
	default:
		return w, validation("scope must be own|league|world")
	}
	switch w.Kind {
	case worldgen.AlertKindNews:
		for _, c := range w.Categories {
			if !validAlertNewsCategories[c] {
				return w, validation("NEWS.categories must contain known news categories")
			}
		}
		return w, nil
	case worldgen.AlertKindMatch:
		switch w.When {
		case "OWN_KICKOFF", "OWN_FULL_TIME":
			return w, nil
		default:
			return w, validation("MATCH.when must be OWN_KICKOFF|OWN_FULL_TIME")
		}
	case worldgen.AlertKindCalendar:
		switch w.When {
		case "DATE_CHANGED", "WINDOW_OPEN", "WINDOW_CLOSE", "SEASON_ENDED":
			return w, nil
		default:
			return w, validation("CALENDAR.when must be DATE_CHANGED|WINDOW_OPEN|WINDOW_CLOSE|SEASON_ENDED")
		}
	case worldgen.AlertKindFocus:
		if w.Threshold < 0 || w.Threshold > focus.Cap {
			return w, validation("FOCUS.threshold must be between 0 and the Focus cap")
		}
		if w.Edge == "" {
			w.Edge = worldgen.AlertEdgeRising
		}
		switch w.Edge {
		case worldgen.AlertEdgeRising, worldgen.AlertEdgeFalling:
			return w, nil
		default:
			return w, validation(`FOCUS.edge must be "rising" or "falling"`)
		}
	default:
		return w, validation("kind must be NEWS|MATCH|CALENDAR|FOCUS")
	}
}

func validation(detail string) *apiError {
	return errFor(ErrValidation, "err.validation",
		map[string]any{"detail": detail}, map[string]any{"detail": detail})
}

func (g *Gateway) initializeFocusWatch(m *worldgen.Manager, w *worldgen.AlertWatch) {
	w.Armed = false
	w.ScheduledAt = 0
	if w.FiredAlertID != 0 {
		return
	}
	if w.Edge == worldgen.AlertEdgeFalling {
		w.Armed = m.FocusBalance >= w.Threshold+worldgen.AlertFocusHysteresisFP
		return
	}
	w.Armed = m.FocusBalance <= w.Threshold-worldgen.AlertFocusHysteresisFP
}

func (g *Gateway) alertsData(m *worldgen.Manager, st *worldgen.AlertState, limit int) map[string]any {
	pending := make([]map[string]any, 0, min(limit, len(st.Pending)))
	for _, a := range st.Pending {
		if len(pending) >= limit {
			break
		}
		pending = append(pending, map[string]any{
			"id":        a.ID,
			"game_time": gameTimeISO(a.GameTime),
			"kind":      a.Kind,
			"reason":    a.Reason,
			"refs":      a.Refs,
			"message":   a.Message,
		})
	}
	return map[string]any{
		"resource":          worldgen.AlertResourceURI(m.ID),
		"enabled":           st.Enabled,
		"watches":           publicAlertWatches(st.Watches),
		"pending":           pending,
		"highest_issued_id": st.NextID,
		"acked_through":     st.AckedThrough,
		"next_cursor":       strconv.FormatInt(st.NextID, 10),
		"subscribe_hint":    "Subscribe to " + worldgen.AlertResourceURI(m.ID) + " and call get_alerts when notifications/resources/updated arrives.",
	}
}

func publicAlertWatches(watches []worldgen.AlertWatch) []map[string]any {
	out := make([]map[string]any, 0, len(watches))
	for _, w := range watches {
		item := map[string]any{
			"id":   w.ID,
			"kind": w.Kind,
		}
		if len(w.Categories) > 0 {
			item["categories"] = append([]string{}, w.Categories...)
		}
		if w.Scope != "" {
			item["scope"] = w.Scope
		}
		if w.When != "" {
			item["when"] = w.When
		}
		if w.LeadMinutes != 0 {
			item["lead_minutes"] = w.LeadMinutes
		}
		if w.Kind == worldgen.AlertKindFocus {
			item["threshold"] = w.Threshold
			item["edge"] = w.Edge
		}
		out = append(out, item)
	}
	return out
}

func (g *Gateway) registerAlertResources(s *mcp.Server) {
	s.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "agent-alerts",
		Description: "Manager-scoped Agent Alert wake resource. Read returns static guidance; use get_alerts for state.",
		MIMEType:    "application/json",
		URITemplate: worldgen.AlertResourceTemplate,
	}, g.readAlertsResource)
}

func (g *Gateway) readAlertsResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	if err := g.authorizeAlertURI(ctx, req.Params.URI); err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]any{
		"resource": req.Params.URI,
		"hint":     "Call get_alerts for pending alert summaries; call ack_alerts after handling them.",
	})
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(body),
		}},
	}, nil
}

func (g *Gateway) subscribeAlertResource(ctx context.Context, req *mcp.SubscribeRequest) error {
	return g.authorizeAlertURI(ctx, req.Params.URI)
}

func (g *Gateway) unsubscribeAlertResource(ctx context.Context, req *mcp.UnsubscribeRequest) error {
	return g.authorizeAlertURI(ctx, req.Params.URI)
}

func (g *Gateway) authorizeAlertURI(ctx context.Context, uri string) error {
	id, aerr := managerIDFromCtx(ctx)
	if aerr != nil {
		return mcp.ResourceNotFoundError(uri)
	}
	want := worldgen.AlertResourceURI(id)
	if uri != want {
		return mcp.ResourceNotFoundError(uri)
	}
	return nil
}

func (g *Gateway) OnManagerAlert(managerID int64) {
	g.serverMu.RLock()
	s := g.mcpSrv
	g.serverMu.RUnlock()
	if s == nil {
		return
	}
	go s.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{
		URI: worldgen.AlertResourceURI(managerID),
	})
}
