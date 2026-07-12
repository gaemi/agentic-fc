package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gaemi/agentic-fc/internal/layout"
)

// Tabs (docs/07 §3 — the minimal viewer subset; more screens come with
// their systems).
const (
	tabMedia = iota
	tabTable
	tabClubs
	tabFixtures
	tabAdminSettings
	tabCount
)

const (
	viewerHistoryLimit     = 1000
	scrollPageLines        = 8
	scrollWheelLines       = 4
	pollInterval           = 2 * time.Second
	uiRefreshInterval      = 30 * time.Second
	uiRefreshEveryPolls    = int(uiRefreshInterval / pollInterval)
	settingsUpdateDebounce = 250 * time.Millisecond
	runtimeSettingCount    = 3
	minMatchModalHeight    = 6
	sceneFrameRows         = sceneCanvasHeight + 2 // canvas rows plus box borders
	goalFlashWindowMinutes = 4
	matchAnimationInterval = 180 * time.Millisecond
)

// Sentinel for modalBox lines that are already width-aligned ASCII art.
const preformattedLinePrefix = "\x1f"

type attrPair struct {
	key string
	val int
}

var playerProfileAttrOrder = []string{
	"REFLEXES", "ONE_ON_ONES", "HANDLING", "AERIAL_REACH", "COMMAND_OF_AREA",
	"COMMUNICATION", "DISTRIBUTION", "SWEEPING", "ECCENTRICITY", "PUNCHING",
	"FINISHING", "LONG_SHOTS", "FIRST_TOUCH", "PASSING", "CROSSING",
	"DRIBBLING", "TECHNIQUE", "HEADING", "TACKLING", "MARKING", "SET_PIECES",
	"AGGRESSION", "VISION", "DECISIONS", "COMPOSURE", "CONCENTRATION",
	"POSITIONING", "OFF_BALL", "ANTICIPATION", "WORK_RATE", "BRAVERY",
	"TEAMWORK", "LEADERSHIP", "DETERMINATION", "FLAIR",
	"ACCELERATION", "PACE", "AGILITY", "BALANCE", "STRENGTH", "STAMINA",
	"NATURAL_FITNESS", "JUMPING_REACH",
}

var playerProfileAttrRank = buildPlayerProfileAttrRank()

func buildPlayerProfileAttrRank() map[string]int {
	rank := make(map[string]int, len(playerProfileAttrOrder))
	for i, key := range playerProfileAttrOrder {
		rank[key] = i
	}
	return rank
}

type matchModalKind string

const (
	modalNone    matchModalKind = ""
	modalLive    matchModalKind = "live"
	modalReplay  matchModalKind = "replay"
	modalWaiting matchModalKind = "waiting"
)

const (
	matchSideHome = "HOME"
	matchSideAway = "AWAY"
)

// Model is the viewer's Bubble Tea model. Client may be nil in tests —
// all data arrives via messages either way.
type Model struct {
	Client *Client

	UI            map[string]string
	World         WorldInfo
	News          []NewsArticle
	NewsIdx       int
	ArticleOffset int
	Table         Table
	Clubs         []ClubSummary
	Club          ClubDetail
	ClubIdx       int
	PlayerIdx     int
	Fixtures      []Fixture
	FixtureIdx    int
	MatchDetail   MatchDetail
	ReplayOffset  int
	MatchModal    matchModalKind
	MatchModalID  int64
	// LineupView flips the open match pop-up between the broadcast body and
	// the team-sheet panel ("l"); it resets when the pop-up closes.
	LineupView             bool
	matchAnimationFrame    int
	matchAnimationRun      uint64
	matchAnimationSceneSig string
	matchAnimationPaused   bool
	Matches                []LiveMatchView
	MatchIdx               int
	Notice                 string
	NoticeTTL              int
	LatestNewsID           int64
	LiveCount              int
	AdminMode              bool
	Settings               AdminSettings
	SettingsIdx            int
	SettingsDirty          bool
	SettingsRev            int
	Tab                    int
	Tier                   int
	Width                  int
	Height                 int
	Err                    string
	ConnErr                string // last /v1/ui fetch failure; "" once the catalog arrives

	uiRefreshCountdown int
}

func NewModel(c *Client) Model {
	return Model{Client: c, Tier: 1, UI: map[string]string{}, LiveCount: -1, AdminMode: c != nil && c.AdminToken != ""}
}

// Messages.
type (
	WorldMsg    WorldInfo
	UIMsg       map[string]string
	NewsMsg     []NewsArticle
	TableMsg    Table
	ClubsMsg    []ClubSummary
	ClubMsg     ClubDetail
	FixturesMsg []Fixture
	MatchMsg    MatchDetail
	MatchesMsg  []LiveMatchView
	SettingsMsg struct {
		Settings AdminSettings
		Updated  bool
	}
	SettingsErrMsg struct {
		Err      error
		Updating bool
	}
	SettingsCommitMsg struct{ Rev int }
	ErrMsg            struct{ Err error }
	UIErrMsg          struct{ Err error }
	tickMsg           struct{}
	matchAnimationMsg struct{ Run uint64 }
)

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchUI(), m.fetchWorld(), m.fetchNews(), m.fetchTable(), m.fetchClubs(), m.fetchFixtures(), m.fetchLive(), m.fetchAdminSettings(), tick())
}

func tick() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func matchAnimationTick(run uint64) tea.Cmd {
	return tea.Tick(matchAnimationInterval, func(time.Time) tea.Msg {
		return matchAnimationMsg{Run: run}
	})
}

func (m Model) fetchWorld() tea.Cmd {
	if m.Client == nil {
		return nil
	}
	c := m.Client
	return func() tea.Msg {
		w, err := c.World()
		if err != nil {
			return ErrMsg{err}
		}
		return WorldMsg(w)
	}
}

func (m Model) fetchUI() tea.Cmd {
	if m.Client == nil {
		return nil
	}
	c := m.Client
	return func() tea.Msg {
		s, err := c.UIStrings()
		if err != nil {
			return UIErrMsg{err}
		}
		return UIMsg(s)
	}
}

func (m Model) fetchTable() tea.Cmd {
	if m.Client == nil {
		return nil
	}
	c, tier := m.Client, m.Tier
	return func() tea.Msg {
		t, err := c.Table(tier)
		if err != nil {
			return ErrMsg{err}
		}
		return TableMsg(t)
	}
}

func (m Model) fetchNews() tea.Cmd {
	if m.Client == nil {
		return nil
	}
	c := m.Client
	return func() tea.Msg {
		n, err := c.News(viewerHistoryLimit)
		if err != nil {
			return ErrMsg{err}
		}
		return NewsMsg(n)
	}
}

func (m Model) fetchClubs() tea.Cmd {
	if m.Client == nil {
		return nil
	}
	c := m.Client
	return func() tea.Msg {
		clubs, err := c.Clubs()
		if err != nil {
			return ErrMsg{err}
		}
		return ClubsMsg(clubs)
	}
}

func (m Model) fetchClub() tea.Cmd {
	if m.Client == nil || len(m.Clubs) == 0 {
		return nil
	}
	if m.ClubIdx < 0 {
		m.ClubIdx = 0
	}
	if m.ClubIdx >= len(m.Clubs) {
		m.ClubIdx = len(m.Clubs) - 1
	}
	c, id := m.Client, m.Clubs[m.ClubIdx].ID
	return func() tea.Msg {
		club, err := c.Club(id)
		if err != nil {
			return ErrMsg{err}
		}
		return ClubMsg(club)
	}
}

func (m Model) fetchFixtures() tea.Cmd {
	if m.Client == nil {
		return nil
	}
	c, tier := m.Client, m.Tier
	return func() tea.Msg {
		fx, err := c.Fixtures(tier, viewerHistoryLimit)
		if err != nil {
			return ErrMsg{err}
		}
		return FixturesMsg(fx)
	}
}

func (m Model) fetchMatch() tea.Cmd {
	if m.Client == nil || len(m.Fixtures) == 0 {
		return nil
	}
	if m.FixtureIdx < 0 {
		m.FixtureIdx = 0
	}
	if m.FixtureIdx >= len(m.Fixtures) {
		m.FixtureIdx = len(m.Fixtures) - 1
	}
	f := m.Fixtures[m.FixtureIdx]
	if f.Status != "RESULT" {
		return nil
	}
	c, id := m.Client, f.ID
	return func() tea.Msg {
		md, err := c.Match(id)
		if err != nil {
			return ErrMsg{err}
		}
		return MatchMsg(md)
	}
}

func (m Model) fetchLive() tea.Cmd {
	if m.Client == nil {
		return nil
	}
	c := m.Client
	return func() tea.Msg {
		lv, err := c.LiveMatches()
		if err != nil {
			return ErrMsg{err}
		}
		return MatchesMsg(lv)
	}
}

func (m Model) fetchAdminSettings() tea.Cmd {
	if m.Client == nil || !m.AdminMode {
		return nil
	}
	c := m.Client
	return func() tea.Msg {
		settings, err := c.AdminSettings()
		if err != nil {
			return SettingsErrMsg{Err: err}
		}
		return SettingsMsg{Settings: settings}
	}
}

