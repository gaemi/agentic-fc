package mcpserver

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/engine"
	"github.com/gaemi/agentic-fc/internal/focus"
	"github.com/gaemi/agentic-fc/internal/narrative"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// The observation surface (docs/11 §4/§5): wide-shallow to narrow-deep
// reads plus the scout commission. Everything here is read-mostly — the
// only mutations are Focus charges, get_news session cursors, and the
// scout event (the documented exception, FR-15).

func (g *Gateway) registerObservationTools(s *mcp.Server) {
	mcp.AddTool(s, appTool(&mcp.Tool{
		Name:        string(focus.GetSituation),
		Description: "1 FP. The wide-shallow dashboard: position, next fixture, urgent items, headlines.",
	}), handleUI(g, g.getSituation, situationCard))
	mcp.AddTool(s, appTool(&mcp.Tool{
		Name:        string(focus.GetNews),
		Description: "1 FP. Condensed news since a cursor (per-session; defaults to the current game-day).",
	}), handleUI(g, g.getNews, newsCard))
	mcp.AddTool(s, appTool(&mcp.Tool{
		Name:        string(focus.GetLeague),
		Description: "2 FP. League table, results, fixtures, and the managers section.",
	}), handleUI(g, g.getLeague, leagueCard))
	mcp.AddTool(s, appTool(&mcp.Tool{
		Name:        string(focus.GetClub),
		Description: "2 FP own / 4 FP other. Club profile — full internals for your own club, public reads otherwise.",
	}), handleUI(g, g.getClub, clubCard))
	mcp.AddTool(s, appTool(&mcp.Tool{
		Name:        string(focus.GetSquad),
		Description: "3 FP own / 4 FP other. Squad list with visible attributes (exact own, ranges otherwise).",
	}), handleUI(g, g.getSquad, squadCard))
	mcp.AddTool(s, appTool(&mcp.Tool{
		Name:        string(focus.GetPerson),
		Description: "4 FP. The narrow-deep single-entity view (player or manager).",
	}), handleUI(g, g.getPerson, personCard))
	mcp.AddTool(s, appTool(&mcp.Tool{
		Name:        string(focus.GetMatch),
		Description: "1 FP own / 3 FP other. Match or fixture view (results arrive with the match engine).",
	}), handleUI(g, g.getMatch, matchCard))
	mcp.AddTool(s, appTool(&mcp.Tool{
		Name:        string(focus.SearchPlayers),
		Description: "4 FP. Filtered shallow player search; fidelity follows scouting knowledge.",
	}), handleUI(g, g.searchPlayers, searchCard))
	mcp.AddTool(s, appTool(&mcp.Tool{
		Name:        string(focus.Scout),
		Description: "12 FP. Commission a scouting process (~1–2 game-weeks); results arrive as a private report.",
	}), handleUI(g, g.scout, scoutCard))
}

// ---- get_situation ----

func (g *Gateway) getSituation(mid int64, sid string, _ emptyIn) map[string]any {
	return g.run(mid, sid, focus.GetSituation, nil, flatCost(focus.GetSituation),
		func(cc *callCtx) (any, *apiError) {
			w := g.Host.World()
			m := cc.manager
			data := map[string]any{
				"season_phase": seasonPhase(w, cc.now),
				"headlines":    g.headlines(cc, 5),
			}
			if m.ClubID == 0 {
				data["employment"] = "UNEMPLOYED"
				data["reputation"] = g.descriptor("desc.reputation." + reputationBand(m.Reputation))
				data["vacancies"] = []any{} // job market arrives with careers
				return data, nil
			}
			club, _ := g.clubOf(m)
			data["league_position"] = g.currentPos(w, m.ClubID)
			data["last_results"] = g.clubRecentResults(w, m.ClubID, 3)
			if _, f := nextKickoff(w, cc.now, m.ClubID); f != nil {
				data["next_fixture"] = fixtureRef(w, f)
			}
			// "Expiring" = in its final season: the expiry year equals the
			// CURRENT season (it was hardcoded ==1, which was only right in
			// season 1 — latent bug, current-season contract rule). Youth are
			// excluded: the academy auto-renews, so theirs are never urgent.
			season := worldgen.DateOf(cc.now).Season
			expiring := 0
			injuries := []map[string]any{}
			suspensions := []map[string]any{}
			for i := range w.Players {
				p := &w.Players[i]
				if p.ClubID != m.ClubID {
					continue
				}
				if !p.Youth && p.Contract != nil && p.Contract.ExpirySeasonYear == season {
					expiring++
				}
				// The dashboard is the OWN view, so the medical room's expected
				// return date is visible here (FR-22: the band is public, the
				// date is own-club — same rule as get_person).
				if p.InjuredUntil > cc.now {
					injuries = append(injuries, map[string]any{
						"player": p.ID, "name": p.Name,
						"severity":        g.descriptor("desc.injury." + currentInjuryBand(p)),
						"expected_return": gameTimeISO(p.InjuredUntil),
					})
				}
				// Bans are announced facts, so the count itself is public.
				if p.SuspendedMatches > 0 {
					suspensions = append(suspensions, map[string]any{
						"player": p.ID, "name": p.Name, "matches": p.SuspendedMatches,
					})
				}
			}
			data["urgent"] = map[string]any{
				"injuries":           injuries,
				"suspensions":        suspensions,
				"expiring_contracts": expiring,
				"board": map[string]any{
					"objective_finish": club.BoardObjectiveFinish,
					"confidence":       g.descriptor("desc.confidence." + confidenceBand(club.Confidence)),
				},
			}
			return data, nil
		})
}

// ---- get_news ----

type getNewsIn struct {
	Since      string   `json:"since,omitempty"`
	Categories []string `json:"categories,omitempty"`
	Scope      string   `json:"scope,omitempty"` // own | league | world (default own)
	Limit      int      `json:"limit,omitempty"` // ≤100, default 50
}

func (g *Gateway) getNews(mid int64, sid string, in getNewsIn) map[string]any {
	return g.run(mid, sid, focus.GetNews, in, flatCost(focus.GetNews),
		func(cc *callCtx) (any, *apiError) {
			w := g.Host.World()
			m := cc.manager

			limit := in.Limit
			if limit <= 0 {
				limit = 50
			}
			if limit > 100 {
				limit = 100
			}
			scope := in.Scope
			if scope == "" {
				scope = "own"
			}
			switch scope {
			case "own", "league", "world":
			default:
				return nil, errFor(ErrValidation, "err.validation",
					map[string]any{"detail": "scope must be own|league|world"}, nil)
			}

			// Cursor: explicit > per-session state > start of game-day.
			var afterID int64
			switch {
			case in.Since != "":
				id, err := strconv.ParseInt(in.Since, 10, 64)
				if err != nil {
					return nil, errFor(ErrValidation, "err.validation",
						map[string]any{"detail": "bad cursor"}, nil)
				}
				afterID = id
			case g.cursors[sid] > 0:
				afterID = g.cursors[sid]
			default:
				dayStart := cc.now - cc.now%sim.MinutesPerDay
				afterID = -1 // sentinel: filter by day start below
				for _, n := range w.News {
					if n.GameTime >= dayStart {
						afterID = n.ID - 1
						break
					}
				}
				if afterID == -1 { // nothing today yet
					afterID = w.NewsNextID
				}
			}

			categories := map[string]bool{}
			for _, c := range in.Categories {
				categories[c] = true
			}

			items := make([]map[string]any, 0, limit)
			cursor := afterID
			for _, n := range w.News {
				if n.ID <= afterID {
					continue
				}
				if len(items) >= limit {
					break
				}
				if n.Key == "feed.matchday.preview" {
					cursor = n.ID
					continue
				}
				if !g.newsVisible(&n, m, scope) {
					continue
				}
				cursor = n.ID
				if len(categories) > 0 && !categories[n.Category] {
					continue
				}
				items = append(items, g.renderNews(&n))
			}
			// The session cursor advances to what this call consumed —
			// deterministic under replay (session id rides the input log).
			g.cursors[sid] = cursor
			return map[string]any{
				"items":  items,
				"cursor": strconv.FormatInt(cursor, 10),
			}, nil
		})
}

