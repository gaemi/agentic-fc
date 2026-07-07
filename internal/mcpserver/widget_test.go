package mcpserver

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gaemi/agentic-fc/internal/narrative"
)

// TestLeagueCardRendersFromEnvelope proves a read renders an action card purely
// from the (public) envelope, in the spectator's locale — English chrome, then
// Korean chrome from the same data (FR-35c).
func TestLeagueCardRendersFromEnvelope(t *testing.T) {
	g, _, _, man := newGateway(t)
	env := g.getLeague(firstManagerID(man), "", getLeagueIn{})
	if env["ok"] != true {
		t.Fatalf("get_league failed: %+v", env)
	}

	en := leagueCard(g, narrative.LocaleEN, getLeagueIn{}, env)
	for _, want := range []string{"Observed", "get_league", "Reviewed the league standings.", "Leaders"} {
		if !strings.Contains(en, want) {
			t.Fatalf("en league card missing %q:\n%s", want, en)
		}
	}

	ko := leagueCard(g, narrative.LocaleKO, getLeagueIn{}, env)
	for _, want := range []string{"관찰", "리그 순위를 확인했습니다.", "선두"} {
		if !strings.Contains(ko, want) {
			t.Fatalf("ko league card missing %q:\n%s", want, ko)
		}
	}
	if strings.Contains(ko, "Observed") {
		t.Fatalf("ko card leaked English chrome:\n%s", ko)
	}
}

// TestLeagueCardReflectsQueriedSection locks regression fix #2: a get_league
// call for a non-default section must not claim "standings" — the headline
// follows the section actually returned, and the requested sections surface as
// the intent line.
func TestLeagueCardReflectsQueriedSection(t *testing.T) {
	g, _, _, man := newGateway(t)
	in := getLeagueIn{Sections: []string{"fixtures"}}
	env := g.getLeague(firstManagerID(man), "", in)
	if env["ok"] != true {
		t.Fatalf("get_league failed: %+v", env)
	}
	card := leagueCard(g, narrative.LocaleEN, in, env)
	if !strings.Contains(card, "Checked the upcoming fixtures.") {
		t.Fatalf("fixtures query should read as fixtures:\n%s", card)
	}
	if strings.Contains(card, "Reviewed the league standings.") {
		t.Fatalf("card wrongly claims standings for a fixtures-only query:\n%s", card)
	}
	if !strings.Contains(card, "fixtures") {
		t.Fatalf("card should show the requested sections (intent):\n%s", card)
	}
}

// TestDispositionCardShowsChange proves a write renders "what changed and how":
// the re-targeted axis by localized name and its signed target.
func TestDispositionCardShowsChange(t *testing.T) {
	g, _, _, man := newGateway(t)
	in := updateDispositionIn{Targets: map[string]int{"D1": 5}}
	env := g.updateDisposition(firstManagerID(man), "", in)
	if env["ok"] != true {
		t.Fatalf("update_disposition failed: %+v", env)
	}

	en := dispositionCard(g, narrative.LocaleEN, in, env)
	for _, want := range []string{"Decided", "update_disposition", "Risk Appetite", "+5", "Mindset v"} {
		if !strings.Contains(en, want) {
			t.Fatalf("en disposition card missing %q:\n%s", want, en)
		}
	}
	ko := dispositionCard(g, narrative.LocaleKO, in, env)
	for _, want := range []string{"결정", "리스크 성향", "+5"} {
		if !strings.Contains(ko, want) {
			t.Fatalf("ko disposition card missing %q:\n%s", want, ko)
		}
	}
}

