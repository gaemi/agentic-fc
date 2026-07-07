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
	tabMatch
	tabCount
)

const (
	viewerHistoryLimit = 1000
	scrollPageLines    = 8
	scrollWheelLines   = 4
	pollInterval       = 2 * time.Second
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
	Matches       []LiveMatchView
	MatchIdx      int
	Notice        string
	NoticeTTL     int
	LatestNewsID  int64
	LiveCount     int
	// PitchHidden is the M-tier toggle (docs/07 §4.1: the pitch strip is an
	// enhanced element — 'p' collapses it back to pure commentary).
	PitchHidden bool
	// PaneMode is the stats/ratings selector 'r' cycles (docs/07 §4.1). On S
	// it swaps the commentary area (0 commentary · 1 stats · 2 ratings); on M
	// it picks the side pane (0 stats · 1 ratings · 2 hidden). L/XL show both
	// panes persistently and ignore it.
	PaneMode int

	Tab    int
	Tier   int
	Width  int
	Height int
	Err    string
}

func NewModel(c *Client) Model {
	return Model{Client: c, Tier: 1, UI: map[string]string{}, LiveCount: -1}
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
	ErrMsg      struct{ Err error }
	tickMsg     struct{}
)

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchUI(), m.fetchWorld(), m.fetchNews(), m.fetchTable(), m.fetchClubs(), m.fetchFixtures(), m.fetchLive(), tick())
}

