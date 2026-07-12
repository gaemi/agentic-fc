package mcpserver

import (
	"fmt"
	"strings"

	"github.com/gaemi/agentic-fc/internal/focus"
	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/narrative"
)

// Per-tool widget renderers. Each summarizes the AGENT'S
// action from the (already-masked) envelope only — never a world re-fetch (so
// FR-22 can't reopen) — with chrome + descriptors rendered through narrative
// keys in the spectator's locale (en+ko). Descriptor values are localized from
// their KEY, never scraped from the envelope's English text.

// envData / envList pull the (masked) result payload out of the envelope.
func envData(env map[string]any) map[string]any {
	return anyMap(env["data"])
}

func envList(env map[string]any, key string) []map[string]any {
	if d := envData(env); d != nil {
		return mapList(d[key])
	}
	return nil
}

func anyMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func mapList(v any) []map[string]any {
	switch rows := v.(type) {
	case []map[string]any:
		if len(rows) == 0 {
			return nil
		}
		return rows
	case []any:
		out := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			if m := anyMap(row); m != nil {
				out = append(out, m)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	return nil
}

func mstr(m map[string]any, key string) string {
	if m != nil {
		if s, ok := m[key].(string); ok {
			return s
		}
	}
	return ""
}

func mint64(m map[string]any, key string) int64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		return 0
	}
}

// moneyDisplay reads the human string out of a money map ({amount, display}).
func moneyDisplay(v any) string {
	if m := anyMap(v); m != nil {
		return mstr(m, "display")
	}
	return ""
}

// descText localizes a Descriptor by re-rendering its KEY in the target locale —
// the envelope's `text` is English (FR-22a) and must never be scraped for i18n.
func (g *Gateway) descText(loc narrative.Locale, desc any) string {
	if m := anyMap(desc); m != nil {
		if k := mstr(m, "key"); k != "" {
			return g.tr(loc, k)
		}
	}
	return ""
}

// row appends a labelled row only when the value is non-empty (a card never
// shows a blank field for an envelope section that wasn't returned).
func (c *widgetCard) row(label, value string) {
	if value != "" {
		c.rows = append(c.rows, widgetRow{label: label, value: value})
	}
}

// changeRow is row for a decided change (highlighted value); it thins the same
// way — a blank value (e.g. an unset dial in a zero-value plan) is omitted.
func (c *widgetCard) changeRow(label, value string) {
	if value != "" {
		c.rows = append(c.rows, widgetRow{label: label, value: value, change: true})
	}
}

// countRow renders a numeric envelope field; a missing or wrong-typed value
// (never a struct/list dump or "<nil>") is omitted.
func (c *widgetCard) countRow(label string, v any) {
	switch v.(type) {
	case int, int64, float64:
		c.row(label, fmt.Sprint(v))
	}
}

func (c *widgetCard) section(title string, lines []widgetLine) {
	if title != "" && len(lines) > 0 {
		c.sections = append(c.sections, widgetSection{title: title, lines: lines})
	}
}

func limitLines(lines []widgetLine, n int) []widgetLine {
	if n > 0 && len(lines) > n {
		return lines[:n]
	}
	return lines
}

func tableLines(rows []map[string]any) []widgetLine {
	out := make([]widgetLine, 0, len(rows))
	for _, row := range rows {
		club := clubName(row["club"])
		if club == "" {
			continue
		}
		parts := []string{}
		if pos := row["pos"]; pos != nil {
			parts = append(parts, fmt.Sprintf("#%v", pos))
		}
		if pts := row["points"]; pts != nil {
			parts = append(parts, fmt.Sprintf("%v pts", pts))
		}
		if form := mstr(row, "form"); form != "" {
			parts = append(parts, form)
		}
		out = append(out, widgetLine{primary: club, meta: strings.Join(parts, " · ")})
	}
	return out
}

