package worldgen

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/sim"
)

// Region is a map cluster feeding place names and rivalries (docs/09 §4.1).
type Region struct {
	ID      int64   `json:"id"`
	Name    string  `json:"name"`
	Culture Culture `json:"culture"`
}

// Colors is a TUI-safe kit pair, clash-checked within a division.
type Colors struct {
	Primary   string `json:"primary"`
	Secondary string `json:"secondary"`
}

type Stadium struct {
	Name     string `json:"name"`
	Capacity int    `json:"capacity"`
}

// Tendencies is the club's persistent character, all 1–20 (docs/09 §4.1).
type Tendencies struct {
	Wealth             int `json:"wealth"`
	BoardPatience      int `json:"board_patience"`
	BoardAmbition      int `json:"board_ambition"`
	FanPatience        int `json:"fan_patience"`
	FanPassion         int `json:"fan_passion"`
	YouthEmphasis      int `json:"youth_emphasis"`
	TrainingFacilities int `json:"training_facilities"`
	YouthFacilities    int `json:"youth_facilities"`
}

type Club struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	ShortName    string     `json:"short_name"`
	Culture      Culture    `json:"culture"`
	RegionID     int64      `json:"region_id"`
	DivisionTier int        `json:"division_tier"` // 1 = top
	Colors       Colors     `json:"colors"`
	Stadium      Stadium    `json:"stadium"`
	Tendencies   Tendencies `json:"tendencies"`

	// Stage 5: media prediction & board objective (docs/09 §3).
	PredictedFinish      int `json:"predicted_finish"`
	BoardObjectiveFinish int `json:"board_objective_finish"`
	ConfidenceBaseline   int `json:"confidence_baseline"` // 0–100, the season's starting point
	// Confidence is the live board confidence (0–100), seeded to
	// ConfidenceBaseline and moved by league results vs expectation. The agent sees
	// it only as a Descriptor (confidenceBand / securityBand), never the number.
	// Zero reads as uninitialised; LoadSnapshot reseeds it.
	Confidence int `json:"confidence,omitempty"`

	// Sacking state machine. SackState is "" (OK) → WARNED →
	// ULTIMATUM; the board's patience with the current manager, on the CLUB because
	// managers move. During an ultimatum the club must gain UltimatumStartPoints +
	// target points by UltimatumUntil or the manager is dismissed.
	SackState            string       `json:"sack_state,omitempty"`
	UltimatumUntil       sim.GameTime `json:"ultimatum_until,omitempty"`
	UltimatumStartPoints int          `json:"ultimatum_start_points,omitempty"`

	// Stage 6: one intake day per club in the spring window.
	YouthIntakeDay int `json:"youth_intake_day"` // day-of-season

	// Stage 7: economy, in Crowns minor units (docs/09 §4.5).
	BalanceMinor          int64 `json:"balance_minor"`
	WageBillWeeklyMinor   int64 `json:"wage_bill_weekly_minor"`
	WageBudgetWeeklyMinor int64 `json:"wage_budget_weekly_minor"`
	TransferBudgetMinor   int64 `json:"transfer_budget_minor"`
}

// Contract is a player's deal; wages are weekly, Crowns minor units.
type Contract struct {
	WageWeeklyMinor  int64 `json:"wage_weekly_minor"`
	ExpirySeasonYear int   `json:"expiry_season_year"` // ends June 30 of that season
}