func (m Model) updateAdminSettings(runtime RuntimeSettings) tea.Cmd {
	if m.Client == nil || !m.AdminMode {
		return nil
	}
	c := m.Client
	return func() tea.Msg {
		settings, err := c.UpdateAdminSettings(runtime)
		if err != nil {
			return SettingsErrMsg{Err: err, Updating: true}
		}
		return SettingsMsg{Settings: settings, Updated: true}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width, m.Height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.MatchModal != modalNone {
				m.closeMatchModal()
				return m, nil
			}
		case "1":
			m.Tab = tabMedia
			m.closeMatchModal()
			m.clearNotice()
		case "2":
			m.Tab = tabTable
			m.closeMatchModal()
			m.clearNotice()
		case "3":
			m.Tab = tabClubs
			m.closeMatchModal()
			m.clearNotice()
		case "4":
			m.Tab = tabFixtures
			m.closeMatchModal()
			m.clearNotice()
		case "5":
			if m.AdminMode {
				m.Tab = tabAdminSettings
				m.closeMatchModal()
				m.clearNotice()
				return m, m.fetchAdminSettings()
			}
		case "enter", " ":
			if msg.String() == " " && m.MatchModal == modalLive {
				return m, m.toggleMatchAnimation()
			}
			if m.Tab == tabFixtures && m.MatchModal == modalNone {
				return m.openSelectedFixture()
			}
		case "up", "k":
			if m.MatchModal != modalNone {
				if m.MatchModal == modalReplay {
					m.ReplayOffset = scrollBack(m.ReplayOffset, scrollWheelLines)
				}
				break
			}
			switch m.Tab {
			case tabMedia:
				if m.NewsIdx > 0 {
					m.NewsIdx--
					m.ArticleOffset = 0
				}
			case tabClubs:
				if m.ClubIdx > 0 {
					m.ClubIdx--
					return m, m.fetchClub()
				}
			case tabFixtures:
				if m.FixtureIdx > 0 {
					m.FixtureIdx--
					m.ReplayOffset = 0
				}
			case tabAdminSettings:
				if m.SettingsIdx > 0 {
					m.SettingsIdx--
				}
			}
		case "down", "j":
			if m.MatchModal != modalNone {
				if m.MatchModal == modalReplay {
					m.ReplayOffset = scrollForward(m.ReplayOffset, len(m.MatchDetail.Commentary), scrollWheelLines)
				}
				break
			}
			switch m.Tab {
			case tabMedia:
				if m.NewsIdx+1 < len(m.News) {
					m.NewsIdx++
					m.ArticleOffset = 0
				}
			case tabClubs:
				if m.ClubIdx+1 < len(m.Clubs) {
					m.ClubIdx++
					return m, m.fetchClub()
				}
			case tabFixtures:
				if m.FixtureIdx+1 < len(m.Fixtures) {
					m.FixtureIdx++
					m.ReplayOffset = 0
				}
			case tabAdminSettings:
				if m.SettingsIdx < runtimeSettingCount-1 {
					m.SettingsIdx++
				}
			}
		case "l":
			if m.MatchModal == modalLive || m.MatchModal == modalReplay {
				m.LineupView = !m.LineupView
			}
		case "tab":
			if m.Tab == tabClubs && len(m.Club.Squad) > 0 {
				m.PlayerIdx = (m.PlayerIdx + 1) % len(m.Club.Squad)
			}
		case "shift+tab":
			if m.Tab == tabClubs && len(m.Club.Squad) > 0 {
				m.PlayerIdx = (m.PlayerIdx + len(m.Club.Squad) - 1) % len(m.Club.Squad)
			}
		case "pgup":
			if m.MatchModal == modalReplay {
				m.ReplayOffset = scrollBack(m.ReplayOffset, scrollPageLines)
				break
			}
			switch m.Tab {
			case tabMedia:
				m.ArticleOffset = scrollBack(m.ArticleOffset, scrollPageLines)
			}
		case "pgdown":
			if m.MatchModal == modalReplay {
				m.ReplayOffset = scrollForward(m.ReplayOffset, len(m.MatchDetail.Commentary), scrollPageLines)
				break
			}
			switch m.Tab {
			case tabMedia:
				m.ArticleOffset = scrollForward(m.ArticleOffset, m.articleScrollLineCount(), scrollPageLines)
			}
		case "+", "=":
			if m.Tab == tabAdminSettings {
				return m.adjustRuntimeSetting(1)
			}
		case "-", "_":
			if m.Tab == tabAdminSettings {
				return m.adjustRuntimeSetting(-1)
			}
		case "[", "{":
			if m.Tab == tabAdminSettings {
				return m.adjustRuntimeSetting(-1)
			}
		case "]", "}":
			if m.Tab == tabAdminSettings {
				return m.adjustRuntimeSetting(1)
			}
		case "left":
			if m.MatchModal == modalWaiting {
				break
			}
			if m.MatchModal == modalLive {
				if n := len(m.Matches); n > 0 {
					m.MatchIdx = (m.MatchIdx + n - 1) % n // wrap: cycling, not clamping
					m.MatchModalID = m.Matches[m.MatchIdx].Fixture
					if idx := m.fixtureIndexByID(m.MatchModalID); idx >= 0 {
						m.FixtureIdx = idx
					}
					m.resetMatchAnimationScene()
				}
				break
			}
			if m.MatchModal == modalReplay {
				m.ReplayOffset = scrollBack(m.ReplayOffset, scrollPageLines)
				break
			}
			if (m.Tab == tabTable || m.Tab == tabFixtures) && m.Tier > 1 {
				m.Tier--
				return m, tea.Batch(m.fetchTable(), m.fetchFixtures())
			}
		case "right":
			if m.MatchModal == modalWaiting {
				break
			}
			if m.MatchModal == modalLive {
				if n := len(m.Matches); n > 0 {
					m.MatchIdx = (m.MatchIdx + 1) % n
					m.MatchModalID = m.Matches[m.MatchIdx].Fixture
					if idx := m.fixtureIndexByID(m.MatchModalID); idx >= 0 {
						m.FixtureIdx = idx
					}
					m.resetMatchAnimationScene()
				}
				break
			}
			if m.MatchModal == modalReplay {
				m.ReplayOffset = scrollForward(m.ReplayOffset, len(m.MatchDetail.Commentary), scrollPageLines)
				break
			}
			if (m.Tab == tabTable || m.Tab == tabFixtures) && (m.World.Divisions == 0 || m.Tier < m.World.Divisions) {
				m.Tier++
				return m, tea.Batch(m.fetchTable(), m.fetchFixtures())
			}
		}
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case WorldMsg:
		m.World = WorldInfo(msg)
		m.Err = ""
	case UIMsg:
		m.UI = msg
		m.ConnErr = ""
		m.uiRefreshCountdown = uiRefreshEveryPolls
	case UIErrMsg:
		// The catalog endpoint is unauthenticated and always present, so a
		// failure here — unlike per-pane poll errors — means the server
		// itself is unreachable or unhealthy.
		m.ConnErr = msg.Err.Error()
		m.Err = msg.Err.Error()
	case NewsMsg:
		if len(msg) > 0 && msg[0].ID > m.LatestNewsID {
			if m.LatestNewsID != 0 {
				m.setNotice(strings.ReplaceAll(m.ui("ui.notice.news"), "{title}", msg[0].Title))
			}
			m.LatestNewsID = msg[0].ID
		}
		m.News = []NewsArticle(msg)
		if m.NewsIdx >= len(m.News) {
			m.NewsIdx = 0
			m.ArticleOffset = 0
		}
		if len(m.News) == 0 {
			m.ArticleOffset = 0
		}
	case TableMsg:
		m.Table = Table(msg)
	case ClubsMsg:
		m.Clubs = []ClubSummary(msg)
		if m.ClubIdx >= len(m.Clubs) {
			m.ClubIdx = 0
		}
		if len(m.Clubs) > 0 && m.Club.ID != m.Clubs[m.ClubIdx].ID {
			return m, m.fetchClub()
		}
	case ClubMsg:
		m.Club = ClubDetail(msg)
		if m.PlayerIdx >= len(m.Club.Squad) {
			m.PlayerIdx = 0
		}
	case FixturesMsg:
		m.Fixtures = msg
		if m.FixtureIdx >= len(m.Fixtures) {
			m.FixtureIdx = 0
		}
		if m.MatchModal == modalReplay && m.MatchModalID != 0 {
			if idx := m.fixtureIndexByID(m.MatchModalID); idx >= 0 {
				m.FixtureIdx = idx
			}
		}
		if m.MatchModal == modalWaiting {
			if idx := m.fixtureIndexByID(m.MatchModalID); idx >= 0 {
				m.FixtureIdx = idx
				if m.Fixtures[idx].Status == "RESULT" {
					m.MatchModal = modalReplay
					m.MatchDetail = MatchDetail{}
					m.ReplayOffset = 0
					return m, m.fetchMatch()
				}
			} else {
				m.closeMatchModal()
				m.setNotice(m.ui("ui.match.ended"))
			}
		}
	case MatchMsg:
		detail := MatchDetail(msg)
		if m.MatchModal != modalReplay || detail.Fixture != m.MatchModalID {
			break
		}
		m.MatchDetail = detail
		if m.ReplayOffset >= len(m.MatchDetail.Commentary) {
			m.ReplayOffset = 0
		}
	case MatchesMsg:
		if m.LiveCount >= 0 && len(msg) > m.LiveCount {
			m.setNotice(strings.ReplaceAll(m.ui("ui.notice.match"), "{count}", fmt.Sprint(len(msg))))
		}
		m.LiveCount = len(msg)
		m.Matches = msg
		if m.MatchModal == modalLive {
			if idx := m.liveIndexForFixture(m.MatchModalID); idx >= 0 {
				m.MatchIdx = idx
				m.syncMatchAnimationScene()
			} else {
				return m.liveModalFinished()
			}
		} else if m.MatchModal == modalWaiting {
			if idx := m.liveIndexForFixture(m.MatchModalID); idx >= 0 {
				m.MatchIdx = idx
				m.MatchModal = modalLive
				m.startMatchAnimation()
				return m, matchAnimationTick(m.matchAnimationRun)
			} else {
				m.closeMatchModal()
				m.setNotice(m.ui("ui.match.ended"))
			}
		} else if m.MatchIdx >= len(m.Matches) {
			m.MatchIdx = 0
		}
	case SettingsMsg:
		if msg.Updated {
			if !m.SettingsDirty || sameRuntimeSettings(m.Settings.Runtime, msg.Settings.Runtime) {
				m.Settings = msg.Settings
				m.SettingsDirty = false
				m.setNotice(m.ui("ui.admin.settings.saved"))
			} else {
				m.Settings.Schema = msg.Settings.Schema
			}
		} else if !m.SettingsDirty {
			m.Settings = msg.Settings
		}
	case SettingsErrMsg:
		if msg.Updating {
			m.SettingsDirty = false
			m.setNotice(msg.Err.Error())
		} else {
			m.Err = msg.Err.Error()
		}
	case SettingsCommitMsg:
		if msg.Rev == m.SettingsRev && m.SettingsDirty {
			return m, m.updateAdminSettings(m.Settings.Runtime)
		}
	case ErrMsg:
		m.Err = msg.Err.Error()
	case matchAnimationMsg:
		if msg.Run != m.matchAnimationRun || m.MatchModal != modalLive || m.matchAnimationPaused {
			break
		}
		m.matchAnimationFrame++
		return m, matchAnimationTick(m.matchAnimationRun)
	case tickMsg:
		m.ageNotice()
		cmds := []tea.Cmd{m.fetchWorld(), m.fetchNews(), m.fetchTable(), m.fetchClubs(), m.fetchClub(), m.fetchFixtures(), m.fetchLive(), tick()}
		if cmd := m.refreshUIIfDue(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if m.AdminMode && m.Tab == tabAdminSettings {
			cmds = append(cmds, m.fetchAdminSettings())
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// refreshUIIfDue keeps a long-running Console synchronized with server-side
// catalog changes. An empty catalog retries every normal poll so a Console
// started before the daemon recovers without being restarted.
func (m *Model) refreshUIIfDue() tea.Cmd {
	if len(m.UI) == 0 {
		return m.fetchUI()
	}
	m.uiRefreshCountdown--
	if m.uiRefreshCountdown > 0 {
		return nil
	}
	return m.fetchUI()
}

const noticeTicks = 4

func (m *Model) setNotice(text string) {
	m.Notice = text
	m.NoticeTTL = noticeTicks
}

func (m *Model) clearNotice() {
	m.Notice = ""
	m.NoticeTTL = 0
}

func (m *Model) ageNotice() {
	if m.NoticeTTL <= 0 {
		m.clearNotice()
		return
	}
	m.NoticeTTL--
	if m.NoticeTTL == 0 {
		m.Notice = ""
	}
}

func (m *Model) closeMatchModal() {
	m.stopMatchAnimation()
	m.MatchModal = modalNone
	m.MatchModalID = 0
	m.LineupView = false
}

func (m *Model) startMatchAnimation() {
	m.matchAnimationRun++
	m.matchAnimationFrame = 0
	m.matchAnimationSceneSig = ""
	m.matchAnimationPaused = false
	m.syncMatchAnimationScene()
}

func (m *Model) stopMatchAnimation() {
	m.matchAnimationRun++
	m.matchAnimationFrame = 0
	m.matchAnimationSceneSig = ""
	m.matchAnimationPaused = false
}

func (m *Model) toggleMatchAnimation() tea.Cmd {
	m.matchAnimationRun++
	m.matchAnimationPaused = !m.matchAnimationPaused
	if m.matchAnimationPaused {
		return nil
	}
	return matchAnimationTick(m.matchAnimationRun)
}

func (m Model) matchAnimationHelp() string {
	if m.matchAnimationPaused {
		return m.ui("ui.match.modal.animation_resume")
	}
	return m.ui("ui.match.modal.animation_pause")
}

func (m *Model) resetMatchAnimationScene() {
	m.matchAnimationFrame = 0
	m.matchAnimationSceneSig = ""
	m.syncMatchAnimationScene()
}

func (m *Model) syncMatchAnimationScene() {
	if m.MatchModal != modalLive || m.MatchIdx < 0 || m.MatchIdx >= len(m.Matches) {
		return
	}
	mv := m.Matches[m.MatchIdx]
	current := ""
	if len(mv.Commentary) > 0 {
		current = mv.Commentary[len(mv.Commentary)-1]
	}
	signature := fmt.Sprintf("%d:%d:%s", mv.Fixture, len(mv.Commentary), current)
	if signature != m.matchAnimationSceneSig {
		m.matchAnimationSceneSig = signature
		m.matchAnimationFrame = 0
	}
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelUp {
		if m.MatchModal == modalReplay {
			m.ReplayOffset = scrollBack(m.ReplayOffset, scrollWheelLines)
			return m, nil
		}
		switch m.Tab {
		case tabMedia:
			m.ArticleOffset = scrollBack(m.ArticleOffset, scrollWheelLines)
		}
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		if m.MatchModal == modalReplay {
			m.ReplayOffset = scrollForward(m.ReplayOffset, len(m.MatchDetail.Commentary), scrollWheelLines)
			return m, nil
		}
		switch m.Tab {
		case tabMedia:
			m.ArticleOffset = scrollForward(m.ArticleOffset, m.articleScrollLineCount(), scrollWheelLines)
		}
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if m.MatchModal != modalNone {
		return m, nil
	}
	if msg.Y == 2 {
		if tab, ok := m.clickedTab(msg.X); ok {
			m.Tab = tab
			m.closeMatchModal()
			m.clearNotice()
			return m, nil
		}
	}
	bodyWidth := m.Width - 2
	if bodyWidth <= 0 {
		bodyWidth = 100
	}
	bodyY, bodyX := 4, 1
	switch m.Tab {
	case tabMedia:
		listWidth := bodyWidth
		if bodyWidth >= 110 {
			listWidth = bodyWidth / 3
			if listWidth < 34 {
				listWidth = 34
			}
			if listWidth > 52 {
				listWidth = 52
			}
		}
		if msg.X < bodyX || msg.X >= bodyX+listWidth {
			return m, nil
		}
		row := msg.Y - bodyY - 3
		maxRows := m.Height - 10
		if bodyWidth < 110 {
			row = msg.Y - (bodyY + maxInt(1, (m.Height-6)/2+1)) - 3
			maxRows = m.Height / 2
		}
		if idx, ok := clickedListIndex(row, m.NewsIdx, len(m.News), maxRows); ok {
			m.NewsIdx = idx
			m.ArticleOffset = 0
		}
	case tabTable:
		row := msg.Y - bodyY - 4
		if row >= 0 && row < len(m.Table.Rows) {
			id := m.Table.Rows[row].ClubID
			for i, c := range m.Clubs {
				if c.ID == id {
					m.Tab = tabClubs
					m.ClubIdx = i
					m.PlayerIdx = 0
					return m, m.fetchClub()
				}
			}
		}
	case tabClubs:
		listWidth := 0
		if bodyWidth >= 120 {
			listWidth = bodyWidth / 3
			if listWidth < 36 {
				listWidth = 36
			}
			if listWidth > 54 {
				listWidth = 54
			}
		}
		if listWidth > 0 && msg.X >= bodyX && msg.X < bodyX+listWidth {
			row := msg.Y - bodyY - 3
			maxRows := m.Height - 10
			if idx, ok := clickedListIndex(row, m.ClubIdx, len(m.Clubs), maxRows); ok {
				m.ClubIdx = idx
				m.PlayerIdx = 0
				return m, m.fetchClub()
			}
			return m, nil
		}
		if idx, ok := m.clickedSquadIndex(msg.Y - bodyY); ok {
			m.PlayerIdx = idx
		}
	case tabFixtures:
		if msg.X < bodyX || msg.X >= bodyX+bodyWidth {
			return m, nil
		}
		row := msg.Y - bodyY - 3
		maxRows := m.Height - 10
		if idx, ok := clickedListIndex(row, m.FixtureIdx, len(m.Fixtures), maxRows); ok {
			m.FixtureIdx = idx
			return m.openSelectedFixture()
		}
	case tabAdminSettings:
		row := m.adminSettingsRowAt(bodyWidth, msg.Y-bodyY)
		if row >= 0 && row < runtimeSettingCount {
			m.SettingsIdx = row
		}
	}
	return m, nil
}

type tabSpan struct {
	tab        int
	start, end int
}

func (m Model) clickedTab(x int) (int, bool) {
	for _, span := range m.tabSpans() {
		if x >= span.start && x <= span.end {
			return span.tab, true
		}
	}
	return 0, false
}

func (m Model) tabSpans() []tabSpan {
	division := strings.ReplaceAll(m.ui("ui.header.division"), "{tier}", fmt.Sprint(m.Tier))
	names := m.tabNames()
	x := lipgloss.Width(division) + 3 + 1 // +1 for the frame's left border.
	spans := make([]tabSpan, 0, len(names))
	for i, n := range names {
		label := fmt.Sprintf("%d %s", i+1, n)
		w := lipgloss.Width(label)
		spans = append(spans, tabSpan{tab: i, start: x, end: x + w - 1})
		x += w + 2
	}
	return spans
}

func clickedListIndex(row, selected, total, visibleRows int) (int, bool) {
	if row < 0 || total == 0 {
		return 0, false
	}
	start := visibleStart(selected, total, visibleRows)
	idx := start + row
	if idx < 0 || idx >= total {
		return 0, false
	}
	return idx, true
}

func scrollBack(offset, step int) int {
	offset -= step
	if offset < 0 {
		return 0
	}
	return offset
}

func scrollForward(offset, total, step int) int {
	if total <= 0 {
		return 0
	}
	offset += step
	if offset >= total {
		return total - 1
	}
	return offset
}

func (m Model) articleScrollLineCount() int {
	if len(m.News) == 0 || m.NewsIdx < 0 || m.NewsIdx >= len(m.News) {
		return 0
	}
	return len(wrapParagraphs(m.News[m.NewsIdx].Body, m.articleDetailWidth()))
}

func (m Model) articleDetailWidth() int {
	width := m.Width - 2
	if width <= 0 {
		width = 100
	}
	if width >= 110 {
		listWidth := width / 3
		if listWidth < 34 {
			listWidth = 34
		}
		if listWidth > 52 {
			listWidth = 52
		}
		return width - listWidth - 1
	}
	return width
}

// disconnected reports the pre-first-connection failure state: the console
// has never obtained the server-rendered catalog and the catalog fetch
// itself failed. Per-pane poll errors (admin auth, a single bad endpoint)
// do not qualify — only the unauthenticated /v1/ui probe decides, so a
// healthy daemon is never reported as unreachable. In this state every
// string would otherwise render as a raw key, so the view swaps the tab
// body for guidance built from the English fallbacks.
func (m Model) disconnected() bool {
	return len(m.UI) == 0 && m.ConnErr != ""
}

func (m Model) viewDisconnected(width int) string {
	server := ""
	if m.Client != nil {
		server = m.Client.Base
	}
	lines := []string{
		"",
		styleHeader.Render(truncate(m.ui("ui.disconnected.title"), width)),
		"",
		truncate(strings.ReplaceAll(m.ui("ui.disconnected.server"), "{url}", server), width),
		"",
		truncate(m.ui("ui.disconnected.hint_daemon"), width),
		truncate(m.ui("ui.disconnected.hint_server"), width),
		"",
		styleDim.Render(truncate(m.ui("ui.disconnected.retrying")+" "+m.ConnErr, width)),
	}
	return strings.Join(lines, "\n")
}

// ui resolves a server-rendered string with a graceful key fallback.
func (m Model) ui(key string) string {
	if s, ok := m.UI[key]; ok {
		return s
	}
	if s, ok := uiFallbacks[key]; ok {
		return s
	}
	return key
}

var uiFallbacks = map[string]string{
	// Chrome shown before the first successful /v1/ui fetch — without these
	// a console launched ahead of its daemon renders raw keys.
	"ui.app.title":                    "Agentic FC",
	"ui.header.division":              "Division {tier}",
	"ui.tab.media":                    "Media",
	"ui.tab.table":                    "Table",
	"ui.tab.clubs":                    "Clubs",
	"ui.tab.fixtures":                 "Fixtures/Results",
	"ui.tab.admin_settings":           "Settings",
	"ui.media.empty":                  "No press items yet — waiting for the first story.",
	"ui.error.prefix":                 "Problem:",
	"ui.disconnected.title":           "Cannot reach the world server",
	"ui.disconnected.server":          "Server: {url}",
	"ui.disconnected.hint_daemon":     "Is the agenticfc daemon running? This console reconnects once it is up.",
	"ui.disconnected.hint_server":     "If it listens at another address, relaunch with -server <url>.",
	"ui.disconnected.retrying":        "Retrying:",
	"ui.help.media":                   "↑/↓ story · PgUp/PgDn article",
	"ui.help.table":                   "←/→ division",
	"ui.help.clubs":                   "↑/↓ club · Tab player",
	"ui.help.fixtures":                "↑/↓ fixture · Enter/Space open · ←/→ division",
	"ui.help.settings":                "↑/↓ setting · +/- or [/] adjust",
	"ui.help.quit":                    "q quit",
	"ui.match.current_scene":          "Current scene",
	"ui.match.history":                "Earlier flow",
	"ui.match.goalflash":              "GOAL",
	"ui.match.latest":                 "Latest scores",
	"ui.match.legend":                 "G goal · o chance · x card · + injury · s sub · ! shootout",
	"ui.match.timeline":               "Timeline",
	"ui.match.momentum":               "Momentum",
	"ui.match.phase.first":            "1H",
	"ui.match.phase.second":           "2H",
	"ui.match.scene.goal":             "Goal scene",
	"ui.match.scene.chance":           "Chance building",
	"ui.match.scene.save":             "Keeper's save",
	"ui.match.scene.cross":            "Wide delivery",
	"ui.match.scene.cutback":          "Cut-back",
	"ui.match.scene.through":          "Through ball",
	"ui.match.scene.longshot":         "From range",
	"ui.match.scene.setpiece":         "Set piece",
	"ui.match.scene.counter":          "Counter attack",
	"ui.match.scene.scramble":         "Six-yard scramble",
	"ui.match.scene.dribble":          "Dribble",
	"ui.match.scene.card":             "Referee's book",
	"ui.match.scene.injury":           "Stoppage",
	"ui.match.scene.sub":              "Technical area",
	"ui.match.scene.build":            "Build-up",
	"ui.match.scene.kickoff":          "Kick-off",
	"ui.match.scene.interval":         "Interval",
	"ui.match.scene.fulltime":         "Full time",
	"ui.match.scene.shootout":         "Penalty shootout",
	"ui.match.modal.animation_pause":  "Space pause",
	"ui.match.modal.animation_resume": "Space animate",
	"ui.match.report":                 "The story of the match",
	"ui.match.modal.lineups":          "L lineups",
	"ui.match.modal.lineups_back":     "L broadcast",
	"ui.match.lineups.bench":          "Bench",
	"ui.match.lineups.empty":          "Lineups are not available for this match.",
}

var (
	styleHeader = lipgloss.NewStyle().Bold(true)
	styleActive = lipgloss.NewStyle().Bold(true).Underline(true)
	styleDim    = lipgloss.NewStyle().Faint(true)
)

func (m Model) View() string {
	tier := layout.Compute(m.Width, m.Height)
	if m.Width > 0 && tier == layout.TierXS {
		return m.viewTooSmall()
	}

	width, height := m.Width, m.Height
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 30
	}
	bodyWidth := width - 2
	noticeInline := m.Notice != "" && width < 100
	errRows := 0
	if m.Err != "" || noticeInline {
		errRows = 1
	}
	bodyHeight := height - 6 - errRows
	if bodyHeight < 0 {
		bodyHeight = 0
	}

	var body string
	switch {
	case m.disconnected():
		body = m.viewDisconnected(bodyWidth)
	case m.Tab == tabMedia:
		body = m.viewMedia(bodyWidth, bodyHeight)
	case m.Tab == tabTable:
		body = m.viewTable(bodyWidth, bodyHeight)
	case m.Tab == tabClubs:
		body = m.viewClubs(bodyWidth, bodyHeight)
	case m.Tab == tabFixtures:
		body = m.viewFixtures(bodyWidth, bodyHeight)
	case m.Tab == tabAdminSettings:
		body = m.viewAdminSettings(bodyWidth, bodyHeight)
	default:
		body = m.viewMedia(bodyWidth, bodyHeight)
	}

	headerText := fmt.Sprintf("%s · %s · [%s]",
		m.World.Name, m.World.ClockText, m.World.TempoLabel)
	if m.disconnected() {
		// No world facts yet — an empty "· · []" header reads broken.
		headerText = ""
	}
	header := styleHeader.Render(truncate(headerText, bodyWidth))
	footer := styleDim.Render(m.footerKeyBar(bodyWidth))
	status := ""
	if m.Err != "" {
		status = m.ui("ui.error.prefix") + " " + m.Err
		if noticeInline {
			status += " · " + m.Notice
		}
	} else if noticeInline {
		status = m.Notice
	}
	statusLine := ""
	if status != "" {
		statusLine = styleDim.Render(truncate(status, bodyWidth))
	}
	var overlays []Overlay
	if !noticeInline {
		overlays = m.noticeOverlays(width, height)
	}
	if ov, ok := m.matchModalOverlay(width, height); ok {
		overlays = append(overlays, ov)
	}
	return appFrame{
		Width:    width,
		Height:   height,
		Title:    m.ui("ui.app.title"),
		Header:   header,
		Tabs:     m.tabBar(bodyWidth),
		Body:     body,
		Error:    statusLine,
		Footer:   footer,
		Overlays: overlays,
	}.Render()
}

// footerKeyBar keeps the active screen's controls readable and reserves the
// right edge for the global quit key. The tab row already advertises screen
// navigation, so repeating every tab here would crowd out the useful controls
// on compact and double-width locales.
func (m Model) footerKeyBar(width int) string {
	left := ""
	switch m.MatchModal {
	case modalReplay:
		left = m.ui("ui.match.modal.replay_help")
	case modalLive, modalWaiting:
		left = m.ui("ui.match.modal.close")
	default:
		switch m.Tab {
		case tabMedia:
			left = m.ui("ui.help.media")
		case tabTable:
			left = m.ui("ui.help.table")
		case tabClubs:
			left = m.ui("ui.help.clubs")
		case tabFixtures:
			left = m.ui("ui.help.fixtures")
		case tabAdminSettings:
			left = m.ui("ui.help.settings")
		}
	}
	return splitKeyBar(left, m.ui("ui.help.quit"), width)
}

func splitKeyBar(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	right = truncate(right, width)
	rightWidth := lipgloss.Width(right)
	if left == "" || rightWidth >= width {
		return fitLine(right, width, alignRight)
	}
	const separator = " · "
	leftWidth := width - lipgloss.Width(separator) - rightWidth
	if leftWidth <= 0 {
		return fitLine(right, width, alignRight)
	}
	return fitLine(left, leftWidth, alignLeft) + separator + right
}

func (m Model) matchModalOverlay(width, height int) (Overlay, bool) {
	if m.MatchModal == modalNone {
		return Overlay{}, false
	}
	boxWidth, boxHeight := matchModalSize(width, height)
	if boxWidth < 20 || boxHeight < minMatchModalHeight {
		return Overlay{}, false
	}
	text := m.renderMatchModalText(boxWidth, boxHeight)
	x := (width - boxWidth) / 2
	y := (height - boxHeight) / 2
	if x < 1 {
		x = 1
	}
	if y < 4 {
		y = 4
	}
	if maxHeight := height - y - 1; boxHeight > maxHeight {
		boxHeight = maxHeight
		if boxHeight < minMatchModalHeight {
			return Overlay{}, false
		}
		text = m.renderMatchModalText(boxWidth, boxHeight)
	}
	return textOverlay(x, y, 90, text), true
}

func (m Model) renderMatchModalText(width, height int) string {
	switch m.MatchModal {
	case "live":
		return m.liveMatchModal(width, height)
	case "waiting":
		return m.waitingMatchModal(width, height)
	default:
		return m.replayMatchModal(width, height)
	}
}

func matchModalSize(width, height int) (int, int) {
	xMargin := 1
	yMargin := 3
	if height < 24 {
		yMargin = 2
	}
	boxWidth := width - xMargin*2
	boxHeight := height - yMargin*2
	if boxWidth < 20 || boxHeight < minMatchModalHeight {
		return 0, 0
	}
	return boxWidth, boxHeight
}

func matchModalCompact(boxWidth, boxHeight int) bool {
	return boxWidth <= 80 || boxHeight <= 18
}

func (m Model) waitingMatchModal(width, height int) string {
	title := m.ui("ui.match.live")
	lines := []string{m.ui("ui.match.waiting_result")}
	if idx := m.fixtureIndexByID(m.MatchModalID); idx >= 0 {
		f := m.Fixtures[idx]
		title = strings.TrimSpace(f.Home + " - " + f.Away)
		lines = []string{
			fmt.Sprintf("%s · %s", f.KickoffText, f.Competition),
			m.ui("ui.match.waiting_result"),
		}
	}
	return modalBox(width, height, title, lines)
}

func (m Model) liveMatchModal(width, height int) string {
	if len(m.Matches) == 0 {
		return modalBox(width, height, m.ui("ui.match.live"), []string{m.ui("ui.match.none")})
	}
	idx := m.MatchIdx
	if idx < 0 || idx >= len(m.Matches) {
		idx = 0
	}
	mv := m.Matches[idx]
	title := fmt.Sprintf("%s %d-%d %s", mv.Home, mv.HomeGoals, mv.AwayGoals, mv.Away)
	compact := matchModalCompact(width, height)
	contentRows := height - 2
	current := ""
	if len(mv.Commentary) > 0 {
		current = mv.Commentary[len(mv.Commentary)-1]
	}
	sc := matchSceneFromLive(mv, current)
	lines := []string{
		fmt.Sprintf("%d' %s · %s · %d/%d · %s · %s · %s", mv.Minute, m.matchPhaseLabel(mv.Minute), mv.Competition, idx+1, len(m.Matches), m.ui("ui.match.modal.close"), m.matchAnimationHelp(), m.ui("ui.match.modal.lineups")),
	}
	flash := m.goalFlashLine(mv, width-2)
	if flash == "" && goalProse(current) {
		// The timed flash has expired (or the marker is missing) but the
		// visible beat is still the scored goal — keep the goal signal up
		// while an action scene plays.
		flash = m.plainGoalBanner(width - 2)
	}
	if flash != "" {
		// Already exactly content-wide; wrapText would collapse its double
		// spaces and leave a ragged right edge on the banner.
		lines = append(lines, preformattedLinePrefix+flash)
	}
	if m.LineupView {
		lines[0] = fmt.Sprintf("%d' %s · %s · %d/%d · %s · %s", mv.Minute, m.matchPhaseLabel(mv.Minute), mv.Competition, idx+1, len(m.Matches), m.ui("ui.match.modal.close"), m.ui("ui.match.modal.lineups_back"))
		lines = append(lines, "")
		for _, row := range m.lineupLines(mv.Home, mv.Away, mv.HomeLineup, mv.AwayLineup, width-4) {
			lines = append(lines, preformattedLinePrefix+row)
		}
		return modalBox(width, height, title, lines)
	}
	lines = append(lines,
		fmt.Sprintf("%s H %d · A %d", m.ui("ui.match.stat.shots"), mv.Stats.HomeShots, mv.Stats.AwayShots),
		fmt.Sprintf("%s H %d · A %d   %s H %d · A %d",
			m.ui("ui.match.stat.cards"), mv.Stats.HomeCards, mv.Stats.AwayCards,
			m.ui("ui.match.stat.subs"), mv.Stats.HomeSubs, mv.Stats.AwaySubs),
	)
	patternLimit := 2
	if compact {
		patternLimit = 1
	}
	if mix := m.chanceMixLabel(mv.Stats.ChanceTypes, mv.Stats.ChanceTypesBySide, patternLimit); mix != "" {
		lines = append(lines, fmt.Sprintf("%s %s", m.ui("ui.match.stat.chance_mix"), mix))
	}
	if !compact {
		lines = append(lines, m.diagnosticLines(mv.Stats.Diagnostics, width-4, 3)...)
	}
	if !compact && contentRows >= 24 {
		if rows := m.broadcastRows(mv, width-4); len(rows) > 0 {
			lines = append(lines, rows...)
			// Extra-tall layouts (docs/07 §2: terminal >= 42 rows, i.e.
			// content >= 34 after modal margins) can afford to explain the
			// timeline glyphs right where they appear.
			if contentRows >= 34 {
				lines = append(lines, styleDim.Render(truncate(m.ui("ui.match.legend"), width-4)))
			}
		}
	}
	if !compact && contentRows-len(lines) >= 14 {
		if frame := sceneFrameDirAt(m, sc, width-2, sceneFrameRows, m.matchAnimationFrame, liveAttackingAway(mv)); len(frame) > 0 {
			lines = append(lines, "")
			lines = append(lines, frame...)
		}
	}
	lines = append(lines, "")
	lines = append(lines, m.ui("ui.match.current_scene"))
	if current == "" {
		lines = append(lines, "▶ "+sceneLabel(m, sc))
	} else {
		lines = append(lines, "▶ "+current)
	}
	ratingsRoom := contentRows - len(lines) - 2
	if !compact && len(mv.Ratings) > 0 && ratingsRoom >= 8 {
		if summary := liveRatingsSummary(m, mv, width-4); summary != "" {
			lines = append(lines, summary)
		}
	}
	ticker := ""
	// Gate on height alone: a narrow-but-tall terminal still has the spare
	// closing row the ticker needs (docs/07: tall live layouts).
	if height > 18 {
		ticker = m.elsewhereTicker(mv.Fixture, width-4)
	}
	historyRows := contentRows - len(lines) - 2
	if ticker != "" {
		historyRows -= 2 // keep the closing ticker row (plus its separator)
	}
	if historyRows > 0 {
		history := recentHistory(beatLines(mv.Beats, mv.Commentary), historyRows)
		if len(history) > 0 {
			lines = append(lines, "", m.ui("ui.match.history"))
			lines = append(lines, history...)
		}
	}
	if ticker != "" && len(lines) < contentRows {
		if len(lines)+1 < contentRows {
			lines = append(lines, "")
		}
		lines = append(lines, ticker)
	}
	return modalBox(width, height, title, lines)
}

func (m Model) replayMatchModal(width, height int) string {
	f := Fixture{}
	if m.MatchModalID != 0 {
		if idx := m.fixtureIndexByID(m.MatchModalID); idx >= 0 {
			f = m.Fixtures[idx]
		}
	} else if len(m.Fixtures) > 0 && m.FixtureIdx >= 0 && m.FixtureIdx < len(m.Fixtures) {
		f = m.Fixtures[m.FixtureIdx]
	}
	targetID := f.ID
	if targetID == 0 {
		targetID = m.MatchModalID
	}
	if m.MatchDetail.Fixture == 0 || (targetID != 0 && m.MatchDetail.Fixture != targetID) {
		title := strings.TrimSpace(f.Home + " - " + f.Away)
		if title == "-" || title == "" {
			title = m.ui("ui.fixture.replay")
		}
		return modalBox(width, height, title, []string{m.ui("ui.match.loading")})
	}
	md := m.MatchDetail
	title := fmt.Sprintf("%s %d-%d %s", md.Home, md.HomeGoals, md.AwayGoals, md.Away)
	compact := matchModalCompact(width, height)
	if m.LineupView {
		lines := []string{
			fmt.Sprintf("%s · %s · %s · %s", md.KickoffText, md.Competition, m.ui("ui.match.modal.replay_help"), m.ui("ui.match.modal.lineups_back")),
			"",
		}
		for _, row := range m.lineupLines(md.Home, md.Away, md.HomeLineup, md.AwayLineup, width-4) {
			lines = append(lines, preformattedLinePrefix+row)
		}
		return modalBox(width, height, title, lines)
	}
	lines := []string{
		fmt.Sprintf("%s · %s · %s · %s", md.KickoffText, md.Competition, m.ui("ui.match.modal.replay_help"), m.ui("ui.match.modal.lineups")),
		fmt.Sprintf("%s H %d · A %d", m.ui("ui.match.stat.shots"), md.HomeShots, md.AwayShots),
	}
	patternLimit := 2
	if compact {
		patternLimit = 1
	}
	if mix := m.chanceMixLabel(md.ChanceTypes, md.ChanceTypesBySide, patternLimit); mix != "" {
		lines = append(lines, fmt.Sprintf("%s %s", m.ui("ui.match.stat.chance_mix"), mix))
	}
	if !compact {
		lines = append(lines, m.diagnosticLines(md.Diagnostics, width-4, 3)...)
	}
	if md.Winner != "" {
		lines = append(lines, fmt.Sprintf("%s %s", m.ui("ui.match.winner"), md.Winner))
	}
	if !compact && len(md.Story) > 0 {
		lines = append(lines, "", m.ui("ui.match.report"))
		lines = append(lines, md.Story...)
	}
	if len(md.Scorers) > 0 {
		lines = append(lines, "", m.ui("ui.match.scorers"))
		const compactScorerLimit = 3
		shown := 0
		for _, e := range md.Scorers {
			if compact && shown >= compactScorerLimit {
				break
			}
			lines = append(lines, fmt.Sprintf("%d' %s · %s", e.Minute, e.Club, e.Player))
			shown++
		}
	}
	appendEvents := func(title string, events []MatchEvent) {
		if compact || len(events) == 0 {
			return
		}
		lines = append(lines, "", title)
		for _, e := range events {
			detail := e.Player
			if e.Detail != "" {
				detail += " " + e.Detail
			}
			lines = append(lines, fmt.Sprintf("%d' %s · %s", e.Minute, e.Club, detail))
		}
	}
	appendEvents(m.ui("ui.match.cards"), md.Cards)
	if !compact && len(md.Subs) > 0 {
		lines = append(lines, "", m.ui("ui.match.subs"))
		for _, s := range md.Subs {
			on := s.On
			if on == "" {
				on = "-"
			}
			lines = append(lines, fmt.Sprintf("%d' %s · %s -> %s %s", s.Minute, s.Club, s.Off, on, s.Reason))
		}
	}
	if !compact && len(md.Ratings) > 0 {
		lines = append(lines, "", m.ui("ui.match.ratings"))
		for i, r := range md.Ratings {
			if i >= 5 {
				break
			}
			club := ""
			if r.Side == matchSideHome {
				club = md.Home
			} else if r.Side == matchSideAway {
				club = md.Away
			}
			if club == "" {
				lines = append(lines, fmt.Sprintf("%d.%d %s", r.RatingX10/10, r.RatingX10%10, r.Name))
				continue
			}
			lines = append(lines, fmt.Sprintf("%d.%d %s · %s", r.RatingX10/10, r.RatingX10%10, club, r.Name))
		}
	}
	start := m.ReplayOffset
	if start < 0 || start >= len(md.Commentary) {
		start = 0
	}
	current := ""
	if start < len(md.Commentary) {
		current = md.Commentary[start]
	}
	sc := matchSceneFromLine(current, nil)
	if goalProse(current) {
		// Replays never show the timed live flash, so browsing onto a goal
		// beat raises a static banner above its action scene instead.
		lines = append(lines, preformattedLinePrefix+m.plainGoalBanner(width-2))
	}
	if !compact && height-2-len(lines) >= 14 {
		if frame := sceneFrameDirAt(m, sc, width-2, sceneFrameRows, 0, replayGoalAway(md, start)); len(frame) > 0 {
			lines = append(lines, "")
			lines = append(lines, frame...)
		}
	}
	display := beatLines(md.Beats, md.Commentary)
	lines = append(lines, "", m.ui("ui.match.current_scene"))
	if current == "" {
		lines = append(lines, "▶ "+sceneLabel(m, sc))
	} else {
		lines = append(lines, "▶ "+display[start])
	}
	lines = append(lines, "", m.ui("ui.match.replay"))
	commentRows := height - len(lines) - 2
	if commentRows < 3 {
		commentRows = 3
	}
	for i := start + 1; i < len(display) && len(lines) < height-2; i++ {
		lines = append(lines, "· "+display[i])
		if i-start >= commentRows {
			break
		}
	}
	if len(md.Commentary) == 0 && md.Archived {
		lines = append(lines, m.ui("ui.match.replay.archived"))
	}
	return modalBox(width, height, title, lines)
}

// lineupLines renders the pop-up's team-sheet panel from the already-public
// lineup rows the daemon serves. Wide boxes show home and away side by side;
// narrow ones stack home above away. Overflow is clipped by modalBox — the
// panel is a summary, not a scroller.
func (m Model) lineupLines(homeName, awayName string, home, away []LineupEntry, width int) []string {
	if len(home) == 0 && len(away) == 0 {
		return []string{m.ui("ui.match.lineups.empty")}
	}
	if width >= 64 {
		colW := (width - 3) / 2
		left := m.lineupColumn(homeName, home, colW)
		right := m.lineupColumn(awayName, away, colW)
		rows := maxInt(len(left), len(right))
		lines := make([]string, 0, rows)
		for i := 0; i < rows; i++ {
			l, r := "", ""
			if i < len(left) {
				l = left[i]
			}
			if i < len(right) {
				r = right[i]
			}
			lines = append(lines, fitLine(l, colW, alignLeft)+" │ "+r)
		}
		return lines
	}
	lines := append(m.lineupColumn(homeName, home, width), "")
	return append(lines, m.lineupColumn(awayName, away, width)...)
}

// lineupPositionOrder ranks positions the way a team sheet reads: keeper,
// back line, midfield, then the front — inside a band, right before left.
var lineupPositionOrder = map[string]int{
	"GK": 0, "DR": 1, "DC": 2, "DL": 3, "DM": 4,
	"MC": 5, "MR": 6, "ML": 7, "AM": 8, "WR": 9, "WL": 10, "ST": 11,
}

// sortLineup orders the starter prefix by position band (stable, so the
// served order breaks ties); entrants and bench rows keep their story order
// after the starters. Current daemons already serve team-sheet order, so
// this is an idempotent guard for older daemons that served raw XI order.
func sortLineup(entries []LineupEntry) []LineupEntry {
	starters := 0
	for _, e := range entries {
		if e.OnMinute > 0 || e.Bench {
			break
		}
		starters++
	}
	out := make([]LineupEntry, len(entries))
	copy(out, entries)
	sort.SliceStable(out[:starters], func(i, j int) bool {
		oi, iOK := lineupPositionOrder[out[i].Position]
		oj, jOK := lineupPositionOrder[out[j].Position]
		if iOK != jOK {
			return iOK // unknown positions sink to the end of the sheet
		}
		return oi < oj
	})
	return out
}

// lineupColumn renders one side: bold club name, then starters and entrants,
// with any unused live bench under a dim bench label.
func (m Model) lineupColumn(club string, entries []LineupEntry, width int) []string {
	rows := []string{styleHeader.Render(truncate(club, width))}
	benchShown := false
	for _, e := range sortLineup(entries) {
		if e.Bench && !benchShown {
			rows = append(rows, styleDim.Render(truncate(m.ui("ui.match.lineups.bench"), width)))
			benchShown = true
		}
		row := truncate(lineupRow(e), width)
		if e.Bench {
			row = styleDim.Render(row)
		}
		rows = append(rows, row)
	}
	return rows
}

// lineupRow formats one team-sheet row: position, name, then the public
// story markers — ▲on' ▼off' goals G/G2, cards Y/Y2/R — and the rating.
func lineupRow(e LineupEntry) string {
	pos := e.Position
	if pos == "" {
		pos = "--"
	}
	s := fmt.Sprintf("%-2s  %s", pos, e.Name)
	if e.OnMinute > 0 {
		s += fmt.Sprintf(" ▲%d'", e.OnMinute)
	}
	if e.OffMinute > 0 {
		s += fmt.Sprintf(" ▼%d'", e.OffMinute)
	}
	switch {
	case e.Goals == 1:
		s += " G"
	case e.Goals > 1:
		s += fmt.Sprintf(" G%d", e.Goals)
	}
	switch {
	case e.Yellows == 1:
		s += " Y"
	case e.Yellows > 1:
		s += fmt.Sprintf(" Y%d", e.Yellows)
	}
	if e.Red {
		s += " R"
	}
	if e.RatingX10 > 0 {
		s += fmt.Sprintf(" · %d.%d", e.RatingX10/10, e.RatingX10%10)
	}
	return s
}

// beatLines returns minute-stamped display strings for commentary, falling
// back to the plain lines when the daemon predates beats.
func beatLines(beats []CommentaryBeat, fallback []string) []string {
	if len(beats) != len(fallback) || len(beats) == 0 {
		return fallback
	}
	out := make([]string, len(beats))
	for i, b := range beats {
		if b.Minute < 1 {
			// The opening whistle is recorded before the clock moves; "0'"
			// is not a football minute, so kickoff prose stays unstamped.
			out[i] = b.Text
			continue
		}
		out[i] = fmt.Sprintf("%d' %s", b.Minute, b.Text)
	}
	return out
}

func recentHistory(commentary []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	if len(commentary) == 0 {
		return nil
	}
	if len(commentary) == 1 {
		return nil
	}
	history := commentary[:len(commentary)-1]
	if len(history) > limit {
		history = history[len(history)-limit:]
	}
	out := make([]string, 0, len(history))
	for _, line := range history {
		out = append(out, "· "+line)
	}
	return out
}

func liveRatingsSummary(m Model, mv LiveMatchView, width int) string {
	ratings := balancedRatings(mv.Ratings, 1)
	if len(ratings) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ratings))
	for _, r := range ratings {
		club := mv.Home
		if r.Side == matchSideAway {
			club = mv.Away
		}
		parts = append(parts, fmt.Sprintf("%s %d.%d %s", club, r.RatingX10/10, r.RatingX10%10, r.Name))
	}
	return truncate(m.ui("ui.match.ratings")+": "+strings.Join(parts, " · "), width)
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func containsWordAny(s string, words ...string) bool {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z')
	})
	for _, word := range words {
		for _, field := range fields {
			if field == word {
				return true
			}
		}
	}
	return false
}