func resultLines(rows []map[string]any) []widgetLine {
	out := make([]widgetLine, 0, len(rows))
	for _, row := range rows {
		home, away := clubName(row["home"]), clubName(row["away"])
		if home == "" || away == "" {
			continue
		}
		primary := home + " v " + away
		if row["home_goals"] != nil && row["away_goals"] != nil {
			primary = fmt.Sprintf("%s %v-%v %s", home, row["home_goals"], row["away_goals"], away)
		}
		meta := ""
		if gt := row["game_time"]; gt != nil {
			meta = fmt.Sprint(gt)
		} else if comp := row["competition"]; comp != nil {
			meta = fmt.Sprint(comp)
		}
		out = append(out, widgetLine{primary: primary, meta: meta})
	}
	return out
}

func fixtureLines(rows []map[string]any) []widgetLine {
	out := make([]widgetLine, 0, len(rows))
	for _, row := range rows {
		home, away := clubName(row["home"]), clubName(row["away"])
		if home == "" || away == "" {
			continue
		}
		meta := ""
		if kickoff := row["kickoff"]; kickoff != nil {
			meta = fmt.Sprint(kickoff)
		}
		if round := row["round"]; round != nil {
			if meta != "" {
				meta = fmt.Sprintf("R%v · %s", round, meta)
			} else {
				meta = fmt.Sprintf("R%v", round)
			}
		}
		out = append(out, widgetLine{primary: home + " v " + away, meta: meta})
	}
	return out
}

func headlineLines(g *Gateway, loc narrative.Locale, rows []map[string]any) []widgetLine {
	out := make([]widgetLine, 0, len(rows))
	for _, row := range rows {
		title := ""
		if headline := anyMap(row["headline"]); headline != nil {
			if key := mstr(headline, "key"); key != "" {
				article := g.newsArticle(fmt.Sprint(row["category"]), key, anyMap(headline["params"]), loc, mint64(row, "id"))
				title = mstr(article, "title")
			}
		}
		if title == "" {
			if article := anyMap(row["article"]); article != nil {
				title = mstr(article, "title")
			}
		}
		if title == "" {
			if headline := anyMap(row["headline"]); headline != nil {
				title = mstr(headline, "text")
			}
		}
		if title == "" {
			continue
		}
		meta := ""
		if gt := row["game_time"]; gt != nil {
			meta = fmt.Sprint(gt)
		}
		out = append(out, widgetLine{primary: title, meta: meta})
	}
	return out
}

func trLines(g *Gateway, loc narrative.Locale, prefix string, n int) []widgetLine {
	out := make([]widgetLine, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, widgetLine{primary: g.tr(loc, fmt.Sprintf("%s.%d", prefix, i))})
	}
	return out
}

// guideHighlightCount is intentionally the number of localized summary bullets
// rendered in the human card, not the full model-facing guide length.
const guideHighlightCount = 4

func guideCard(g *Gateway, loc narrative.Locale, _ emptyIn, env map[string]any) string {
	c := g.baseCard(loc, "read", "widget.badge.read", string(focus.GetGuide), env)
	c.headline = g.tr(loc, "widget.headline.guide")
	d := envData(env)
	c.section(g.tr(loc, "widget.section.first_step_highlights"), trLines(g, loc, "widget.guide.first", guideHighlightCount))
	c.section(g.tr(loc, "widget.section.strategy_highlights"), trLines(g, loc, "widget.guide.loop", guideHighlightCount))
	if vocab := anyMap(d["vocabularies"]); vocab != nil {
		c.row(g.tr(loc, "widget.row.vocabularies"), fmt.Sprint(len(vocab)))
	}
	return renderCard(c)
}

func timeCard(g *Gateway, loc narrative.Locale, _ emptyIn, env map[string]any) string {
	c := g.baseCard(loc, "read", "widget.badge.observed", string(focus.GetTime), env)
	c.headline = g.tr(loc, "widget.headline.time")
	d := envData(env)
	if tempo := d["tempo"]; tempo != nil {
		c.row(g.tr(loc, "widget.row.tempo"), fmt.Sprint(tempo))
	}
	if speed := d["game_speed"]; speed != nil {
		c.row(g.tr(loc, "widget.row.speed"), fmt.Sprintf("%vx", speed))
	}
	if next := anyMap(d["next_match_window"]); next != nil {
		c.row(g.tr(loc, "widget.row.next_match"), mstr(next, "kickoff"))
	}
	return renderCard(c)
}

