// Package consoleapi is the core's second interface (docs/05): view streams
// and admin operations over HTTP+JSON with SSE for live feeds. It never
// exposes gameplay verbs — shaping stays MCP-only (FR-30).
//
// Read models are built field-by-field into DTOs that structurally cannot
// carry hidden attributes (FR-22): no Hidden map, no Ability Pool, no
// Potential Cap, no Reputation, no club tendencies, no seed. The leak test
// scans every viewer endpoint for the forbidden vocabulary.
package consoleapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/narrative"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

type runtimeSettingsValidationError struct {
	Key string
}

func (e runtimeSettingsValidationError) Error() string { return e.Key }

// Server hosts the Console API. Viewer endpoints are unauthenticated
// (FR-32); admin endpoints require the Admin Token (FR-33).
type Server struct {
	AdminToken string
	Host       Host
	Feed       *Hub
	Catalogs   narrative.Catalogs

	// HeartbeatInterval paces SSE keep-alives (docs/05 A11: streams stay
	// open across pauses with heartbeats). Tests shorten it.
	HeartbeatInterval time.Duration
}

func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)

	mux.HandleFunc("GET /v1/world", s.handleWorld)
	mux.HandleFunc("GET /v1/ui", s.handleUI)
	mux.HandleFunc("GET /v1/news", s.handleNews)
	mux.HandleFunc("GET /v1/tables", s.handleTables)
	mux.HandleFunc("GET /v1/clubs", s.handleClubs)
	mux.HandleFunc("GET /v1/clubs/{id}", s.handleClub)
	mux.HandleFunc("GET /v1/fixtures", s.handleFixtures)
	mux.HandleFunc("GET /v1/matches/{id}", s.handleMatch)
	mux.HandleFunc("GET /v1/matches/live", s.handleLiveMatches)
	mux.HandleFunc("GET /v1/feed", s.handleFeed)

	mux.HandleFunc("GET /v1/admin/status", s.admin(s.handleAdminStatus))
	mux.HandleFunc("GET /v1/admin/settings", s.admin(s.handleAdminSettings))
	mux.HandleFunc("PATCH /v1/admin/settings", s.admin(s.handleAdminSettingsPatch))
	mux.HandleFunc("GET /v1/admin/managers", s.admin(s.handleAdminManagers))
	mux.HandleFunc("POST /v1/admin/start", s.admin(s.handleAdminStart))
	mux.HandleFunc("POST /v1/admin/pause", s.admin(s.handleAdminPause(true)))
	mux.HandleFunc("POST /v1/admin/resume", s.admin(s.handleAdminPause(false)))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) locale(r *http.Request) narrative.Locale {
	return narrative.ResolveTag(r.URL.Query().Get("locale"))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// httpError writes a localized error body: the stable key plus the text
// rendered for the request locale (FR-35c applies to failure states too).
func (s *Server) httpError(w http.ResponseWriter, r *http.Request, code int, key string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error_key": key,
		"error":     s.Catalogs.Render(s.locale(r), key, nil),
	})
}

// ---- Viewer: world summary ----

type worldDTO struct {
	Name             string            `json:"name"`
	State            string            `json:"state"`
	Tempo            string            `json:"tempo"`
	TempoLabel       string            `json:"tempo_label"`
	GameTime         int64             `json:"game_time"`
	ClockText        string            `json:"clock_text"`
	Date             worldgen.GameDate `json:"date"`
	Divisions        int               `json:"divisions"`
	ClubsPerDivision int               `json:"clubs_per_division"`
	Clubs            int               `json:"clubs"`
	Players          int               `json:"players"`
	Managers         int               `json:"managers"`
}

func (s *Server) handleWorld(w http.ResponseWriter, r *http.Request) {
	loc := s.locale(r)
	var dto worldDTO
	s.Host.Locked(func() {
		wd := s.Host.World()
		now := s.Host.Engine().Now()
		tempo := s.Host.Tempo()
		dto = worldDTO{
			Name:             wd.Config.Name,
			State:            s.Host.State(),
			Tempo:            tempo.String(),
			TempoLabel:       s.Catalogs.Render(loc, "term.tempo."+tempo.String(), nil),
			GameTime:         int64(now),
			ClockText:        renderClock(s.Catalogs, loc, now),
			Date:             worldgen.DateOf(now),
			Divisions:        wd.Config.Divisions,
			ClubsPerDivision: wd.Config.ClubsPerDivision,
			Clubs:            len(wd.Clubs),
			Players:          len(wd.Players),
			Managers:         len(wd.Managers),
		}
	})
	writeJSON(w, dto)
}

// ---- Viewer: UI strings (the Console stays catalog-free, docs/07 §6) ----

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	loc := s.locale(r)
	strs := map[string]string{}
	for key := range s.Catalogs[narrative.LocaleEN] {
		if strings.HasPrefix(key, "ui.") || strings.HasPrefix(key, "term.") ||
			strings.HasPrefix(key, "attr.") || strings.HasPrefix(key, "desc.") {
			strs[key] = s.Catalogs.Render(loc, key, nil)
		}
	}
	writeJSON(w, map[string]any{"locale": loc, "strings": strs})
}

// ---- Viewer: media/news ----

type newsArticleDTO struct {
	ID            int64   `json:"id"`
	GameTime      int64   `json:"game_time"`
	TimeText      string  `json:"time_text"`
	Category      string  `json:"category"`
	CategoryLabel string  `json:"category_label"`
	Source        string  `json:"source"`
	Title         string  `json:"title"`
	Deck          string  `json:"deck"`
	Body          string  `json:"body"`
	Refs          []int64 `json:"refs,omitempty"`
}

func (s *Server) handleNews(w http.ResponseWriter, r *http.Request) {
	loc := s.locale(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 30
	}
	if limit > viewerHistoryLimit {
		limit = viewerHistoryLimit
	}
	out := make([]newsArticleDTO, 0, limit)
	s.Host.Locked(func() {
		wd := s.Host.World()
		for i := len(wd.News) - 1; i >= 0 && len(out) < limit; i-- {
			n := &wd.News[i]
			if n.ManagerID != 0 {
				continue // private scout reports are manager inbox items, not public media.
			}
			if n.Key == "feed.matchday.preview" {
				continue // Kickoff schedule briefings are useful operationally, but noisy as media articles.
			}
			out = append(out, s.newsArticleDTO(loc, n))
		}
	})
	writeJSON(w, map[string]any{"items": out})
}