type Player struct {
	ID        int64              `json:"id"`
	Name      string             `json:"name"`
	Culture   Culture            `json:"culture"`
	Age       int                `json:"age"`
	HeightCm  int                `json:"height_cm"`
	WeightKg  int                `json:"weight_kg"`
	ClubID    int64              `json:"club_id"` // 0 = free agent
	Position  string             `json:"position"`
	Group     attr.PositionGroup `json:"group"`
	Archetype string             `json:"archetype"`
	Foot      attr.PreferredFoot `json:"foot"`
	WeakFoot  int                `json:"weak_foot"`

	AbilityPool  int `json:"ability_pool"`  // 0–200, sampled from the band
	PotentialCap int `json:"potential_cap"` // pool + age-shrinking headroom

	Visible map[attr.Visible]int `json:"visible"`
	// Hidden holds the 1–20 hidden attributes (trajectory minus the cap,
	// personality, volatility). PotentialCap and Reputation use their own
	// scales and live in dedicated fields.
	Hidden      map[attr.Hidden]int        `json:"hidden"`
	Reputation  int                        `json:"reputation"` // 0–10,000
	Familiarity attr.PositionalFamiliarity `json:"familiarity"`

	Contract *Contract `json:"contract,omitempty"` // nil for free agents
	Youth    bool      `json:"youth"`
	// Retired marks a player whose career has ended (player lifecycle): the row
	// stays in World.Players forever — news and results reference player ids, so
	// deletion would dangle them — but every market, selection, and scouting
	// predicate skips it. omitempty keeps generated worlds compact.
	Retired bool `json:"retired,omitempty"`

	// InjuredUntil is the game time the player's current injury heals. Recovery
	// is a pure timestamp comparison — selection skips a player
	// while kickoff < InjuredUntil; no recovery event exists. The exact time
	// derives from hidden InjuryProne/Recovery, so only the OWNING manager sees
	// a return date; everyone else gets a coarse band (FR-22). Zero (and any
	// past time) reads as fit.
	InjuredUntil sim.GameTime `json:"injured_until,omitempty"`
	// Injuries is the injury history: season + coarse severity band only,
	// never exact durations (FR-22).
	Injuries []InjuryRecord `json:"injuries,omitempty"`
	// Career archives each played season's line at the rollover
	// — only seasons with at least one appearance.
	Career []SeasonRecord `json:"career,omitempty"`
	// FormX10 is the rolling last-formWindow match ratings (×10 ints), appended
	// at each full time and cleared at the rollover — form is season-scoped,
	// like the club form strings.
	FormX10 []int `json:"form_x10,omitempty"`

	// Match state. Condition is energy (0–100, drains in matches,
	// recovers over days); Sharpness is match fitness (0–100). Season stats
	// accumulate across played fixtures; RatingSumX10 holds rating×10 as an
	// integer so nothing that reaches the world hash is a float (NFR-2). The
	// average rating is RatingSumX10 / (10·Apps).
	Condition    int `json:"condition"`
	Sharpness    int `json:"sharpness"`
	SeasonApps   int `json:"season_apps,omitempty"`
	SeasonGoals  int `json:"season_goals,omitempty"`
	RatingSumX10 int `json:"rating_sum_x10,omitempty"`
}

// InjuryRecord is one past injury: the season it happened and its coarse
// severity band (DAYS | WEEKS | MONTH) — the only injury facts that ever cross
// the wire (FR-22).
type InjuryRecord struct {
	SeasonYear int    `json:"season_year"`
	Band       string `json:"band"`
}

// SeasonRecord is one archived season of a player's career: the season line
// exactly as it stood at the rollover, club included. Ratings stay ×10
// integers (no float on the hash); the average renders at read time.
type SeasonRecord struct {
	SeasonYear   int   `json:"season_year"`
	ClubID       int64 `json:"club_id,omitempty"`
	Apps         int   `json:"apps"`
	Goals        int   `json:"goals"`
	RatingSumX10 int   `json:"rating_sum_x10"`
}

// SeasonSummary is one archived season of the world: the final tables (index
// tier-1), the cup winner, and the season's full result ledger
// (compact archiveCopy form: commentary stripped, every fact kept).
type SeasonSummary struct {
	SeasonYear  int           `json:"season_year"`
	FinalTables [][]Standing  `json:"final_tables"`
	CupWinnerID int64         `json:"cup_winner_id,omitempty"`
	Results     []MatchResult `json:"results,omitempty"`
}

// FocusSpend is one Focus charge, kept for get_focus history (docs/11 §3).
type FocusSpend struct {
	Tool     string       `json:"tool"`
	Cost     int          `json:"cost"`
	GameTime sim.GameTime `json:"game_time"`
}

// ManagerStatus is a manager's lifecycle state. Only RETIRED is distinguished
// from the zero value, so an empty string reads as ACTIVE.
type ManagerStatus string

const (
	ManagerActive  ManagerStatus = "ACTIVE"
	ManagerRetired ManagerStatus = "RETIRED"
)