func focusCard(g *Gateway, loc narrative.Locale, _ emptyIn, env map[string]any) string {
	c := g.baseCard(loc, "read", "widget.badge.observed", string(focus.GetFocus), env)
	c.headline = g.tr(loc, "widget.headline.focus")
	d := envData(env)
	if balance := d["balance"]; balance != nil {
		if focusCap := d["cap"]; focusCap != nil {
			c.row(g.tr(loc, "widget.row.focus_balance"), fmt.Sprintf("%v / %v", balance, focusCap))
		} else {
			c.row(g.tr(loc, "widget.row.focus_balance"), fmt.Sprint(balance))
		}
	}
	if regen := d["regen_per_game_hour"]; regen != nil {
		c.row(g.tr(loc, "widget.row.focus_regen"), fmt.Sprint(regen))
	}
	spends := mapList(d["spends"])
	c.row(g.tr(loc, "widget.row.spends"), fmt.Sprint(len(spends)))
	lines := make([]widgetLine, 0, len(spends))
	for _, spend := range spends {
		lines = append(lines, widgetLine{
			primary: fmt.Sprintf("%v", spend["tool"]),
			meta:    fmt.Sprintf("-%v · %v", spend["cost"], spend["game_time"]),
		})
	}
	c.section(g.tr(loc, "widget.section.recent_spends"), limitLines(lines, 5))
	return renderCard(c)
}

func mindsetCard(g *Gateway, loc narrative.Locale, _ emptyIn, env map[string]any) string {
	c := g.baseCard(loc, "read", "widget.badge.observed", string(focus.GetMindset), env)
	c.headline = g.tr(loc, "widget.headline.mindset")
	d := envData(env)
	if employment := anyMap(d["employment"]); employment != nil {
		if status := mstr(employment, "status"); status != "" {
			c.row(g.tr(loc, "widget.row.employment"), g.enumLabel(loc, "employment", status))
		}
		if club := mstr(employment, "club_name"); club != "" {
			c.row(g.tr(loc, "widget.row.club"), club)
		}
	}
	if board := anyMap(d["board"]); board != nil {
		if objective := board["objective_finish"]; objective != nil {
			c.row(g.tr(loc, "widget.row.objective"), fmt.Sprint(objective))
		}
	}
	if ms, ok := d["mindset"].(mindset.Mindset); ok {
		c.row(g.tr(loc, "widget.row.version"), fmt.Sprint(ms.Version))
		c.row(g.tr(loc, "widget.row.priorities"), fmt.Sprint(len(ms.Priorities)))
		c.row(g.tr(loc, "widget.row.directives"), fmt.Sprint(len(ms.Directives)))
		if len(ms.Priorities) > 0 {
			lines := []widgetLine{}
			for _, p := range ms.Priorities {
				lines = append(lines, widgetLine{primary: g.enumLabel(loc, "goal", string(p.Goal)), meta: fmt.Sprintf("#%d", p.Rank)})
			}
			c.section(g.tr(loc, "widget.section.priorities"), limitLines(lines, 5))
		}
		if ms.Tactical.Formation != "" {
			lines := []widgetLine{{primary: ms.Tactical.Formation, meta: g.enumLabel(loc, "mentality", ms.Tactical.Mentality)}}
			if ms.Tactical.Pressing != "" || ms.Tactical.Tempo != "" {
				lines = append(lines, widgetLine{
					primary: g.enumLabel(loc, "pressing", ms.Tactical.Pressing),
					meta:    g.enumLabel(loc, "tempo", ms.Tactical.Tempo),
				})
			}
			c.section(g.tr(loc, "widget.section.tactical_plan"), lines)
		}
	}
	return renderCard(c)
}