func (m Model) goalFlashLine(mv LiveMatchView, width int) string {
	if len(mv.Markers) == 0 {
		return ""
	}
	latest := mv.Markers[len(mv.Markers)-1]
	age := mv.Minute - latest.Minute
	if latest.Kind != "GOAL" || age < 0 || age > goalFlashWindowMinutes {
		return ""
	}
	side := mv.Home
	if latest.Side == matchSideAway {
		side = mv.Away
	}
	msg := fmt.Sprintf("  %s  %d'  %s  ", strings.ToUpper(m.ui("ui.match.goalflash")), latest.Minute, side)
	if lipgloss.Width(msg) > width {
		msg = truncate(msg, width)
	}
	pad := width - lipgloss.Width(msg)
	left := pad / 2
	right := pad - left
	return strings.Repeat("█", left) + msg + strings.Repeat("█", right)
}

// plainGoalBanner is the untimed variant of goalFlashLine for moments where
// the marker window is unavailable — replay browsing and live beats whose
// flash already expired — but the visible line still announces a goal.
func (m Model) plainGoalBanner(width int) string {
	if width <= 0 {
		return ""
	}
	msg := fmt.Sprintf("  %s  ", strings.ToUpper(m.ui("ui.match.goalflash")))
	if lipgloss.Width(msg) > width {
		msg = truncate(msg, width)
	}
	pad := width - lipgloss.Width(msg)
	left := pad / 2
	return strings.Repeat("█", left) + msg + strings.Repeat("█", pad-left)
}