// newsVisible applies privacy and scope: private items belong to their
// manager alone and surface only in the personal ("own"/default) feed — never
// in the public world/league breadth scopes; world-wide items (no clubs)
// reach every scope.
func (g *Gateway) newsVisible(n *worldgen.NewsItem, m *worldgen.Manager, scope string) bool {
	if n.ManagerID != 0 {
		return n.ManagerID == m.ID && scope != "world" && scope != "league"
	}
	if len(n.ClubIDs) == 0 {
		return true
	}
	switch scope {
	case "world":
		return true
	case "league":
		tier := g.tierOf(m)
		for _, id := range n.ClubIDs {
			if c := g.clubByID(id); c != nil && c.DivisionTier == tier {
				return true
			}
		}
		return false
	default: // own
		for _, id := range n.ClubIDs {
			if id == m.ClubID && id != 0 {
				return true
			}
		}
		return false
	}
}

// poolDerivedNewsParams are news params that are pure functions of a player's
// hidden Ability Pool — a transfer fee (pool²·k) or a wage demand both invert to
// the exact pool. They ride in the news item for the human Console + emit feed,
// but must never reach an agent: any pool-derived numeric added to a news key
// MUST be listed here. Enforced at renderNews, the sole News→agent boundary.
var poolDerivedNewsParams = map[string]bool{"fee": true, "wage": true}

// agentNewsParams returns a copy of a news item's params with the pool-derived
// numerics stripped. renderMessage echoes params verbatim in the
// structured headline, so this is the guard that keeps a hidden attribute from
// crossing the wire even though the headline TEXT template never prints them.
func agentNewsParams(params map[string]any) map[string]any {
	safe := make(map[string]any, len(params))
	for k, v := range params {
		if poolDerivedNewsParams[k] {
			continue
		}
		safe[k] = v
	}
	return safe
}

func (g *Gateway) renderNews(n *worldgen.NewsItem) map[string]any {
	params := agentNewsParams(n.Params)
	headline := g.renderMessage(n.Key, params)
	return map[string]any{
		"id":        n.ID,
		"game_time": gameTimeISO(n.GameTime),
		"category":  n.Category,
		"headline":  headline,
		"article":   g.newsArticle(n.Category, n.Key, params, narrative.LocaleEN, n.ID),
		"refs":      n.ClubIDs,
	}
}

func (g *Gateway) newsArticle(category, key string, params map[string]any, loc narrative.Locale, newsID int64) map[string]any {
	title := g.renderMessageText(loc, key, params)
	articleClass := category
	switch articleClass {
	case "transfer", "match", "injury", "board", "decision", "career", "youth", "contract", "scout":
	default:
		articleClass = "media"
	}
	sourceClass := articleClass
	articleParams := map[string]any{"headline": title}
	if key == "feed.matchday.results" {
		articleClass = "matchday.results"
		title = g.tr2(loc, narrative.ArticleTemplateKey("title", articleClass, newsID), params)
		articleParams = g.matchdayResultsArticleParams(loc, params, title)
	}
	return map[string]any{
		"source": g.tr(loc, "news.article.source."+sourceClass),
		"title":  title,
		"deck":   g.tr2(loc, narrative.ArticleTemplateKey("deck", articleClass, newsID), articleParams),
		"body":   g.tr2(loc, narrative.ArticleTemplateKey("body", articleClass, newsID), articleParams),
	}
}

func (g *Gateway) matchdayResultsArticleParams(loc narrative.Locale, params map[string]any, title string) map[string]any {
	out := copyArticleParams(params, title)
	out["results"] = g.matchdayResultLines(loc, params["results"])
	out["table"] = g.matchdayTableLines(loc, params["table"])
	out["story"] = g.matchdayStoryLine(loc, params["story"])
	return out
}

func copyArticleParams(params map[string]any, title string) map[string]any {
	out := make(map[string]any, len(params)+1)
	for k, v := range params {
		out[k] = v
	}
	out["headline"] = title
	return out
}

func (g *Gateway) matchdayResultLines(loc narrative.Locale, raw any) string {
	rows := mapsFromAny(raw)
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		key := "term.matchday.result_line"
		if winner, ok := row["winner"].(string); ok && winner != "" {
			key = "term.matchday.result_line.winner"
		}
		lines = append(lines, g.tr2(loc, key, row))
	}
	return joinNonEmpty(lines)
}

func (g *Gateway) matchdayTableLines(loc narrative.Locale, raw any) string {
	rows := mapsFromAny(raw)
	if len(rows) == 0 {
		return g.tr(loc, "term.matchday.table_cup")
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, g.tr2(loc, "term.matchday.table_leader", row))
	}
	return joinNonEmpty(lines)
}

func (g *Gateway) matchdayStoryLine(loc narrative.Locale, raw any) string {
	rows := mapsFromAny(raw)
	if len(rows) == 0 {
		return ""
	}
	story := rows[0]
	lines := []string{}
	margin, hasMargin := numericParam(story["best_margin"])
	draws, hasDraws := numericParam(story["draws"])
	if hasMargin && margin > 0 {
		lines = append(lines, g.tr2(loc, "term.matchday.story_margin", story))
	}
	if hasDraws && draws > 0 {
		lines = append(lines, g.tr2(loc, "term.matchday.story_draws", story))
	}
	if hasDraws && draws == 0 && hasMargin && margin > 0 {
		lines = append(lines, g.tr(loc, "term.matchday.story_all_winners"))
	}
	if len(lines) == 0 {
		// Defensive fallback for malformed persisted params; normal engine
		// payloads always contain draws and best_margin.
		lines = append(lines, g.tr(loc, "term.matchday.story_unavailable"))
	}
	return joinNonEmpty(lines)
}

func mapsFromAny(raw any) []map[string]any {
	switch v := raw.(type) {
	case nil:
		return nil
	case map[string]any:
		return []map[string]any{v}
	case []map[string]any:
		return v
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if row, ok := item.(map[string]any); ok {
				out = append(out, row)
			}
		}
		return out
	default:
		return nil
	}
}

func numericParam(raw any) (float64, bool) {
	switch v := raw.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case json.Number:
		got, err := v.Float64()
		return got, err == nil
	default:
		return 0, false
	}
}

