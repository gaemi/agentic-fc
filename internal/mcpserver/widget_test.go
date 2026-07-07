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
	for _, want := range []string{"Observed", "get_league", "Reviewed the league standings.", "Leaders", "Top table"} {
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
	g.attachWidget(res, env, "<div>card</div>", nil)

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
	g.attachWidget(res, map[string]any{"ok": true}, "<div>card</div>", map[narrative.Locale]string{narrative.LocaleKO: "<div>카드</div>"})

	if len(res.Content) != 0 {
		t.Fatalf("apps mode must leave Content empty, got %#v", res.Content)
	}
	w, ok := res.Meta[widgetMetaKey].(map[string]any)
	if !ok || w["mimeType"] != widgetMIME || w["html"] != "<div>card</div>" {
		t.Fatalf("apps mode widget meta wrong: %v", res.Meta)
	}
	byLocale, ok := w["html_by_locale"].(map[string]string)
	if !ok || byLocale["ko"] != "<div>카드</div>" {
		t.Fatalf("apps mode locale widgets wrong: %v", w)
	}
	if _, ok := byLocale["en"]; ok {
		t.Fatalf("apps mode should not invent empty locale widgets: %v", byLocale)
	}
}

func TestWidgetAppUsesMCPAppsBridge(t *testing.T) {
	g, _, _, _ := newGateway(t)
	html := g.widgetAppHTML(narrative.LocaleEN)
	for _, want := range []string{"ui/initialize", "ui/notifications/initialized", "ui/notifications/tool-result", "window.openai", "openai:set_globals", "supportedLocales", widgetMetaKey} {
		if !strings.Contains(html, want) {
			t.Fatalf("MCP Apps resource missing %q:\n%s", want, html)
		}
	}
}

func TestAppToolAddsCodexOpenAICompatibilityMetadata(t *testing.T) {
	tool := appTool(&mcp.Tool{Name: "get_time"})
	uiMeta, ok := tool.Meta["ui"].(map[string]any)
	if !ok || uiMeta["resourceUri"] != widgetURI {
		t.Fatalf("standard UI metadata = %#v", tool.Meta)
	}
	if got := tool.Meta["ui/resourceUri"]; got != widgetURI {
		t.Fatalf("flat ui/resourceUri = %v", got)
	}
	if got := tool.Meta["openai/outputTemplate"]; got != widgetURI {
		t.Fatalf("openai/outputTemplate = %v", got)
	}
	if _, ok := tool.Meta["openai/widgetAccessible"].(bool); !ok {
		t.Fatalf("missing openai/widgetAccessible: %#v", tool.Meta)
	}
}

// TestAttachWidgetMetaMode locks the generic-client-safe fallback: the
// card rides in _meta, entirely out of the model-facing Content stream (nil).
func TestAttachWidgetMetaMode(t *testing.T) {
	g, _, _, _ := newGateway(t)
	g.WidgetMode = widgetMeta
	res := &mcp.CallToolResult{}
	g.attachWidget(res, map[string]any{"ok": true}, "<div>card</div>", nil)

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
		sections: []widgetSection{{
			title: "Rows",
			lines: []widgetLine{{primary: `<script>alert(2)</script>`, meta: "m"}},
		}},
	})
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Fatalf("dynamic value not escaped — injection risk:\n%s", html)
	}
	if strings.Contains(html, "<script>alert(2)</script>") {
		t.Fatalf("section value not escaped — injection risk:\n%s", html)
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

func TestWidgetLocaleUsesOpenAILocale(t *testing.T) {
	g, _, _, _ := newGateway(t)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{"openai/locale": "ko-KR"}}}
	if got := g.widgetLocaleForTool(req); got != narrative.LocaleKO {
		t.Fatalf("tool locale = %q, want ko", got)
	}
}

func TestWidgetLocaleIgnoresUnsupportedClientLocale(t *testing.T) {
	g, _, _, _ := newGateway(t)
	g.Locale = narrative.LocaleKO
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{"openai/locale": "fr-FR"}}}
	if got := g.widgetLocaleForTool(req); got != narrative.LocaleKO {
		t.Fatalf("operator locale must override unsupported client locale: got %q", got)
	}

	g.Locale = ""
	req = &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{
		"openai/locale": "",
		"webplus/i18n":  map[string]any{"primary": "ko-KR"},
	}}}
	if got := g.widgetLocaleForTool(req); got != narrative.LocaleKO {
		t.Fatalf("empty openai locale should fall through to webplus locale: got %q", got)
	}

	t.Setenv("LC_ALL", "ko_KR.UTF-8")
	req = &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{"openai/locale": "fr-FR"}}}
	if got := g.widgetLocaleForTool(req); got != narrative.LocaleEN {
		t.Fatalf("unsupported explicit client locale should fall back to English: got %q", got)
	}

	req = &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{"openai/locale": "session-123"}}}
	if got := g.widgetLocaleForTool(req); got != narrative.LocaleKO {
		t.Fatalf("junk client locale should not suppress system fallback: got %q", got)
	}
}