func modalBox(width, height int, title string, lines []string) string {
	content := width - 2
	out := []string{"╔" + fitLine(title, content, alignCenter) + "╗"}
	for _, line := range lines {
		if raw, ok := strings.CutPrefix(line, preformattedLinePrefix); ok {
			out = append(out, "║"+fitLine(raw, content, alignLeft)+"║")
			if len(out) >= height-1 {
				break
			}
			continue
		}
		for _, wrapped := range wrapText(line, content) {
			out = append(out, "║"+fitLine(wrapped, content, alignLeft)+"║")
			if len(out) >= height-1 {
				break
			}
		}
		if len(out) >= height-1 {
			break
		}
	}
	for len(out) < height-1 {
		out = append(out, "║"+strings.Repeat(" ", content)+"║")
	}
	out = append(out, "╚"+strings.Repeat("═", content)+"╝")
	return strings.Join(out, "\n")
}

func (m Model) noticeOverlays(width, height int) []Overlay {
	if m.Notice == "" || width < layout.MinCols || height < layout.MinRows {
		return nil
	}
	boxWidth := 34
	lines := mascotBubble(m.Notice, boxWidth)
	x := width - boxWidth - 4
	if x < 2 {
		x = 2
	}
	y := height - len(lines) - 3
	if y < 4 {
		y = 4
	}
	return []Overlay{{X: x, Y: y, Z: 80, Lines: lines}}
}