type Manager struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	Culture       Culture `json:"culture"`
	Age           int     `json:"age"`
	ClubID        int64   `json:"club_id"` // 0 = unemployed pool
	Archetype     string  `json:"archetype"`
	Reputation    int     `json:"reputation"` // 0–10,000
	Coaching      int     `json:"coaching"`
	ManManagement int     `json:"man_management"`

	// Lifecycle. Status is ACTIVE unless the manager has RETIRED
	// (out of the game — a bound token then rejects calls with MANAGER_RETIRED and
	// Focus regen stops). Employment is SEPARATE: an ACTIVE manager is employed
	// (ClubID != 0) or unemployed (ClubID 0). Caretaker marks an auto-installed
	// interim manager. Both are zero-safe: an empty Status reads as ACTIVE.
	Status    ManagerStatus `json:"status,omitempty"`
	Caretaker bool          `json:"caretaker,omitempty"`

	Mindset mindset.Mindset `json:"mindset"`

	// Focus economy state (docs/11 §2), synced lazily on MCP calls.
	// Regen is exactly 1 FP per (60 / RegenPerGameHour) game-minutes —
	// integer ticks, no float drift (NFR-2).
	FocusBalance   int          `json:"focus_balance"`
	FocusRegenMark sim.GameTime `json:"focus_regen_mark"`
	FocusSpends    []FocusSpend `json:"focus_spends,omitempty"` // newest last, cap 20

	Alerts *AlertState `json:"alerts,omitempty"`

	// Disposition drift accounting (FR-16b): drift applies on the decision
	// cadence; the anchor/credit pair converts elapsed game time into
	// whole drift points without float accumulation.
	DriftAnchor        sim.GameTime `json:"drift_anchor,omitempty"`
	DriftCreditMinutes int64        `json:"drift_credit_minutes,omitempty"`

	// LastActiveGameTime is the GAME time of the most recent authenticated MCP
	// call on this manager's token — the avatar-liveness signal for retirement
	// exemption (FR-14e, manager careers). Written only by the gateway (manager careers) on
	// each accepted call, so it stays zero for autonomous managers; the engine
	// reads it to grant the 2-game-year retirement grace period. Game time (not
	// wall-clock) keeps it deterministic: it lands in the snapshot and is
	// reproduced by the input-log replay contract (NFR-2). Zero is the explicit
	// "never active ⇒ autonomous ⇒ never exempt" sentinel.
	LastActiveGameTime sim.GameTime `json:"last_active_game_time,omitempty"`
}

// Standing is one last-season table row (docs/09 §4.4).
type Standing struct {
	Pos          int   `json:"pos"`
	ClubID       int64 `json:"club_id"`
	Played       int   `json:"played"`
	Won          int   `json:"won"`
	Drawn        int   `json:"drawn"`
	Lost         int   `json:"lost"`
	GoalsFor     int   `json:"goals_for"`
	GoalsAgainst int   `json:"goals_against"`
	Points       int   `json:"points"`
}

// Rivalry links two clubs; ClubA < ClubB. Weight 1–3 feeds derby effects.
type Rivalry struct {
	ClubA  int64 `json:"club_a"`
	ClubB  int64 `json:"club_b"`
	Weight int   `json:"weight"`
}

const (
	CompetitionLeague = "LEAGUE"
	CompetitionCup    = "CUP"
)

type Fixture struct {
	ID           int64        `json:"id"`
	Competition  string       `json:"competition"`
	DivisionTier int          `json:"division_tier,omitempty"` // league only
	Round        int          `json:"round"`
	Kickoff      sim.GameTime `json:"kickoff"`
	HomeID       int64        `json:"home_id"`
	AwayID       int64        `json:"away_id"`
}

// NewsItem is one condensed narrative record (docs/11 §4 get_news):
// structured key + params; display layers render per locale.
type NewsItem struct {
	ID       int64          `json:"id"`
	GameTime sim.GameTime   `json:"game_time"`
	Category string         `json:"category"` // transfer|match|injury|board|media|decision|career|youth|contract
	Key      string         `json:"key"`
	Params   map[string]any `json:"params,omitempty"`
	// ClubIDs scope the item (get_news scope=own/league); empty = world-wide.
	ClubIDs []int64 `json:"club_ids,omitempty"`
	// ManagerID marks a private item (scout reports) — visible only to
	// that manager, never on public surfaces.
	ManagerID int64 `json:"manager_id,omitempty"`
}

// NewsCap bounds the ring (tunable, docs/98): old items fall off; agents
// follow cursors, spectators follow the live feed.
const NewsCap = 2000

// Evidence is one scout impression (docs/11 §1.4): prose key + confidence.
type Evidence struct {
	Key        string         `json:"key"`
	Params     map[string]any `json:"params,omitempty"`
	Confidence string         `json:"confidence"` // LOW | MEDIUM | HIGH
	GameTime   sim.GameTime   `json:"game_time"`
}

// Knowledge is one manager's accumulated view of one player (FR-22a):
// Level widens to exact (0 ±wide bands … 3 exact); re-scouting deepens.
type Knowledge struct {
	Level    int        `json:"level"` // 0–3
	Evidence []Evidence `json:"evidence,omitempty"`
}