func settingsCard(g *Gateway, loc narrative.Locale, _ emptyIn, env map[string]any) string {
	c := g.baseCard(loc, "read", "widget.badge.observed", string(focus.GetSettings), env)
	c.headline = g.tr(loc, "widget.headline.settings")
	d := envData(env)
	if world := anyMap(d["world"]); world != nil {
		c.row(g.tr(loc, "widget.row.world"), mstr(world, "name"))
		if phase := world["season_phase"]; phase != nil {
			c.row(g.tr(loc, "widget.row.phase"), fmt.Sprint(phase))
		}
	}
	if pacing := anyMap(d["pacing"]); pacing != nil {
		if profile := pacing["run_profile"]; profile != nil {
			c.row(g.tr(loc, "widget.row.run_profile"), fmt.Sprint(profile))
		}
		if speed := pacing["base_game_speed"]; speed != nil {
			c.row(g.tr(loc, "widget.row.speed"), fmt.Sprintf("%vx", speed))
		}
		if effective := pacing["current_effective_speed"]; effective != nil {
			c.row(g.tr(loc, "widget.row.effective_speed"), fmt.Sprintf("%vx", effective))
		}
	}
	return renderCard(c)
}

func situationCard(g *Gateway, loc narrative.Locale, _ emptyIn, env map[string]any) string {
	c := g.baseCard(loc, "read", "widget.badge.observed", string(focus.GetSituation), env)
	c.headline = g.tr(loc, "widget.headline.situation")
	d := envData(env)
	if phase := d["season_phase"]; phase != nil {
		c.row(g.tr(loc, "widget.row.phase"), fmt.Sprint(phase))
	}
	if pos := d["league_position"]; pos != nil {
		c.row(g.tr(loc, "widget.row.league_position"), fmt.Sprint(pos))
	}
	c.row(g.tr(loc, "widget.row.headlines"), fmt.Sprint(len(envList(env, "headlines"))))
	if urgent := anyMap(d["urgent"]); urgent != nil {
		if board := anyMap(urgent["board"]); board != nil {
			c.row(g.tr(loc, "widget.row.board"), g.descText(loc, board["confidence"]))
		}
		if injuries := mapList(urgent["injuries"]); injuries != nil {
			c.row(g.tr(loc, "widget.row.injuries"), fmt.Sprint(len(injuries)))
		}
		if suspensions := mapList(urgent["suspensions"]); len(suspensions) > 0 {
			c.row(g.tr(loc, "widget.row.suspensions"), fmt.Sprint(len(suspensions)))
		}
		c.countRow(g.tr(loc, "widget.row.expiring_contracts"), urgent["expiring_contracts"])
	}
	if next := anyMap(d["next_fixture"]); next != nil {
		c.section(g.tr(loc, "widget.section.next_fixture"), fixtureLines([]map[string]any{next}))
	}
	c.section(g.tr(loc, "widget.section.last_results"), resultLines(envList(env, "last_results")))
	c.section(g.tr(loc, "widget.section.headlines"), limitLines(headlineLines(g, loc, envList(env, "headlines")), 4))
	return renderCard(c)
}

func newsCard(g *Gateway, loc narrative.Locale, _ getNewsIn, env map[string]any) string {
	c := g.baseCard(loc, "read", "widget.badge.read", string(focus.GetNews), env)
	c.headline = g.tr(loc, "widget.headline.news")
	items := envList(env, "items")
	c.row(g.tr(loc, "widget.row.items"), fmt.Sprint(len(items)))
	if len(items) == 0 {
		c.headline = g.tr(loc, "widget.headline.news.empty")
		return renderCard(c)
	}
	top := items[0]
	if headline := anyMap(top["headline"]); headline != nil {
		params := anyMap(headline["params"])
		article := g.newsArticle(fmt.Sprint(top["category"]), mstr(headline, "key"), params, loc, mint64(top, "id"))
		c.headline = mstr(article, "title")
		c.row(g.tr(loc, "widget.row.source"), mstr(article, "source"))
		c.row(g.tr(loc, "widget.row.category"), fmt.Sprint(top["category"]))
		c.body = append(c.body, mstr(article, "deck"), mstr(article, "body"))
	} else if article := anyMap(top["article"]); article != nil {
		c.headline = mstr(article, "title")
		c.row(g.tr(loc, "widget.row.source"), mstr(article, "source"))
		c.row(g.tr(loc, "widget.row.category"), fmt.Sprint(top["category"]))
		c.body = append(c.body, mstr(article, "deck"), mstr(article, "body"))
	}
	return renderCard(c)
}

