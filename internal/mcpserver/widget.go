package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gaemi/agentic-fc/internal/focus"
	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/narrative"
)

// MCP UI widgets. Every meaningful tool call carries a human-readable
// "action card" beside the structured envelope: it frames the call as agent
// BEHAVIOUR — "what the agent queried and learned", "what it changed and how" —
// so a spectator can follow the manager's reasoning. It never re-renders game
// state (the TUI Console owns that); it renders the AGENT'S actions.
//
// Compatibility result-attachment cards are rendered server-side in the spectator's
// locale (FR-35c: system language, English fallback) purely from the
// already-masked envelope. The official MCP Apps resource gets localized chrome
// from the same catalogs and renders the host-provided structured envelope. It
// never re-fetches world state, so it cannot reopen FR-22, and the AI's
// structured JSON is untouched.

// widgetMode selects how per-result cards are attached. The default MCP Apps
// path advertises _meta.ui.resourceUri and also places the pre-rendered card in
// result _meta so the app can render as soon as it receives tool-result.
type widgetMode int

const (
	// widgetApps uses the MCP Apps pattern: tool metadata links to a registered
	// UI resource, and result _meta carries the already-masked card for the app.
	widgetApps widgetMode = iota
	// widgetMeta is a compatibility mode that rides the rendered card in
	// result _meta, out of the model-facing Content stream.
	widgetMeta widgetMode = iota
	// widgetContentBlock is the MCP-UI EmbeddedResource compatibility mode. Use only for
	// hosts that render embedded text/html resources from tool results.
	widgetContentBlock
)

const widgetURI = "ui://agenticfc/action-card"
const widgetMIME = "text/html;profile=mcp-app"
const widgetMetaKey = "agenticfc/widget"

// widgetRenderer builds the card HTML for one tool from its localized locale,
// typed input, and the (masked) envelope. Empty string = no card.
type widgetRenderer[In any] func(g *Gateway, loc narrative.Locale, in In, env map[string]any) string

// handleUI is the widget-aware handler wrapper: it runs the tool, then (on a
// successful call) renders the action card and attaches it via the seam. The
// SDK still fills StructuredContent from the returned envelope, so the AI's data
// is unchanged whether or not a card is attached.
func handleUI[In any](g *Gateway, fn func(managerID int64, sessionID string, in In) map[string]any,
	render widgetRenderer[In]) mcp.ToolHandlerFor[In, map[string]any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, map[string]any, error) {
		id, aerr := managerIDFromCtx(ctx)
		if aerr != nil {
			return nil, g.errEnvelope(nil, aerr), nil
		}
		session := ""
		if req != nil && req.Session != nil {
			session = req.Session.ID()
		}
		env := fn(id, session, in)
		res := &mcp.CallToolResult{}
		if ok, _ := env["ok"].(bool); ok {
			if htmlCard := render(g, g.widgetLocale(), in, env); htmlCard != "" {
				g.attachWidget(res, env, htmlCard)
			}
		}
		return res, env, nil
	}
}

// SetWidgetMode selects the UI transport. The default/"apps" follows the MCP
// Apps pattern (_meta.ui.resourceUri on tools + resources/read). "meta" and
// "content" are compatibility fallbacks for hosts that do not implement MCP Apps.
func (g *Gateway) SetWidgetMode(mode string) {
	if mode == "meta" {
		g.WidgetMode = widgetMeta
		return
	}
	if mode == "content" {
		g.WidgetMode = widgetContentBlock
		return
	}
	g.WidgetMode = widgetApps
}

// widgetLocale is the spectator's display locale: an explicit Gateway.Locale
// wins, else the system language (FR-35c), else English.
func (g *Gateway) widgetLocale() narrative.Locale {
	if g.Locale != "" {
		return g.Locale
	}
	return narrative.FromEnv(os.Getenv)
}

// attachWidget is the single swappable seam. It never fails the tool result: a
// marshal error just drops the card (the AI still gets its data).
func (g *Gateway) attachWidget(res *mcp.CallToolResult, env map[string]any, htmlCard string) {
	switch g.WidgetMode {
	case widgetApps:
		if res.Meta == nil {
			res.Meta = mcp.Meta{}
		}
		res.Meta[widgetMetaKey] = map[string]any{"mimeType": widgetMIME, "html": htmlCard}
		return
	case widgetMeta:
		if res.Meta == nil {
			res.Meta = mcp.Meta{}
		}
		res.Meta[widgetMetaKey] = map[string]any{"mimeType": widgetMIME, "html": htmlCard}
		return
	}
	// content-block compatibility mode: keep the structured JSON as the first
	// (model-facing) block so the AI is unaffected, then the widget resource.
	envJSON, err := json.Marshal(env)
	if err != nil {
		return
	}
	res.Content = []mcp.Content{
		&mcp.TextContent{Text: string(envJSON)},
		&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{
			URI: widgetURI, MIMEType: widgetMIME, Text: htmlCard,
		}},
	}
}