func mascotBubble(text string, width int) []string {
	if width < 30 {
		width = 30
	}
	inner := width - 2
	art := []string{"o-o", "/|\\", "/ \\"}
	artWidth := lipgloss.Width(art[0])
	textWidth := inner - artWidth - 1
	wrapped := wrapText(text, textWidth)
	if len(wrapped) == 0 {
		wrapped = []string{""}
	}
	lines := []string{"+" + strings.Repeat("-", inner) + "+"}
	for i, line := range wrapped {
		mark := strings.Repeat(" ", artWidth)
		if i < len(art) {
			mark = fitLine(art[i], artWidth, alignLeft)
		}
		lines = append(lines, "|"+mark+" "+fitLine(line, textWidth, alignLeft)+"|")
	}
	for i := len(wrapped); i < len(art); i++ {
		lines = append(lines, "|"+fitLine(art[i], artWidth, alignLeft)+" "+strings.Repeat(" ", textWidth)+"|")
	}
	lines = append(lines, "+"+strings.Repeat("-", inner)+"+")
	return lines
}

func (m Model) viewTooSmall() string {
	msg := m.ui("ui.terminal.too_small")
	msg = strings.NewReplacer(
		"{min_cols}", fmt.Sprint(layout.MinCols),
		"{min_rows}", fmt.Sprint(layout.MinRows),
		"{cols}", fmt.Sprint(m.Width),
		"{rows}", fmt.Sprint(m.Height),
	).Replace(msg)
	return msg
}

func (m Model) tabBar(_ int) string {
	division := strings.ReplaceAll(m.ui("ui.header.division"), "{tier}", fmt.Sprint(m.Tier))
	names := m.tabNames()
	parts := make([]string, len(names))
	for i, n := range names {
		label := fmt.Sprintf("%d %s", i+1, n)
		if i == m.Tab {
			parts[i] = styleActive.Render(label)
		} else {
			parts[i] = styleDim.Render(label)
		}
	}
	return division + "   " + strings.Join(parts, "  ")
}

func (m Model) tabNames() []string {
	names := []string{m.ui("ui.tab.media"), m.ui("ui.tab.table"), m.ui("ui.tab.clubs"), m.ui("ui.tab.fixtures")}
	if m.AdminMode {
		names = append(names, m.ui("ui.tab.admin_settings"))
	}
	return names
}