func tick() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tickMsg{} })
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
			return ErrMsg{err}
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

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width, m.Height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "1":
			m.Tab = tabMedia
			m.clearNotice()
		case "2":
			m.Tab = tabTable
			m.clearNotice()
		case "3":
			m.Tab = tabClubs
			m.clearNotice()
		case "4":
			m.Tab = tabFixtures
			m.clearNotice()
		case "5":
			m.Tab = tabMatch
			m.clearNotice()
		case "up", "k":
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
					m.MatchDetail = MatchDetail{}
					return m, m.fetchMatch()
				}
			}
		case "down", "j":
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
					m.MatchDetail = MatchDetail{}
					return m, m.fetchMatch()
				}
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
			switch m.Tab {
			case tabMedia:
				m.ArticleOffset = scrollBack(m.ArticleOffset, scrollPageLines)
			case tabFixtures:
				m.ReplayOffset = scrollBack(m.ReplayOffset, scrollPageLines)
			}
		case "pgdown":
			switch m.Tab {
			case tabMedia:
				m.ArticleOffset = scrollForward(m.ArticleOffset, m.articleScrollLineCount(), scrollPageLines)
			case tabFixtures:
				m.ReplayOffset = scrollForward(m.ReplayOffset, len(m.MatchDetail.Commentary), scrollPageLines)
			}
		case "p":
			// Match-screen keys only act there — a toggle pressed on another
			// tab must not silently rearrange hidden Match state.
			if m.Tab == tabMatch {
				m.PitchHidden = !m.PitchHidden
			}
		case "r":
			if m.Tab == tabMatch {
				m.PaneMode = (m.PaneMode + 1) % 3
			}
		case "left":
			if m.Tab == tabMatch {
				if n := len(m.Matches); n > 0 {
					m.MatchIdx = (m.MatchIdx + n - 1) % n // wrap: cycling, not clamping
				}
				break
			}
			if (m.Tab == tabTable || m.Tab == tabFixtures) && m.Tier > 1 {
				m.Tier--
				return m, tea.Batch(m.fetchTable(), m.fetchFixtures())
			}
		case "right":
			if m.Tab == tabMatch {
				if n := len(m.Matches); n > 0 {
					m.MatchIdx = (m.MatchIdx + 1) % n
				}
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
		if len(m.Fixtures) > 0 {
			selected := m.Fixtures[m.FixtureIdx]
			if selected.Status == "RESULT" && m.MatchDetail.Fixture != selected.ID {
				return m, m.fetchMatch()
			}
			if selected.Status != "RESULT" {
				m.MatchDetail = MatchDetail{}
				m.ReplayOffset = 0
			}
		}
	case MatchMsg:
		m.MatchDetail = MatchDetail(msg)
		if m.ReplayOffset >= len(m.MatchDetail.Commentary) {
			m.ReplayOffset = 0
		}
	case MatchesMsg:
		if m.LiveCount >= 0 && len(msg) > m.LiveCount {
			m.setNotice(strings.ReplaceAll(m.ui("ui.notice.match"), "{count}", fmt.Sprint(len(msg))))
		}
		m.LiveCount = len(msg)
		m.Matches = msg
		if m.MatchIdx >= len(m.Matches) {
			m.MatchIdx = 0
		}
	case ErrMsg:
		m.Err = msg.Err.Error()
	case tickMsg:
		m.ageNotice()
		return m, tea.Batch(m.fetchWorld(), m.fetchNews(), m.fetchTable(), m.fetchClubs(), m.fetchClub(), m.fetchFixtures(), m.fetchLive(), tick())
	}
	return m, nil
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

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelUp {
		switch m.Tab {
		case tabMedia:
			m.ArticleOffset = scrollBack(m.ArticleOffset, scrollWheelLines)
		case tabFixtures:
			m.ReplayOffset = scrollBack(m.ReplayOffset, scrollWheelLines)
		}
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		switch m.Tab {
		case tabMedia:
			m.ArticleOffset = scrollForward(m.ArticleOffset, m.articleScrollLineCount(), scrollWheelLines)
		case tabFixtures:
			m.ReplayOffset = scrollForward(m.ReplayOffset, len(m.MatchDetail.Commentary), scrollWheelLines)
		}
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if msg.Y == 2 {
		if tab, ok := m.clickedTab(msg.X); ok {
			m.Tab = tab
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
		listWidth := bodyWidth
		if bodyWidth >= 120 {
			listWidth = bodyWidth / 2
			if listWidth < 56 {
				listWidth = 56
			}
			if listWidth > 78 {
				listWidth = 78
			}
		}
		if msg.X < bodyX || msg.X >= bodyX+listWidth {
			return m, nil
		}
		row := msg.Y - bodyY - 3
		maxRows := m.Height - 10
		if idx, ok := clickedListIndex(row, m.FixtureIdx, len(m.Fixtures), maxRows); ok {
			m.FixtureIdx = idx
			m.ReplayOffset = 0
			m.MatchDetail = MatchDetail{}
			return m, m.fetchMatch()
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
	names := []string{m.ui("ui.tab.media"), m.ui("ui.tab.table"), m.ui("ui.tab.clubs"), m.ui("ui.tab.fixtures"), m.ui("ui.tab.match")}
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

// ui resolves a server-rendered string with a graceful key fallback.
func (m Model) ui(key string) string {
	if s, ok := m.UI[key]; ok {
		return s
	}
	return key
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
	errRows := 0
	if m.Err != "" {
		errRows = 1
	}
	bodyHeight := height - 6 - errRows
	if bodyHeight < 0 {
		bodyHeight = 0
	}

	var body string
	switch m.Tab {
	case tabMedia:
		body = m.viewMedia(bodyWidth, bodyHeight)
	case tabTable:
		body = m.viewTable(bodyWidth, bodyHeight)
	case tabClubs:
		body = m.viewClubs(bodyWidth, bodyHeight)
	case tabFixtures:
		body = m.viewFixtures(bodyWidth, bodyHeight)
	case tabMatch:
		body = m.viewMatch(tier, bodyWidth, bodyHeight)
	default:
		body = m.viewMedia(bodyWidth, bodyHeight)
	}

	header := styleHeader.Render(truncate(fmt.Sprintf("%s · %s · [%s]",
		m.World.Name, m.World.ClockText, m.World.TempoLabel), bodyWidth))
	footer := styleDim.Render(truncate(m.ui("ui.help.keys"), bodyWidth))
	err := ""
	if m.Err != "" {
		err = styleDim.Render(truncate(m.ui("ui.error.prefix")+" "+m.Err, bodyWidth))
	}
	return appFrame{
		Width:    width,
		Height:   height,
		Title:    m.ui("ui.app.title"),
		Header:   header,
		Tabs:     m.tabBar(bodyWidth),
		Body:     body,
		Error:    err,
		Footer:   footer,
		Overlays: m.noticeOverlays(width, height),
	}.Render()
}

func (m Model) noticeOverlays(width, height int) []Overlay {
	if m.Notice == "" || width < 72 || height < 18 {
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
	if width < 24 {
		width = 24
	}
	textWidth := width - 10
	wrapped := wrapText(text, textWidth)
	if len(wrapped) == 0 {
		wrapped = []string{""}
	}
	lines := []string{
		"   ╭" + strings.Repeat("─", textWidth+2) + "╮",
	}
	for _, line := range wrapped {
		lines = append(lines, " ◖●●◗ │ "+fitLine(line, textWidth, alignLeft)+" │")
	}
	lines = append(lines,
		"  /▔▔\\╰"+strings.Repeat("─", textWidth+2)+"╯",
		"  \\__/  "+strings.Repeat(" ", textWidth),
	)
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
	names := []string{m.ui("ui.tab.media"), m.ui("ui.tab.table"), m.ui("ui.tab.clubs"), m.ui("ui.tab.fixtures"), m.ui("ui.tab.match")}
	parts := make([]string, tabCount)
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
	if width < 24 {
		return []string{styleDim.Render(truncate(n.Source+" · "+n.TimeText+" · "+n.Category, width))}
	}
	inner := width
	if inner > 86 {
		inner = 86
	}
	top := fitLine("╔"+strings.Repeat("═", inner-2)+"╗", width, alignCenter)
	title := strings.ToUpper(m.ui("ui.app.title") + " " + m.ui("ui.tab.media"))
	source := n.Source + " · " + n.TimeText
	section := "[" + strings.ToUpper(n.Category) + "]"
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
	if width >= 80 {
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
		lines = append(lines, strings.Split(badge, "\n")...)
		lines = append(lines, styleDim.Render(truncate(fmt.Sprintf("%s · %s · %s", c.Region, c.Stadium, c.Manager), width)))
	}
	if c.Caretaker {
		lines = append(lines, styleDim.Render(truncate(m.ui("ui.club.caretaker"), width)))
	}
	lines = append(lines,
		fmt.Sprintf("%s %d  %s %d  %s %s  %s %s",
			m.ui("ui.club.predicted"), c.PredictedFinish,
			m.ui("ui.club.objective"), c.BoardObjectiveFinish,
			m.ui("ui.club.confidence"), c.Board["confidence"],
			m.ui("ui.club.security"), c.Board["security"]),
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
		lines = append(lines, styleDim.Render(truncate(m.ui("ui.club.squad"), width)))
		lines = append(lines, m.squadTable(width, tableHeight-1, c.Squad, m.PlayerIdx))
		lines = append(lines, "", m.playerDetail(width, 8, c.Squad))
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
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, truncate(fmt.Sprintf("%-18s %2d  %s", m.ui("attr."+p.key), p.val, attrBar(p.val, 20)), width))
	}
	return out
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
	if width >= 120 {
		listWidth := width / 2
		if listWidth < 56 {
			listWidth = 56
		}
		if listWidth > 78 {
			listWidth = 78
		}
		detailWidth := width - listWidth - 1
		list := lipgloss.NewStyle().Width(listWidth).Render(m.fixtureList(listWidth, height))
		detail := lipgloss.NewStyle().Width(detailWidth).Render(m.fixtureDetail(detailWidth, height))
		return lipgloss.JoinHorizontal(lipgloss.Top, list, " ", detail)
	}
	listRows := height / 2
	if listRows < 8 {
		listRows = 8
	}
	if listRows > height-6 {
		listRows = height - 6
	}
	if listRows < 1 {
		listRows = height
	}
	return m.fixtureList(width, listRows) + "\n" + m.fixtureDetail(width, height-listRows-1)
}

func (m Model) fixtureList(width, height int) string {
	cols := []tableColumn{
		{Header: "", Width: 2, Align: alignLeft},
		{Header: m.ui("ui.col.status"), Width: colWidth(m.ui("ui.col.status"), 8), Align: alignLeft},
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
		if f.Status == "RESULT" {
			fixture = fmt.Sprintf("%s %d-%d %s", f.Home, f.HomeGoals, f.AwayGoals, f.Away)
		}
		rows = append(rows, []string{mark, m.fixtureStatus(f), f.KickoffText, roundCell(f), fixture})
	}
	return renderTextTable(width, cols, rows)
}

func (m Model) fixtureStatus(f Fixture) string {
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

func roundCell(f Fixture) string {
	if f.Round != 0 {
		return intCell(f.Round)
	}
	if f.Season != 0 {
		return "S" + intCell(f.Season)
	}
	return ""
}

func (m Model) fixtureDetail(width, height int) string {
	if len(m.Fixtures) == 0 {
		return ""
	}
	f := m.Fixtures[m.FixtureIdx]
	if f.Status != "RESULT" {
		lines := []string{
			styleHeader.Render(truncate(f.Home+" - "+f.Away, width)),
			styleDim.Render(truncate(f.KickoffText+" · "+f.Competition, width)),
			"",
			truncate(m.ui("ui.fixture.scheduled_detail"), width),
		}
		if len(lines) > height {
			lines = lines[:height]
		}
		return strings.Join(lines, "\n")
	}
	md := m.MatchDetail
	if md.Fixture != f.ID {
		return styleDim.Render(truncate(m.ui("ui.match.loading"), width))
	}
	lines := []string{
		styleHeader.Render(truncate(fmt.Sprintf("%s %d-%d %s", md.Home, md.HomeGoals, md.AwayGoals, md.Away), width)),
		styleDim.Render(truncate(md.KickoffText+" · "+md.Competition+" · "+m.fixtureStatus(f), width)),
		fmt.Sprintf("%s %d  %s %d", m.ui("ui.match.stat.shots"), md.HomeShots, m.ui("ui.match.away"), md.AwayShots),
	}
	if mix := m.chanceMixLabel(md.ChanceTypes, 3); mix != "" {
		lines = append(lines, truncate(fmt.Sprintf("%s %s", m.ui("ui.match.stat.chance_mix"), mix), width))
	}
	for _, line := range m.diagnosticLines(md.Diagnostics, width, 3) {
		lines = append(lines, line)
	}
	if md.Winner != "" {
		lines = append(lines, fmt.Sprintf("%s %s", m.ui("ui.match.winner"), md.Winner))
	}
	lines = append(lines, "")
	appendEvents := func(title string, events []MatchEvent) {
		if len(events) == 0 {
			return
		}
		lines = append(lines, styleDim.Render(truncate(title, width)))
		for _, e := range events {
			detail := e.Player
			if e.Detail != "" {
				detail += " " + e.Detail
			}
			lines = append(lines, truncate(fmt.Sprintf("%d' %s · %s", e.Minute, e.Club, detail), width))
		}
	}
	appendEvents(m.ui("ui.match.scorers"), md.Scorers)
	appendEvents(m.ui("ui.match.cards"), md.Cards)
	if len(md.Subs) > 0 {
		lines = append(lines, styleDim.Render(truncate(m.ui("ui.match.subs"), width)))
		for _, s := range md.Subs {
			on := s.On
			if on == "" {
				on = "-"
			}
			lines = append(lines, truncate(fmt.Sprintf("%d' %s · %s -> %s %s", s.Minute, s.Club, s.Off, on, s.Reason), width))
		}
	}
	if len(md.Ratings) > 0 {
		lines = append(lines, styleDim.Render(truncate(m.ui("ui.match.ratings"), width)))
		limit := 5
		if len(md.Ratings) < limit {
			limit = len(md.Ratings)
		}
		for i := 0; i < limit; i++ {
			r := md.Ratings[i]
			lines = append(lines, truncate(fmt.Sprintf("%d.%d %s", r.RatingX10/10, r.RatingX10%10, r.Name), width))
		}
	}
	if len(md.Commentary) > 0 {
		lines = append(lines, "", styleDim.Render(truncate(m.ui("ui.match.replay"), width)))
		remaining := height - len(lines)
		if remaining < 1 {
			remaining = 1
		}
		start := m.ReplayOffset
		if start < 0 {
			start = 0
		}
		if start >= len(md.Commentary) {
			start = 0
		}
		for i := start; i < len(md.Commentary) && len(lines) < height; i++ {
			lines = append(lines, truncate(md.Commentary[i], width))
		}
		if start+remaining < len(md.Commentary) {
			lines = append(lines, styleDim.Render(truncate(m.ui("ui.match.replay.more"), width)))
		}
	} else if md.Archived {
		lines = append(lines, "", styleDim.Render(truncate(m.ui("ui.match.replay.archived"), width)))
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
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

// The Live Match screen (docs/07 §4.1): score header, the ASCII pitch band,
// the commentary tail, and — per tier — the stats/ratings/momentum/ticker
// furniture. The pitch is the enhanced element and degrades FIRST: tier S is
// pure commentary (classic CM text layout; 'r' swaps in the stats or ratings
// block), M draws a compact toggleable strip plus ONE side pane ('r' cycles
// stats → ratings → hidden), L adds the momentum line and both panes (ticker
// when tall), XL keeps a persistent ticker column. It reads only what the
// Console API already carries — presentation, never sim state.
func (m Model) viewMatch(tier layout.Tier, width, height int) string {
	if len(m.Matches) == 0 {
		return styleDim.Render(m.ui("ui.match.none"))
	}
	mv := m.Matches[m.MatchIdx]

	var b strings.Builder
	b.WriteString(styleHeader.Render(fmt.Sprintf("%s %d - %d %s", mv.Home, mv.HomeGoals, mv.AwayGoals, mv.Away)))
	b.WriteString(styleDim.Render(fmt.Sprintf("  %d'  %s  (%d/%d)",
		mv.Minute, mv.Competition, m.MatchIdx+1, len(m.Matches))))
	b.WriteString("\n")
	used := 1 // score line inside the framed content area

	if tier >= layout.TierL && width >= 100 {
		board := m.renderScoreboard(width, mv)
		b.WriteString(board)
		used += strings.Count(board, "\n")
		if flash := m.renderGoalFlash(width, mv); flash != "" {
			b.WriteString(flash)
			used += strings.Count(flash, "\n")
		}
	}

	// Momentum sparkline under the header — L/XL only (§4.1 matrix).
	if tier >= layout.TierL && len(mv.Momentum) > 0 {
		b.WriteString(styleDim.Render(m.ui("ui.match.momentum")+" "+momentumGlyphs(mv.Momentum)) + "\n")
		used++
	}

	// Side furniture per tier: S swaps the main block, M gets one pane,
	// L/XL get both (+ticker XL always, L only when tall).
	var left, right, ticker string
	paneRows := height - used
	if paneRows < 4 {
		paneRows = 4
	}
	switch tier {
	case layout.TierS:
		if m.PaneMode == 1 {
			return b.String() + m.paneStats(mv)
		}
		if m.PaneMode == 2 {
			return b.String() + m.paneRatings(mv, paneRows)
		}
	case layout.TierM:
		if m.PaneMode == 0 {
			right = m.paneStats(mv)
		} else if m.PaneMode == 1 {
			right = m.paneRatings(mv, paneRows)
		}
	case layout.TierL:
		left, right = m.paneStats(mv), m.paneRatings(mv, paneRows)
		if m.Height >= tickerMinRowsL {
			ticker = m.paneTicker(paneRows)
		}
	case layout.TierXL:
		left, right = m.paneStats(mv), m.paneRatings(mv, paneRows)
		ticker = m.paneTicker(paneRows)
	}

	// Centre column: pitch band + commentary tail, sized to what the panes
	// leave over.
	centerWidth := width
	for _, pane := range []string{left, right, ticker} {
		if pane != "" {
			centerWidth -= paneWidth + 1
		}
	}
	if centerWidth < 24 {
		centerWidth = 24
	}
	rows := 0
	switch {
	case tier == layout.TierM && !m.PitchHidden:
		rows = pitchRowsStrip
	case tier == layout.TierL:
		rows = pitchRowsFull
	case tier == layout.TierXL:
		rows = pitchRowsTall
	}
	var center strings.Builder
	if rows > 0 {
		width := centerWidth - 2
		if width > pitchMaxWidth {
			width = pitchMaxWidth
		}
		center.WriteString(renderPitch(width, rows, mv.Markers))
		center.WriteString(styleDim.Render(m.ui("ui.match.legend")) + "\n")
		used += rows + 1
	}
	max := height - used
	if max < 4 {
		max = 4
	}
	lines := mv.Commentary
	center.WriteString(m.renderCommentaryLog(centerWidth, max, lines))

	cols := []string{}
	if left != "" {
		cols = append(cols, lipgloss.NewStyle().Width(paneWidth).Render(left))
	}
	cols = append(cols, lipgloss.NewStyle().Width(centerWidth).Render(center.String()))
	if right != "" {
		cols = append(cols, lipgloss.NewStyle().Width(paneWidth).Render(right))
	}
	if ticker != "" {
		cols = append(cols, lipgloss.NewStyle().Width(paneWidth).Render(ticker))
	}
	if len(cols) == 1 {
		return b.String() + center.String()
	}
	return b.String() + lipgloss.JoinHorizontal(lipgloss.Top, cols...)
}

func (m Model) renderCommentaryLog(width, maxRows int, lines []string) string {
	if maxRows <= 0 {
		return ""
	}
	if width < 16 {
		if len(lines) > maxRows {
			lines = lines[len(lines)-maxRows:]
		}
		return strings.Join(lines, "\n")
	}
	content := width - 2
	out := []string{styleDim.Render("┌" + fitLine(m.ui("ui.match.replay"), content, alignCenter) + "┐")}
	available := maxRows - 2
	if available < 1 {
		available = 1
	}
	if len(lines) > available {
		lines = lines[len(lines)-available:]
	}
	for _, line := range lines {
		out = append(out, "│"+fitLine(truncate(line, content), content, alignLeft)+"│")
	}
	for len(out) < maxRows-1 {
		out = append(out, "│"+strings.Repeat(" ", content)+"│")
	}
	out = append(out, styleDim.Render("└"+strings.Repeat("─", content)+"┘"))
	if len(out) > maxRows {
		out = out[:maxRows]
	}
	return strings.Join(out, "\n")
}

// Side-pane geometry (docs/07 §4.1). The ticker needs a tall L terminal; XL
// keeps it persistently.
const (
	paneWidth      = 26
	tickerMinRowsL = 40
)

// momentumGlyphs renders the sparkline: one glyph per 10-minute bucket, home
// pressure up, away pressure down.
func momentumGlyphs(buckets []int) string {
	var b strings.Builder
	for _, v := range buckets {
		switch {
		case v > 2:
			b.WriteRune('▲')
		case v > 0:
			b.WriteRune('△')
		case v < -2:
			b.WriteRune('▼')
		case v < 0:
			b.WriteRune('▽')
		default:
			b.WriteRune('·')
		}
	}
	return b.String()
}

// paneStats renders the Match stats block: shots, cards, subs — home | away.
func (m Model) paneStats(mv LiveMatchView) string {
	var b strings.Builder
	b.WriteString(styleHeader.Render(m.ui("ui.match.stats")) + "\n")
	row := func(label string, home, away int) {
		b.WriteString(fmt.Sprintf("%s %3d %3d\n", fitLine(label, 12, alignLeft), home, away))
	}
	row(m.ui("ui.match.stat.shots"), mv.Stats.HomeShots, mv.Stats.AwayShots)
	row(m.ui("ui.match.stat.cards"), mv.Stats.HomeCards, mv.Stats.AwayCards)
	row(m.ui("ui.match.stat.subs"), mv.Stats.HomeSubs, mv.Stats.AwaySubs)
	if mix := m.chanceMixLabel(mv.Stats.ChanceTypes, 2); mix != "" {
		b.WriteString(fmt.Sprintf("%s %s\n", fitLine(m.ui("ui.match.stat.chance_mix"), 12, alignLeft), mix))
	}
	for _, line := range m.diagnosticLines(mv.Stats.Diagnostics, paneWidth, 4) {
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (m Model) diagnosticLines(d MatchDiagnostics, width, limit int) []string {
	lines := make([]string, 0, limit)
	add := func(label, value string) {
		if value == "" || len(lines) >= limit {
			return
		}
		lines = append(lines, truncate(fmt.Sprintf("%s %s", label, value), width))
	}
	add(m.ui("ui.match.stat.quality"), m.qualityLabel(d.ShotQuality, 3))
	add(m.ui("ui.match.stat.aerial"), m.aerialLabel(d.AerialDuels, d.AerialWins))
	add(m.ui("ui.match.stat.press"), m.sideLabel(d.PressTurnovers))
	add(m.ui("ui.match.stat.setpieces"), m.sideLabel(d.SetPieceThreat))
	return lines
}

func (m Model) qualityLabel(counts map[string]int, limit int) string {
	order := []string{"HIGH", "MEDIUM", "LOW"}
	parts := make([]string, 0, len(order))
	for _, band := range order {
		if n := counts[band]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", m.qualityBandLabel(band), n))
		}
	}
	if len(parts) > limit {
		parts = parts[:limit]
	}
	return strings.Join(parts, " · ")
}

func (m Model) qualityBandLabel(key string) string {
	label := m.ui("term.quality." + key)
	if label != "term.quality."+key {
		return label
	}
	return strings.ToLower(key)
}

func (m Model) aerialLabel(duels, wins map[string]int) string {
	homeDuels, awayDuels := duels["HOME"], duels["AWAY"]
	if homeDuels == 0 && awayDuels == 0 {
		return ""
	}
	return fmt.Sprintf("H %d/%d · A %d/%d", wins["HOME"], homeDuels, wins["AWAY"], awayDuels)
}

func (m Model) sideLabel(counts map[string]int) string {
	home, away := counts["HOME"], counts["AWAY"]
	if home == 0 && away == 0 {
		return ""
	}
	return fmt.Sprintf("H %d · A %d", home, away)
}

func (m Model) chanceMixLabel(types map[string]int, limit int) string {
	type pair struct {
		key string
		val int
	}
	pairs := make([]pair, 0, len(types))
	for k, v := range types {
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

func (m Model) chanceTypeLabel(key string) string {
	label := m.ui("term.chance_type." + key)
	if label != "term.chance_type."+key {
		return label
	}
	return strings.ToLower(strings.ReplaceAll(key, "_", " "))
}

// paneRatings renders the live ratings rows (both sides, server-sorted),
// trimmed to the available height.
func (m Model) paneRatings(mv LiveMatchView, maxRows int) string {
	var b strings.Builder
	b.WriteString(styleHeader.Render(m.ui("ui.match.ratings")) + "\n")
	side := ""
	written := 1
	for _, r := range mv.Ratings {
		if written >= maxRows {
			break
		}
		if r.Side != side {
			side = r.Side
			name := mv.Home
			if side == "AWAY" {
				name = mv.Away
			}
			b.WriteString(styleDim.Render(truncate(name, paneWidth-2)) + "\n")
			written++
		}
		b.WriteString(fmt.Sprintf("%d.%d %s\n", r.RatingX10/10, r.RatingX10%10, truncate(r.Name, paneWidth-5)))
		written++
	}
	return b.String()
}

// paneTicker renders the other grounds' latest scores.
func (m Model) paneTicker(maxRows int) string {
	var b strings.Builder
	b.WriteString(styleHeader.Render(m.ui("ui.match.latest")) + "\n")
	written := 1
	for i, other := range m.Matches {
		if i == m.MatchIdx {
			continue
		}
		if written >= maxRows {
			break
		}
		b.WriteString(fmt.Sprintf("%s %d-%d %s %d'\n",
			truncate(other.Home, 8), other.HomeGoals, other.AwayGoals,
			truncate(other.Away, 8), other.Minute))
		written++
	}
	if written == 1 {
		b.WriteString(styleDim.Render(truncate(m.ui("ui.match.none"), paneWidth-1)) + "\n")
	}
	return b.String()
}

// Pitch band geometry (docs/07 §4.1 tier matrix).
const (
	pitchRowsStrip = 5  // M: compact strip
	pitchRowsFull  = 9  // L: full band
	pitchRowsTall  = 11 // XL: expanded band
	pitchMaxWidth  = 100
)

// renderPitch draws the abstract CM-style field: border, halfway line, centre
// spot, goalmouths — with the recent event markers overlaid. Home attacks
// RIGHT: a home goal lands in the right goalmouth, an away goal in the left;
// midfield events sit on their side of the halfway line. Markers stagger down
// the rows oldest→newest so the latest event reads at the bottom. Deliberately
// zones-and-glyphs, not ball physics — the engine samples key moments.
func renderPitch(width, rows int, markers []LiveMarker) string {
	if width < 24 {
		width = 24
	}
	grid := make([][]rune, rows)
	for r := range grid {
		line := make([]rune, width)
		for c := range line {
			line[c] = ' '
		}
		grid[r] = line
	}
	for c := 0; c < width; c++ {
		grid[0][c], grid[rows-1][c] = '-', '-'
	}
	for r := 0; r < rows; r++ {
		grid[r][0], grid[r][width-1] = '|', '|'
	}
	grid[0][0], grid[0][width-1] = '+', '+'
	grid[rows-1][0], grid[rows-1][width-1] = '+', '+'
	mid := width / 2
	for r := 1; r < rows-1; r++ {
		grid[r][mid] = ':'
	}
	gy := rows / 2
	grid[gy][mid] = 'o' // centre spot
	grid[gy][1], grid[gy][width-2] = '[', ']'

	show := markers
	if len(show) > rows-2 {
		show = show[len(show)-(rows-2):]
	}
	for i, mk := range show {
		col := mid
		switch mk.Side {
		case "HOME": // attacking right
			col = width * 3 / 4
		case "AWAY": // attacking left
			col = width / 4
		}
		if mk.Kind == "GOAL" {
			switch mk.Side {
			case "HOME":
				col = width - 3
			case "AWAY":
				col = 2
			}
		}
		row := 1 + i%(rows-2)
		grid[row][col] = markerGlyph(mk.Kind)
	}

	var b strings.Builder
	for _, line := range grid {
		b.WriteString(string(line))
		b.WriteString("\n")
	}
	return b.String()
}

func markerGlyph(kind string) rune {
	switch kind {
	case "GOAL":
		return 'G'
	case "CHANCE":
		return 'o'
	case "CARD":
		return 'x'
	case "INJURY":
		return '+'
	case "SUB":
		return 's'
	case "SHOOTOUT":
		return '!'
	}
	return '.'
}

func (m Model) renderScoreboard(width int, mv LiveMatchView) string {
	inner := width
	if inner > 118 {
		inner = 118
	}
	score := joinBigSegments(bigNumber(mv.HomeGoals), bigHyphen(), bigNumber(mv.AwayGoals))
	top := fitLine("┌"+strings.Repeat("─", inner-2)+"┐", width, alignCenter)
	bottom := fitLine("└"+strings.Repeat("─", inner-2)+"┘", width, alignCenter)
	home := truncate(mv.Home, 20)
	away := truncate(mv.Away, 20)
	title := fitLine(home+"  "+m.uiMissingSafe("ui.match.scoreboard")+"  "+away, inner-2, alignCenter)
	lines := []string{top, fitLine("│"+title+"│", width, alignCenter)}
	for _, row := range score {
		lines = append(lines, fitLine("│"+fitLine(row, inner-2, alignCenter)+"│", width, alignCenter))
	}
	lines = append(lines, bottom)
	return strings.Join(lines, "\n") + "\n"
}

func (m Model) uiMissingSafe(key string) string {
	v := m.ui(key)
	if v == key {
		return ""
	}
	return v
}

func (m Model) renderGoalFlash(width int, mv LiveMatchView) string {
	goal := latestGoal(mv.Markers)
	if goal == nil {
		return ""
	}
	side := mv.Home
	if goal.Side == "AWAY" {
		side = mv.Away
	}
	msg := fmt.Sprintf(">>>  %s  %d'  %s  <<<", strings.ToUpper(m.ui("ui.match.goalflash")), goal.Minute, side)
	inner := minInt(width, 96)
	if inner < 24 {
		inner = width
	}
	burst := []string{
		strings.Repeat("▓", inner),
		"▓" + fitLine("✦ ✦ ✦", inner-2, alignCenter) + "▓",
		"▓" + fitLine(msg, inner-2, alignCenter) + "▓",
		"▓" + fitLine(strings.ToUpper(m.ui("ui.match.scoreboard")+" "+m.ui("ui.match.goalflash")), inner-2, alignCenter) + "▓",
		strings.Repeat("▓", inner),
	}
	for i := range burst {
		burst[i] = styleHeader.Render(fitLine(burst[i], width, alignCenter))
	}
	return strings.Join(burst, "\n") + "\n"
}

func latestGoal(markers []LiveMarker) *LiveMarker {
	if len(markers) == 0 {
		return nil
	}
	last := &markers[len(markers)-1]
	if last.Kind != "GOAL" {
		return nil
	}
	return last
}

func bigNumber(n int) []string {
	if n < 0 {
		n = 0
	}
	out := []string{"", "", "", "", ""}
	for i, r := range fmt.Sprint(n) {
		g := bigDigit(r)
		for row := range out {
			if i > 0 {
				out[row] += " "
			}
			out[row] += g[row]
		}
	}
	return out
}

func joinBigSegments(parts ...[]string) []string {
	out := []string{"", "", "", "", ""}
	for i, part := range parts {
		for row := range out {
			if i > 0 {
				out[row] += "   "
			}
			out[row] += part[row]
		}
	}
	return out
}

func bigHyphen() []string {
	return []string{"     ", "     ", "█████", "     ", "     "}
}

func bigDigit(r rune) []string {
	switch r {
	case '0':
		return []string{"█████", "█   █", "█   █", "█   █", "█████"}
	case '1':
		return []string{"  █  ", " ██  ", "  █  ", "  █  ", "█████"}
	case '2':
		return []string{"█████", "    █", "█████", "█    ", "█████"}
	case '3':
		return []string{"█████", "    █", " ████", "    █", "█████"}
	case '4':
		return []string{"█   █", "█   █", "█████", "    █", "    █"}
	case '5':
		return []string{"█████", "█    ", "█████", "    █", "█████"}
	case '6':
		return []string{"█████", "█    ", "█████", "█   █", "█████"}
	case '7':
		return []string{"█████", "    █", "   █ ", "  █  ", "  █  "}
	case '8':
		return []string{"█████", "█   █", "█████", "█   █", "█████"}
	case '9':
		return []string{"█████", "█   █", "█████", "    █", "█████"}
	}
	return []string{"     ", "     ", "     ", "     ", "     "}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