func (s *Server) newsArticleDTO(loc narrative.Locale, n *worldgen.NewsItem) newsArticleDTO {
	params := consoleNewsParams(n.Params)
	title := s.renderNewsText(loc, n.Key, params)
	class := n.Category
	switch class {
	case "transfer", "match", "injury", "board", "decision", "career", "youth", "contract", "scout":
	default:
		class = "media"
	}
	sourceClass := class
	articleParams := map[string]any{"headline": title}
	if n.Key == "feed.matchday.results" {
		class = "matchday.results"
		title = s.Catalogs.Render(loc, narrative.ArticleTemplateKey("title", class, n.ID), params)
		articleParams = s.matchdayResultsArticleParams(loc, params, title)
	}
	return newsArticleDTO{
		ID:            n.ID,
		GameTime:      int64(n.GameTime),
		TimeText:      renderClock(s.Catalogs, loc, n.GameTime),
		Category:      n.Category,
		CategoryLabel: s.Catalogs.Render(loc, "news.category."+class, nil),
		Source:        s.Catalogs.Render(loc, "news.article.source."+sourceClass, nil),
		Title:         title,
		Deck:          s.Catalogs.Render(loc, narrative.ArticleTemplateKey("deck", class, n.ID), articleParams),
		Body:          s.Catalogs.Render(loc, narrative.ArticleTemplateKey("body", class, n.ID), articleParams),
		Refs:          n.ClubIDs,
	}
}

func (s *Server) matchdayResultsArticleParams(loc narrative.Locale, params map[string]any, title string) map[string]any {
	out := copyConsoleArticleParams(params, title)
	out["results"] = s.matchdayResultLines(loc, params["results"])
	out["table"] = s.matchdayTableLines(loc, params["table"])
	out["story"] = s.matchdayStoryLine(loc, params["story"])
	return out
}

func copyConsoleArticleParams(params map[string]any, title string) map[string]any {
	out := make(map[string]any, len(params)+1)
	for k, v := range params {
		out[k] = v
	}
	out["headline"] = title
	return out
}

func (s *Server) matchdayResultLines(loc narrative.Locale, raw any) string {
	rows := mapsFromAny(raw)
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		key := "term.matchday.result_line"
		if winner, ok := row["winner"].(string); ok && winner != "" {
			key = "term.matchday.result_line.winner"
		}
		lines = append(lines, s.Catalogs.Render(loc, key, row))
	}
	return joinNonEmpty(lines)
}

func (s *Server) matchdayTableLines(loc narrative.Locale, raw any) string {
	rows := mapsFromAny(raw)
	if len(rows) == 0 {
		return s.Catalogs.Render(loc, "term.matchday.table_cup", nil)
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, s.Catalogs.Render(loc, "term.matchday.table_leader", row))
	}
	return joinNonEmpty(lines)
}

func (s *Server) matchdayStoryLine(loc narrative.Locale, raw any) string {
	rows := mapsFromAny(raw)
	if len(rows) == 0 {
		return ""
	}
	story := rows[0]
	lines := []string{}
	margin, hasMargin := numericParam(story["best_margin"])
	draws, hasDraws := numericParam(story["draws"])
	if hasMargin && margin > 0 {
		lines = append(lines, s.Catalogs.Render(loc, "term.matchday.story_margin", story))
	}
	if hasDraws && draws > 0 {
		lines = append(lines, s.Catalogs.Render(loc, "term.matchday.story_draws", story))
	}
	if hasDraws && draws == 0 && hasMargin && margin > 0 {
		lines = append(lines, s.Catalogs.Render(loc, "term.matchday.story_all_winners", nil))
	}
	if len(lines) == 0 {
		// Defensive fallback for malformed persisted params; normal engine
		// payloads always contain draws and best_margin.
		lines = append(lines, s.Catalogs.Render(loc, "term.matchday.story_unavailable", nil))
	}
	return joinNonEmpty(lines)
}

var consolePoolDerivedNewsParams = map[string]bool{"fee": true, "wage": true}

func consoleNewsParams(params map[string]any) map[string]any {
	safe := make(map[string]any, len(params))
	for k, v := range params {
		if consolePoolDerivedNewsParams[k] {
			continue
		}
		safe[k] = v
	}
	return safe
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

func (s *Server) renderNewsText(loc narrative.Locale, key string, params map[string]any) string {
	resolved := make(map[string]any, len(params))
	for k, v := range params {
		resolved[k] = v
	}
	if a, ok := resolved["attr_key"].(string); ok {
		resolved["attr"] = s.Catalogs.Render(loc, "attr."+a, nil)
		delete(resolved, "attr_key")
	}
	if wk, ok := resolved["window_key"].(string); ok {
		resolved["window"] = s.Catalogs.Render(loc, "term.window."+wk, nil)
		delete(resolved, "window_key")
	}
	if comp, ok := resolved["competition"].(string); ok {
		resolved["competition"] = s.Catalogs.Render(loc, "term.competition."+comp, nil)
	}
	if club, ok := resolved["club"].(string); ok && club == "" {
		resolved["club"] = s.Catalogs.Render(loc, "term.free_agent", nil)
	}
	return s.Catalogs.Render(loc, key, resolved)
}

// ---- Viewer: tables ----

type tableRowDTO struct {
	Pos    int    `json:"pos"`
	ClubID int64  `json:"club_id"`
	Club   string `json:"club"`
	Short  string `json:"short"`
	Played int    `json:"played"`
	Won    int    `json:"won"`
	Drawn  int    `json:"drawn"`
	Lost   int    `json:"lost"`
	GF     int    `json:"gf"`
	GA     int    `json:"ga"`
	Points int    `json:"points"`
}

func (s *Server) handleTables(w http.ResponseWriter, r *http.Request) {
	loc := s.locale(r)
	tier, err := strconv.Atoi(r.URL.Query().Get("tier"))
	if err != nil {
		tier = 1
	}
	var out struct {
		Tier  int           `json:"tier"`
		Label string        `json:"label"`
		Rows  []tableRowDTO `json:"rows"`
	}
	found := false
	s.Host.Locked(func() {
		wd := s.Host.World()
		if tier < 1 || tier > wd.Config.Divisions {
			return
		}
		found = true
		out.Tier = tier
		// The live Table is seeded at generation and moves at every league
		// full time.
		out.Label = s.Catalogs.Render(loc, "ui.table.live", nil)
		names := map[int64]*worldgen.Club{}
		for i := range wd.Clubs {
			names[wd.Clubs[i].ID] = &wd.Clubs[i]
		}
		for _, row := range wd.Table[tier-1] {
			c := names[row.ClubID]
			out.Rows = append(out.Rows, tableRowDTO{
				Pos: row.Pos, ClubID: row.ClubID, Club: c.Name, Short: c.ShortName,
				Played: row.Played, Won: row.Won, Drawn: row.Drawn, Lost: row.Lost,
				GF: row.GoalsFor, GA: row.GoalsAgainst, Points: row.Points,
			})
		}
	})
	if !found {
		s.httpError(w, r, http.StatusNotFound, "error.unknown_tier")
		return
	}
	writeJSON(w, out)
}

// ---- Viewer: clubs ----

type clubDTO struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Short     string `json:"short"`
	Tier      int    `json:"tier"`
	Region    string `json:"region"`
	Primary   string `json:"primary_color"`
	Secondary string `json:"secondary_color"`
	Stadium   string `json:"stadium"`
	Capacity  int    `json:"capacity"`
	Manager   string `json:"manager,omitempty"`
	Caretaker bool   `json:"caretaker,omitempty"`
	Security  string `json:"security,omitempty"`
}