func joinNonEmpty(lines []string) string {
	out := []string{}
	for _, line := range lines {
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// headlines returns the newest visible items for the get_situation dashboard.
// The dashboard is the manager's personal overview, so it reads the "own"
// feed: world-wide events + their own club + their private items (scout
// reports) — but not other clubs' league minutiae.
func (g *Gateway) headlines(cc *callCtx, n int) []map[string]any {
	w := g.Host.World()
	out := []map[string]any{}
	for i := len(w.News) - 1; i >= 0 && len(out) < n; i-- {
		item := &w.News[i]
		if g.newsVisible(item, cc.manager, "own") {
			out = append(out, g.renderHeadline(item))
		}
	}
	return out
}

// renderHeadline keeps get_situation wide and shallow. Full article bodies and
// matchday result/table payloads stay behind get_news; the dashboard retains
// enough information to identify and re-render its headline before drilling
// down. Extend the strip list whenever a news key adds another heavy nested
// detail param; the dashboard must never grow into a second get_news payload.
func (g *Gateway) renderHeadline(n *worldgen.NewsItem) map[string]any {
	out := g.renderNews(n)
	if article, ok := out["article"].(map[string]any); ok {
		delete(article, "body")
	}
	if headline, ok := out["headline"].(map[string]any); ok {
		if params, ok := headline["params"].(map[string]any); ok {
			for _, key := range []string{"fixtures", "results", "story", "table"} {
				delete(params, key)
			}
		}
	}
	return out
}

// ---- get_league ----

type getLeagueIn struct {
	Division int      `json:"division,omitempty"` // default: own (or 1)
	Sections []string `json:"sections,omitempty"` // table|results|fixtures|managers
}

func (g *Gateway) getLeague(mid int64, sid string, in getLeagueIn) map[string]any {
	return g.run(mid, sid, focus.GetLeague, in, flatCost(focus.GetLeague),
		func(cc *callCtx) (any, *apiError) {
			w := g.Host.World()
			tier := in.Division
			if tier == 0 {
				tier = g.tierOf(cc.manager)
			}
			if tier < 1 || tier > w.Config.Divisions {
				return nil, errFor(ErrNotFound, "err.not_found",
					map[string]any{"id": tier}, map[string]any{"division": tier})
			}
			sections := map[string]bool{}
			if len(in.Sections) == 0 {
				sections["table"], sections["results"] = true, true
			}
			for _, s := range in.Sections {
				sections[s] = true
			}

			data := map[string]any{"division": tier}
			if sections["table"] {
				rows := []map[string]any{}
				for _, row := range g.currentTable(w, tier) {
					c := g.clubByID(row.ClubID)
					rows = append(rows, map[string]any{
						"pos": row.Pos, "club": clubRef(c),
						"played": row.Played, "won": row.Won, "drawn": row.Drawn,
						"lost": row.Lost, "gf": row.GoalsFor, "ga": row.GoalsAgainst,
						"points": row.Points, "form": formString(w, row.ClubID),
					})
				}
				data["table"] = rows
			}
			if sections["results"] {
				data["results"] = g.recentResults(w, tier, 20)
			}
			if sections["fixtures"] {
				fixtures := []map[string]any{}
				for i := range w.Fixtures {
					f := &w.Fixtures[i]
					if f.Competition != worldgen.CompetitionLeague ||
						f.DivisionTier != tier || f.Kickoff < cc.now {
						continue
					}
					fixtures = append(fixtures, fixtureRef(w, f))
					if len(fixtures) >= 20 {
						break
					}
				}
				data["fixtures"] = fixtures
			}
			if sections["managers"] {
				rows := []map[string]any{}
				for i := range w.Managers {
					mm := &w.Managers[i]
					c := g.clubByID(mm.ClubID)
					if c == nil || c.DivisionTier != tier {
						continue
					}
					rows = append(rows, map[string]any{
						"manager":  map[string]any{"manager": mm.ID, "name": mm.Name},
						"club":     clubRef(c),
						"tenure":   "1925-07", // careers begin at world start
						"security": g.descriptor("desc.security." + securityBand(c.Confidence)),
					})
				}
				data["managers"] = rows
			}
			return data, nil
		})
}

// ---- get_club ----

type getClubIn struct {
	Club int64 `json:"club,omitempty"` // default own
}

func (g *Gateway) getClub(mid int64, sid string, in getClubIn) map[string]any {
	cost := func(cc *callCtx) int {
		c, _ := focus.CostOwnOther(focus.GetClub, g.targetsOwnClub(cc.manager, in.Club))
		return c
	}
	return g.run(mid, sid, focus.GetClub, in, cost,
		func(cc *callCtx) (any, *apiError) {
			w := g.Host.World()
			m := cc.manager
			id := in.Club
			if id == 0 {
				if m.ClubID == 0 {
					return nil, errFor(ErrUnemployedScope, "err.unemployed_scope", nil, nil)
				}
				id = m.ClubID
			}
			c := g.clubByID(id)
			if c == nil {
				return nil, errFor(ErrNotFound, "err.not_found",
					map[string]any{"id": id}, map[string]any{"club": id})
			}

			if id == m.ClubID {
				// Own club: full internals.
				squad := []map[string]any{}
				for i := range w.Players {
					p := &w.Players[i]
					if p.ClubID != id {
						continue
					}
					squad = append(squad, map[string]any{
						"player": p.ID, "name": p.Name, "age": p.Age,
						"position": p.Position, "condition": p.Condition,
						"morale": g.descriptor("desc.mood.STEADY"),
					})
				}
				return map[string]any{
					"club": clubRef(c),
					"finances": map[string]any{
						"balance":            money(c.BalanceMinor),
						"wage_bill_weekly":   money(c.WageBillWeeklyMinor),
						"wage_budget_weekly": money(c.WageBudgetWeeklyMinor),
						"transfer_budget":    money(c.TransferBudgetMinor),
					},
					"board": map[string]any{
						"objective_finish": c.BoardObjectiveFinish,
						"confidence":       g.descriptor("desc.confidence." + confidenceBand(c.Confidence)),
					},
					"fan_mood": g.descriptor("desc.mood.STEADY"),
					"facilities": map[string]any{
						"training": g.descriptor("desc.facility." + facilityBand(c.Tendencies.TrainingFacilities)),
						"youth":    g.descriptor("desc.facility." + facilityBand(c.Tendencies.YouthFacilities)),
					},
					"squad": squad,
				}, nil
			}

			// Other club: public profile only — internals never leak.
			var mgr *worldgen.Manager
			for i := range w.Managers {
				if w.Managers[i].ClubID == id {
					mgr = &w.Managers[i]
				}
			}
			headliners := topByPool(w, m, id, 3)
			players := []map[string]any{}
			for _, p := range headliners {
				players = append(players, map[string]any{
					"player": p.ID, "name": p.Name, "age": p.Age, "position": p.Position,
				})
			}
			out := map[string]any{
				"club":             clubRef(c),
				"division":         c.DivisionTier,
				"last_season_pos":  lastSeasonPos(w, id),
				"stadium":          map[string]any{"name": c.Stadium.Name, "capacity": c.Stadium.Capacity},
				"headline_players": players,
			}
			if mgr != nil {
				out["manager"] = map[string]any{"manager": mgr.ID, "name": mgr.Name}
				out["manager_reputation"] = g.descriptor("desc.reputation." + reputationBand(mgr.Reputation))
			}
			return out, nil
		})
}

// ---- get_squad ----

type getSquadIn struct {
	Club   int64  `json:"club,omitempty"`
	Detail string `json:"detail,omitempty"` // attributes|condition|contracts
}

func (g *Gateway) getSquad(mid int64, sid string, in getSquadIn) map[string]any {
	cost := func(cc *callCtx) int {
		c, _ := focus.CostOwnOther(focus.GetSquad, g.targetsOwnClub(cc.manager, in.Club))
		return c
	}
	return g.run(mid, sid, focus.GetSquad, in, cost,
		func(cc *callCtx) (any, *apiError) {
			w := g.Host.World()
			m := cc.manager
			id := in.Club
			if id == 0 {
				if m.ClubID == 0 {
					return nil, errFor(ErrUnemployedScope, "err.unemployed_scope", nil, nil)
				}
				id = m.ClubID
			}
			if g.clubByID(id) == nil {
				return nil, errFor(ErrNotFound, "err.not_found",
					map[string]any{"id": id}, map[string]any{"club": id})
			}
			detail := in.Detail
			if detail == "" {
				detail = "attributes"
			}
			own := id == m.ClubID

			rows := []map[string]any{}
			for i := range w.Players {
				p := &w.Players[i]
				if p.ClubID != id {
					continue
				}
				row := map[string]any{
					"player": p.ID, "name": p.Name, "age": p.Age,
					"position": p.Position, "youth": p.Youth,
					"body": map[string]any{
						"height_cm": p.HeightCm,
						"weight_kg": p.WeightKg,
					},
					"foot":      string(p.Foot),
					"weak_foot": g.weakFootProfile(m, w, p),
					"familiarity": g.descriptor("desc.familiarity." +
						familiarityKey(p)),
					"season_stats": seasonStats(p),
					// List views carry the BAND, not the raw ring — ratings are
					// public per match, but a squad scan should read as
					// descriptors (UNKNOWN under formMinSamples).
					"form": g.descriptor("desc.form." + formBand(p)),
				}
				switch detail {
				case "attributes":
					row["attributes"] = maskedVisible(m, w, p)
				case "condition":
					row["condition"], row["sharpness"] = p.Condition, p.Sharpness
				case "contracts":
					// Contract summary is own-club only (docs/11 §4 get_squad):
					// other clubs expose nothing here. get_person still surfaces
					// expiry-year alone for others.
					if own && p.Contract != nil {
						row["wage_weekly"] = money(p.Contract.WageWeeklyMinor)
						row["contract_expiry_season"] = p.Contract.ExpirySeasonYear
					}
				default:
					return nil, errFor(ErrValidation, "err.validation",
						map[string]any{"detail": "detail must be attributes|condition|contracts"}, nil)
				}
				rows = append(rows, row)
			}
			return map[string]any{"club": id, "detail": detail, "players": rows}, nil
		})
}

// ---- get_person ----

type personRef struct {
	Player  int64 `json:"player,omitempty"`
	Manager int64 `json:"manager,omitempty"`
}

type getPersonIn struct {
	Ref personRef `json:"ref"`
}

func (g *Gateway) getPerson(mid int64, sid string, in getPersonIn) map[string]any {
	return g.run(mid, sid, focus.GetPerson, in, flatCost(focus.GetPerson),
		func(cc *callCtx) (any, *apiError) {
			switch {
			case in.Ref.Player != 0:
				return g.personPlayer(cc, in.Ref.Player)
			case in.Ref.Manager != 0:
				return g.personManager(in.Ref.Manager)
			default:
				return nil, errFor(ErrValidation, "err.validation",
					map[string]any{"detail": "ref needs a player or manager"}, nil)
			}
		})
}

// participantIDs is a side's starters plus its sub-ons, in entry order — the
// archived-result mirror of LiveMatch.Participants.
func participantIDs(xi []int64, subs []worldgen.SubEvent, clubID int64) []int64 {
	out := append([]int64{}, xi...)
	for _, s := range subs {
		if s.ClubID == clubID && s.On != 0 {
			out = append(out, s.On)
		}
	}
	return out
}

// subRows renders a match's substitutions — public on-pitch events, id-keyed
// like scorers and cards. A row without "on" is an uncovered withdrawal (the
// side played short); "reason" is the TV-visible why (INJURY | FATIGUE |
// TACTICAL) when recorded.
func subRows(subs []worldgen.SubEvent) []map[string]any {
	out := make([]map[string]any, 0, len(subs))
	for _, s := range subs {
		row := map[string]any{"minute": s.Minute, "club": s.ClubID, "off": s.Off}
		if s.On != 0 {
			row["on"] = s.On
		}
		if s.Reason != "" {
			row["reason"] = s.Reason
		}
		out = append(out, row)
	}
	return out
}

// injuryHistory renders a player's past injuries: season + severity band only
// (FR-22 — never a duration or date).
func injuryHistory(g *Gateway, p *worldgen.Player) []any {
	out := []any{}
	for _, rec := range p.Injuries {
		out = append(out, map[string]any{
			"season":   rec.SeasonYear,
			"severity": g.descriptor("desc.injury." + rec.Band),
		})
	}
	return out
}

// careerHistory renders the archived per-season lines — history earned in
// play (docs/09 §4.4), populated by the rollover since the archival phase.
func careerHistory(g *Gateway, p *worldgen.Player) []any {
	out := []any{}
	for _, rec := range p.Career {
		row := map[string]any{
			"season": rec.SeasonYear, "apps": rec.Apps, "goals": rec.Goals,
		}
		if rec.Apps > 0 {
			row["rating_avg"] = publicRatingAverage(rec.RatingSumX10, rec.Apps)
		}
		if c := g.clubByID(rec.ClubID); c != nil {
			row["club"] = c.Name
		}
		out = append(out, row)
	}
	return out
}

// currentInjuryBand is the severity band of the player's live injury — the
// most recent history entry, which injureOne appends as it sets InjuredUntil.
func currentInjuryBand(p *worldgen.Player) string {
	if len(p.Injuries) == 0 {
		return "DAYS"
	}
	return p.Injuries[len(p.Injuries)-1].Band
}

func (g *Gateway) personPlayer(cc *callCtx, id int64) (any, *apiError) {
	w := g.Host.World()
	m := cc.manager
	var p *worldgen.Player
	for i := range w.Players {
		if w.Players[i].ID == id {
			p = &w.Players[i]
		}
	}
	if p == nil {
		return nil, errFor(ErrNotFound, "err.not_found",
			map[string]any{"id": id}, map[string]any{"player": id})
	}

	positions := map[string]any{}
	for pos, v := range p.Familiarity {
		positions[pos] = g.descriptor("desc.familiarity." +
			strconvUpper(attr.FamiliarityDescriptor(v)))
	}
	data := map[string]any{
		"player": p.ID,
		"name":   p.Name,
		"age":    p.Age,
		"body": map[string]any{
			"height_cm": p.HeightCm,
			"weight_kg": p.WeightKg,
		},
		"foot":           string(p.Foot),
		"weak_foot":      g.weakFootProfile(m, w, p),
		"position":       p.Position,
		"positions":      positions,
		"attributes":     maskedVisible(m, w, p),
		"youth":          p.Youth,
		"retired":        p.Retired,
		"condition":      p.Condition,
		"sharpness":      p.Sharpness,
		"season_stats":   seasonStats(p),
		"injury_history": injuryHistory(g, p),
		"career_history": careerHistory(g, p),
	}
	// The deep view gets the numbers behind the band: rolling average + sample
	// count (the raw ring itself stays internal — averages, not sequences).
	if n := len(p.FormX10); n > 0 {
		sum := 0
		for _, v := range p.FormX10 {
			sum += v
		}
		data["form"] = map[string]any{
			"rating_avg": publicRatingAverage(sum, n),
			"matches":    n,
			"band":       g.descriptor("desc.form." + formBand(p)),
		}
	}
	// A live injury: everyone sees the coarse severity band; ONLY the owning
	// manager sees the medical room's expected return date — the exact lay-off
	// derives from hidden attributes, and the date is that consequence, visible
	// where football-management games usually show it: to the player's own club.
	if p.InjuredUntil > cc.now {
		inj := map[string]any{"severity": g.descriptor("desc.injury." + currentInjuryBand(p))}
		if p.ClubID == m.ClubID && m.ClubID != 0 {
			inj["expected_return"] = gameTimeISO(p.InjuredUntil)
		}
		data["injury"] = inj
	}
	// A live ban is a public announced fact: everyone sees the remaining
	// matches.
	if p.SuspendedMatches > 0 {
		data["suspension"] = map[string]any{"matches": p.SuspendedMatches}
	}
	if c := g.clubByID(p.ClubID); c != nil {
		data["club"] = clubRef(c)
	}
	if p.Contract != nil {
		if p.ClubID == m.ClubID && m.ClubID != 0 {
			data["contract"] = map[string]any{
				"wage_weekly":   money(p.Contract.WageWeeklyMinor),
				"expiry_season": p.Contract.ExpirySeasonYear,
			}
		} else {
			data["contract"] = map[string]any{
				"expiry_season": p.Contract.ExpirySeasonYear,
			}
		}
	}
	// Hidden state surfaces as Descriptors and evidence only (FR-22).
	descriptors := []map[string]any{}
	if knowsPersonality(m, w, p) {
		key := attr.PersonalityDescriptor(p.Hidden, p.Visible)
		descriptors = append(descriptors, g.descriptor("desc.player."+key))
	}
	data["descriptors"] = descriptors
	evidence := []map[string]any{}
	if mk, ok := w.Knowledge[m.ID]; ok {
		if k, ok := mk[p.ID]; ok {
			for _, ev := range k.Evidence {
				evidence = append(evidence, map[string]any{
					"line":       g.renderMessage(ev.Key, ev.Params),
					"confidence": ev.Confidence,
					"game_time":  gameTimeISO(ev.GameTime),
				})
			}
		}
	}
	data["evidence"] = evidence
	return data, nil
}

func (g *Gateway) personManager(id int64) (any, *apiError) {
	target, ok := g.managers[id]
	if !ok {
		return nil, errFor(ErrNotFound, "err.not_found",
			map[string]any{"id": id}, map[string]any{"manager": id})
	}
	data := map[string]any{
		"manager":    target.ID,
		"name":       target.Name,
		"age":        target.Age,
		"reputation": g.descriptor("desc.reputation." + reputationBand(target.Reputation)),
		"style":      g.descriptor("desc.style." + styleKey(target.Archetype)),
		"career": map[string]any{
			"tenure_since": "1925-07", // careers begin at world start
		},
	}
	if c := g.clubByID(target.ClubID); c != nil {
		data["club"] = clubRef(c)
		data["security"] = g.descriptor("desc.security." + securityBand(c.Confidence))
	} else {
		data["club"] = nil
	}
	return data, nil
}

// ---- get_match ----

type getMatchIn struct {
	Match   int64 `json:"match,omitempty"`
	Fixture int64 `json:"fixture,omitempty"`
	Since   int   `json:"since,omitempty"` // live commentary cursor (index into the log)
}

func (g *Gateway) getMatch(mid int64, sid string, in getMatchIn) map[string]any {
	cost := func(cc *callCtx) int {
		// Ownership uses the same fixture|match id fallback as the handler, so
		// an own match requested via `match` is billed at the own-club rate,
		// not overcharged as an other-club read (docs/11 §4).
		id := in.Fixture
		if id == 0 {
			id = in.Match
		}
		own := false
		if f := g.fixtureByID(id); f != nil {
			own = f.HomeID == cc.manager.ClubID || f.AwayID == cc.manager.ClubID
		} else if r := g.Host.World().ArchivedResultFor(id); r != nil {
			// The rollover clears Fixtures, so an archived own fixture is
			// invisible to fixtureByID — without this fallback a manager
			// re-reading their own past match would be overcharged as an
			// other-club read.
			own = r.HomeID == cc.manager.ClubID || r.AwayID == cc.manager.ClubID
		}
		c, _ := focus.CostOwnOther(focus.GetMatch, own)
		return c
	}
	return g.run(mid, sid, focus.GetMatch, in, cost,
		func(cc *callCtx) (any, *apiError) {
			w := g.Host.World()
			id := in.Fixture
			if id == 0 {
				id = in.Match
			}
			if id == 0 {
				return nil, errFor(ErrValidation, "err.validation",
					map[string]any{"detail": "match or fixture id required"}, nil)
			}
			if res := w.ResultFor(id); res != nil {
				return g.finishedMatch(w, res), nil
			}
			if lm := w.LiveMatches[id]; lm != nil {
				return g.liveMatch(w, lm, cc.manager.ClubID, in.Since), nil
			}
			// Past seasons live in the History ledger: the full
			// finished view minus the commentary prose (not kept — the empty
			// list is honest), flagged archived with its season.
			if res := w.ArchivedResultFor(id); res != nil {
				out := g.finishedMatch(w, res)
				out["archived"] = true
				out["season"] = worldgen.DateOf(res.Kickoff).Season
				return out, nil
			}
			if f := g.fixtureByID(id); f != nil {
				out := fixtureRef(w, f)
				out["status"] = "SCHEDULED"
				return out, nil
			}
			return nil, errFor(ErrNotFound, "err.not_found",
				map[string]any{"id": id}, map[string]any{"match": id})
		})
}

// publicRatingAverage rounds an accumulated ×10 rating to two decimal places
// using integer arithmetic. World state remains integer-only, and Go's JSON
// encoder emits the resulting shortest decimal instead of a repeating quotient.
func publicRatingAverage(sumX10, matches int) float64 {
	if matches <= 0 {
		return 0
	}
	hundredths := (sumX10*10 + matches/2) / matches
	return float64(hundredths) / 100
}

// seasonStats renders a player's accumulated season line. The average rating
// derives from the ×10 integer sum, so no float enters the world hash.
func seasonStats(p *worldgen.Player) map[string]any {
	avg := publicRatingAverage(p.RatingSumX10, p.SeasonApps)
	return map[string]any{
		"apps": p.SeasonApps, "goals": p.SeasonGoals, "rating_avg": avg,
	}
}

// Form banding (tunable docs/98): the average of the rolling
// last-formWindow ratings, ×10 integer thresholds. Fewer than formMinSamples
// played reads UNKNOWN — two hot games aren't a streak.
const (
	formMinSamples  = 3
	formInThreshold = 70 // avg ≥ 7.0 → IN_FORM
	formOkThreshold = 64 // avg ≥ 6.4 → STEADY, below → OUT_OF_FORM
)

func formBand(p *worldgen.Player) string {
	if len(p.FormX10) < formMinSamples {
		return "UNKNOWN"
	}
	sum := 0
	for _, v := range p.FormX10 {
		sum += v
	}
	avg := sum / len(p.FormX10)
	switch {
	case avg >= formInThreshold:
		return "IN_FORM"
	case avg >= formOkThreshold:
		return "STEADY"
	default:
		return "OUT_OF_FORM"
	}
}

// playerIndex builds an id→player lookup for rendering a lineup once per call.
func playerIndex(w *worldgen.World) map[int64]*worldgen.Player {
	idx := make(map[int64]*worldgen.Player, len(w.Players))
	for i := range w.Players {
		idx[w.Players[i].ID] = &w.Players[i]
	}
	return idx
}

func matchEventRow(e worldgen.MatchEvent) map[string]any {
	row := map[string]any{"minute": e.Minute, "player": e.PlayerID, "club": e.ClubID}
	if e.Detail != "" {
		row["detail"] = e.Detail
	}
	return row
}

// finishedMatch renders a completed fixture: score, scorers, cards, and both
// lineups with player ratings (×10 stored → decimal on the wire).
func (g *Gateway) finishedMatch(w *worldgen.World, r *worldgen.MatchResult) map[string]any {
	idx := playerIndex(w)
	lineup := func(xi []int64) []map[string]any {
		rows := make([]map[string]any, 0, len(xi))
		for _, pid := range xi {
			row := map[string]any{"player": pid}
			if p := idx[pid]; p != nil {
				row["name"], row["position"] = p.Name, p.Position
			}
			if rt, ok := r.RatingsX10[pid]; ok {
				row["rating"] = float64(rt) / 10
			}
			rows = append(rows, row)
		}
		return rows
	}
	scorers := make([]map[string]any, 0, len(r.Scorers))
	for _, s := range r.Scorers {
		scorers = append(scorers, matchEventRow(s))
	}
	cards := make([]map[string]any, 0, len(r.Cards))
	for _, c := range r.Cards {
		cards = append(cards, matchEventRow(c))
	}
	out := map[string]any{
		"fixture": r.FixtureID, "status": "FINISHED", "competition": r.Competition,
		"home": clubRef(g.clubByID(r.HomeID)), "away": clubRef(g.clubByID(r.AwayID)),
		"home_goals": r.HomeGoals, "away_goals": r.AwayGoals,
		"game_time": gameTimeISO(r.Kickoff),
		"scorers":   scorers, "cards": cards,
		// Lineups list everyone who played — starters plus sub-ons — so every
		// rated participant has a row; the subs ledger below
		// says who replaced whom and when.
		"home_lineup": lineup(participantIDs(r.HomeXI, r.Subs, r.HomeID)),
		"away_lineup": lineup(participantIDs(r.AwayXI, r.Subs, r.AwayID)),
		"subs":        subRows(r.Subs),
		"commentary":  g.renderCommentary(r.Commentary),
		"adjustments": adjustmentRows(r.Adjustments),
		"stats":       g.matchStats(r.HomeShots, r.AwayShots, r.ChanceTypes, r.ChanceTypesBySide, r.Diagnostics),
	}
	// Cup ties carry the advancing club — decisive or on penalties.
	if r.Winner != 0 {
		out["winner"] = clubRef(g.clubByID(r.Winner))
	}
	return out
}

// liveCommentaryWindow is how many trailing commentary lines a fresh live poll
// returns (docs/11 get_match live "last ~20 lines").
const liveCommentaryWindow = 20

// liveMatch renders an in-progress fixture. Score, clock, the commentary log,
// a stats snapshot, and the in-match adjustments are all public — a live match
// is a public spectacle and a tactical shift is a visible on-pitch action the
// commentary already narrates. Only the own-team-state block (per-player
// condition, bookings) is scoped to the viewer's own match — a convenience
// aggregation of already-public data, not a privacy boundary. `since` is a
// stable integer cursor into the persisted commentary slice.
func (g *Gateway) liveMatch(w *worldgen.World, lm *worldgen.LiveMatch, viewerClub int64, since int) map[string]any {
	scorers := make([]map[string]any, 0, len(lm.Scorers))
	for _, s := range lm.Scorers {
		scorers = append(scorers, matchEventRow(s))
	}
	// Cards are as public live as finished (a red visibly ejects — without
	// this row an ejected player would just vanish from the derived on-pitch
	// set with no structured fact behind it.
	cards := make([]map[string]any, 0, len(lm.Cards))
	for _, c := range lm.Cards {
		cards = append(cards, matchEventRow(c))
	}
	from := len(lm.Commentary) - liveCommentaryWindow
	if since > 0 {
		from = since
	}
	if from < 0 {
		from = 0
	}
	if from > len(lm.Commentary) {
		from = len(lm.Commentary)
	}
	out := map[string]any{
		"fixture": lm.FixtureID, "status": "LIVE", "competition": lm.Competition,
		"home": clubRef(g.clubByID(lm.HomeID)), "away": clubRef(g.clubByID(lm.AwayID)),
		"home_goals": lm.HomeGoals, "away_goals": lm.AwayGoals,
		"minute": lm.Clock, "scorers": scorers, "cards": cards, "subs": subRows(lm.Subs),
		"commentary":  g.renderCommentary(lm.Commentary[from:]),
		"cursor":      len(lm.Commentary),
		"stats":       g.matchStats(lm.HomeShots, lm.AwayShots, lm.ChanceTypes, lm.ChanceTypesBySide, lm.Diagnostics),
		"adjustments": adjustmentRows(lm.Adjustments),
	}
	if viewerClub != 0 && (lm.HomeID == viewerClub || lm.AwayID == viewerClub) {
		// The CURRENT on-pitch set, not the starting XI — a sub-on appears the
		// moment they enter and a withdrawn player drops out.
		out["own_team"] = g.ownTeamState(w, lm, viewerClub)
	}
	return out
}

// renderCommentary renders stored commentary lines to {minute, line} rows.
func (g *Gateway) renderCommentary(lines []worldgen.CommentaryLine) []map[string]any {
	out := make([]map[string]any, 0, len(lines))
	for _, l := range lines {
		out = append(out, map[string]any{"minute": l.Minute, "line": g.renderMessage(l.Key, l.Params)})
	}
	return out
}

func adjustmentRows(adjs []worldgen.Adjustment) []map[string]any {
	out := make([]map[string]any, 0, len(adjs))
	for _, a := range adjs {
		out = append(out, map[string]any{"minute": a.Minute, "club": a.ClubID, "key": a.Key})
	}
	return out
}

// ownTeamState is the viewer's own-match dashboard: each current on-pitch
// player's derived live condition and any bookings picked up (own-match intel
// — never shown for a rival's live match).
func (g *Gateway) ownTeamState(w *worldgen.World, lm *worldgen.LiveMatch, club int64) map[string]any {
	booked := map[int64]string{}
	for _, c := range lm.Cards {
		if c.ClubID == club {
			booked[c.PlayerID] = c.Detail
		}
	}
	idx := playerIndex(w)
	xi := lm.OnPitch(club)
	players := make([]map[string]any, 0, len(xi))
	for _, pid := range xi {
		row := map[string]any{"player": pid}
		if p := idx[pid]; p != nil {
			row["name"], row["condition"] = p.Name, engine.LivePlayerCondition(p, lm)
		}
		if card, ok := booked[pid]; ok {
			row["card"] = card
		}
		players = append(players, row)
	}
	return map[string]any{"players": players}
}

// ---- search_players ----

type searchPlayersIn struct {
	Position       string `json:"position,omitempty"`
	AgeMin         int    `json:"age_min,omitempty"`
	AgeMax         int    `json:"age_max,omitempty"`
	MaxFee         int64  `json:"max_fee,omitempty"`  // inert until the transfer engine (roadmap 5)
	MaxWage        int64  `json:"max_wage,omitempty"` // Crowns minor units
	Division       int    `json:"division,omitempty"`
	ContractStatus string `json:"contract_status,omitempty"` // expiring|listed|free_agent
	Sort           string `json:"sort,omitempty"`            // value|age (default value)
	Limit          int    `json:"limit,omitempty"`           // ≤30
}

func (g *Gateway) searchPlayers(mid int64, sid string, in searchPlayersIn) map[string]any {
	return g.run(mid, sid, focus.SearchPlayers, in, flatCost(focus.SearchPlayers),
		func(cc *callCtx) (any, *apiError) {
			w := g.Host.World()
			m := cc.manager
			limit := in.Limit
			if limit <= 0 || limit > 30 {
				limit = 30
			}
			switch in.Sort {
			case "", "value", "age":
			default:
				return nil, errFor(ErrValidation, "err.validation",
					map[string]any{"detail": "sort must be value|age"}, nil)
			}

			var listedSet map[int64]bool
			if in.ContractStatus == "listed" {
				listedSet = worldgen.ExplicitlyListed(w)
			}

			var matches []*worldgen.Player
			for i := range w.Players {
				p := &w.Players[i]
				if p.Youth || p.Retired {
					// A retired player shares ClubID 0 with real free agents —
					// without the flag check they would surface as signable.
					continue
				}
				if in.Position != "" && p.Position != in.Position {
					continue
				}
				if in.AgeMin != 0 && p.Age < in.AgeMin {
					continue
				}
				if in.AgeMax != 0 && p.Age > in.AgeMax {
					continue
				}
				if in.MaxWage != 0 && p.Contract != nil && p.Contract.WageWeeklyMinor > in.MaxWage {
					continue
				}
				if in.Division != 0 {
					c := g.clubByID(p.ClubID)
					if c == nil || c.DivisionTier != in.Division {
						continue
					}
				}
				switch in.ContractStatus {
				case "expiring":
					if p.Contract == nil || p.Contract.ExpirySeasonYear != worldgen.DateOf(cc.now).Season {
						continue
					}
				case "free_agent":
					if p.ClubID != 0 {
						continue
					}
				case "listed":
					// Explicitly on the market — a club has put the player up for
					// sale (worldgen.ExplicitlyListed), which is what an agent's
					// explicit SIGN can complete. Surplus castoffs are traded only by
					// the autonomous market, so they are not shown here. docs/11 §4.
					if !listedSet[p.ID] {
						continue
					}
				case "":
				default:
					return nil, errFor(ErrValidation, "err.validation",
						map[string]any{"detail": "contract_status must be expiring|listed|free_agent"}, nil)
				}
				matches = append(matches, p)
			}

			switch in.Sort {
			case "age":
				sort.Slice(matches, func(i, j int) bool {
					if matches[i].Age != matches[j].Age {
						return matches[i].Age < matches[j].Age
					}
					return matches[i].ID < matches[j].ID
				})
			default: // value desc — search finds, scout reveals. Rank by the
				// viewer's MASKED pool bucket, not raw pool: same-bucket
				// unscouted players tie and break on id, so the ordering never
				// leaks exact intra-bucket precision (FR-22).
				sort.Slice(matches, func(i, j int) bool {
					bi := bucketedPool(matches[i].AbilityPool, effectiveLevel(m, w, matches[i]))
					bj := bucketedPool(matches[j].AbilityPool, effectiveLevel(m, w, matches[j]))
					if bi != bj {
						return bi > bj
					}
					return matches[i].ID < matches[j].ID
				})
			}
			if len(matches) > limit {
				matches = matches[:limit]
			}

			rows := []map[string]any{}
			for _, p := range matches {
				clubName := ""
				if c := g.clubByID(p.ClubID); c != nil {
					clubName = c.Name
				}
				rows = append(rows, map[string]any{
					"player": p.ID, "name": p.Name, "age": p.Age,
					"position": p.Position, "club": clubName,
					"foot":                string(p.Foot),
					"weak_foot":           g.weakFootProfile(m, w, p),
					"headline_attributes": headlineAttrs(m, w, p),
					"value_band":          valueBand(p.AbilityPool, effectiveLevel(m, w, p)),
					"flags": map[string]any{
						"free_agent": p.ClubID == 0,
						"expiring":   p.Contract != nil && p.Contract.ExpirySeasonYear == worldgen.DateOf(cc.now).Season,
					},
				})
			}
			return map[string]any{"players": rows}, nil
		})
}

// ---- scout ----

type scoutIn struct {
	Target  personRef `json:"target,omitempty"`
	Profile string    `json:"profile,omitempty"` // a concrete position, e.g. "ST"
}

func (g *Gateway) scout(mid int64, sid string, in scoutIn) map[string]any {
	return g.run(mid, sid, focus.Scout, in, flatCost(focus.Scout),
		func(cc *callCtx) (any, *apiError) {
			m := cc.manager
			if m.ClubID == 0 { // no club, no scouts (docs/11 §7)
				return nil, errFor(ErrUnemployedScope, "err.unemployed_scope", nil, nil)
			}
			var spec string
			switch {
			case in.Target.Player != 0:
				var target *worldgen.Player
				for i := range g.Host.World().Players {
					if g.Host.World().Players[i].ID == in.Target.Player {
						target = &g.Host.World().Players[i]
						break
					}
				}
				if target == nil {
					return nil, errFor(ErrNotFound, "err.not_found",
						map[string]any{"id": in.Target.Player},
						map[string]any{"player": in.Target.Player})
				}
				if target.Retired {
					return nil, errFor(ErrValidation, "err.validation",
						map[string]any{"detail": "player has retired"},
						map[string]any{"player": in.Target.Player})
				}
				spec = fmt.Sprintf("p%d", in.Target.Player)
			case in.Profile != "":
				spec = "profile:" + in.Profile
			default:
				return nil, errFor(ErrValidation, "err.validation",
					map[string]any{"detail": "scout needs a target or a profile"}, nil)
			}
			due := g.Host.Engine().ScheduleScout(m.ID, spec, cc.now)
			return map[string]any{
				"commissioned": true,
				"report_due":   gameTimeISO(due),
			}, nil
		})
}

// ---- shared lookups ----

func (g *Gateway) clubByID(id int64) *worldgen.Club {
	if id == 0 {
		return nil
	}
	w := g.Host.World()
	for i := range w.Clubs {
		if w.Clubs[i].ID == id {
			return &w.Clubs[i]
		}
	}
	return nil
}

func (g *Gateway) playerByID(id int64) *worldgen.Player {
	if id == 0 {
		return nil
	}
	w := g.Host.World()
	for i := range w.Players {
		if w.Players[i].ID == id {
			return &w.Players[i]
		}
	}
	return nil
}

func (g *Gateway) fixtureByID(id int64) *worldgen.Fixture {
	if id == 0 {
		return nil
	}
	w := g.Host.World()
	for i := range w.Fixtures {
		if w.Fixtures[i].ID == id {
			return &w.Fixtures[i]
		}
	}
	return nil
}

func (g *Gateway) tierOf(m *worldgen.Manager) int {
	if c := g.clubByID(m.ClubID); c != nil {
		return c.DivisionTier
	}
	return 1
}

func (g *Gateway) targetsOwnClub(m *worldgen.Manager, requested int64) bool {
	return requested == 0 || (m.ClubID != 0 && requested == m.ClubID)
}

func (g *Gateway) renderMessage(key string, params map[string]any) map[string]any {
	if params == nil {
		params = map[string]any{}
	}
	return map[string]any{
		"key":    key,
		"params": params,
		"text":   g.renderMessageText(narrative.LocaleEN, key, params),
	}
}

func (g *Gateway) renderMessageText(loc narrative.Locale, key string, params map[string]any) string {
	resolved := make(map[string]any, len(params))
	for k, v := range params {
		resolved[k] = v
	}
	if a, ok := resolved["attr_key"].(string); ok {
		resolved["attr"] = g.Catalogs.Render(loc, "attr."+a, nil)
		delete(resolved, "attr_key")
	}
	if wk, ok := resolved["window_key"].(string); ok {
		resolved["window"] = g.Catalogs.Render(loc, "term.window."+wk, nil)
		delete(resolved, "window_key")
	}
	if comp, ok := resolved["competition"].(string); ok {
		resolved["competition"] = g.Catalogs.Render(loc, "term.competition."+comp, nil)
	}
	if club, ok := resolved["club"].(string); ok && club == "" {
		resolved["club"] = g.Catalogs.Render(loc, "term.free_agent", nil)
	}
	return g.Catalogs.Render(loc, key, resolved)
}

func clubRef(c *worldgen.Club) map[string]any {
	if c == nil {
		return nil
	}
	return map[string]any{"club": c.ID, "name": c.Name}
}

func lastSeasonPos(w *worldgen.World, clubID int64) int {
	for _, table := range w.LastSeason {
		for _, row := range table {
			if row.ClubID == clubID {
				return row.Pos
			}
		}
	}
	return 0
}

// currentTable is the live current-season standings for a division,
// falling back to last season only if the live table is somehow absent.
func (g *Gateway) currentTable(w *worldgen.World, tier int) []worldgen.Standing {
	if tier >= 1 && tier <= len(w.Table) {
		return w.Table[tier-1]
	}
	if tier >= 1 && tier <= len(w.LastSeason) {
		return w.LastSeason[tier-1]
	}
	return nil
}

// currentPos is a club's live league position, or its last-season finish
// before a ball is kicked.
func (g *Gateway) currentPos(w *worldgen.World, clubID int64) int {
	for _, table := range w.Table {
		for _, row := range table {
			if row.ClubID == clubID {
				return row.Pos
			}
		}
	}
	return lastSeasonPos(w, clubID)
}

// clubRecentResults returns a club's newest finished results (any competition)
// as condensed rows — the dashboard's "last N" line.
func (g *Gateway) clubRecentResults(w *worldgen.World, clubID int64, limit int) []map[string]any {
	rows := []map[string]any{}
	for i := len(w.Results) - 1; i >= 0 && len(rows) < limit; i-- {
		r := &w.Results[i]
		if r.HomeID != clubID && r.AwayID != clubID {
			continue
		}
		rows = append(rows, map[string]any{
			"fixture":     r.FixtureID,
			"home":        clubRef(g.clubByID(r.HomeID)),
			"away":        clubRef(g.clubByID(r.AwayID)),
			"home_goals":  r.HomeGoals,
			"away_goals":  r.AwayGoals,
			"competition": r.Competition,
		})
	}
	return rows
}

// formString derives a club's recent league form (e.g. "WWDLW", oldest→newest,
// last 5) from the ordered Results slice at read time — never stored, so it
// can't drift from the results it summarizes.
func formString(w *worldgen.World, clubID int64) string {
	var form []byte
	for i := range w.Results {
		r := &w.Results[i]
		if r.Competition != worldgen.CompetitionLeague {
			continue
		}
		var scored, conceded int
		switch clubID {
		case r.HomeID:
			scored, conceded = r.HomeGoals, r.AwayGoals
		case r.AwayID:
			scored, conceded = r.AwayGoals, r.HomeGoals
		default:
			continue
		}
		switch {
		case scored > conceded:
			form = append(form, 'W')
		case scored == conceded:
			form = append(form, 'D')
		default:
			form = append(form, 'L')
		}
	}
	if len(form) > 5 {
		form = form[len(form)-5:]
	}
	return string(form)
}

// recentResults returns the newest finished league results in a division as
// condensed rows (clubs + score), oldest-to-newest within the returned window.
func (g *Gateway) recentResults(w *worldgen.World, tier, limit int) []map[string]any {
	rows := []map[string]any{}
	for i := len(w.Results) - 1; i >= 0 && len(rows) < limit; i-- {
		r := &w.Results[i]
		if r.Competition != worldgen.CompetitionLeague || r.DivisionTier != tier {
			continue
		}
		rows = append(rows, map[string]any{
			"fixture":    r.FixtureID,
			"home":       clubRef(g.clubByID(r.HomeID)),
			"away":       clubRef(g.clubByID(r.AwayID)),
			"home_goals": r.HomeGoals,
			"away_goals": r.AwayGoals,
			"game_time":  gameTimeISO(r.Kickoff),
		})
	}
	return rows
}

// topByPool picks a club's n headline players for the viewer. Selection and
// ordering key on the viewer's MASKED pool bucket, not raw pool: for another
// club's players (unscouted) same-bucket ties fall to id, so a public profile
// never leaks exact intra-bucket ranking (FR-22, docs/11 §4). Own-squad /
// fully-scouted players resolve exactly, as earned.
func topByPool(w *worldgen.World, viewer *worldgen.Manager, clubID int64, n int) []*worldgen.Player {
	var squad []*worldgen.Player
	for i := range w.Players {
		p := &w.Players[i]
		if p.ClubID == clubID && !p.Youth {
			squad = append(squad, p)
		}
	}
	sort.Slice(squad, func(i, j int) bool {
		bi := bucketedPool(squad[i].AbilityPool, effectiveLevel(viewer, w, squad[i]))
		bj := bucketedPool(squad[j].AbilityPool, effectiveLevel(viewer, w, squad[j]))
		if bi != bj {
			return bi > bj
		}
		return squad[i].ID < squad[j].ID
	})
	if len(squad) > n {
		squad = squad[:n]
	}
	return squad
}

// headlineAttrs picks the player's two strongest visible attributes and
// masks them for the viewer (search shows what stands out, not the sheet).
func headlineAttrs(m *worldgen.Manager, w *worldgen.World, p *worldgen.Player) map[string]any {
	// Rank by the viewer's MASKED value, not raw: for an unscouted player,
	// choosing the two headline attributes by exact value would leak their
	// intra-bucket ranking through *which* attributes surface (FR-22). Ties
	// (same bucket) break on the attribute identity — public, not value-derived.
	level := effectiveLevel(m, w, p)
	type av struct {
		a    attr.Visible
		rank int
	}
	list := make([]av, 0, len(p.Visible))
	for a, v := range p.Visible {
		rank := v
		if level < 3 {
			rank, _ = bucketRange(v, knowledgeBuckets[level])
		}
		list = append(list, av{a, rank})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].rank != list[j].rank {
			return list[i].rank > list[j].rank
		}
		return list[i].a < list[j].a
	})
	masked := maskedVisible(m, w, p)
	out := map[string]any{}
	for _, e := range list[:min(2, len(list))] {
		out[string(e.a)] = masked[string(e.a)]
	}
	return out
}