func (m Model) viewAdminSettings(width, height int) string {
	if !m.AdminMode {
		return styleDim.Render(truncate(m.ui("ui.admin.token_required"), width))
	}
	settings := m.Settings.Runtime
	if settings.GameSpeed == 0 {
		return styleDim.Render(truncate(m.ui("ui.admin.settings.loading"), width))
	}
	rows := m.adminSettingsRows(settings)
	cols := m.adminSettingsColumns()
	var b strings.Builder
	b.WriteString(m.adminSettingsPreamble(width))
	b.WriteString(renderTextTable(width, cols, rows))
	if m.Settings.Schema.Determinism != "" {
		b.WriteString("\n\n")
		b.WriteString(styleDim.Render(truncate(m.ui("ui.admin.settings.determinism"), width)) + "\n")
		for _, line := range wrapText(m.Settings.Schema.Determinism, width) {
			b.WriteString(truncate(line, width) + "\n")
		}
	}
	if len(m.Settings.Schema.RequiresWorldRebuild) > 0 {
		b.WriteString("\n")
		b.WriteString(styleDim.Render(truncate(m.ui("ui.admin.settings.rebuild_required"), width)) + "\n")
		b.WriteString(truncate(strings.Join(m.Settings.Schema.RequiresWorldRebuild, ", "), width) + "\n")
	}
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func (m Model) adminSettingsPreamble(width int) string {
	return styleHeader.Render(truncate(m.ui("ui.admin.settings.title"), width)) + "\n" +
		styleDim.Render(truncate(m.ui("ui.admin.settings.help"), width)) + "\n\n"
}

func (m Model) adminSettingsColumns() []tableColumn {
	return []tableColumn{
		{Header: "", Width: 2, Align: alignLeft},
		{Header: m.ui("ui.admin.settings.setting"), Width: 30, Align: alignLeft},
		{Header: m.ui("ui.admin.settings.value"), Width: 12, Align: alignRight},
		{Header: m.ui("ui.admin.settings.allowed"), Width: 20, Align: alignLeft},
	}
}

func (m Model) adminSettingsRows(settings RuntimeSettings) [][]string {
	return [][]string{
		{selector(m.SettingsIdx == 0), m.ui("ui.admin.settings.game_speed"), fmt.Sprintf("%dx", settings.GameSpeed), strings.Join(intsToStrings(speedOptions(m.Settings.Schema)), "/")},
		{selector(m.SettingsIdx == 1), m.ui("ui.admin.settings.idle_acceleration"), fmt.Sprintf("%dx", settings.IdleAcceleration), fmt.Sprintf("%d..%d", idleMin(m.Settings.Schema), idleMax(m.Settings.Schema))},
		{selector(m.SettingsIdx == 2), m.ui("ui.admin.settings.offseason_acceleration"), fmt.Sprintf("%dx", settings.OffseasonAcceleration), fmt.Sprintf("%d..%d", offseasonMin(m.Settings.Schema), offseasonMax(m.Settings.Schema))},
	}
}

func (m Model) adminSettingsRowAt(width, bodyRelativeY int) int {
	table := renderTextTable(width, m.adminSettingsColumns(), nil)
	tableDataOffset := len(strings.Split(strings.TrimRight(table, "\n"), "\n")) - 1
	dataStart := strings.Count(m.adminSettingsPreamble(width), "\n") + tableDataOffset
	return bodyRelativeY - dataStart
}

func selector(active bool) string {
	if active {
		return ">"
	}
	return ""
}

func (m Model) adjustRuntimeSetting(delta int) (tea.Model, tea.Cmd) {
	settings := m.Settings.Runtime
	if settings.GameSpeed == 0 {
		return m, nil
	}
	switch m.SettingsIdx {
	case 0:
		settings.GameSpeed = nextSpeed(settings.GameSpeed, delta, speedOptions(m.Settings.Schema))
	case 1:
		settings.IdleAcceleration = clampInt(settings.IdleAcceleration+delta, idleMin(m.Settings.Schema), idleMax(m.Settings.Schema))
	case 2:
		settings.OffseasonAcceleration = clampInt(settings.OffseasonAcceleration+delta, offseasonMin(m.Settings.Schema), offseasonMax(m.Settings.Schema))
	}
	m.Settings.Runtime = settings
	m.SettingsDirty = true
	m.SettingsRev++
	rev := m.SettingsRev
	return m, tea.Tick(settingsUpdateDebounce, func(time.Time) tea.Msg {
		return SettingsCommitMsg{Rev: rev}
	})
}

func nextSpeed(current, delta int, options []int) int {
	if len(options) == 0 {
		options = []int{5, 15, 30, 60}
	}
	idx := 1
	for i, v := range options {
		if v == current {
			idx = i
			break
		}
	}
	idx = (idx + delta) % len(options)
	if idx < 0 {
		idx += len(options)
	}
	return options[idx]
}

func speedOptions(schema SettingsSchema) []int {
	if len(schema.GameSpeedOptions) > 0 {
		return schema.GameSpeedOptions
	}
	return []int{5, 15, 30, 60}
}

func sameRuntimeSettings(a, b RuntimeSettings) bool {
	return a.GameSpeed == b.GameSpeed &&
		a.IdleAcceleration == b.IdleAcceleration &&
		a.OffseasonAcceleration == b.OffseasonAcceleration
}

func intsToStrings(values []int) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = fmt.Sprint(v)
	}
	return out
}

func idleMin(schema SettingsSchema) int {
	if schema.IdleAccelerationMin == 0 {
		return 2
	}
	return schema.IdleAccelerationMin
}

func idleMax(schema SettingsSchema) int {
	if schema.IdleAccelerationMax == 0 {
		return 64
	}
	return schema.IdleAccelerationMax
}

func offseasonMin(schema SettingsSchema) int {
	if schema.OffseasonAccelMin == 0 {
		return 2
	}
	return schema.OffseasonAccelMin
}

func offseasonMax(schema SettingsSchema) int {
	if schema.OffseasonAccelMax == 0 {
		return 240
	}
	return schema.OffseasonAccelMax
}

func (m Model) viewMedia(width, height int) string {
	if len(m.News) == 0 {
		return styleDim.Render(m.ui("ui.media.empty"))
	}
	if m.NewsIdx >= len(m.News) {
		m.NewsIdx = 0
	}
	if width >= 110 {
		listWidth := width / 3
		if listWidth < 34 {
			listWidth = 34
		}
		if listWidth > 52 {
			listWidth = 52
		}
		detailWidth := width - listWidth - 1
		list := lipgloss.NewStyle().Width(listWidth).Render(m.mediaList(listWidth, height))
		detail := lipgloss.NewStyle().Width(detailWidth).Render(m.mediaDetail(detailWidth, height, m.News[m.NewsIdx]))
		return lipgloss.JoinHorizontal(lipgloss.Top, list, " ", detail)
	}
	detailRows := height / 2
	if detailRows < 8 {
		detailRows = 8
	}
	if detailRows > height-5 {
		detailRows = height - 5
	}
	if detailRows < 1 {
		detailRows = height
	}
	return m.mediaDetail(width, detailRows, m.News[m.NewsIdx]) + "\n" +
		styleDim.Render(truncate(m.ui("ui.media.recent"), width)) + "\n" +
		m.mediaList(width, height-detailRows-2)
}

func (m Model) mediaList(width, height int) string {
	cols := []tableColumn{
		{Header: "", Width: 2, Align: alignLeft},
		{Header: m.ui("ui.col.source"), Width: colWidth(m.ui("ui.col.source"), 12), Align: alignLeft},
		{Header: m.ui("ui.col.article"), MinWidth: 16, Flex: true, Align: alignLeft},
	}
	maxRows := height - 4
	if maxRows < 0 {
		maxRows = 0
	}
	rows := make([][]string, 0, len(m.News))
	start := visibleStart(m.NewsIdx, len(m.News), maxRows)
	for i := start; i < len(m.News) && len(rows) < maxRows; i++ {
		n := m.News[i]
		mark := " "
		if i == m.NewsIdx {
			mark = ">"
		}
		rows = append(rows, []string{mark, n.Source, n.Title})
	}
	return renderTextTable(width, cols, rows)
}