// TestAttachWidgetContentMode locks the opt-in MCP-UI seam: the structured JSON
// stays the first (model-facing) content block so the AI is unaffected, and the
// card rides as a second EmbeddedResource (MCP-UI, text/html) that a UI host renders.
func TestAttachWidgetContentMode(t *testing.T) {
	g, _, _, _ := newGateway(t)
	g.WidgetMode = widgetContentBlock
	env := map[string]any{"ok": true, "data": map[string]any{"x": 1}}
	res := &mcp.CallToolResult{}
	g.attachWidget(res, env, "<div>card</div>")

	if len(res.Content) != 2 {
		t.Fatalf("want 2 content blocks (json + widget), got %d", len(res.Content))
	}
	txt, ok := res.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(txt.Text, `"ok":true`) {
		t.Fatalf("first block must be the envelope JSON, got %#v", res.Content[0])
	}
	er, ok := res.Content[1].(*mcp.EmbeddedResource)
	if !ok || er.Resource == nil {
		t.Fatalf("second block must be an EmbeddedResource, got %#v", res.Content[1])
	}
	if er.Resource.MIMEType != widgetMIME || er.Resource.Text != "<div>card</div>" || er.Resource.URI != widgetURI {
		t.Fatalf("widget resource wrong: %+v", er.Resource)
	}
	if res.Meta != nil {
		t.Fatalf("content mode must not use _meta, got %v", res.Meta)
	}
}

// TestAttachWidgetAppsMode locks the official MCP Apps default: result content
// stays untouched because tools advertise _meta.ui.resourceUri and the host
// fetches the UI resource separately, while result _meta carries the already
// rendered card for the app's ui/notifications/tool-result handler.
func TestAttachWidgetAppsMode(t *testing.T) {
	g, _, _, _ := newGateway(t)
	res := &mcp.CallToolResult{}
	g.attachWidget(res, map[string]any{"ok": true}, "<div>card</div>")

	if res.Content != nil {
		t.Fatalf("apps mode must leave Content nil, got %#v", res.Content)
	}
	w, ok := res.Meta[widgetMetaKey].(map[string]any)
	if !ok || w["mimeType"] != widgetMIME || w["html"] != "<div>card</div>" {
		t.Fatalf("apps mode widget meta wrong: %v", res.Meta)
	}
}

func TestWidgetAppUsesMCPAppsBridge(t *testing.T) {
	g, _, _, _ := newGateway(t)
	html := g.widgetAppHTML(narrative.LocaleEN)
	for _, want := range []string{"ui/initialize", "ui/notifications/initialized", "ui/notifications/tool-result", widgetMetaKey} {
		if !strings.Contains(html, want) {
			t.Fatalf("MCP Apps resource missing %q:\n%s", want, html)
		}
	}
}

// TestAttachWidgetMetaMode locks the generic-client-safe fallback: the
// card rides in _meta, entirely out of the model-facing Content stream (nil).
func TestAttachWidgetMetaMode(t *testing.T) {
	g, _, _, _ := newGateway(t)
	g.WidgetMode = widgetMeta
	res := &mcp.CallToolResult{}
	g.attachWidget(res, map[string]any{"ok": true}, "<div>card</div>")

	if res.Content != nil {
		t.Fatalf("meta mode must leave Content nil (AI-clean), got %#v", res.Content)
	}
	w, ok := res.Meta[widgetMetaKey].(map[string]any)
	if !ok {
		t.Fatalf("meta mode must set _meta[%q], got %v", widgetMetaKey, res.Meta)
	}
	if w["mimeType"] != widgetMIME || w["html"] != "<div>card</div>" {
		t.Fatalf("meta widget payload wrong: %v", w)
	}
}

// TestWidgetEscapesDynamicContent guards against markup injection: world text
// (a club name, an axis value) must be HTML-escaped so it can never break out of
// the card's structure.
func TestWidgetEscapesDynamicContent(t *testing.T) {
	html := renderCard(widgetCard{
		kind:     "read",
		badge:    "Observed",
		tool:     "get_league",
		headline: "x",
		rows:     []widgetRow{{label: "Club", value: `<script>alert(1)</script>`}},
	})
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Fatalf("dynamic value not escaped — injection risk:\n%s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Fatalf("expected escaped value in card:\n%s", html)
	}
}

// TestWidgetLocaleExplicitWins confirms an explicit Gateway.Locale overrides the
// system language (the test-injectable path; empty falls back to FromEnv).
func TestWidgetLocaleExplicitWins(t *testing.T) {
	g, _, _, _ := newGateway(t)
	g.Locale = narrative.LocaleKO
	if got := g.widgetLocale(); got != narrative.LocaleKO {
		t.Fatalf("explicit locale not honoured: got %q", got)
	}
}