func familiarityKey(p *worldgen.Player) string {
	return strconvUpper(attr.FamiliarityDescriptor(p.Familiarity[p.Position]))
}

func (g *Gateway) matchStats(homeShots, awayShots int, chanceTypes, chanceTypesBySide map[string]int, diag worldgen.MatchDiagnostics) map[string]any {
	stats := map[string]any{
		"home_shots": homeShots,
		"away_shots": awayShots,
	}
	if patterns := g.matchPatternRows(chanceTypes, chanceTypesBySide); len(patterns) > 0 {
		stats["match_patterns"] = patterns
	}
	if quality := shotQualityRows(diag); len(quality) > 0 {
		stats["shot_quality"] = quality
	}
	if sides := sideCountRows(diag.AerialDuels); len(sides) > 0 {
		stats["aerial_duels"] = sides
	}
	if sides := sideCountRows(diag.AerialWins); len(sides) > 0 {
		stats["aerial_wins"] = sides
	}
	if sides := sideCountRows(diag.PressTurnovers); len(sides) > 0 {
		stats["press_turnovers"] = sides
	}
	if sides := sideCountRows(diag.SetPieceThreat); len(sides) > 0 {
		stats["set_piece_threat"] = sides
	}
	if tilt := tacticalTiltRows(diag.TacticalTilt); len(tilt) > 0 {
		stats["tactical_tilt"] = tilt
	}
	return stats
}