func clubCard(g *Gateway, loc narrative.Locale, _ getClubIn, env map[string]any) string {
	c := g.baseCard(loc, "read", "widget.badge.observed", string(focus.GetClub), env)
	c.headline = g.tr(loc, "widget.headline.club")
	d := envData(env)
	c.row(g.tr(loc, "widget.row.club"), clubName(d["club"]))
	if div, ok := d["division"]; ok {
		c.row(g.tr(loc, "widget.row.division"), fmt.Sprint(div))
	}
	if board, ok := d["board"].(map[string]any); ok {
		c.row(g.tr(loc, "widget.row.board"), g.descText(loc, board["confidence"]))
	}
	if sq, ok := d["squad"].([]map[string]any); ok {
		c.row(g.tr(loc, "widget.row.players"), fmt.Sprint(len(sq)))
	}
	return renderCard(c)
}

func squadCard(g *Gateway, loc narrative.Locale, _ getSquadIn, env map[string]any) string {
	c := g.baseCard(loc, "read", "widget.badge.observed", string(focus.GetSquad), env)
	c.headline = g.tr(loc, "widget.headline.squad")
	players := envList(env, "players")
	c.row(g.tr(loc, "widget.row.players"), fmt.Sprint(len(players)))
	if len(players) > 0 {
		c.row(g.tr(loc, "widget.row.sample"), mstr(players[0], "name"))
	}
	return renderCard(c)
}

func personCard(g *Gateway, loc narrative.Locale, _ getPersonIn, env map[string]any) string {
	c := g.baseCard(loc, "read", "widget.badge.observed", string(focus.GetPerson), env)
	d := envData(env)
	// A manager envelope carries a "manager" key; a player one carries "player".
	// Read the kind from the RESULT, not the request, so the headline can't lie.
	if _, isManager := d["manager"]; isManager {
		c.headline = g.tr(loc, "widget.headline.manager")
	} else {
		c.headline = g.tr(loc, "widget.headline.person")
	}
	c.row(g.tr(loc, "widget.row.name"), mstr(d, "name"))
	c.row(g.tr(loc, "widget.row.position"), mstr(d, "position"))
	if age, ok := d["age"]; ok {
		c.row(g.tr(loc, "widget.row.age"), fmt.Sprint(age))
	}
	c.row(g.tr(loc, "widget.row.club"), clubName(d["club"]))
	if cond, ok := d["condition"]; ok {
		c.row(g.tr(loc, "widget.row.condition"), fmt.Sprint(cond))
	}
	return renderCard(c)
}

func matchCard(g *Gateway, loc narrative.Locale, _ getMatchIn, env map[string]any) string {
	c := g.baseCard(loc, "read", "widget.badge.observed", string(focus.GetMatch), env)
	c.headline = g.tr(loc, "widget.headline.match")
	d := envData(env)
	if home, away := clubName(d["home"]), clubName(d["away"]); home != "" || away != "" {
		c.row(g.tr(loc, "widget.row.fixture"), g.tr2(loc, "widget.match.vs", map[string]any{"home": home, "away": away}))
	}
	if status := mstr(d, "status"); status != "" {
		c.row(g.tr(loc, "widget.row.status"), g.tr(loc, "widget.match."+status))
	}
	// Only a complete score renders — a thin/scheduled envelope with one goal
	// field (or neither) omits the row rather than printing "2–<nil>".
	if hg, okH := d["home_goals"]; okH {
		if ag, okA := d["away_goals"]; okA {
			c.row(g.tr(loc, "widget.row.score"), fmt.Sprintf("%v–%v", hg, ag))
		}
	}
	c.row(g.tr(loc, "widget.row.competition"), mstr(d, "competition"))
	return renderCard(c)
}

