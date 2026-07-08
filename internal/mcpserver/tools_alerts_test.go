package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gaemi/agentic-fc/internal/engine"
	"github.com/gaemi/agentic-fc/internal/focus"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

func TestGuideTeachesAgentAlerts(t *testing.T) {
	g, _, _, manifest := newGateway(t)
	mid := firstManagerID(manifest)

	data := dataOf(t, g.getGuide(mid, "s1", emptyIn{}))
	alerts, ok := data["long_running_alerts"].([]string)
	if !ok || len(alerts) == 0 {
		t.Fatalf("guide missing long_running_alerts: %+v", data)
	}
	joined := strings.Join(alerts, "\n")
	for _, want := range []string{"configure_alerts", "get_alerts", "ack_alerts", "resources/subscribe"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("guide alert section missing %q:\n%s", want, joined)
		}
	}
}

func TestNewsAlertAndAck(t *testing.T) {
	g, host, _, manifest := newGateway(t)
	mid := firstManagerID(manifest)

	cfg := configureAlertsIn{Watches: []configureAlertWatch{{
		Kind:       worldgen.AlertKindNews,
		Categories: []string{"match"},
		Scope:      "own",
	}}}
	dataOf(t, g.configureAlerts(mid, "s1", cfg))

	var first sim.GameTime
	clubID := g.managers[mid].ClubID
	for _, f := range host.world.Fixtures {
		if f.HomeID == clubID || f.AwayID == clubID {
			first = f.Kickoff
			break
		}
	}
	if first == 0 {
		t.Fatal("fixture for manager club not found")
	}
	if _, err := host.eng.RunUntil(first + engine.MatchWindowMinutes); err != nil {
		t.Fatal(err)
	}

	alerts := dataOf(t, g.getAlerts(mid, "s1", getAlertsIn{}))
	pending := alerts["pending"].([]map[string]any)
	if len(pending) == 0 {
		t.Fatalf("expected match news alert, got %+v", alerts)
	}
	if pending[0]["kind"] != worldgen.AlertKindNews || pending[0]["reason"] != "match" {
		t.Fatalf("unexpected alert: %+v", pending[0])
	}
	highest := alerts["highest_issued_id"].(int64)
	if code := errCode(g.ackAlerts(mid, "s1", ackAlertsIn{Through: highest + 1})); code != "VALIDATION" {
		t.Fatalf("future ack code = %q, want VALIDATION", code)
	}
	dataOf(t, g.ackAlerts(mid, "s1", ackAlertsIn{Through: highest}))
	after := dataOf(t, g.getAlerts(mid, "s1", getAlertsIn{}))
	if got := len(after["pending"].([]map[string]any)); got != 0 {
		t.Fatalf("pending after ack = %d", got)
	}
}

func TestFocusAlertSchedulesDeterministically(t *testing.T) {
	g, host, _, manifest := newGateway(t)
	mid := firstManagerID(manifest)

	host.LockedWrite(func() {
		m := g.managers[mid]
		m.FocusBalance = 20
		m.FocusRegenMark = 0
	})
	dataOf(t, g.configureAlerts(mid, "s1", configureAlertsIn{Watches: []configureAlertWatch{{
		Kind:      worldgen.AlertKindFocus,
		Threshold: 25,
		Edge:      worldgen.AlertEdgeRising,
	}}}))

	// 5 FP at 1 FP / 30 game minutes.
	if _, err := host.eng.RunUntil(sim.GameTime(150)); err != nil {
		t.Fatal(err)
	}
	alerts := dataOf(t, g.getAlerts(mid, "s1", getAlertsIn{}))
	pending := alerts["pending"].([]map[string]any)
	if len(pending) != 1 {
		t.Fatalf("pending focus alerts = %d: %+v", len(pending), alerts)
	}
	if pending[0]["kind"] != worldgen.AlertKindFocus || pending[0]["reason"] != worldgen.AlertEdgeRising {
		t.Fatalf("unexpected focus alert: %+v", pending[0])
	}
	if bal := host.world.Managers[0].FocusBalance; bal < 24 || bal > focus.Cap {
		t.Fatalf("focus not synced through alert event: %d", bal)
	}
}

func TestAlertResourceIsManagerSpecific(t *testing.T) {
	_, _, _, manifest := newGateway(t)
	a := manifest.Managers[0].ManagerID
	b := manifest.Managers[1].ManagerID
	if worldgen.AlertResourceURI(a) == worldgen.AlertResourceURI(b) {
		t.Fatalf("alert resource URI is not manager-specific: %q", worldgen.AlertResourceURI(a))
	}
	if uri := worldgen.AlertResourceURI(a); !strings.HasPrefix(uri, "agenticfc://manager/") || !strings.HasSuffix(uri, "/alerts") {
		t.Fatalf("unexpected alert URI: %q", worldgen.AlertResourceURI(a))
	}
}

func TestAlertResourceAuthorization(t *testing.T) {
	g, _, _, manifest := newGateway(t)
	a := manifest.Managers[0]
	b := manifest.Managers[1]
	ctxA := bearerCtx(t, g, a.Token)
	uriA := worldgen.AlertResourceURI(a.ManagerID)
	uriB := worldgen.AlertResourceURI(b.ManagerID)

	if _, err := g.readAlertsResource(ctxA, readAlertReq(uriA)); err != nil {
		t.Fatalf("read own alert resource: %v", err)
	}
	if err := g.subscribeAlertResource(ctxA, subscribeAlertReq(uriA)); err != nil {
		t.Fatalf("subscribe own alert resource: %v", err)
	}
	assertResourceNotFound(t, g.subscribeAlertResource(ctxA, subscribeAlertReq(uriB)))
	assertResourceNotFound(t, g.unsubscribeAlertResource(ctxA, unsubscribeAlertReq(uriB)))

	_, err := g.readAlertsResource(ctxA, readAlertReq(uriB))
	assertResourceNotFound(t, err)
	_, err = g.readAlertsResource(context.Background(), readAlertReq(uriA))
	assertResourceNotFound(t, err)
}

func bearerCtx(t *testing.T, g *Gateway, token string) context.Context {
	t.Helper()
	var out context.Context
	handler := auth.RequireBearerToken(g.VerifyToken, nil)(http.HandlerFunc(
		func(_ http.ResponseWriter, r *http.Request) {
			out = r.Context()
		}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if out == nil {
		t.Fatal("bearer middleware did not produce authenticated context")
	}
	return out
}

func readAlertReq(uri string) *mcp.ReadResourceRequest {
	return &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: uri}}
}

func subscribeAlertReq(uri string) *mcp.SubscribeRequest {
	return &mcp.SubscribeRequest{Params: &mcp.SubscribeParams{URI: uri}}
}

func unsubscribeAlertReq(uri string) *mcp.UnsubscribeRequest {
	return &mcp.UnsubscribeRequest{Params: &mcp.UnsubscribeParams{URI: uri}}
}

func assertResourceNotFound(t *testing.T, err error) {
	t.Helper()
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != mcp.CodeResourceNotFound {
		t.Fatalf("error = %v, want resource-not-found", err)
	}
}