func shotQualityRows(diag worldgen.MatchDiagnostics) []map[string]any {
	if len(diag.ShotQualityBySide) == 0 {
		out := make([]map[string]any, 0, len(diag.ShotQuality))
		for _, band := range []string{"HIGH", "MEDIUM", "LOW"} {
			if n := diag.ShotQuality[band]; n > 0 {
				out = append(out, map[string]any{"side": "UNKNOWN", "band": band, "count": n})
			}
		}
		return out
	}
	out := make([]map[string]any, 0, len(diag.ShotQualityBySide))
	for _, side := range []string{"HOME", "AWAY"} {
		for _, band := range []string{"HIGH", "MEDIUM", "LOW"} {
			if n := diag.ShotQualityBySide[side+"_"+band]; n > 0 {
				out = append(out, map[string]any{"side": side, "band": band, "count": n})
			}
		}
	}
	// A match may straddle an upgrade: old moments exist only in the aggregate,
	// while new moments have a side. Preserve the unattributed remainder rather
	// than dropping it or guessing which team took those shots.
	for _, band := range []string{"HIGH", "MEDIUM", "LOW"} {
		attributed := diag.ShotQualityBySide["HOME_"+band] + diag.ShotQualityBySide["AWAY_"+band]
		if unknown := diag.ShotQuality[band] - attributed; unknown > 0 {
			out = append(out, map[string]any{"side": "UNKNOWN", "band": band, "count": unknown})
		}
	}
	return out
}