func searchCard(g *Gateway, loc narrative.Locale, _ searchPlayersIn, env map[string]any) string {
	c := g.baseCard(loc, "read", "widget.badge.observed", string(focus.SearchPlayers), env)
	c.headline = g.tr(loc, "widget.headline.search")
	players := envList(env, "players")
	c.row(g.tr(loc, "widget.row.found"), fmt.Sprint(len(players)))
	if len(players) > 0 {
		top := players[0]
		v := mstr(top, "name")
		if vb, ok := top["value_band"].(map[string]any); ok {
			if band := moneyDisplay(vb["low"]) + "–" + moneyDisplay(vb["high"]); band != "–" {
				v += " · " + band
			}
		}
		c.row(g.tr(loc, "widget.row.top"), v)
	}
	return renderCard(c)
}

func scoutCard(g *Gateway, loc narrative.Locale, _ scoutIn, env map[string]any) string {
	c := g.baseCard(loc, "write", "widget.badge.scouted", string(focus.Scout), env)
	c.headline = g.tr(loc, "widget.headline.scout")
	c.row(g.tr(loc, "widget.row.due"), mstr(envData(env), "report_due"))
	return renderCard(c)
}

func configureAlertsCard(g *Gateway, loc narrative.Locale, in configureAlertsIn, env map[string]any) string {
	kind, badge, headline := "write", "widget.badge.decided", "widget.headline.alerts_configured"
	if in.Enabled == nil && in.Watches == nil {
		kind, badge, headline = "read", "widget.badge.observed", "widget.headline.alerts"
	}
	c := g.baseCard(loc, kind, badge, string(focus.ConfigureAlerts), env)
	c.headline = g.tr(loc, headline)
	alertRows(g, loc, &c, envData(env))
	return renderCard(c)
}

func getAlertsCard(g *Gateway, loc narrative.Locale, _ getAlertsIn, env map[string]any) string {
	c := g.baseCard(loc, "read", "widget.badge.observed", string(focus.GetAlerts), env)
	c.headline = g.tr(loc, "widget.headline.alerts")
	alertRows(g, loc, &c, envData(env))
	return renderCard(c)
}

func ackAlertsCard(g *Gateway, loc narrative.Locale, _ ackAlertsIn, env map[string]any) string {
	c := g.baseCard(loc, "write", "widget.badge.decided", string(focus.AckAlerts), env)
	c.headline = g.tr(loc, "widget.headline.alerts_acked")
	d := envData(env)
	c.countRow(g.tr(loc, "widget.row.acked_through"), d["acked_through"])
	c.countRow(g.tr(loc, "widget.row.pending"), d["pending_count"])
	return renderCard(c)
}

func alertRows(g *Gateway, loc narrative.Locale, c *widgetCard, d map[string]any) {
	if enabled, ok := d["enabled"].(bool); ok {
		c.row(g.tr(loc, "widget.row.enabled"), g.tr(loc, fmt.Sprintf("widget.bool.%t", enabled)))
	}
	watches := mapList(d["watches"])
	pending := mapList(d["pending"])
	c.row(g.tr(loc, "widget.row.watches"), fmt.Sprint(len(watches)))
	c.row(g.tr(loc, "widget.row.pending"), fmt.Sprint(len(pending)))
	c.row(g.tr(loc, "widget.row.resource"), mstr(d, "resource"))
	lines := make([]widgetLine, 0, len(pending))
	for _, item := range pending {
		primary := mstr(item, "message")
		if primary == "" {
			primary = mstr(item, "reason")
		}
		if primary == "" {
			continue
		}
		meta := ""
		if gt := item["game_time"]; gt != nil {
			meta = fmt.Sprint(gt)
		}
		lines = append(lines, widgetLine{primary: primary, meta: meta})
	}
	c.section(g.tr(loc, "widget.section.pending_alerts"), limitLines(lines, 5))
}