func TestWidgetLocaleHeaderFallsThroughWhenUnsupported(t *testing.T) {
	g, _, _, _ := newGateway(t)
	t.Setenv("LC_ALL", "ko_KR.UTF-8")
	req := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: map[string][]string{
		"Accept-Language": {"fr-FR,zz;q=0.5"},
	}}}
	if got := g.widgetLocaleForTool(req); got != narrative.LocaleEN {
		t.Fatalf("unsupported Accept-Language should fall back to English: got %q", got)
	}
}

func TestWidgetLocaleHeaderUsesHighestQSupportedLanguage(t *testing.T) {
	g, _, _, _ := newGateway(t)
	req := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: map[string][]string{
		"Accept-Language": {"fr-FR,ko-KR;q=0.4,en;q=0.9"},
	}}}
	if got := g.widgetLocaleForTool(req); got != narrative.LocaleEN {
		t.Fatalf("header locale = %q, want en", got)
	}
}

func TestWidgetLocaleForResourceUsesOpenAILocale(t *testing.T) {
	g, _, _, _ := newGateway(t)
	req := &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{Meta: mcp.Meta{"openai/locale": "ko-KR"}}}
	if got := g.widgetLocaleForResource(req); got != narrative.LocaleKO {
		t.Fatalf("resource locale = %q, want ko", got)
	}
}

func TestLocalizedWidgetHTMLPinnedLocale(t *testing.T) {
	g, _, _, _ := newGateway(t)
	g.Locale = narrative.LocaleKO
	htmls := localizedWidgetHTML(g, narrative.LocaleKO, "<div>ko</div>", emptyIn{}, nil,
		func(*Gateway, narrative.Locale, emptyIn, map[string]any) string {
			return "<div>other</div>"
		})
	if htmls != nil {
		t.Fatalf("pinned locale should use primary html only, got %#v", htmls)
	}
}

func TestSituationCardNormalizesJSONShapedLists(t *testing.T) {
	g, _, _, _ := newGateway(t)
	env := map[string]any{
		"ok": true,
		"data": map[string]any{
			"urgent": map[string]any{
				"injuries": []any{
					map[string]any{"name": "A"},
					map[string]any{"name": "B"},
				},
			},
			"last_results": []any{
				map[string]any{
					"home":       map[string]any{"name": "Home"},
					"away":       map[string]any{"name": "Away"},
					"home_goals": 1,
					"away_goals": 0,
				},
			},
			"headlines": []any{
				map[string]any{"article": map[string]any{"title": "No timestamp"}},
				map[string]any{
					"category": "contract",
					"headline": map[string]any{
						"key":    "news.contract.renewed",
						"params": map[string]any{"club": "Stanton Albion", "count": 2},
					},
				},
			},
		},
	}
	card := situationCard(g, narrative.LocaleEN, emptyIn{}, env)
	for _, want := range []string{"Injuries", `<span class="nfw-v">2</span>`, "Home 1-0 Away"} {
		if !strings.Contains(card, want) {
			t.Fatalf("situation card missing %q:\n%s", want, card)
		}
	}
	if strings.Contains(card, "<nil>") {
		t.Fatalf("situation card should not render nil metadata:\n%s", card)
	}
	ko := situationCard(g, narrative.LocaleKO, emptyIn{}, env)
	if !strings.Contains(ko, "재계약") || strings.Contains(ko, "fresh terms") {
		t.Fatalf("situation headline should be localized in ko:\n%s", ko)
	}
}

func TestWidgetCardsOmitNilAndEmptyRows(t *testing.T) {
	g, _, _, _ := newGateway(t)
	for name, card := range map[string]string{
		"time":     timeCard(g, narrative.LocaleEN, emptyIn{}, map[string]any{"ok": true, "data": map[string]any{}}),
		"settings": settingsCard(g, narrative.LocaleEN, emptyIn{}, map[string]any{"ok": true, "data": map[string]any{"world": map[string]any{}, "pacing": map[string]any{}}}),
		"situation": situationCard(g, narrative.LocaleEN, emptyIn{}, map[string]any{"ok": true, "data": map[string]any{
			"urgent": map[string]any{"injuries": []any{}},
		}}),
	} {
		if strings.Contains(card, "<nil>") {
			t.Fatalf("%s card should not render nil metadata:\n%s", name, card)
		}
		if name == "situation" && strings.Contains(card, "Injuries") {
			t.Fatalf("empty injuries should not render a zero-count row:\n%s", card)
		}
	}
}