func sideCountRows(counts map[string]int) []map[string]any {
	out := make([]map[string]any, 0, 2)
	for _, side := range []string{"HOME", "AWAY"} {
		if n := counts[side]; n > 0 {
			out = append(out, map[string]any{"side": side, "count": n})
		}
	}
	return out
}

func tacticalTiltRows(counts map[string]int) []map[string]any {
	type pair struct {
		side   string
		family string
		val    int
	}
	pairs := make([]pair, 0, len(counts))
	for k, v := range counts {
		if v <= 0 {
			continue
		}
		if !strings.HasPrefix(k, "HOME_") && !strings.HasPrefix(k, "AWAY_") {
			continue
		}
		side, family := k[:4], k[5:]
		pairs = append(pairs, pair{side: side, family: family, val: v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].val != pairs[j].val {
			return pairs[i].val > pairs[j].val
		}
		if pairs[i].side != pairs[j].side {
			return pairs[i].side < pairs[j].side
		}
		return pairs[i].family < pairs[j].family
	})
	out := make([]map[string]any, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, map[string]any{"side": p.side, "pattern": p.family, "count": p.val})
	}
	return out
}

func (g *Gateway) matchPatternRows(types, bySide map[string]int) []map[string]any {
	type pair struct {
		side string
		key  string
		val  int
	}
	pairs := make([]pair, 0, len(bySide)+len(types))
	if len(bySide) > 0 {
		for _, side := range []string{"HOME", "AWAY"} {
			prefix := side + "_"
			for k, v := range bySide {
				if v > 0 && strings.HasPrefix(k, prefix) {
					pairs = append(pairs, pair{side: side, key: strings.TrimPrefix(k, prefix), val: v})
				}
			}
		}
		// Preserve the unattributed aggregate remainder for matches that span
		// this upgrade. Side-aware values are authoritative if they exceed an
		// old or malformed aggregate; a negative remainder is never emitted.
		for k, total := range types {
			attributed := bySide["HOME_"+k] + bySide["AWAY_"+k]
			if unknown := total - attributed; unknown > 0 {
				pairs = append(pairs, pair{side: "UNKNOWN", key: k, val: unknown})
			}
		}
	} else {
		for k, v := range types {
			if v > 0 {
				pairs = append(pairs, pair{side: "UNKNOWN", key: k, val: v})
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		sideRank := func(side string) int {
			switch side {
			case "HOME":
				return 0
			case "AWAY":
				return 1
			default:
				return 2
			}
		}
		if sideRank(pairs[i].side) != sideRank(pairs[j].side) {
			return sideRank(pairs[i].side) < sideRank(pairs[j].side)
		}
		if pairs[i].val != pairs[j].val {
			return pairs[i].val > pairs[j].val
		}
		return pairs[i].key < pairs[j].key
	})
	out := make([]map[string]any, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, map[string]any{
			"side":    p.side,
			"pattern": p.key,
			"label":   g.Catalogs.Render(narrative.LocaleEN, "term.chance_type."+p.key, nil),
			"count":   p.val,
		})
	}
	return out
}

func (g *Gateway) weakFootProfile(m *worldgen.Manager, w *worldgen.World, p *worldgen.Player) map[string]any {
	key := strconvUpper(attr.WeakFootDescriptor(p.WeakFoot))
	out := map[string]any{"descriptor": g.descriptor("desc.weak_foot." + key)}
	level := effectiveLevel(m, w, p)
	if level >= 3 {
		out["value"] = p.WeakFoot
		return out
	}
	lo, hi := bucketRange(p.WeakFoot, knowledgeBuckets[level])
	out["range"] = []int{lo, hi}
	return out
}

func strconvUpper(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '-' {
			out[i] = '_'
			continue
		}
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

// styleKey maps archetype display names to catalog keys.
func styleKey(archetype string) string {
	switch archetype {
	case "The Idealist":
		return "IDEALIST"
	case "The Pragmatist":
		return "PRAGMATIST"
	case "The Firefighter":
		return "FIREFIGHTER"
	case "The Trader":
		return "TRADER"
	case "The Professor":
		return "PROFESSOR"
	case "The Motivator":
		return "MOTIVATOR"
	case "The Tyrant":
		return "TYRANT"
	case "The Gambler":
		return "GAMBLER"
	default:
		return "PRAGMATIST"
	}
}