// ---- shaping-write renderers ----
//
// These read the DECISION from the result envelope (the mindset types the call
// produced), so they show "what the manager changed". Row labels AND enum
// values (goals, verbs, strengths, tactical dials) are localized via the
// enum.* catalog vocabularies;
// formations and position codes stay as-is (universal football notation).
// enumLabel falls back to the raw token for any missing entry, so a new enum
// value degrades to the old behavior instead of erroring (FR-35c discipline);
// the drift test keeps the vocabularies complete.

// enumLabel localizes a domain token via its enum.<class>.<TOKEN> entry.
func (g *Gateway) enumLabel(loc narrative.Locale, class, token string) string {
	if token == "" {
		return ""
	}
	key := "enum." + class + "." + token
	if s := g.Catalogs.Render(loc, key, nil); s != key {
		return s
	}
	return token
}

func prioritiesCard(g *Gateway, loc narrative.Locale, _ setPrioritiesIn, env map[string]any) string {
	c := g.baseCard(loc, "write", "widget.badge.decided", string(focus.SetPriorities), env)
	c.headline = g.tr(loc, "widget.headline.priorities")
	if ps, ok := envData(env)["priorities"].([]mindset.Priority); ok {
		c.row(g.tr(loc, "widget.row.count"), fmt.Sprint(len(ps)))
		for _, p := range ps {
			if p.Rank == 1 {
				c.changeRow(g.tr(loc, "widget.row.top_priority"), g.enumLabel(loc, "goal", string(p.Goal)))
				break
			}
		}
	}
	return renderCard(c)
}

func addDirectiveCard(g *Gateway, loc narrative.Locale, _ addDirectiveIn, env map[string]any) string {
	c := g.baseCard(loc, "write", "widget.badge.decided", string(focus.AddDirective), env)
	c.headline = g.tr(loc, "widget.headline.directive_add")
	d := envData(env)
	if dir, ok := d["directive"].(mindset.Directive); ok {
		c.changeRow(g.tr(loc, "widget.row.directive"),
			g.enumLabel(loc, "verb", string(dir.Verb))+" · "+g.enumLabel(loc, "strength", string(dir.Strength)))
	}
	c.countRow(g.tr(loc, "widget.row.active"), d["active_directives"])
	return renderCard(c)
}

func removeDirectiveCard(g *Gateway, loc narrative.Locale, _ removeDirectiveIn, env map[string]any) string {
	c := g.baseCard(loc, "write", "widget.badge.decided", string(focus.RemoveDirective), env)
	c.headline = g.tr(loc, "widget.headline.directive_remove")
	c.countRow(g.tr(loc, "widget.row.active"), envData(env)["active_directives"])
	return renderCard(c)
}

func tacticalCard(g *Gateway, loc narrative.Locale, in updateTacticalPlanIn, env map[string]any) string {
	c := g.baseCard(loc, "write", "widget.badge.decided", string(focus.UpdateTacticalPlan), env)
	c.headline = g.tr(loc, "widget.headline.tactical")
	if tp, ok := envData(env)["tactical_plan"].(mindset.TacticalPlan); ok {
		row := func(changed bool, label, value string) {
			if changed {
				c.changeRow(label, value)
				return
			}
			c.row(label, value)
		}
		row(in.Formation != "", g.tr(loc, "widget.row.formation"), tp.Formation)
		row(in.Mentality != "", g.tr(loc, "widget.row.mentality"), g.enumLabel(loc, "mentality", tp.Mentality))
		row(in.Pressing != "", g.tr(loc, "widget.row.pressing"), g.enumLabel(loc, "pressing", tp.Pressing))
		row(in.Tempo != "", g.tr(loc, "widget.row.tempo"), g.enumLabel(loc, "tempo", tp.Tempo))
		row(in.Width != "", g.tr(loc, "widget.row.width"), g.enumLabel(loc, "width", tp.Width))
		row(in.Directness != "", g.tr(loc, "widget.row.directness"), g.enumLabel(loc, "directness", tp.Directness))
		row(in.Counter != nil, g.tr(loc, "widget.row.counter"), g.tr(loc, fmt.Sprintf("widget.bool.%t", tp.Counter)))
	}
	return renderCard(c)
}