func (s *Server) clubToDTO(loc narrative.Locale, wd *worldgen.World, c *worldgen.Club) clubDTO {
	region := ""
	for i := range wd.Regions {
		if wd.Regions[i].ID == c.RegionID {
			region = wd.Regions[i].Name
		}
	}
	dto := clubDTO{
		ID: c.ID, Name: c.Name, Short: c.ShortName, Tier: c.DivisionTier,
		Region: region, Primary: c.Colors.Primary, Secondary: c.Colors.Secondary,
		Stadium: c.Stadium.Name, Capacity: c.Stadium.Capacity,
		Security: s.Catalogs.Render(loc, "desc.security."+consoleSecurityBand(c.Confidence), nil),
	}
	if m := managerForClub(wd, c.ID); m != nil {
		dto.Manager = m.Name
		dto.Caretaker = m.Caretaker
	}
	return dto
}

func (s *Server) handleClubs(w http.ResponseWriter, r *http.Request) {
	loc := s.locale(r)
	var out []clubDTO
	s.Host.Locked(func() {
		wd := s.Host.World()
		for i := range wd.Clubs {
			out = append(out, s.clubToDTO(loc, wd, &wd.Clubs[i]))
		}
	})
	writeJSON(w, out)
}

// playerDTO carries the visible surface only (FR-22): identity, public body
// profile, visible attributes, and threshold descriptors. Hidden attributes,
// Ability Pool, Potential Cap, and Reputation never leave the core.
type playerDTO struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Age      int    `json:"age"`
	HeightCm int    `json:"height_cm"`
	WeightKg int    `json:"weight_kg"`
	Position string `json:"position"`
	Group    string `json:"group"`
	Foot     string `json:"foot"`
	WeakFoot int    `json:"weak_foot"`
	Youth    bool   `json:"youth"`
	// Familiarity is the stable descriptor key at the primary position;
	// FamiliarityLabel is that key rendered for the request locale
	// (FR-35c — descriptors are user-facing text).
	Familiarity          string         `json:"familiarity"`
	FamiliarityLabel     string         `json:"familiarity_label"`
	FootLabel            string         `json:"foot_label"`
	WeakFootDescriptor   string         `json:"weak_foot_descriptor"`
	WeakFootLabel        string         `json:"weak_foot_label"`
	Attributes           map[string]int `json:"attributes"`
	ContractExpirySeason int            `json:"contract_expiry_season,omitempty"`
}

func (s *Server) playerToDTO(loc narrative.Locale, p *worldgen.Player) playerDTO {
	attrs := make(map[string]int, len(p.Visible))
	for a, v := range p.Visible {
		attrs[string(a)] = v
	}
	// attr.FamiliarityDescriptor is the canonical docs/08 name; its stable
	// uppercase form is the key, localized through the catalogs here.
	key := strings.ToUpper(attr.FamiliarityDescriptor(p.Familiarity[p.Position]))
	wfKey := descriptorToken(attr.WeakFootDescriptor(p.WeakFoot))
	dto := playerDTO{
		ID: p.ID, Name: p.Name, Age: p.Age, Position: p.Position,
		HeightCm: p.HeightCm, WeightKg: p.WeightKg,
		Group: string(p.Group), Foot: string(p.Foot),
		FootLabel: s.Catalogs.Render(loc, "desc.foot."+string(p.Foot), nil),
		WeakFoot:  p.WeakFoot, Youth: p.Youth,
		Familiarity:        key,
		FamiliarityLabel:   s.Catalogs.Render(loc, "desc.familiarity."+key, nil),
		WeakFootDescriptor: wfKey,
		WeakFootLabel:      s.Catalogs.Render(loc, "desc.weak_foot."+wfKey, nil),
		Attributes:         attrs,
	}
	if p.Contract != nil {
		dto.ContractExpirySeason = p.Contract.ExpirySeasonYear
	}
	return dto
}