func appTool(t *mcp.Tool) *mcp.Tool {
	if t.Meta == nil {
		t.Meta = mcp.Meta{}
	}
	t.Meta["ui"] = map[string]any{"resourceUri": widgetURI}
	return t
}

func (g *Gateway) registerUIResources(s *mcp.Server) {
	s.AddResource(&mcp.Resource{
		URI:         widgetURI,
		Name:        "agenticfc_action_card",
		Title:       "Agentic FC action card",
		Description: "MCP Apps resource for rendering Agentic FC tool results.",
		MIMEType:    widgetMIME,
	}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI:      widgetURI,
			MIMEType: widgetMIME,
			Text:     g.widgetAppHTML(g.widgetLocale()),
		}}}, nil
	})
}

// ---- card model ----

type widgetCard struct {
	kind     string // "read" | "write" — drives the badge colour
	badge    string // localized action label
	tool     string // tool name (mono)
	gameTime string
	focus    string // localized focus line
	version  string // localized mindset version (writes only)
	headline string
	asked    string // optional query line (reads)
	rows     []widgetRow
	body     []string
}

type widgetRow struct {
	label  string
	value  string
	change bool // highlight the value as a decided change
}

func (c widgetCard) metaLine() string {
	parts := make([]string, 0, 3)
	for _, p := range []string{c.gameTime, c.focus, c.version} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

// baseCard fills the header (badge, tool, game-time, focus, version) from the
// envelope meta — shared by every renderer so each only supplies headline+rows.
func (g *Gateway) baseCard(loc narrative.Locale, kind, badgeKey, tool string, env map[string]any) widgetCard {
	c := widgetCard{kind: kind, badge: g.tr(loc, badgeKey), tool: tool}
	meta, _ := env["meta"].(map[string]any)
	if meta == nil {
		return c
	}
	if gt, ok := meta["game_time"].(string); ok {
		c.gameTime = gt
	}
	c.focus = g.focusLine(loc, meta)
	if kind == "write" {
		if v, ok := meta["mindset_version"]; ok {
			c.version = g.tr2(loc, "widget.version", map[string]any{"version": v})
		}
	}
	return c
}

func (g *Gateway) focusLine(loc narrative.Locale, meta map[string]any) string {
	f, ok := meta["focus"].(map[string]any)
	if !ok {
		return ""
	}
	if toInt(f["spent"]) == 0 {
		return g.tr(loc, "widget.focus.free")
	}
	return g.tr2(loc, "widget.focus.spent", map[string]any{"spent": f["spent"], "balance": f["balance"]})
}

// tr / tr2 are locale renders with and without params.
func (g *Gateway) tr(loc narrative.Locale, key string) string {
	return g.Catalogs.Render(loc, key, nil)
}
func (g *Gateway) tr2(loc narrative.Locale, key string, p map[string]any) string {
	return g.Catalogs.Render(loc, key, p)
}

// ---- HTML ----

// widgetCSS is compact, self-contained (no external assets), and scoped to
// .nfw so it is safe in a sandboxed host iframe.
const widgetCSS = `<style>` +
	`.nfw{--b:#0e1116;--l:#2b323d;--t:#e7ecf3;--t2:#96a2b2;--t3:#6b7686;--rd:#57a6ff;--wr:#3ddc84;` +
	`--mono:ui-monospace,SFMono-Regular,Menlo,monospace;font-family:system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;` +
	`background:var(--b);color:var(--t);border:1px solid var(--l);border-radius:12px;padding:13px 15px;max-width:480px}` +
	`.nfw *{box-sizing:border-box}` +
	`.nfw-hd{display:flex;align-items:center;gap:8px;flex-wrap:wrap}` +
	`.nfw-bg{font-size:10.5px;font-weight:500;text-transform:uppercase;letter-spacing:.06em;padding:2px 8px;border-radius:6px}` +
	`.nfw--read .nfw-bg{color:var(--rd);background:rgba(87,166,255,.13)}` +
	`.nfw--write .nfw-bg{color:var(--wr);background:rgba(61,220,132,.14)}` +
	`.nfw-tl{font-family:var(--mono);font-size:12px;color:var(--t2)}` +
	`.nfw-mt{margin-left:auto;font-size:11px;color:var(--t3);font-family:var(--mono)}` +
	`.nfw-hl{font-size:14px;font-weight:500;margin:10px 0 2px;line-height:1.35}` +
	`.nfw-as{font-size:11.5px;color:var(--t3);font-family:var(--mono);margin:2px 0 4px}` +
	`.nfw-rs{margin-top:7px}` +
	`.nfw-r{display:flex;justify-content:space-between;gap:12px;padding:5px 0;font-size:13px;border-top:1px solid var(--l)}` +
	`.nfw-rs .nfw-r:first-child{border-top:none}` +
	`.nfw-k{color:var(--t3);font-size:12px}` +
	`.nfw-v{color:var(--t);text-align:right}` +
	`.nfw-r--ch .nfw-v{color:var(--wr);font-weight:500}` +
	`.nfw-p{margin:9px 0 0;color:var(--t2);font-size:12.5px;line-height:1.45}` +
	`.nfw-sr{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0,0,0,0)}` +
	`</style>`

// widgetAppHTML is the MCP Apps resource. It speaks the MCP Apps JSON-RPC
// postMessage lifecycle, then renders the card from tool-result _meta. It also
// accepts older ad-hoc postMessage shapes as a defensive fallback.
func (g *Gateway) widgetAppHTML(loc narrative.Locale) string {
	msg := func(key string) string { return g.tr(loc, key) }
	labels, _ := json.Marshal(map[string]string{
		"observed":   msg("widget.badge.observed"),
		"decided":    msg("widget.badge.decided"),
		"problem":    msg("widget.app.problem"),
		"mcpApp":     msg("widget.app.mcp_app"),
		"waiting":    msg("widget.app.waiting"),
		"toolResult": msg("widget.app.tool_result"),
		"time":       msg("widget.headline.time"),
		"settings":   msg("widget.headline.settings"),
		"situation":  msg("widget.headline.situation"),
		"news":       msg("widget.headline.news"),
		"directive":  msg("widget.headline.directive_add"),
		"tactical":   msg("widget.headline.tactical"),
		"generic":    msg("widget.headline.generic"),
		"incomplete": msg("widget.app.incomplete"),
	})
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">` + widgetCSS + `<style>body{margin:0;background:transparent}.nfw{max-width:none;border-radius:0;border:0}.nfw-pre{white-space:pre-wrap;font:12px ui-monospace,SFMono-Regular,Menlo,monospace;color:#96a2b2;margin-top:8px}</style></head><body><div id="root" class="nfw nfw--read"><div class="nfw-hd"><span class="nfw-bg">` + html.EscapeString(msg("ui.app.title")) + `</span><span class="nfw-tl">` + html.EscapeString(msg("widget.app.mcp_app")) + `</span></div><div class="nfw-hl">` + html.EscapeString(msg("widget.app.waiting")) + `</div></div><script>
(function(){
  const labels=` + string(labels) + `;
  const metaKey='` + widgetMetaKey + `';
  let root=document.getElementById('root');
  let nextId=1;
  const pending=new Map();
  const esc=(v)=>String(v==null?'':v).replace(/[&<>"']/g,(c)=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const pick=(m)=>m&&(
    m.toolResult?.structuredContent || m.toolResult?.structured_content ||
    m.result?.structuredContent || m.result?.structured_content ||
    m.structuredContent || m.structured_content ||
    m.content?.structuredContent || m.data?.structuredContent || m.data
  );
  function metaLine(meta){
    const parts=[];
    if(meta?.game_time) parts.push(meta.game_time);
    if(meta?.tempo) parts.push(meta.tempo);
    if(meta?.focus) parts.push('FP '+meta.focus.balance+'/'+meta.focus.cap);
    return parts.join(' · ');
  }
  function post(msg){ try{window.parent&&window.parent.postMessage(msg,'*');}catch(e){} }
  function sendRequest(method,params){
    const id=nextId++;
    post({jsonrpc:'2.0',id,method,params});
    return new Promise((resolve,reject)=>{
      pending.set(id,{resolve,reject});
      setTimeout(()=>{ if(pending.has(id)){ pending.delete(id); reject(new Error('timeout')); } },2500);
    });
  }
  function sendNotification(method,params){ post({jsonrpc:'2.0',method,params:params||{}}); }
  function notifySize(){
    requestAnimationFrame(()=>{
      const el=document.documentElement;
      sendNotification('ui/notifications/size-changed',{width:Math.ceil(el.scrollWidth),height:Math.ceil(el.scrollHeight)});
    });
  }
  function renderHTML(html){
    document.body.innerHTML=html;
    root=document.querySelector('.nfw')||document.body.firstElementChild;
    notifySize();
  }
  function renderToolResult(result){
    const meta=result?._meta||result?.meta||{};
    const widget=meta[metaKey]||meta.agenticfc_widget;
    if(widget&&widget.html){ renderHTML(widget.html); return; }
    render(result?.structuredContent||result?.structured_content||pick(result)||result);
  }
  function render(env){
    if(!env || typeof env!=='object') return;
    const ok=env.ok===true;
    const meta=env.meta||{};
    const data=env.data||{};
    const err=env.error||{};
    const tool=(meta.tool||labels.toolResult);
    const title=ok?(data.directive||data.tactical_plan||data.formation?labels.decided:labels.observed):labels.problem;
    const headline=ok?summary(data):err.message||err.message_key||labels.incomplete;
    root.className='nfw '+(ok?'nfw--read':'nfw--write');
    root.innerHTML='<div class="nfw-hd"><span class="nfw-bg">'+esc(title)+'</span><span class="nfw-tl">'+esc(tool)+'</span><span class="nfw-mt">'+esc(metaLine(meta))+'</span></div><div class="nfw-hl">'+esc(headline)+'</div><div class="nfw-pre">'+esc(JSON.stringify(data||err,null,2)).slice(0,4000)+'</div>';
    notifySize();
  }
  function summary(data){
    if(data.title) return data.title;
    if(data.game_time) return labels.time;
    if(data.world&&data.pacing) return labels.settings;
    if(data.items) return labels.news;
    if(data.season_phase) return labels.situation;
    if(data.directive) return labels.directive;
    if(data.tactical_plan||data.formation) return labels.tactical;
    return labels.generic;
  }
  window.addEventListener('message',(ev)=>{
    const m=ev.data||{};
    if(m.id&&pending.has(m.id)){
      const p=pending.get(m.id); pending.delete(m.id);
      if(m.error) p.reject(m.error); else p.resolve(m.result);
      return;
    }
    if(m.method==='ui/notifications/tool-result'){ renderToolResult(m.params); return; }
    if(m.method==='ui/resource-teardown'){ post({jsonrpc:'2.0',id:m.id,result:{}}); return; }
    const env=pick(m); if(env) render(env);
  });
  sendRequest('ui/initialize',{
    protocolVersion:'2026-01-26',
    clientInfo:{name:'agentic-fc-action-card',version:'dev'},
    appCapabilities:{availableDisplayModes:['inline','fullscreen']}
  }).then(()=>sendNotification('ui/notifications/initialized',{})).catch(()=>{});
})();
</script></body></html>`
}

func renderCard(c widgetCard) string {
	var b strings.Builder
	b.WriteString(widgetCSS)
	b.WriteString(`<div class="nfw nfw--`)
	b.WriteString(c.kind)
	b.WriteString(`"><h2 class="nfw-sr">`)
	b.WriteString(esc(c.badge + ": " + c.headline))
	b.WriteString(`</h2><div class="nfw-hd"><span class="nfw-bg">`)
	b.WriteString(esc(c.badge))
	b.WriteString(`</span><span class="nfw-tl">`)
	b.WriteString(esc(c.tool))
	b.WriteString(`</span><span class="nfw-mt">`)
	b.WriteString(esc(c.metaLine()))
	b.WriteString(`</span></div>`)
	if c.headline != "" {
		b.WriteString(`<div class="nfw-hl">`)
		b.WriteString(esc(c.headline))
		b.WriteString(`</div>`)
	}
	if c.asked != "" {
		b.WriteString(`<div class="nfw-as">`)
		b.WriteString(esc(c.asked))
		b.WriteString(`</div>`)
	}
	if len(c.rows) > 0 {
		b.WriteString(`<div class="nfw-rs">`)
		for _, r := range c.rows {
			b.WriteString(`<div class="nfw-r`)
			if r.change {
				b.WriteString(` nfw-r--ch`)
			}
			b.WriteString(`"><span class="nfw-k">`)
			b.WriteString(esc(r.label))
			b.WriteString(`</span><span class="nfw-v">`)
			b.WriteString(esc(r.value))
			b.WriteString(`</span></div>`)
		}
		b.WriteString(`</div>`)
	}
	for _, p := range c.body {
		if p == "" {
			continue
		}
		b.WriteString(`<p class="nfw-p">`)
		b.WriteString(esc(p))
		b.WriteString(`</p>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// esc HTML-escapes any dynamic text (club names, values) before it enters the
// card — a widget must never let world data break out of its markup.
func esc(s string) string { return html.EscapeString(s) }

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

// signed formats a disposition value with an explicit sign (+5, 0, −3).
func signed(v any) string {
	n := toInt(v)
	if n > 0 {
		return "+" + fmt.Sprint(n)
	}
	return fmt.Sprint(n)
}

func clubName(ref any) string {
	if m, ok := ref.(map[string]any); ok {
		if n, ok := m["name"].(string); ok {
			return n
		}
	}
	return ""
}

// ---- per-tool renderers ----

// leagueCard frames a get_league read: which division the agent looked at, which
// sections it asked for, and a takeaway per section actually returned. The
// headline follows the primary section PRESENT in the envelope (not a hardcoded
// "standings"), so a fixtures- or managers-only query reads truthfully; the rows
// come from the envelope, the "asked" line from the request (the agent's intent).
func leagueCard(g *Gateway, loc narrative.Locale, in getLeagueIn, env map[string]any) string {
	c := g.baseCard(loc, "read", "widget.badge.observed", string(focus.GetLeague), env)
	data, _ := env["data"].(map[string]any)
	if data == nil {
		c.headline = g.tr(loc, "widget.headline.league")
		return renderCard(c)
	}
	if len(in.Sections) > 0 {
		c.asked = strings.Join(in.Sections, ", ")
	}
	c.rows = append(c.rows, widgetRow{label: g.tr(loc, "widget.row.division"), value: fmt.Sprint(data["division"])})

	table, hasTable := data["table"].([]map[string]any)
	results, hasResults := data["results"].([]map[string]any)
	fixtures, hasFixtures := data["fixtures"].([]map[string]any)
	managers, hasManagers := data["managers"].([]map[string]any)

	switch { // headline = the primary section present (table is the default act)
	case hasTable:
		c.headline = g.tr(loc, "widget.headline.league")
	case hasResults:
		c.headline = g.tr(loc, "widget.headline.league.results")
	case hasFixtures:
		c.headline = g.tr(loc, "widget.headline.league.fixtures")
	case hasManagers:
		c.headline = g.tr(loc, "widget.headline.league.managers")
	default:
		c.headline = g.tr(loc, "widget.headline.league")
	}

	if hasTable && len(table) > 0 {
		top := table[0]
		c.rows = append(c.rows,
			widgetRow{label: g.tr(loc, "widget.row.leaders"),
				value: clubName(top["club"]) + " · " + g.tr2(loc, "widget.pts", map[string]any{"pts": top["points"]})},
			widgetRow{label: g.tr(loc, "widget.row.clubs"), value: fmt.Sprint(len(table))},
		)
	}
	if hasResults {
		c.rows = append(c.rows, widgetRow{label: g.tr(loc, "widget.row.results"), value: fmt.Sprint(len(results))})
	}
	if hasFixtures {
		c.rows = append(c.rows, widgetRow{label: g.tr(loc, "widget.row.fixtures"), value: fmt.Sprint(len(fixtures))})
	}
	if hasManagers {
		c.rows = append(c.rows, widgetRow{label: g.tr(loc, "widget.row.managers"), value: fmt.Sprint(len(managers))})
	}
	return renderCard(c)
}

// dispositionCard frames an update_disposition write: which axes the agent
// re-targeted and to what — the "how the manager changed" the user wants. It
// reads solely from the result envelope's axes (the applied ∪ drifting set the
// call produced), never the raw request, so the card can never show more than
// the envelope actually returned (the FR-22 guardrail this seed proves).
func dispositionCard(g *Gateway, loc narrative.Locale, _ updateDispositionIn, env map[string]any) string {
	c := g.baseCard(loc, "write", "widget.badge.decided", string(focus.UpdateDisposition), env)
	c.headline = g.tr(loc, "widget.headline.disposition")
	data, _ := env["data"].(map[string]any)
	axes, _ := data["axes"].(map[string]any)
	for _, a := range mindset.AllAxes {
		ax, ok := axes[string(a)].(map[string]any)
		if !ok {
			continue
		}
		var value string
		if target, drifting := ax["target"]; drifting {
			value = signed(target) + " · " + g.tr2(loc, "widget.disposition.drifting", map[string]any{"current": signed(ax["current"])})
		} else {
			value = signed(ax["current"]) + " · " + g.tr(loc, "widget.disposition.applied")
		}
		c.rows = append(c.rows, widgetRow{label: g.tr(loc, "widget.axis."+string(a)), value: value, change: true})
	}
	return renderCard(c)
}