func (m Model) mediaDetail(width, height int, n NewsArticle) string {
	lines := m.articleMasthead(width, n)
	lines = append(lines,
		styleHeader.Render(truncate(n.Title, width)),
	)
	lines = append(lines, wrapText(n.Deck, width)...)
	lines = append(lines, articleRule(width))
	body := wrapParagraphs(n.Body, width)
	offset := m.ArticleOffset
	if offset < 0 || offset >= len(body) {
		offset = 0
	}
	lines = append(lines, body[offset:]...)
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func (m Model) articleMasthead(width int, n NewsArticle) []string {
	category := n.CategoryLabel
	if category == "" {
		category = n.Category
	}
	if width < 24 {
		return []string{styleDim.Render(truncate(n.Source+" · "+n.TimeText+" · "+category, width))}
	}
	inner := width
	if inner > 86 {
		inner = 86
	}
	top := fitLine("╔"+strings.Repeat("═", inner-2)+"╗", width, alignCenter)
	title := strings.ToUpper(m.ui("ui.app.title") + " " + m.ui("ui.tab.media"))
	source := n.Source + " · " + n.TimeText
	section := "[" + strings.ToUpper(category) + "]"
	return []string{
		top,
		fitLine("║"+fitLine(title, inner-2, alignCenter)+"║", width, alignCenter),
		fitLine("║"+fitLine(source, inner-2, alignCenter)+"║", width, alignCenter),
		fitLine("║"+fitLine(section, inner-2, alignCenter)+"║", width, alignCenter),
		fitLine("╚"+strings.Repeat("═", inner-2)+"╝", width, alignCenter),
	}
}

func articleRule(width int) string {
	if width < 8 {
		return ""
	}
	return strings.Repeat("─", width)
}

func (m Model) viewTable(width, height int) string {
	var b strings.Builder
	b.WriteString(styleDim.Render(truncate(m.Table.Label, width)) + "\n")
	cols := []tableColumn{
		{Header: m.ui("ui.col.pos"), Width: colWidth(m.ui("ui.col.pos"), 4), Align: alignRight},
		{Header: m.ui("ui.col.club"), MinWidth: 12, Flex: true, Align: alignLeft},
		{Header: m.ui("ui.col.played"), Width: colWidth(m.ui("ui.col.played"), 3), Align: alignRight},
		{Header: m.ui("ui.col.won"), Width: colWidth(m.ui("ui.col.won"), 3), Align: alignRight},
		{Header: m.ui("ui.col.drawn"), Width: colWidth(m.ui("ui.col.drawn"), 3), Align: alignRight},
		{Header: m.ui("ui.col.lost"), Width: colWidth(m.ui("ui.col.lost"), 3), Align: alignRight},
		{Header: m.ui("ui.col.gf"), Width: colWidth(m.ui("ui.col.gf"), 4), Align: alignRight},
		{Header: m.ui("ui.col.ga"), Width: colWidth(m.ui("ui.col.ga"), 4), Align: alignRight},
		{Header: m.ui("ui.col.pts"), Width: colWidth(m.ui("ui.col.pts"), 4), Align: alignRight},
	}
	maxRows := height - 5 // label + table top/header/separator/bottom
	if maxRows < 0 {
		maxRows = 0
	}
	rows := make([][]string, 0, len(m.Table.Rows))
	for i, r := range m.Table.Rows {
		if i >= maxRows {
			break
		}
		rows = append(rows, []string{
			intCell(r.Pos), r.Club, intCell(r.Played), intCell(r.Won), intCell(r.Drawn),
			intCell(r.Lost), intCell(r.GF), intCell(r.GA), intCell(r.Points),
		})
	}
	b.WriteString(renderTextTable(width, cols, rows))
	return b.String()
}

func (m Model) viewClubs(width, height int) string {
	if len(m.Clubs) == 0 {
		return styleDim.Render(m.ui("ui.clubs.empty"))
	}
	if width >= 120 {
		listWidth := width / 3
		if listWidth < 36 {
			listWidth = 36
		}
		if listWidth > 54 {
			listWidth = 54
		}
		detailWidth := width - listWidth - 1
		list := lipgloss.NewStyle().Width(listWidth).Render(m.clubList(listWidth, height))
		detail := lipgloss.NewStyle().Width(detailWidth).Render(m.clubDetail(detailWidth, height))
		return lipgloss.JoinHorizontal(lipgloss.Top, list, " ", detail)
	}
	return m.clubDetail(width, height)
}

func (m Model) clubList(width, height int) string {
	cols := []tableColumn{
		{Header: "", Width: 2, Align: alignLeft},
		{Header: m.ui("ui.col.club"), MinWidth: 14, Flex: true, Align: alignLeft},
		{Header: m.ui("ui.col.div"), Width: colWidth(m.ui("ui.col.div"), 4), Align: alignRight},
		{Header: m.ui("ui.col.security"), Width: colWidth(m.ui("ui.col.security"), 12), Align: alignLeft},
	}
	maxRows := height - 4
	if maxRows < 0 {
		maxRows = 0
	}
	rows := make([][]string, 0, len(m.Clubs))
	start := visibleStart(m.ClubIdx, len(m.Clubs), maxRows)
	for i := start; i < len(m.Clubs) && len(rows) < maxRows; i++ {
		c := m.Clubs[i]
		mark := " "
		if i == m.ClubIdx {
			mark = ">"
		}
		rows = append(rows, []string{mark, c.Name, intCell(c.Tier), c.Security})
	}
	return renderTextTable(width, cols, rows)
}

func (m Model) clubDetail(width, height int) string {
	c := m.Club
	if c.ID == 0 {
		if m.ClubIdx < len(m.Clubs) {
			return styleDim.Render(truncate(m.Clubs[m.ClubIdx].Name, width))
		}
		return styleDim.Render(m.ui("ui.clubs.empty"))
	}
	lines := []string{}
	badge := clubBadge(c.Name)
	if width >= 80 && height >= 24 {
		info := []string{
			styleHeader.Render(truncate(c.Name, width-24)),
			styleDim.Render(truncate(fmt.Sprintf("%s · %s", c.Region, c.Stadium), width-24)),
			truncate(fmt.Sprintf("%s %s", m.ui("ui.club.manager"), c.Manager), width-24),
			truncate(fmt.Sprintf("%s %d  %s %d", m.ui("ui.club.predicted"), c.PredictedFinish, m.ui("ui.club.objective"), c.BoardObjectiveFinish), width-24),
			truncate(fmt.Sprintf("%s %s  %s %s",
				m.ui("ui.club.confidence"), c.Board["confidence"],
				m.ui("ui.club.security"), c.Board["security"]), width-24),
		}
		lines = append(lines, strings.Split(lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(20).Render(badge),
			"  ",
			lipgloss.NewStyle().Width(width-22).Render(strings.Join(info, "\n")),
		), "\n")...)
	} else {
		lines = append(lines, styleHeader.Render(truncate(c.Name, width)))
		if height >= 24 {
			lines = append(lines, strings.Split(badge, "\n")...)
		}
		lines = append(lines, styleDim.Render(truncate(fmt.Sprintf("%s · %s · %s", c.Region, c.Stadium, c.Manager), width)))
	}
	if c.Caretaker {
		lines = append(lines, styleDim.Render(truncate(m.ui("ui.club.caretaker"), width)))
	}
	// Compact layouts do not have room for the identity header's two board
	// rows, so retain the one-line summary there. Wide layouts already show the
	// same facts beside the badge and should move straight on to club context.
	if width < 80 || height < 24 {
		lines = append(lines, fmt.Sprintf("%s %d  %s %d  %s %s  %s %s",
			m.ui("ui.club.predicted"), c.PredictedFinish,
			m.ui("ui.club.objective"), c.BoardObjectiveFinish,
			m.ui("ui.club.confidence"), c.Board["confidence"],
			m.ui("ui.club.security"), c.Board["security"]))
	}
	lines = append(lines,
		fmt.Sprintf("%s %s  %s %s  %s %s  %s %s",
			m.ui("ui.club.fan_mood"), c.Board["fan_mood"],
			m.ui("ui.club.balance"), c.Finances["cash"],
			m.ui("ui.club.wage_bill"), c.Finances["salary_bill"],
			m.ui("ui.club.transfer_budget"), c.Finances["market_funds"]),
		"",
	)
	used := len(lines)
	tableHeight := height - used
	if tableHeight < 4 {
		tableHeight = 4
	}
	if width >= 96 {
		squadWidth := width / 2
		if squadWidth < 48 {
			squadWidth = 48
		}
		if squadWidth > 72 {
			squadWidth = 72
		}
		detailWidth := width - squadWidth - 1
		squad := styleDim.Render(truncate(m.ui("ui.club.squad"), squadWidth)) + "\n" +
			m.squadTable(squadWidth, tableHeight-1, c.Squad, m.PlayerIdx)
		player := m.playerDetail(detailWidth, tableHeight, c.Squad)
		lines = append(lines, strings.Split(lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(squadWidth).Render(squad),
			" ",
			lipgloss.NewStyle().Width(detailWidth).Render(player),
		), "\n")...)
	} else {
		remaining := height - len(lines)
		playerRows := 0
		if height >= 18 {
			playerRows = clampInt(height/3, 5, 8)
		}
		squadBlockRows := remaining
		if playerRows > 0 {
			squadBlockRows = remaining - playerRows - 1
		}
		if squadBlockRows < 5 {
			playerRows = 0
			squadBlockRows = remaining
		}
		if squadBlockRows > 1 {
			lines = append(lines, styleDim.Render(truncate(m.ui("ui.club.squad"), width)))
			lines = append(lines, m.squadTable(width, squadBlockRows-1, c.Squad, m.PlayerIdx))
		}
		if playerRows > 0 {
			lines = append(lines, "", m.playerDetail(width, playerRows, c.Squad))
		}
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func (m Model) squadTable(width, height int, squad []Player, selected int) string {
	cols := []tableColumn{
		{Header: "", Width: 2, Align: alignLeft},
		{Header: m.ui("ui.col.position"), Width: colWidth(m.ui("ui.col.position"), 4), Align: alignLeft},
		{Header: m.ui("ui.col.name"), MinWidth: 12, Flex: true, Align: alignLeft},
		{Header: m.ui("ui.col.age"), Width: colWidth(m.ui("ui.col.age"), 3), Align: alignRight},
		{Header: m.ui("ui.col.attributes"), MinWidth: 18, Flex: true, Align: alignLeft},
		{Header: m.ui("ui.col.contract"), Width: colWidth(m.ui("ui.col.contract"), 8), Align: alignRight},
	}
	maxRows := height - 4
	if maxRows < 0 {
		maxRows = 0
	}
	rows := make([][]string, 0, len(squad))
	for i, p := range squad {
		if i >= maxRows {
			break
		}
		mark := " "
		if i == selected {
			mark = ">"
		}
		contract := ""
		if p.ContractExpirySeason != 0 {
			contract = fmt.Sprint(p.ContractExpirySeason)
		}
		rows = append(rows, []string{mark, p.Position, p.Name, intCell(p.Age), m.playerAttrSummary(p), contract})
	}
	return renderTextTable(width, cols, rows)
}

func (m Model) playerDetail(width, height int, squad []Player) string {
	if len(squad) == 0 {
		return styleDim.Render(truncate(m.ui("ui.player.empty"), width))
	}
	idx := m.PlayerIdx
	if idx < 0 || idx >= len(squad) {
		idx = 0
	}
	p := squad[idx]
	lines := []string{
		styleHeader.Render(truncate(m.ui("ui.player.dossier"), width)),
		truncate(p.Name, width),
		styleDim.Render(truncate(fmt.Sprintf("%s %d · %s %s · %s %s",
			m.ui("ui.col.age"), p.Age,
			m.ui("ui.col.position"), p.Position,
			m.ui("ui.player.group"), p.Group), width)),
		truncate(fmt.Sprintf("%s %s  ·  %s %s / %s",
			m.ui("ui.player.body"), bodyLabel(p),
			m.ui("ui.player.foot"), footLabel(p), weakFootLabel(p)), width),
		truncate(fmt.Sprintf("%s %s  %s %d",
			m.ui("ui.player.familiarity"), p.FamiliarityLabel,
			m.ui("ui.col.contract"), p.ContractExpirySeason), width),
		"",
		styleDim.Render(truncate(m.ui("ui.player.profile"), width)),
	}
	for _, part := range m.playerAttrLines(p, width) {
		lines = append(lines, part)
	}
	if p.Youth {
		lines = append(lines, styleDim.Render(truncate(m.ui("ui.player.youth"), width)))
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func bodyLabel(p Player) string {
	if p.HeightCm <= 0 || p.WeightKg <= 0 {
		return "-"
	}
	return fmt.Sprintf("%dcm / %dkg", p.HeightCm, p.WeightKg)
}

func footLabel(p Player) string {
	if p.FootLabel != "" {
		return p.FootLabel
	}
	if p.Foot == "" {
		return "-"
	}
	return p.Foot
}

func weakFootLabel(p Player) string {
	if p.WeakFootLabel == "" {
		return "-"
	}
	return p.WeakFootLabel
}

func (m Model) playerAttrLines(p Player, width int) []string {
	pairs := orderedPlayerProfileAttrs(p.Attributes)
	out := make([]string, 0, len(pairs))
	labelWidth := 0
	for _, p := range pairs {
		if w := lipgloss.Width(m.ui("attr." + p.key)); w > labelWidth {
			labelWidth = w
		}
	}
	if labelWidth < 8 {
		labelWidth = 8
	}
	const attrRowGutter = 5 // one gap, two value cells, two gaps before the bar
	const minAttrLabelWidth = 8
	const minAttrBarWidth = 4
	barWidth := 20
	if labelWidth+attrRowGutter+barWidth > width {
		barWidth = width - attrRowGutter - labelWidth
	}
	if barWidth < minAttrBarWidth {
		barWidth = minAttrBarWidth
		labelWidth = width - attrRowGutter - barWidth
	}
	if labelWidth < minAttrLabelWidth {
		labelWidth = minAttrLabelWidth
		barWidth = width - attrRowGutter - labelWidth
	}
	if barWidth < 1 {
		barWidth = 1
	}
	for _, p := range pairs {
		line := fmt.Sprintf("%s %2d  %s",
			fitLine(m.ui("attr."+p.key), labelWidth, alignLeft),
			p.val,
			attrBar(p.val, barWidth))
		out = append(out, truncate(line, width))
	}
	return out
}

func orderedPlayerProfileAttrs(attrs map[string]int) []attrPair {
	pairs := make([]attrPair, 0, len(attrs))
	for k, v := range attrs {
		pairs = append(pairs, attrPair{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		ri, okI := playerProfileAttrRank[pairs[i].key]
		rj, okJ := playerProfileAttrRank[pairs[j].key]
		switch {
		case okI && okJ:
			return ri < rj
		case okI:
			return true
		case okJ:
			return false
		default:
			return pairs[i].key < pairs[j].key
		}
	})
	return pairs
}

func (m Model) clickedSquadIndex(relY int) (int, bool) {
	if len(m.Club.Squad) == 0 {
		return 0, false
	}
	row := relY - 11
	if row < 0 {
		return 0, false
	}
	if row >= len(m.Club.Squad) {
		return 0, false
	}
	return row, true
}

func clubBadge(name string) string {
	sum := 0
	for _, r := range name {
		sum += int(r)
	}
	letters := "AFC"
	parts := strings.Fields(name)
	if len(parts) > 0 {
		initials := ""
		for _, p := range parts {
			if p == "" {
				continue
			}
			initials += strings.ToUpper(string([]rune(p)[0]))
			if len([]rune(initials)) >= 3 {
				break
			}
		}
		if initials != "" {
			letters = initials
		}
	}
	patterns := []string{"◆", "▲", "●", "■", "✦"}
	mark := patterns[sum%len(patterns)]
	return strings.Join([]string{
		"╭────────────╮",
		"│ " + fitLine(mark+" "+letters, 10, alignCenter) + " │",
		"│  AGENTIC   │",
		"│     FC     │",
		"╰────────────╯",
	}, "\n")
}

func attrBar(v, width int) string {
	if v < 0 {
		v = 0
	}
	if v > 20 {
		v = 20
	}
	fill := v * width / 20
	return strings.Repeat("█", fill) + strings.Repeat("░", width-fill)
}

func (m Model) playerAttrSummary(p Player) string {
	type pair struct {
		key string
		val int
	}
	pairs := make([]pair, 0, len(p.Attributes))
	for k, v := range p.Attributes {
		pairs = append(pairs, pair{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].val != pairs[j].val {
			return pairs[i].val > pairs[j].val
		}
		return pairs[i].key < pairs[j].key
	})
	limit := 3
	if len(pairs) < limit {
		limit = len(pairs)
	}
	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		label := m.ui("attr." + pairs[i].key)
		parts = append(parts, fmt.Sprintf("%s %d", label, pairs[i].val))
	}
	return strings.Join(parts, " · ")
}

func visibleStart(selected, total, rows int) int {
	if rows <= 0 || total <= rows {
		return 0
	}
	start := selected - rows/2
	if start < 0 {
		return 0
	}
	maxStart := total - rows
	if start > maxStart {
		return maxStart
	}
	return start
}

func (m Model) viewFixtures(width, height int) string {
	if len(m.Fixtures) == 0 {
		return styleDim.Render(m.ui("ui.fixtures.empty"))
	}
	return m.fixtureList(width, height)
}

func (m Model) fixtureList(width, height int) string {
	cols := []tableColumn{
		{Header: "", Width: 2, Align: alignLeft},
		{Header: m.ui("ui.col.status"), Width: colWidth(m.ui("ui.col.status"), 11), Align: alignLeft},
		{Header: m.ui("ui.col.kickoff"), Width: colWidth(m.ui("ui.col.kickoff"), 18), Align: alignLeft},
		{Header: m.ui("ui.col.round"), Width: colWidth(m.ui("ui.col.round"), 4), Align: alignRight},
		{Header: m.ui("ui.col.fixture"), MinWidth: 18, Flex: true, Align: alignLeft},
	}
	maxRows := height - 4
	if maxRows < 0 {
		maxRows = 0
	}
	rows := make([][]string, 0, len(m.Fixtures))
	start := visibleStart(m.FixtureIdx, len(m.Fixtures), maxRows)
	for i := start; i < len(m.Fixtures) && len(rows) < maxRows; i++ {
		f := m.Fixtures[i]
		mark := " "
		if i == m.FixtureIdx {
			mark = ">"
		}
		fixture := f.Home + " - " + f.Away
		if mv, ok := m.liveMatchForFixture(f.ID); ok {
			fixture = fmt.Sprintf("%s %d-%d %s", f.Home, mv.HomeGoals, mv.AwayGoals, f.Away)
		} else if f.Status == "RESULT" {
			fixture = fmt.Sprintf("%s %d-%d %s", f.Home, f.HomeGoals, f.AwayGoals, f.Away)
		}
		rows = append(rows, []string{mark, m.fixtureStatus(f), f.KickoffText, roundCell(f), fixture})
	}
	return renderTextTable(width, cols, rows)
}

func (m Model) fixtureStatus(f Fixture) string {
	if mv, ok := m.liveMatchForFixture(f.ID); ok {
		// The running minute turns the fixture list into a matchday board.
		return fmt.Sprintf("%s %d'", m.ui("ui.fixture.live"), mv.Minute)
	}
	if f.Status == "LIVE" {
		return m.ui("ui.fixture.live")
	}
	if f.Status == "RESULT" {
		if f.Archived {
			return m.ui("ui.fixture.archived")
		}
		if f.HasReplay {
			return m.ui("ui.fixture.replay")
		}
		return m.ui("ui.fixture.result")
	}
	return m.ui("ui.fixture.scheduled")
}

func (m Model) liveMatchForFixture(id int64) (LiveMatchView, bool) {
	for _, mv := range m.Matches {
		if mv.Fixture == id {
			return mv, true
		}
	}
	return LiveMatchView{}, false
}

func (m Model) liveIndexForFixture(id int64) int {
	for i, mv := range m.Matches {
		if mv.Fixture == id {
			return i
		}
	}
	return -1
}

func (m Model) fixtureIndexByID(id int64) int {
	for i, f := range m.Fixtures {
		if f.ID == id {
			return i
		}
	}
	return -1
}

func (m Model) openSelectedFixture() (tea.Model, tea.Cmd) {
	if len(m.Fixtures) == 0 || m.FixtureIdx < 0 || m.FixtureIdx >= len(m.Fixtures) {
		return m, nil
	}
	f := m.Fixtures[m.FixtureIdx]
	if idx := m.liveIndexForFixture(f.ID); idx >= 0 {
		m.MatchIdx = idx
		m.MatchModal = modalLive
		m.MatchModalID = f.ID
		m.MatchDetail = MatchDetail{}
		m.ReplayOffset = 0
		m.startMatchAnimation()
		return m, matchAnimationTick(m.matchAnimationRun)
	}
	if f.Status == "RESULT" {
		m.MatchModal = modalReplay
		m.MatchModalID = f.ID
		m.ReplayOffset = 0
		if m.MatchDetail.Fixture == f.ID {
			// Refresh on reopen: facts are immutable, prose/localization can change.
			return m, m.fetchMatch()
		}
		m.MatchDetail = MatchDetail{}
		return m, m.fetchMatch()
	}
	if f.Status == "LIVE" {
		m.MatchModal = modalWaiting
		m.MatchModalID = f.ID
		return m, m.fetchLive()
	}
	m.setNotice(m.ui("ui.fixture.scheduled_notice"))
	return m, nil
}

func (m Model) liveModalFinished() (tea.Model, tea.Cmd) {
	m.stopMatchAnimation()
	// Full time swaps the modal kind under the viewer; the replay (or
	// waiting) body should open on its normal broadcast view, not a lineup
	// panel left over from the live match.
	m.LineupView = false
	if idx := m.fixtureIndexByID(m.MatchModalID); idx >= 0 {
		f := m.Fixtures[idx]
		m.FixtureIdx = idx
		if f.Status == "RESULT" {
			m.MatchModal = modalReplay
			m.MatchModalID = f.ID
			m.MatchDetail = MatchDetail{}
			m.ReplayOffset = 0
			return m, m.fetchMatch()
		}
	}
	m.MatchModal = modalWaiting
	m.setNotice(m.ui("ui.match.ended"))
	return m, m.fetchFixtures()
}

func roundCell(f Fixture) string {
	if f.Round != 0 {
		return intCell(f.Round)
	}
	if f.Season != 0 {
		return "S" + intCell(f.Season)
	}
	return ""
}

// truncate trims s to n display CELLS (not runes — Korean and other
// full-width text occupies two cells each, and the fixed-width panes would
// otherwise overflow. ASCII behaves exactly as before.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	out := ""
	for _, r := range s {
		next := out + string(r)
		if lipgloss.Width(next) > n-1 {
			break
		}
		out = next
	}
	return out + "…"
}

func wrapText(s string, width int) []string {
	if width <= 0 {
		return nil
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{}
	line := ""
	for _, w := range words {
		if line == "" {
			line = w
			continue
		}
		next := line + " " + w
		if lipgloss.Width(next) <= width {
			line = next
			continue
		}
		lines = append(lines, truncate(line, width))
		line = w
	}
	if line != "" {
		lines = append(lines, truncate(line, width))
	}
	return lines
}

func wrapParagraphs(s string, width int) []string {
	parts := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	out := []string{}
	blank := false
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
				blank = true
			}
			continue
		}
		out = append(out, wrapText(part, width)...)
		blank = false
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func (m Model) diagnosticLines(d MatchDiagnostics, width, limit int) []string {
	lines := make([]string, 0, limit)
	add := func(label, value string) {
		if value == "" || len(lines) >= limit {
			return
		}
		lines = append(lines, truncate(fmt.Sprintf("%s %s", label, value), width))
	}
	add(m.ui("ui.match.stat.quality"), m.qualityLabel(d.ShotQuality, d.ShotQualityBySide, 3))
	add(m.ui("ui.match.stat.aerial"), m.aerialLabel(d.AerialDuels, d.AerialWins))
	add(m.ui("ui.match.stat.press"), m.sideLabel(d.PressTurnovers))
	add(m.ui("ui.match.stat.setpieces"), m.sideLabel(d.SetPieceThreat))
	return lines
}

func (m Model) qualityLabel(counts, bySide map[string]int, limit int) string {
	order := []string{"HIGH", "MEDIUM", "LOW"}
	bandLabel := func(values map[string]int) string {
		parts := make([]string, 0, len(order))
		for _, band := range order {
			if n := values[band]; n > 0 {
				parts = append(parts, fmt.Sprintf("%s %d", m.qualityBandLabel(band), n))
			}
		}
		if len(parts) > limit {
			parts = parts[:limit]
		}
		return strings.Join(parts, " · ")
	}
	if len(bySide) == 0 {
		return bandLabel(counts)
	}
	var sections []string
	known := map[string]int{}
	for _, side := range []string{"HOME", "AWAY"} {
		values := map[string]int{}
		for _, band := range order {
			values[band] = bySide[side+"_"+band]
			known[band] += values[band]
		}
		if label := bandLabel(values); label != "" {
			sections = append(sections, side[:1]+" "+label)
		}
	}
	// Side counts are authoritative for attribution. The aggregate only fills
	// a positive legacy remainder; never invent a negative unknown bucket when
	// inconsistent input attributes more shots than the aggregate recorded.
	unknown := map[string]int{}
	for _, band := range order {
		if n := counts[band] - known[band]; n > 0 {
			unknown[band] = n
		}
	}
	if label := bandLabel(unknown); label != "" {
		sections = append(sections, "? "+label)
	}
	if len(sections) == 0 {
		// Empty/zero side data keeps the aggregate useful instead of returning
		// an empty label. Unknown non-zero sides surface through the ? remainder.
		return bandLabel(counts)
	}
	return strings.Join(sections, " | ")
}

func (m Model) qualityBandLabel(key string) string {
	label := m.ui("term.quality." + key)
	if label != "term.quality."+key {
		return label
	}
	return strings.ToLower(key)
}

func (m Model) aerialLabel(duels, wins map[string]int) string {
	homeDuels, awayDuels := duels[matchSideHome], duels[matchSideAway]
	if homeDuels == 0 && awayDuels == 0 {
		return ""
	}
	return fmt.Sprintf("H %d/%d · A %d/%d", wins[matchSideHome], homeDuels, wins[matchSideAway], awayDuels)
}

func (m Model) sideLabel(counts map[string]int) string {
	home, away := counts[matchSideHome], counts[matchSideAway]
	if home == 0 && away == 0 {
		return ""
	}
	return fmt.Sprintf("H %d · A %d", home, away)
}

func (m Model) chanceMixLabel(types, bySide map[string]int, limit int) string {
	type pair struct {
		key string
		val int
	}
	format := func(counts map[string]int) string {
		pairs := make([]pair, 0, len(counts))
		for k, v := range counts {
			if v > 0 {
				pairs = append(pairs, pair{k, v})
			}
		}
		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].val != pairs[j].val {
				return pairs[i].val > pairs[j].val
			}
			return pairs[i].key < pairs[j].key
		})
		if len(pairs) > limit {
			pairs = pairs[:limit]
		}
		parts := make([]string, 0, len(pairs))
		for _, p := range pairs {
			parts = append(parts, fmt.Sprintf("%s %d", m.chanceTypeLabel(p.key), p.val))
		}
		return strings.Join(parts, " · ")
	}
	if len(bySide) == 0 {
		if label := format(types); label != "" {
			return "? " + label
		}
		return ""
	}
	home, away, known := map[string]int{}, map[string]int{}, map[string]int{}
	for key, n := range bySide {
		if n <= 0 {
			continue
		}
		switch {
		case strings.HasPrefix(key, matchSideHome+"_"):
			pattern := strings.TrimPrefix(key, matchSideHome+"_")
			home[pattern] += n
			known[pattern] += n
		case strings.HasPrefix(key, matchSideAway+"_"):
			pattern := strings.TrimPrefix(key, matchSideAway+"_")
			away[pattern] += n
			known[pattern] += n
		}
	}
	sections := make([]string, 0, 3)
	if label := format(home); label != "" {
		sections = append(sections, "H "+label)
	}
	if label := format(away); label != "" {
		sections = append(sections, "A "+label)
	}
	unknown := map[string]int{}
	for pattern, total := range types {
		if remainder := total - known[pattern]; remainder > 0 {
			unknown[pattern] = remainder
		}
	}
	if label := format(unknown); label != "" {
		sections = append(sections, "? "+label)
	}
	if len(sections) == 0 {
		if label := format(types); label != "" {
			return "? " + label
		}
	}
	return strings.Join(sections, " | ")
}

func (m Model) chanceTypeLabel(key string) string {
	label := m.ui("term.chance_type." + key)
	if label != "term.chance_type."+key {
		return label
	}
	return strings.ToLower(strings.ReplaceAll(key, "_", " "))
}

func balancedRatings(ratings []LiveRating, perSide int) []LiveRating {
	if perSide <= 0 {
		return nil
	}
	out := make([]LiveRating, 0, perSide*2)
	counts := map[string]int{}
	for _, r := range ratings {
		if r.Side != matchSideHome && r.Side != matchSideAway {
			continue
		}
		if counts[r.Side] >= perSide {
			continue
		}
		out = append(out, r)
		counts[r.Side]++
	}
	return out
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