func descriptorToken(s string) string {
	s = strings.ToUpper(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

func cloneIntMap(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (s *Server) handleClub(w http.ResponseWriter, r *http.Request) {
	loc := s.locale(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.httpError(w, r, http.StatusBadRequest, "error.bad_request")
		return
	}
	var out struct {
		clubDTO
		PredictedFinish      int            `json:"predicted_finish"`
		BoardObjectiveFinish int            `json:"board_objective_finish"`
		Board                map[string]any `json:"board"`
		Finances             map[string]any `json:"finances"`
		Squad                []playerDTO    `json:"squad"`
	}
	found := false
	s.Host.Locked(func() {
		wd := s.Host.World()
		for i := range wd.Clubs {
			if wd.Clubs[i].ID != id {
				continue
			}
			found = true
			out.clubDTO = s.clubToDTO(loc, wd, &wd.Clubs[i])
			out.PredictedFinish = wd.Clubs[i].PredictedFinish
			out.BoardObjectiveFinish = wd.Clubs[i].BoardObjectiveFinish
			out.Board = map[string]any{
				"confidence": s.Catalogs.Render(loc, "desc.confidence."+consoleConfidenceBand(wd.Clubs[i].Confidence), nil),
				"security":   s.Catalogs.Render(loc, "desc.security."+consoleSecurityBand(wd.Clubs[i].Confidence), nil),
				// Fan mood is still a placeholder system-wide descriptor; a live
				// supporter mood model is separate from board confidence.
				"fan_mood": s.Catalogs.Render(loc, "desc.mood.STEADY", nil),
			}
			out.Finances = map[string]any{
				"cash":          crDisplay(wd.Clubs[i].BalanceMinor),
				"salary_bill":   crDisplay(wd.Clubs[i].WageBillWeeklyMinor),
				"salary_budget": crDisplay(wd.Clubs[i].WageBudgetWeeklyMinor),
				"market_funds":  crDisplay(wd.Clubs[i].TransferBudgetMinor),
			}
			for j := range wd.Players {
				if wd.Players[j].ClubID == id {
					out.Squad = append(out.Squad, s.playerToDTO(loc, &wd.Players[j]))
				}
			}
		}
	})
	if !found {
		s.httpError(w, r, http.StatusNotFound, "error.unknown_club")
		return
	}
	writeJSON(w, out)
}

func managerForClub(wd *worldgen.World, clubID int64) *worldgen.Manager {
	for i := range wd.Managers {
		m := &wd.Managers[i]
		if m.ClubID == clubID && m.Status != worldgen.ManagerRetired {
			return m
		}
	}
	return nil
}

func consoleConfidenceBand(confidence int) string {
	switch {
	case confidence >= 70:
		return "HIGH"
	case confidence >= 45:
		return "MODERATE"
	default:
		return "LOW"
	}
}

func consoleSecurityBand(confidence int) string {
	switch {
	case confidence >= 70:
		return "SECURE"
	case confidence >= 45:
		return "STABLE"
	default:
		return "UNDER_PRESSURE"
	}
}

func crDisplay(minor int64) string {
	crowns := minor / 100
	switch {
	case crowns >= 1_000_000:
		return trimZero(fmt.Sprintf("cr%.1fM", float64(crowns)/1_000_000))
	case crowns >= 1_000:
		return fmt.Sprintf("cr%dk", crowns/1_000)
	default:
		return fmt.Sprintf("cr%d", crowns)
	}
}

func trimZero(s string) string {
	if len(s) > 3 && s[len(s)-3:] == ".0M" {
		return s[:len(s)-3] + "M"
	}
	return s
}

// ---- Viewer: fixtures ----

type fixtureDTO struct {
	ID          int64  `json:"id"`
	Status      string `json:"status"` // LIVE | SCHEDULED | RESULT
	Competition string `json:"competition"`
	Round       int    `json:"round"`
	Kickoff     int64  `json:"kickoff"`
	KickoffText string `json:"kickoff_text"`
	Season      int    `json:"season,omitempty"`
	Archived    bool   `json:"archived,omitempty"`
	HomeID      int64  `json:"home_id"`
	Home        string `json:"home"`
	AwayID      int64  `json:"away_id"`
	Away        string `json:"away"`
	HomeGoals   int    `json:"home_goals,omitempty"`
	AwayGoals   int    `json:"away_goals,omitempty"`
	HasReplay   bool   `json:"has_replay,omitempty"`
}

const viewerHistoryLimit = 1000

const defaultFixtureListLimit = 120
const fixtureResultReserveCap = 20

func (s *Server) handleFixtures(w http.ResponseWriter, r *http.Request) {
	loc := s.locale(r)
	tier, _ := strconv.Atoi(r.URL.Query().Get("tier")) // 0 = all + cup
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = defaultFixtureListLimit
	}
	if limit > viewerHistoryLimit {
		limit = viewerHistoryLimit
	}
	var out []fixtureDTO
	s.Host.Locked(func() {
		wd := s.Host.World()
		now := s.Host.Engine().Now()
		names := map[int64]string{}
		for i := range wd.Clubs {
			names[wd.Clubs[i].ID] = wd.Clubs[i].Name
		}
		for i := range wd.Fixtures {
			f := &wd.Fixtures[i]
			if tier != 0 && f.Competition == worldgen.CompetitionLeague && f.DivisionTier != tier {
				continue
			}
			if r := wd.ResultFor(f.ID); r != nil {
				out = append(out, resultFixtureDTO(s.Catalogs, loc, names, r, f.Round, false))
				continue
			}
			_, isLive := wd.LiveMatches[f.ID]
			if f.Kickoff >= now || isLive {
				status := "SCHEDULED"
				if isLive {
					status = "LIVE"
				}
				out = append(out, fixtureDTO{
					ID: f.ID, Status: status, Competition: f.Competition, Round: f.Round,
					Kickoff:     int64(f.Kickoff),
					KickoffText: renderClock(s.Catalogs, loc, f.Kickoff),
					HomeID:      f.HomeID, Home: names[f.HomeID],
					AwayID: f.AwayID, Away: names[f.AwayID],
				})
			}
		}
		historyLimit := fixtureResultReserve(limit)
		if historyLimit == 0 {
			historyLimit = limit
		}
		historyAdded := 0
		for h := len(wd.History) - 1; h >= 0; h-- {
			for i := len(wd.History[h].Results) - 1; i >= 0; i-- {
				r := &wd.History[h].Results[i]
				if tier != 0 && r.Competition == worldgen.CompetitionLeague && r.DivisionTier != tier {
					continue
				}
				out = append(out, resultFixtureDTO(s.Catalogs, loc, names, r, 0, true))
				historyAdded++
				if historyAdded >= historyLimit {
					break
				}
			}
			if historyAdded >= historyLimit {
				break
			}
		}
		sort.Slice(out, func(i, j int) bool {
			ri, rj := fixtureListRank(out[i]), fixtureListRank(out[j])
			if ri != rj {
				return ri < rj
			}
			if out[i].Status == "RESULT" && out[i].Kickoff != out[j].Kickoff {
				return out[i].Kickoff > out[j].Kickoff
			}
			if out[i].Kickoff != out[j].Kickoff {
				return out[i].Kickoff < out[j].Kickoff
			}
			return out[i].ID < out[j].ID
		})
		if len(out) > limit {
			out = trimFixtureList(out, limit)
		}
		out = prioritizeFixtureWindows(out)
	})
	writeJSON(w, out)
}

// prioritizeFixtureWindows keeps the two immediately useful matchday windows
// together at the top: the next kick-off group, then the latest completed
// group. The remaining forward schedule and archive retain their existing
// chronological order.
func prioritizeFixtureWindows(fixtures []fixtureDTO) []fixtureDTO {
	var nextScheduled, latestResult int64
	hasScheduled, hasResult := false, false
	for _, f := range fixtures {
		switch f.Status {
		case "SCHEDULED":
			if !hasScheduled || f.Kickoff < nextScheduled {
				nextScheduled, hasScheduled = f.Kickoff, true
			}
		case "RESULT":
			if !hasResult || f.Kickoff > latestResult {
				latestResult, hasResult = f.Kickoff, true
			}
		}
	}
	out := make([]fixtureDTO, 0, len(fixtures))
	appendMatching := func(match func(fixtureDTO) bool) {
		for _, f := range fixtures {
			if match(f) {
				out = append(out, f)
			}
		}
	}
	appendMatching(func(f fixtureDTO) bool { return f.Status == "LIVE" })
	appendMatching(func(f fixtureDTO) bool { return f.Status == "SCHEDULED" && f.Kickoff == nextScheduled })
	appendMatching(func(f fixtureDTO) bool { return f.Status == "RESULT" && f.Kickoff == latestResult })
	appendMatching(func(f fixtureDTO) bool { return f.Status == "SCHEDULED" && f.Kickoff != nextScheduled })
	appendMatching(func(f fixtureDTO) bool { return f.Status == "RESULT" && f.Kickoff != latestResult })
	return out
}

func fixtureListRank(f fixtureDTO) int {
	if f.Status == "LIVE" {
		return 0
	}
	if f.Status == "SCHEDULED" {
		return 1
	}
	return 2
}

func trimFixtureList(fixtures []fixtureDTO, limit int) []fixtureDTO {
	if len(fixtures) <= limit {
		return fixtures
	}
	if limit <= 1 {
		return fixtures[:limit]
	}
	upcoming := make([]fixtureDTO, 0, limit)
	results := make([]fixtureDTO, 0, min(len(fixtures), limit))
	for _, f := range fixtures {
		if f.Status == "RESULT" {
			results = append(results, f)
			continue
		}
		upcoming = append(upcoming, f)
	}
	if len(results) == 0 || len(upcoming) < limit {
		return fixtures[:limit]
	}
	// Keep a recency tail even when upcoming fixtures saturate the page.
	keepResults := min(len(results), fixtureResultReserve(limit))
	if keepResults == 0 {
		return fixtures[:limit]
	}
	keepUpcoming := limit - keepResults
	out := make([]fixtureDTO, 0, limit)
	out = append(out, upcoming[:min(len(upcoming), keepUpcoming)]...)
	out = append(out, results[:min(len(results), limit-len(out))]...)
	return out
}

func fixtureResultReserve(limit int) int {
	return min(fixtureResultReserveCap, limit/4)
}

func resultFixtureDTO(cats narrative.Catalogs, loc narrative.Locale, names map[int64]string, r *worldgen.MatchResult, round int, archived bool) fixtureDTO {
	season := worldgen.DateOf(r.Kickoff).Season
	return fixtureDTO{
		ID: r.FixtureID, Status: "RESULT", Competition: r.Competition, Round: round,
		Kickoff: int64(r.Kickoff), KickoffText: renderClock(cats, loc, r.Kickoff),
		Season: season, Archived: archived,
		HomeID: r.HomeID, Home: names[r.HomeID],
		AwayID: r.AwayID, Away: names[r.AwayID],
		HomeGoals: r.HomeGoals, AwayGoals: r.AwayGoals,
		HasReplay: len(r.Commentary) > 0,
	}
}

type matchEventDTO struct {
	Minute int    `json:"minute"`
	Club   string `json:"club"`
	Player string `json:"player"`
	Detail string `json:"detail,omitempty"`
}

type matchSubDTO struct {
	Minute int    `json:"minute"`
	Club   string `json:"club"`
	Off    string `json:"off"`
	On     string `json:"on,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type matchDetailDTO struct {
	Fixture           int64                     `json:"fixture"`
	Status            string                    `json:"status"`
	Archived          bool                      `json:"archived,omitempty"`
	Season            int                       `json:"season,omitempty"`
	Competition       string                    `json:"competition"`
	Round             int                       `json:"round,omitempty"`
	KickoffText       string                    `json:"kickoff_text"`
	Home              string                    `json:"home"`
	Away              string                    `json:"away"`
	HomeGoals         int                       `json:"home_goals"`
	AwayGoals         int                       `json:"away_goals"`
	Winner            string                    `json:"winner,omitempty"`
	HomeShots         int                       `json:"home_shots"`
	AwayShots         int                       `json:"away_shots"`
	ChanceTypes       map[string]int            `json:"chance_types,omitempty"`
	ChanceTypesBySide map[string]int            `json:"chance_types_by_side,omitempty"`
	Diagnostics       worldgen.MatchDiagnostics `json:"diagnostics,omitempty"`
	Scorers           []matchEventDTO           `json:"scorers,omitempty"`
	Cards             []matchEventDTO           `json:"cards,omitempty"`
	Subs              []matchSubDTO             `json:"subs,omitempty"`
	Ratings           []liveRatingDTO           `json:"ratings,omitempty"`
	HomeLineup        []lineupEntryDTO          `json:"home_lineup,omitempty"`
	AwayLineup        []lineupEntryDTO          `json:"away_lineup,omitempty"`
	Commentary        []string                  `json:"commentary,omitempty"`
	Beats             []beatDTO                 `json:"beats,omitempty"`
}

// lineupEntryDTO is one team-sheet row for the match pop-up (docs/07 §4.1):
// public identity plus the match-story markers, so consoles can render a
// lineup panel without re-joining the event lists by name. Starters come
// first in stored XI order, players who came on follow in entry order, and
// (live only) unused bench players close the list flagged Bench.
type lineupEntryDTO struct {
	Name      string `json:"name"`
	Position  string `json:"position"`
	RatingX10 int    `json:"rating_x10,omitempty"`
	Goals     int    `json:"goals,omitempty"`
	Yellows   int    `json:"yellows,omitempty"`
	Red       bool   `json:"red,omitempty"`
	// OffMinute/OnMinute are substitution minutes; zero means "not subbed"
	// (a football substitution never happens before the clock starts).
	OffMinute int  `json:"off_minute,omitempty"`
	OnMinute  int  `json:"on_minute,omitempty"`
	Bench     bool `json:"bench,omitempty"`
}

// lineupEntries assembles one side's team sheet from the persisted match
// story. Everything here is already-public spectacle: names, positions, and
// the goal/card/sub events the pop-up lists elsewhere. Ids without a
// resolvable player (defensive) are skipped.
func lineupEntries(clubID int64, xi, bench []int64, subs []worldgen.SubEvent,
	scorers, cards []worldgen.MatchEvent, ratings map[int64]int,
	playerOf func(int64) *worldgen.Player) []lineupEntryDTO {
	rows := []lineupEntryDTO{}
	index := map[int64]int{}
	add := func(id int64, onMinute int, benchRow bool) {
		if _, dup := index[id]; dup {
			return
		}
		p := playerOf(id)
		if p == nil {
			return
		}
		row := lineupEntryDTO{Name: p.Name, Position: p.Position, OnMinute: onMinute, Bench: benchRow}
		if !benchRow {
			row.RatingX10 = ratings[id]
		}
		index[id] = len(rows)
		rows = append(rows, row)
	}
	for _, id := range xi {
		add(id, 0, false)
	}
	for _, sub := range subs {
		if sub.ClubID != clubID {
			continue
		}
		if i, ok := index[sub.Off]; ok {
			rows[i].OffMinute = sub.Minute
		}
		if sub.On != 0 && sub.On != sub.Off {
			add(sub.On, sub.Minute, false)
		}
	}
	for _, e := range scorers {
		if e.ClubID != clubID {
			continue
		}
		if i, ok := index[e.PlayerID]; ok {
			rows[i].Goals++
		}
	}
	for _, e := range cards {
		if e.ClubID != clubID {
			continue
		}
		i, ok := index[e.PlayerID]
		if !ok {
			continue
		}
		// A second yellow is recorded as one RED event after the first
		// YELLOW, so the row naturally reads Y + R.
		if e.Detail == "RED" {
			rows[i].Red = true
		} else {
			rows[i].Yellows++
		}
	}
	for _, id := range bench {
		add(id, 0, true)
	}
	return rows
}

// beatDTO is one minute-stamped commentary beat. Commentary keeps the plain
// rendered strings for older consoles; Beats carries the same lines with
// their football minute for match-report style displays.
type beatDTO struct {
	Minute int    `json:"minute"`
	Text   string `json:"text"`
}

func (s *Server) handleMatch(w http.ResponseWriter, r *http.Request) {
	loc := s.locale(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.httpError(w, r, http.StatusNotFound, "error.not_found")
		return
	}
	var dto matchDetailDTO
	found := false
	s.Host.Locked(func() {
		wd := s.Host.World()
		names := map[int64]string{}
		for i := range wd.Clubs {
			names[wd.Clubs[i].ID] = wd.Clubs[i].Name
		}
		playerIdx := map[int64]*worldgen.Player{}
		for i := range wd.Players {
			playerIdx[wd.Players[i].ID] = &wd.Players[i]
		}
		playerOf := func(id int64) *worldgen.Player { return playerIdx[id] }
		round := 0
		for i := range wd.Fixtures {
			if wd.Fixtures[i].ID == id {
				round = wd.Fixtures[i].Round
				break
			}
		}
		if res := wd.ResultFor(id); res != nil {
			dto = s.matchDetailDTO(loc, names, playerOf, res, round, false)
			found = true
			return
		}
		if res := wd.ArchivedResultFor(id); res != nil {
			dto = s.matchDetailDTO(loc, names, playerOf, res, 0, true)
			found = true
		}
	})
	if !found {
		s.httpError(w, r, http.StatusNotFound, "error.not_found")
		return
	}
	writeJSON(w, dto)
}

func (s *Server) matchDetailDTO(loc narrative.Locale, names map[int64]string, playerOf func(int64) *worldgen.Player, r *worldgen.MatchResult, round int, archived bool) matchDetailDTO {
	playerName := func(id int64) string {
		if p := playerOf(id); p != nil {
			return p.Name
		}
		return ""
	}
	dto := matchDetailDTO{
		Fixture: r.FixtureID, Status: "RESULT", Archived: archived,
		Season: worldgen.DateOf(r.Kickoff).Season, Competition: r.Competition, Round: round,
		KickoffText: renderClock(s.Catalogs, loc, r.Kickoff),
		Home:        names[r.HomeID], Away: names[r.AwayID],
		HomeGoals: r.HomeGoals, AwayGoals: r.AwayGoals,
		HomeShots: r.HomeShots, AwayShots: r.AwayShots,
		ChanceTypes:       cloneIntMap(r.ChanceTypes),
		ChanceTypesBySide: cloneIntMap(r.ChanceTypesBySide),
		Diagnostics:       r.Diagnostics.Clone(),
	}
	if r.Winner != 0 {
		dto.Winner = names[r.Winner]
	}
	event := func(e worldgen.MatchEvent) matchEventDTO {
		return matchEventDTO{Minute: e.Minute, Club: names[e.ClubID], Player: playerName(e.PlayerID), Detail: e.Detail}
	}
	for _, e := range r.Scorers {
		dto.Scorers = append(dto.Scorers, event(e))
	}
	for _, e := range r.Cards {
		dto.Cards = append(dto.Cards, event(e))
	}
	for _, sub := range r.Subs {
		dto.Subs = append(dto.Subs, matchSubDTO{
			Minute: sub.Minute, Club: names[sub.ClubID],
			Off: playerName(sub.Off), On: playerName(sub.On), Reason: sub.Reason,
		})
	}
	dto.HomeLineup = lineupEntries(r.HomeID, r.HomeXI, nil, r.Subs, r.Scorers, r.Cards, r.RatingsX10, playerOf)
	dto.AwayLineup = lineupEntries(r.AwayID, r.AwayXI, nil, r.Subs, r.Scorers, r.Cards, r.RatingsX10, playerOf)
	sides := make(map[int64]string, len(r.HomeXI)+len(r.AwayXI)+2*len(r.Subs))
	for _, id := range r.HomeXI {
		sides[id] = matchSideHome
	}
	for _, id := range r.AwayXI {
		sides[id] = matchSideAway
	}
	for _, sub := range r.Subs {
		side := ""
		if sub.ClubID == r.HomeID {
			side = matchSideHome
		} else if sub.ClubID == r.AwayID {
			side = matchSideAway
		}
		if side != "" {
			sides[sub.Off] = side
			sides[sub.On] = side
		}
	}
	for id, rx := range r.RatingsX10 {
		if name := playerName(id); name != "" {
			dto.Ratings = append(dto.Ratings, liveRatingDTO{Side: sides[id], Name: name, RatingX10: rx})
		}
	}
	sort.Slice(dto.Ratings, func(i, j int) bool {
		if dto.Ratings[i].RatingX10 != dto.Ratings[j].RatingX10 {
			return dto.Ratings[i].RatingX10 > dto.Ratings[j].RatingX10
		}
		return dto.Ratings[i].Name < dto.Ratings[j].Name
	})
	for _, cl := range r.Commentary {
		text := s.Catalogs.Render(loc, cl.Key, cl.Params)
		dto.Commentary = append(dto.Commentary, text)
		dto.Beats = append(dto.Beats, beatDTO{Minute: cl.Minute, Text: text})
	}
	return dto
}

// ---- Viewer: live matches (the TUI Live Match screen, docs/07 §4.1) ----

// liveCommentaryLines is how many trailing rendered lines a live-match poll
// returns — the TUI shows the tail, cadence-paced upstream.
const liveCommentaryLines = 24

type liveMarkerDTO struct {
	Minute int    `json:"minute"`
	Kind   string `json:"kind"` // GOAL | CHANCE | CARD | INJURY | SUB | SHOOTOUT
	Side   string `json:"side"` // HOME | AWAY | NONE
}

// liveStatsDTO feeds the Match stats pane (docs/07 §4.1): shots, cards, and
// substitutions used per side — already-public spectacle, counted from the
// persisted tally.
type liveStatsDTO struct {
	HomeShots         int                       `json:"home_shots"`
	AwayShots         int                       `json:"away_shots"`
	HomeCards         int                       `json:"home_cards"`
	AwayCards         int                       `json:"away_cards"`
	HomeSubs          int                       `json:"home_subs"`
	AwaySubs          int                       `json:"away_subs"`
	ChanceTypes       map[string]int            `json:"chance_types,omitempty"`
	ChanceTypesBySide map[string]int            `json:"chance_types_by_side,omitempty"`
	Diagnostics       worldgen.MatchDiagnostics `json:"diagnostics,omitempty"`
}

// liveRatingDTO is one live-ratings row: the shared band formula
// (worldgen.LiveRatingsX10) read mid-match as "as if it ended now".
type liveRatingDTO struct {
	Side      string `json:"side"` // HOME | AWAY
	Name      string `json:"name"`
	RatingX10 int    `json:"rating_x10"`
}

const (
	matchSideHome = "HOME"
	matchSideAway = "AWAY"
)

type liveMatchDTO struct {
	Fixture     int64            `json:"fixture"`
	Competition string           `json:"competition"`
	Home        string           `json:"home"`
	Away        string           `json:"away"`
	HomeGoals   int              `json:"home_goals"`
	AwayGoals   int              `json:"away_goals"`
	Minute      int              `json:"minute"`
	Commentary  []string         `json:"commentary"`
	Beats       []beatDTO        `json:"beats,omitempty"`
	Markers     []liveMarkerDTO  `json:"markers"`
	Stats       liveStatsDTO     `json:"stats"`
	Ratings     []liveRatingDTO  `json:"ratings"`
	HomeLineup  []lineupEntryDTO `json:"home_lineup,omitempty"`
	AwayLineup  []lineupEntryDTO `json:"away_lineup,omitempty"`
	// Momentum is one signed value per 10-minute bucket (home positive,
	// goals ×3 + chances ×1). Both it and the markers field carry the full
	// match story.
	Momentum []int `json:"momentum"`
}

// handleLiveMatches renders every in-progress fixture for the Console's Live
// Match screen: score, clock, the rendered commentary tail, and the abstract
// event markers the ASCII pitch draws (docs/07 §4.1 — presentation over the
// existing match model; everything here is already-public spectacle).
func (s *Server) handleLiveMatches(w http.ResponseWriter, r *http.Request) {
	loc := s.locale(r)
	out := struct {
		Matches []liveMatchDTO `json:"matches"`
	}{Matches: []liveMatchDTO{}}
	s.Host.Locked(func() {
		wd := s.Host.World()
		clubName := map[int64]string{}
		for i := range wd.Clubs {
			clubName[wd.Clubs[i].ID] = wd.Clubs[i].Name
		}
		playerIdx := map[int64]*worldgen.Player{}
		for i := range wd.Players {
			playerIdx[wd.Players[i].ID] = &wd.Players[i]
		}
		playerOf := func(id int64) *worldgen.Player { return playerIdx[id] }
		// LiveMatches is a map — sort ids so the response order is stable.
		ids := make([]int64, 0, len(wd.LiveMatches))
		for id := range wd.LiveMatches {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			lm := wd.LiveMatches[id]
			dto := liveMatchDTO{
				Fixture: lm.FixtureID, Competition: lm.Competition,
				Home: clubName[lm.HomeID], Away: clubName[lm.AwayID],
				HomeGoals: lm.HomeGoals, AwayGoals: lm.AwayGoals, Minute: lm.Clock,
				Stats: liveStats(lm),
			}
			from := len(lm.Commentary) - liveCommentaryLines
			if from < 0 {
				from = 0
			}
			for _, cl := range lm.Commentary[from:] {
				text := s.Catalogs.Render(loc, cl.Key, cl.Params)
				dto.Commentary = append(dto.Commentary, text)
				dto.Beats = append(dto.Beats, beatDTO{Minute: cl.Minute, Text: text})
			}
			// The full stream feeds both the momentum sparkline and the
			// modal timeline; windowing it would silently drop early goals
			// from the match story.
			all := liveMarkers(lm, clubName)
			dto.Momentum = momentumFrom(all)
			dto.Markers = all
			ratings := worldgen.LiveRatingsX10(lm, playerOf)
			dto.Ratings = liveRatings(lm, ratings, playerOf)
			dto.HomeLineup = lineupEntries(lm.HomeID, lm.HomeXI, lm.HomeBench, lm.Subs, lm.Scorers, lm.Cards, ratings, playerOf)
			dto.AwayLineup = lineupEntries(lm.AwayID, lm.AwayXI, lm.AwayBench, lm.Subs, lm.Scorers, lm.Cards, ratings, playerOf)
			out.Matches = append(out.Matches, dto)
		}
	})
	writeJSON(w, out)
}

// liveStats counts the stats pane's numbers from the persisted tally.
func liveStats(lm *worldgen.LiveMatch) liveStatsDTO {
	st := liveStatsDTO{
		HomeShots: lm.HomeShots, AwayShots: lm.AwayShots,
		HomeSubs: lm.SubsUsed(lm.HomeID), AwaySubs: lm.SubsUsed(lm.AwayID),
		ChanceTypes:       cloneIntMap(lm.ChanceTypes),
		ChanceTypesBySide: cloneIntMap(lm.ChanceTypesBySide),
		Diagnostics:       lm.Diagnostics.Clone(),
	}
	for _, c := range lm.Cards {
		if c.ClubID == lm.HomeID {
			st.HomeCards++
		} else {
			st.AwayCards++
		}
	}
	return st
}

// momentumBuckets is the sparkline resolution: one bucket per 10 minutes.
const momentumBuckets = 9

// momentumFrom folds the full marker stream into the sparkline buckets —
// home positive, goals ×3, chances ×1; discipline and stoppages carry no
// momentum weight.
func momentumFrom(markers []liveMarkerDTO) []int {
	buckets := make([]int, momentumBuckets)
	for _, mk := range markers {
		w := 0
		switch mk.Kind {
		case "GOAL":
			w = 3
		case "CHANCE":
			w = 1
		default:
			continue
		}
		b := mk.Minute / 10
		if b < 0 {
			b = 0
		}
		if b >= momentumBuckets {
			b = momentumBuckets - 1
		}
		switch mk.Side {
		case matchSideHome:
			buckets[b] += w
		case matchSideAway:
			buckets[b] -= w
		}
	}
	return buckets
}

// liveRatings renders both sides' live ratings rows, each side sorted rating
// desc then name (a stable presentation order). Ids without a resolvable
// player (defensive) are skipped — no row is better than a nameless one.
func liveRatings(lm *worldgen.LiveMatch, ratings map[int64]int, playerOf func(int64) *worldgen.Player) []liveRatingDTO {
	rows := []liveRatingDTO{}
	appendSide := func(side string, clubID int64) {
		var part []liveRatingDTO
		for _, pid := range lm.Participants(clubID) {
			p := playerOf(pid)
			if p == nil {
				continue
			}
			part = append(part, liveRatingDTO{Side: side, Name: p.Name, RatingX10: ratings[pid]})
		}
		sort.Slice(part, func(i, j int) bool {
			if part[i].RatingX10 != part[j].RatingX10 {
				return part[i].RatingX10 > part[j].RatingX10
			}
			return part[i].Name < part[j].Name
		})
		rows = append(rows, part...)
	}
	appendSide(matchSideHome, lm.HomeID)
	appendSide(matchSideAway, lm.AwayID)
	return rows
}

// liveMarkers derives the pitch band's abstract markers from the commentary
// the match already persists — deliberately zones-and-events, not ball
// physics: the engine samples key moments (docs/07 §4.1). The side comes from
// the line's club param (every event key carries it), except the shootout
// line, which names the advancing side as {winner}.
func liveMarkers(lm *worldgen.LiveMatch, clubName map[int64]string) []liveMarkerDTO {
	side := func(params map[string]any, key string) string {
		c, _ := params["club"].(string)
		if key == "comment.shootout" {
			// The shootout line names the advancing side as {winner}.
			c, _ = params["winner"].(string)
		}
		switch c {
		case clubName[lm.HomeID]:
			return matchSideHome
		case clubName[lm.AwayID]:
			return matchSideAway
		}
		return "NONE"
	}
	out := []liveMarkerDTO{}
	for _, cl := range lm.Commentary {
		kind := ""
		switch {
		case strings.HasPrefix(cl.Key, "comment.goal"):
			kind = "GOAL"
		case strings.HasPrefix(cl.Key, "comment.chance"), strings.HasPrefix(cl.Key, "comment.save"):
			kind = "CHANCE"
		case strings.HasPrefix(cl.Key, "comment.card"):
			kind = "CARD"
		case cl.Key == "comment.injury":
			kind = "INJURY"
		case strings.HasPrefix(cl.Key, "comment.sub"):
			kind = "SUB"
		case cl.Key == "comment.shootout":
			kind = "SHOOTOUT"
		default:
			continue
		}
		out = append(out, liveMarkerDTO{Minute: cl.Minute, Kind: kind, Side: side(cl.Params, cl.Key)})
	}
	// Uncapped: the momentum sparkline and the modal timeline both need the
	// full match story.
	return out
}

// ---- Viewer: live feed (SSE) ----

func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.httpError(w, r, http.StatusInternalServerError, "error.streaming_unsupported")
		return
	}
	loc := s.locale(r)
	ch, cancel := s.Feed.Subscribe(loc)
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	hb := s.HeartbeatInterval
	if hb <= 0 {
		hb = 15 * time.Second
	}
	ticker := time.NewTicker(hb)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// Heartbeat comment: keeps streams alive across pauses (A11).
			if _, err := w.Write([]byte(": hb\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case b := <-ch:
			if _, err := w.Write([]byte("data: " + string(b) + "\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// ---- Admin (Admin Token gated, FR-33/34) ----

func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Admin-Token")
		if t, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			token = t
		}
		if s.AdminToken == "" ||
			subtle.ConstantTimeCompare([]byte(token), []byte(s.AdminToken)) != 1 {
			s.httpError(w, r, http.StatusUnauthorized, "error.admin_required")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleAdminStatus(w http.ResponseWriter, _ *http.Request) {
	var out struct {
		Seed     uint64 `json:"seed"`
		State    string `json:"state"`
		Tempo    string `json:"tempo"`
		GameTime int64  `json:"game_time"`
		QueueLen int    `json:"queue_len"`
	}
	s.Host.Locked(func() {
		out.Seed = s.Host.Seed()
		out.State = s.Host.State()
		out.Tempo = s.Host.Tempo().String()
		out.GameTime = int64(s.Host.Engine().Now())
		out.QueueLen = s.Host.Engine().Queue().Len()
	})
	writeJSON(w, out)
}

type adminSettingsDTO struct {
	Runtime runtimeSettingsDTO `json:"runtime"`
	Schema  runtimeSchemaDTO   `json:"schema"`
}

type runtimeSettingsDTO struct {
	GameSpeed             int `json:"game_speed"`
	IdleAcceleration      int `json:"idle_acceleration"`
	OffseasonAcceleration int `json:"offseason_acceleration"`
}

type runtimeSchemaDTO struct {
	GameSpeedOptions     []int    `json:"game_speed_options"`
	IdleAccelerationMin  int      `json:"idle_acceleration_min"`
	IdleAccelerationMax  int      `json:"idle_acceleration_max"`
	OffseasonAccelMin    int      `json:"offseason_acceleration_min"`
	OffseasonAccelMax    int      `json:"offseason_acceleration_max"`
	Determinism          string   `json:"determinism"`
	RequiresWorldRebuild []string `json:"requires_world_rebuild"`
}

func (s *Server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.adminSettingsResponse(s.locale(r), s.Host.RuntimeSettings()))
}

type runtimeSettingsPatch struct {
	GameSpeed             *int `json:"game_speed,omitempty"`
	IdleAcceleration      *int `json:"idle_acceleration,omitempty"`
	OffseasonAcceleration *int `json:"offseason_acceleration,omitempty"`
}

func (s *Server) handleAdminSettingsPatch(w http.ResponseWriter, r *http.Request) {
	var patch runtimeSettingsPatch
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&patch); err != nil {
		s.httpError(w, r, http.StatusBadRequest, "error.bad_request")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		s.httpError(w, r, http.StatusBadRequest, "error.bad_request")
		return
	}
	next, err := s.Host.UpdateRuntimeSettings(func(current RuntimeSettings) (RuntimeSettings, error) {
		next := current
		if patch.GameSpeed != nil {
			next.GameSpeed = sim.Speed(*patch.GameSpeed)
		}
		if patch.IdleAcceleration != nil {
			next.IdleAcceleration = *patch.IdleAcceleration
		}
		if patch.OffseasonAcceleration != nil {
			next.OffseasonAcceleration = *patch.OffseasonAcceleration
		}
		if err := validateRuntimeSettings(next); err != nil {
			return current, err
		}
		return next, nil
	})
	var validationErr runtimeSettingsValidationError
	if errors.As(err, &validationErr) {
		s.httpError(w, r, http.StatusBadRequest, validationErr.Key)
		return
	}
	if err != nil {
		s.httpError(w, r, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, s.adminSettingsResponse(s.locale(r), next))
}

func (s *Server) adminSettingsResponse(loc narrative.Locale, settings RuntimeSettings) adminSettingsDTO {
	return adminSettingsDTO{
		Runtime: runtimeSettingsDTO{
			GameSpeed:             int(settings.GameSpeed),
			IdleAcceleration:      settings.IdleAcceleration,
			OffseasonAcceleration: settings.OffseasonAcceleration,
		},
		Schema: runtimeSchemaDTO{
			GameSpeedOptions:     []int{int(sim.Speed5), int(sim.Speed15), int(sim.Speed30), int(sim.Speed60)},
			IdleAccelerationMin:  2,
			IdleAccelerationMax:  64,
			OffseasonAccelMin:    2,
			OffseasonAccelMax:    240,
			Determinism:          s.Catalogs.Render(loc, "ui.admin.settings.determinism_body", nil),
			RequiresWorldRebuild: localizedRuntimeRebuildSettings(s.Catalogs, loc),
		},
	}
}

func localizedRuntimeRebuildSettings(cats narrative.Catalogs, loc narrative.Locale) []string {
	keys := []string{"seed", "divisions", "clubs_per_division", "quality", "economy", "culture_mix"}
	out := make([]string, len(keys))
	for i, key := range keys {
		out[i] = cats.Render(loc, "ui.admin.settings.rebuild."+key, nil)
	}
	return out
}

func validateRuntimeSettings(settings RuntimeSettings) error {
	switch settings.GameSpeed {
	case sim.Speed5, sim.Speed15, sim.Speed30, sim.Speed60:
	default:
		return runtimeSettingsValidationError{Key: "error.runtime_settings.game_speed"}
	}
	if settings.IdleAcceleration < 2 || settings.IdleAcceleration > 64 {
		return runtimeSettingsValidationError{Key: "error.runtime_settings.idle_acceleration"}
	}
	if settings.OffseasonAcceleration < 2 || settings.OffseasonAcceleration > 240 {
		return runtimeSettingsValidationError{Key: "error.runtime_settings.offseason_acceleration"}
	}
	return nil
}

func (s *Server) handleAdminManagers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.Host.Credentials())
}

func (s *Server) handleAdminStart(w http.ResponseWriter, r *http.Request) {
	if err := s.Host.Start(); err != nil {
		s.httpError(w, r, http.StatusConflict, "error.conflict")
		return
	}
	s.Feed.System("feed.world.started")
	writeJSON(w, map[string]string{"state": s.Host.State()})
}

func (s *Server) handleAdminPause(paused bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.Host.SetPaused(paused); err != nil {
			s.httpError(w, r, http.StatusConflict, "error.conflict")
			return
		}
		if paused {
			s.Feed.System("feed.world.paused")
		} else {
			s.Feed.System("feed.world.resumed")
		}
		writeJSON(w, map[string]string{"state": s.Host.State()})
	}
}