// World is the full generated state. It deliberately carries no credentials:
// tokens live in the Manifest so the hash — and the replay contract — cover
// world state only.
type World struct {
	Config     WorldConfig  `json:"config"`
	Derived    Derived      `json:"derived"`
	Regions    []Region     `json:"regions"`
	Clubs      []Club       `json:"clubs"`
	Managers   []Manager    `json:"managers"`    // employed first, then the pool
	Players    []Player     `json:"players"`     // per club: squad then academy; free agents last
	LastSeason [][]Standing `json:"last_season"` // index tier-1
	Rivalries  []Rivalry    `json:"rivalries"`
	Fixtures   []Fixture    `json:"fixtures"` // league season + cup round 1
	CupByes    []int64      `json:"cup_byes"` // club IDs entering at round 2

	// History is the permanent season archive: each rollover
	// appends the finished season's final tables and cup winner BEFORE the
	// per-season reset erases them (LastSeason only ever holds one season).
	// Append-only, one entry per season.
	History []SeasonSummary `json:"history,omitempty"`
	// CupChampionID is the current season's crowned cup winner (0 until the
	// final) — recorded by the engine at the crowning, archived and cleared by
	// the rollover.
	CupChampionID int64 `json:"cup_champion_id,omitempty"`

	// Living-world state — persisted with the snapshot.
	News       []NewsItem                     `json:"news,omitempty"`
	NewsNextID int64                          `json:"news_next_id,omitempty"`
	Knowledge  map[int64]map[int64]*Knowledge `json:"knowledge,omitempty"` // manager → player → view

	// Match state — persisted with the snapshot.
	// Table is the live current-season standings (index tier-1), seeded empty
	// at generation and updated at each league full-time. Results append in
	// full-time (kickoff-drain) order — a slice, not a map, so ordering never
	// enters the hash. LiveMatches holds the running tally of in-progress
	// fixtures so a mid-match snapshot→reload resumes identically (keyed by
	// fixture id; JSON sorts map keys, so the hash stays deterministic).
	Table       [][]Standing         `json:"table,omitempty"`
	Results     []MatchResult        `json:"results,omitempty"`
	LiveMatches map[int64]*LiveMatch `json:"live_matches,omitempty"`
	// NextFixtureID keeps fixture ids monotonic across season regenerations
	// never reset, so persisted news items keep referencing valid
	// ids and season-2 fixtures can't collide with season-1 ones.
	NextFixtureID int64 `json:"next_fixture_id,omitempty"`

	// NextManagerID keeps manager ids monotonic across runtime spawns: caretakers
	// and newgen backfills allocate from here, never reusing
	// a retired manager's id. Set at generation to the last generated id.
	NextManagerID int64 `json:"next_manager_id,omitempty"`

	// NextPlayerID keeps player ids monotonic across runtime spawns: each
	// spring's youth intake allocates from here, never reusing a
	// departed player's id. Set at generation to the last generated id.
	NextPlayerID int64 `json:"next_player_id,omitempty"`
}

// AddNews appends to the ring, assigning the next id.
func (w *World) AddNews(n NewsItem) NewsItem {
	w.NewsNextID++
	n.ID = w.NewsNextID
	w.News = append(w.News, n)
	if len(w.News) > NewsCap {
		w.News = w.News[len(w.News)-NewsCap:]
	}
	return n
}

// KnowledgeFor returns (creating on write) a manager's view of a player.
func (w *World) KnowledgeFor(managerID, playerID int64) *Knowledge {
	if w.Knowledge == nil {
		w.Knowledge = map[int64]map[int64]*Knowledge{}
	}
	mk, ok := w.Knowledge[managerID]
	if !ok {
		mk = map[int64]*Knowledge{}
		w.Knowledge[managerID] = mk
	}
	k, ok := mk[playerID]
	if !ok {
		k = &Knowledge{}
		mk[playerID] = k
	}
	return k
}

// KnowledgeLevel is the read-only variant (no allocation).
func (w *World) KnowledgeLevel(managerID, playerID int64) int {
	if mk, ok := w.Knowledge[managerID]; ok {
		if k, ok := mk[playerID]; ok {
			return k.Level
		}
	}
	return 0
}

// Hash is the canonical world hash: SHA-256 over the world's canonical JSON.
// encoding/json is deterministic here — struct fields serialize in declared
// order and map keys sort — so same config + same seed ⇒ identical hash
// (NFR-2).
func (w *World) Hash() (string, error) {
	b, err := json.Marshal(w)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// ManagerCredential pairs a generated Manager with its issued token
// (docs/09 §5 handover).
type ManagerCredential struct {
	ManagerID   int64  `json:"manager_id"`
	ManagerName string `json:"manager_name"`
	ClubID      int64  `json:"club_id"` // 0 = unemployed
	ClubName    string `json:"club_name,omitempty"`
	Archetype   string `json:"archetype"`
	Reputation  int    `json:"reputation"`
	Token       string `json:"token"`
}

// Manifest is the stage-8 handover document: seed, start state, and Manager
// Tokens. It is not part of the World and never enters the hash.
type Manifest struct {
	WorldName  string              `json:"world_name"`
	Seed       uint64              `json:"seed"`
	StartState string              `json:"start_state"` // ready | running
	Managers   []ManagerCredential `json:"managers"`
}

// Result is everything Generate produces: the world, the credential
// manifest, and the primed event queue (stage 9).
type Result struct {
	World    *World
	Manifest *Manifest
	Queue    *sim.Queue
}
