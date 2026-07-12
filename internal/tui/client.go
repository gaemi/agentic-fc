// Package tui is the Console's Bubble Tea implementation (docs/07):
// Viewer Mode over the Console API. The Console is catalog-free — every
// string it shows was rendered server-side for the session locale
// (docs/07 §6); this package only lays text out.
package tui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gaemi/agentic-fc/internal/narrative"
)

// Client talks to the Console API.
type Client struct {
	Base       string
	Locale     narrative.Locale
	HTTP       *http.Client
	AdminToken string
}

func NewClient(base string, loc narrative.Locale) *Client {
	return &Client{
		Base:   strings.TrimRight(base, "/"),
		Locale: loc,
		HTTP:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) get(path string, out any) error {
	return c.do(http.MethodGet, path, nil, false, out)
}

func (c *Client) adminGet(path string, out any) error {
	return c.do(http.MethodGet, path, nil, true, out)
}

func (c *Client) adminPatch(path string, in, out any) error {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(in); err != nil {
		return err
	}
	return c.do(http.MethodPatch, path, &body, true, out)
}

func (c *Client) do(method, path string, body io.Reader, admin bool, out any) error {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	req, err := http.NewRequest(method, c.Base+path+sep+"locale="+string(c.Locale), body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if admin {
		if c.AdminToken == "" {
			return fmt.Errorf("admin token required")
		}
		req.Header.Set("X-Admin-Token", c.AdminToken)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// The server localizes error bodies (FR-35c); surface that text.
		var apiErr struct {
			Error string `json:"error"`
		}
		if json.NewDecoder(resp.Body).Decode(&apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("%s", apiErr.Error)
		}
		return fmt.Errorf("%s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// WorldInfo mirrors GET /v1/world.
type WorldInfo struct {
	Name       string `json:"name"`
	State      string `json:"state"`
	TempoLabel string `json:"tempo_label"`
	ClockText  string `json:"clock_text"`
	Divisions  int    `json:"divisions"`
}

func (c *Client) World() (WorldInfo, error) {
	var w WorldInfo
	err := c.get("/v1/world", &w)
	return w, err
}

// UIStrings mirrors GET /v1/ui.
func (c *Client) UIStrings() (map[string]string, error) {
	var out struct {
		Strings map[string]string `json:"strings"`
	}
	err := c.get("/v1/ui", &out)
	return out.Strings, err
}

type RuntimeSettings struct {
	GameSpeed             int `json:"game_speed"`
	IdleAcceleration      int `json:"idle_acceleration"`
	OffseasonAcceleration int `json:"offseason_acceleration"`
}

type SettingsSchema struct {
	GameSpeedOptions     []int    `json:"game_speed_options"`
	IdleAccelerationMin  int      `json:"idle_acceleration_min"`
	IdleAccelerationMax  int      `json:"idle_acceleration_max"`
	OffseasonAccelMin    int      `json:"offseason_acceleration_min"`
	OffseasonAccelMax    int      `json:"offseason_acceleration_max"`
	Determinism          string   `json:"determinism"`
	RequiresWorldRebuild []string `json:"requires_world_rebuild"`
}

type AdminSettings struct {
	Runtime RuntimeSettings `json:"runtime"`
	Schema  SettingsSchema  `json:"schema"`
}

func (c *Client) AdminSettings() (AdminSettings, error) {
	var out AdminSettings
	err := c.adminGet("/v1/admin/settings", &out)
	return out, err
}

func (c *Client) UpdateAdminSettings(runtime RuntimeSettings) (AdminSettings, error) {
	var out AdminSettings
	err := c.adminPatch("/v1/admin/settings", runtime, &out)
	return out, err
}

// NewsArticle mirrors GET /v1/news.
type NewsArticle struct {
	ID            int64   `json:"id"`
	GameTime      int64   `json:"game_time"`
	TimeText      string  `json:"time_text"`
	Category      string  `json:"category"`
	CategoryLabel string  `json:"category_label"`
	Source        string  `json:"source"`
	Title         string  `json:"title"`
	Deck          string  `json:"deck"`
	Body          string  `json:"body"`
	Refs          []int64 `json:"refs"`
}

func (c *Client) News(limit int) ([]NewsArticle, error) {
	var out struct {
		Items []NewsArticle `json:"items"`
	}
	err := c.get(fmt.Sprintf("/v1/news?limit=%d", limit), &out)
	return out.Items, err
}

// TableRow / Table mirror GET /v1/tables.
type TableRow struct {
	ClubID int64  `json:"club_id"`
	Pos    int    `json:"pos"`
	Club   string `json:"club"`
	Played int    `json:"played"`
	Won    int    `json:"won"`
	Drawn  int    `json:"drawn"`
	Lost   int    `json:"lost"`
	GF     int    `json:"gf"`
	GA     int    `json:"ga"`
	GD     int    `json:"gd"`
	Points int    `json:"points"`
	// Form is the last five league results, oldest first (W | D | L enums).
	Form []string `json:"form"`
}

type Table struct {
	Tier  int        `json:"tier"`
	Label string     `json:"label"`
	Rows  []TableRow `json:"rows"`
}

func (c *Client) Table(tier int) (Table, error) {
	var t Table
	err := c.get(fmt.Sprintf("/v1/tables?tier=%d", tier), &t)
	return t, err
}

// Clubs mirror GET /v1/clubs and /v1/clubs/{id}.
type ClubSummary struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Short     string `json:"short"`
	Tier      int    `json:"tier"`
	Region    string `json:"region"`
	Stadium   string `json:"stadium"`
	Capacity  int    `json:"capacity"`
	Manager   string `json:"manager"`
	Caretaker bool   `json:"caretaker"`
	Security  string `json:"security"`
}

type Player struct {
	ID                   int64          `json:"id"`
	Name                 string         `json:"name"`
	Age                  int            `json:"age"`
	HeightCm             int            `json:"height_cm"`
	WeightKg             int            `json:"weight_kg"`
	Position             string         `json:"position"`
	Group                string         `json:"group"`
	Foot                 string         `json:"foot"`
	FootLabel            string         `json:"foot_label"`
	WeakFoot             int            `json:"weak_foot"`
	WeakFootLabel        string         `json:"weak_foot_label"`
	Youth                bool           `json:"youth"`
	FamiliarityLabel     string         `json:"familiarity_label"`
	Attributes           map[string]int `json:"attributes"`
	ContractExpirySeason int            `json:"contract_expiry_season"`
	SuspendedMatches     int            `json:"suspended_matches"`
}

type ClubDetail struct {
	ClubSummary
	PredictedFinish      int               `json:"predicted_finish"`
	BoardObjectiveFinish int               `json:"board_objective_finish"`
	Board                map[string]string `json:"board"`
	Finances             map[string]string `json:"finances"`
	Squad                []Player          `json:"squad"`
}

func (c *Client) Clubs() ([]ClubSummary, error) {
	var out []ClubSummary
	err := c.get("/v1/clubs", &out)
	return out, err
}

func (c *Client) Club(id int64) (ClubDetail, error) {
	var out ClubDetail
	err := c.get(fmt.Sprintf("/v1/clubs/%d", id), &out)
	return out, err
}

// Fixture mirrors GET /v1/fixtures.
type Fixture struct {
	ID          int64  `json:"id"`
	Status      string `json:"status"`
	Competition string `json:"competition"`
	Round       int    `json:"round"`
	KickoffText string `json:"kickoff_text"`
	Season      int    `json:"season"`
	Archived    bool   `json:"archived"`
	Home        string `json:"home"`
	Away        string `json:"away"`
	HomeGoals   int    `json:"home_goals"`
	AwayGoals   int    `json:"away_goals"`
	HasReplay   bool   `json:"has_replay"`
}

func (c *Client) Fixtures(tier, limit int) ([]Fixture, error) {
	var fx []Fixture
	err := c.get(fmt.Sprintf("/v1/fixtures?tier=%d&limit=%d", tier, limit), &fx)
	return fx, err
}

type MatchEvent struct {
	Minute int    `json:"minute"`
	Club   string `json:"club"`
	Player string `json:"player"`
	Detail string `json:"detail"`
}

type MatchSub struct {
	Minute int    `json:"minute"`
	Club   string `json:"club"`
	Off    string `json:"off"`
	On     string `json:"on"`
	Reason string `json:"reason"`
}

type MatchDiagnostics struct {
	ShotQuality       map[string]int `json:"shot_quality"`
	ShotQualityBySide map[string]int `json:"shot_quality_by_side"`
	AerialDuels       map[string]int `json:"aerial_duels"`
	AerialWins        map[string]int `json:"aerial_wins"`
	PressTurnovers    map[string]int `json:"press_turnovers"`
	SetPieceThreat    map[string]int `json:"set_piece_threat"`
	TacticalTilt      map[string]int `json:"tactical_tilt"`
}

type MatchDetail struct {
	Fixture           int64            `json:"fixture"`
	Status            string           `json:"status"`
	Archived          bool             `json:"archived"`
	Season            int              `json:"season"`
	Competition       string           `json:"competition"`
	Round             int              `json:"round"`
	KickoffText       string           `json:"kickoff_text"`
	Home              string           `json:"home"`
	Away              string           `json:"away"`
	HomeGoals         int              `json:"home_goals"`
	AwayGoals         int              `json:"away_goals"`
	Winner            string           `json:"winner"`
	HomeShots         int              `json:"home_shots"`
	AwayShots         int              `json:"away_shots"`
	ChanceTypes       map[string]int   `json:"chance_types"`
	ChanceTypesBySide map[string]int   `json:"chance_types_by_side"`
	Diagnostics       MatchDiagnostics `json:"diagnostics"`
	Scorers           []MatchEvent     `json:"scorers"`
	Cards             []MatchEvent     `json:"cards"`
	Subs              []MatchSub       `json:"subs"`
	Ratings           []LiveRating     `json:"ratings"`
	HomeLineup        []LineupEntry    `json:"home_lineup"`
	AwayLineup        []LineupEntry    `json:"away_lineup"`
	// Story is the daemon-rendered post-match report prose.
	Story      []string         `json:"story"`
	Commentary []string         `json:"commentary"`
	Beats      []CommentaryBeat `json:"beats"`
}

// LineupEntry is one team-sheet row of the match pop-up's lineup panel:
// starters in XI order, then players who came on (OnMinute set), then — live
// only — unused bench players flagged Bench.
type LineupEntry struct {
	Name      string `json:"name"`
	Position  string `json:"position"`
	RatingX10 int    `json:"rating_x10"`
	Goals     int    `json:"goals"`
	Yellows   int    `json:"yellows"`
	Red       bool   `json:"red"`
	OffMinute int    `json:"off_minute"`
	OnMinute  int    `json:"on_minute"`
	Bench     bool   `json:"bench"`
}

func (c *Client) Match(id int64) (MatchDetail, error) {
	var md MatchDetail
	err := c.get(fmt.Sprintf("/v1/matches/%d", id), &md)
	return md, err
}

// LiveMarker / LiveMatchView mirror GET /v1/matches/live (docs/07 §4.1).
// CommentaryBeat is a minute-stamped commentary line; older daemons omit it
// and the plain commentary strings remain the fallback.
type CommentaryBeat struct {
	Minute int    `json:"minute"`
	Text   string `json:"text"`
}

type LiveMarker struct {
	Minute int    `json:"minute"`
	Kind   string `json:"kind"` // GOAL | CHANCE | CARD | INJURY | SUB
	Side   string `json:"side"` // HOME | AWAY | NONE
}

// LiveStats / LiveRating mirror the §4.1 side-pane blocks.
type LiveStats struct {
	HomeShots         int              `json:"home_shots"`
	AwayShots         int              `json:"away_shots"`
	HomeCards         int              `json:"home_cards"`
	AwayCards         int              `json:"away_cards"`
	HomeSubs          int              `json:"home_subs"`
	AwaySubs          int              `json:"away_subs"`
	ChanceTypes       map[string]int   `json:"chance_types"`
	ChanceTypesBySide map[string]int   `json:"chance_types_by_side"`
	Diagnostics       MatchDiagnostics `json:"diagnostics"`
}

type LiveRating struct {
	Side      string `json:"side"` // HOME | AWAY
	Name      string `json:"name"`
	RatingX10 int    `json:"rating_x10"`
}

type LiveMatchView struct {
	Fixture     int64            `json:"fixture"`
	Competition string           `json:"competition"`
	Home        string           `json:"home"`
	Away        string           `json:"away"`
	HomeGoals   int              `json:"home_goals"`
	AwayGoals   int              `json:"away_goals"`
	Minute      int              `json:"minute"`
	Commentary  []string         `json:"commentary"`
	Beats       []CommentaryBeat `json:"beats"`
	Markers     []LiveMarker     `json:"markers"`
	Stats       LiveStats        `json:"stats"`
	Ratings     []LiveRating     `json:"ratings"`
	HomeLineup  []LineupEntry    `json:"home_lineup"`
	AwayLineup  []LineupEntry    `json:"away_lineup"`
	Momentum    []int            `json:"momentum"`
}

// HonoursRow / HonoursSeason mirror GET /v1/history (the honours board).
type HonoursRow struct {
	Tier     int    `json:"tier"`
	Champion string `json:"champion"`
	RunnerUp string `json:"runner_up"`
}

type HonoursSeason struct {
	SeasonYear int          `json:"season_year"`
	Divisions  []HonoursRow `json:"divisions"`
	CupWinner  string       `json:"cup_winner"`
}

func (c *Client) History() ([]HonoursSeason, error) {
	var out struct {
		Seasons []HonoursSeason `json:"seasons"`
	}
	err := c.get("/v1/history", &out)
	return out.Seasons, err
}

func (c *Client) LiveMatches() ([]LiveMatchView, error) {
	var out struct {
		Matches []LiveMatchView `json:"matches"`
	}
	err := c.get("/v1/matches/live", &out)
	return out.Matches, err
}

// StreamFeed follows GET /v1/feed (SSE), invoking onLine per feed line
// until ctx ends or the stream drops (caller reconnects).
func (c *Client) StreamFeed(ctx context.Context, onLine func(narrative.Line)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.Base+"/v1/feed?locale="+string(c.Locale), nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{}).Do(req) // no timeout: long-lived stream
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		text := scanner.Text()
		data, ok := strings.CutPrefix(text, "data: ")
		if !ok {
			continue // heartbeats, blanks
		}
		var line narrative.Line
		if err := json.Unmarshal([]byte(data), &line); err != nil {
			continue // system events have a different shape; skip quietly
		}
		if line.Message.Text != "" {
			onLine(line)
		}
	}
	return scanner.Err()
}
