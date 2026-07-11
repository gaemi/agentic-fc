package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Match scenes are composed on a fixed-size cell canvas instead of hand-drawn
// multi-line strings. Every frame is exactly sceneCanvasWidth by
// sceneCanvasHeight cells of single-width runes, so scene art cannot render
// ragged no matter how future scenes are authored or animated.
const (
	sceneCanvasWidth  = 48
	sceneCanvasHeight = 7
)

type sceneCanvas struct {
	cells [sceneCanvasHeight][sceneCanvasWidth]rune
}

func newSceneCanvas() *sceneCanvas {
	c := &sceneCanvas{}
	for y := 0; y < sceneCanvasHeight; y++ {
		for x := 0; x < sceneCanvasWidth; x++ {
			c.cells[y][x] = ' '
		}
	}
	return c
}

// put writes a single cell. Writes outside the canvas are clipped and runes
// that do not occupy exactly one terminal cell are dropped, which keeps every
// composed frame rectangular by construction.
func (c *sceneCanvas) put(x, y int, r rune) {
	if x < 0 || x >= sceneCanvasWidth || y < 0 || y >= sceneCanvasHeight {
		return
	}
	if lipgloss.Width(string(r)) != 1 {
		return
	}
	c.cells[y][x] = r
}

// stamp draws a multi-line sprite treating spaces as transparent, so sprites
// can overlap furniture (nets, crowds) without punching holes in it.
func (c *sceneCanvas) stamp(x, y int, sprite ...string) {
	for dy, row := range sprite {
		dx := 0
		for _, r := range row {
			if r != ' ' {
				c.put(x+dx, y+dy, r)
			}
			dx++
		}
	}
}

// label writes text opaquely: spaces overwrite whatever is beneath them.
func (c *sceneCanvas) label(x, y int, text string) {
	dx := 0
	for _, r := range text {
		c.put(x+dx, y, r)
		dx++
	}
}

func (c *sceneCanvas) ground() {
	for x := 0; x < sceneCanvasWidth; x++ {
		c.put(x, sceneCanvasHeight-1, '_')
	}
}

func (c *sceneCanvas) lines() []string {
	out := make([]string, sceneCanvasHeight)
	for y := range c.cells {
		out[y] = string(c.cells[y][:])
	}
	return out
}

// matchScene is a fully composed, fixed-size animation. Frames are only ever
// produced by composeScene, so every frame shares the canvas dimensions.
type matchScene struct {
	kind   string
	title  string
	frames [][]string
}

func sceneFrame(m Model, scene matchScene, width, height int) []string {
	return sceneFrameAt(m, scene, width, height, 0)
}

func sceneFrameAt(m Model, scene matchScene, width, height, frame int) []string {
	if width < 28 || height < 5 {
		return nil
	}
	art := sceneArt(scene, frame)
	content := width - 2
	if content < sceneCanvasWidth {
		return nil
	}
	if artHeight := len(art) + 2; artHeight < height {
		height = artHeight
	}
	title := sceneLabel(m, scene)
	out := []string{preformattedLinePrefix + "╭" + fitLine(" "+title+" ", content, alignCenter) + "╮"}
	for i := 0; i < height-2; i++ {
		line := ""
		if i < len(art) {
			line = art[i]
		}
		// Canvas lines share one exact width, so centering the block keeps
		// every glyph on the scene's shared coordinate grid across frames.
		out = append(out, preformattedLinePrefix+"│"+fitLine(line, content, alignCenter)+"│")
	}
	out = append(out, preformattedLinePrefix+"╰"+strings.Repeat("─", content)+"╯")
	return out
}

func sceneArt(scene matchScene, frame int) []string {
	if len(scene.frames) == 0 {
		return nil
	}
	if frame < 0 {
		frame = 0
	}
	return scene.frames[frame%len(scene.frames)]
}

func sceneLabel(m Model, scene matchScene) string {
	title := m.ui("ui.match.scene." + scene.kind)
	if strings.HasPrefix(title, "ui.match.scene.") {
		return scene.title
	}
	return title
}

// Shared sprites. Standing figures are three rows tall; feet belong on row 5
// so they stand on the ground line drawn on the bottom row.
var (
	sprPlayer = []string{" o ", "/|\\", "/ \\"}
	sprRunner = []string{" o ", "/|\\", "/ >"}
	sprCheerA = []string{"\\o/", " | ", "/ \\"}
	sprCheerB = []string{" o ", "\\|/", "/ \\"}
	sprJumper = []string{"\\o/", " | ", "/ \\"}
	sprKeeper = []string{" o ", "/|\\", "/ \\"}
	// Open goal mouth: crossbar on top, posts to the ground, light net
	// hatching at the back so figures and balls stay readable inside it.
	sprGoalMouth = []string{
		" _________ ",
		"|      : :|",
		"|      : :|",
		"|      : :|",
	}
)

const (
	goalMouthX     = 37 // left post column of the right-hand goal
	keeperLineX    = 39 // keeper standing on the goal line inside the mouth
	figureRow      = 3  // head row for a standing figure whose feet touch ground
	ballRow        = 4  // torso-height row used for driven balls
	groundBallRow  = 5  // feet-height row used for rolled balls
	sceneBannerRow = 0
)

// pitchWithGoal returns a canvas holding the shared right-hand goal furniture
// with the keeper set on his line. Scenes that move the keeper pass
// keeperUp=false and stamp their own keeper pose.
func pitchWithGoal(keeperUp bool) *sceneCanvas {
	c := newSceneCanvas()
	c.ground()
	c.stamp(goalMouthX, 2, sprGoalMouth...)
	if keeperUp {
		c.stamp(keeperLineX, figureRow, sprKeeper...)
	}
	return c
}

func composeScene(kind, title string, frames ...*sceneCanvas) matchScene {
	if len(frames) == 0 {
		panic("composed match scene requires at least one frame")
	}
	rendered := make([][]string, len(frames))
	for i, frame := range frames {
		rendered[i] = frame.lines()
	}
	return matchScene{kind: kind, title: title, frames: rendered}
}

var matchScenes = buildMatchScenes()

func buildMatchScenes() map[string]matchScene {
	scenes := []matchScene{
		goalScene(), saveScene(), crossScene(), cutbackScene(),
		throughScene(), longshotScene(), setpieceScene(), counterScene(),
		scrambleScene(), dribbleScene(), cardScene(), injuryScene(),
		subScene(), chanceScene(), buildUpScene(),
	}
	out := make(map[string]matchScene, len(scenes))
	for _, s := range scenes {
		out[s.kind] = s
	}
	return out
}

func sceneByKind(kind string) matchScene {
	if s, ok := matchScenes[kind]; ok {
		return s
	}
	return matchScenes["build"]
}

func goalScene() matchScene {
	strike := pitchWithGoal(true)
	strike.stamp(24, figureRow, sprPlayer...)
	strike.stamp(7, figureRow, sprRunner...)
	strike.put(11, groundBallRow, '*')

	flight := pitchWithGoal(true)
	flight.stamp(24, figureRow, sprPlayer...)
	flight.stamp(7, figureRow, sprPlayer...)
	flight.label(16, ballRow, "---*")

	dive := pitchWithGoal(false)
	dive.stamp(24, figureRow, sprPlayer...)
	dive.stamp(7, figureRow, sprPlayer...)
	dive.stamp(39, figureRow, "__o", "  \\")
	dive.label(27, ballRow, "---*")

	inNet := pitchWithGoal(false)
	inNet.stamp(24, figureRow, sprPlayer...)
	inNet.stamp(7, figureRow, sprCheerA...)
	inNet.stamp(39, groundBallRow, "_o_")
	inNet.put(44, ballRow, '*')
	inNet.put(45, ballRow, ')')

	cheerA := pitchWithGoal(false)
	cheerA.label(19, sceneBannerRow, "G O A L !")
	cheerA.stamp(7, figureRow, sprCheerA...)
	cheerA.stamp(14, figureRow, sprCheerA...)
	cheerA.stamp(24, figureRow, sprPlayer...)
	cheerA.stamp(39, groundBallRow, "_o_")
	cheerA.put(44, groundBallRow, '*')

	cheerB := pitchWithGoal(false)
	cheerB.label(19, sceneBannerRow, "G O A L !")
	cheerB.label(13, 1, ". : .")
	cheerB.label(29, 1, ". : .")
	cheerB.stamp(7, figureRow, sprCheerB...)
	cheerB.stamp(14, figureRow, sprCheerB...)
	cheerB.stamp(24, figureRow, sprPlayer...)
	cheerB.stamp(39, groundBallRow, "_o_")
	cheerB.put(44, groundBallRow, '*')

	return composeScene("goal", "GOAL SCENE", strike, flight, dive, inNet, cheerA, cheerB)
}

func saveScene() matchScene {
	strike := pitchWithGoal(true)
	strike.stamp(6, figureRow, sprRunner...)
	strike.stamp(20, figureRow, sprPlayer...)
	strike.put(10, groundBallRow, '*')

	flight := pitchWithGoal(true)
	flight.stamp(6, figureRow, sprPlayer...)
	flight.stamp(20, figureRow, sprPlayer...)
	flight.label(15, ballRow, "----*")

	reach := pitchWithGoal(false)
	reach.stamp(6, figureRow, sprPlayer...)
	reach.stamp(20, figureRow, sprPlayer...)
	reach.stamp(34, 2, "\\o/", " | ", "/ \\")
	reach.label(25, ballRow, "----")
	reach.put(33, 3, '*')

	parry := pitchWithGoal(false)
	parry.label(20, sceneBannerRow, "SAVE!")
	parry.stamp(6, figureRow, sprPlayer...)
	parry.stamp(20, figureRow, sprPlayer...)
	parry.stamp(32, groundBallRow, "_o/")
	parry.put(30, 2, '*')
	parry.put(32, 3, '/')

	clear := pitchWithGoal(false)
	clear.label(20, sceneBannerRow, "SAVE!")
	clear.stamp(6, figureRow, sprPlayer...)
	clear.stamp(20, figureRow, sprRunner...)
	clear.stamp(32, groundBallRow, "_o~")
	clear.put(26, 1, '*')
	clear.put(28, 2, '/')

	return composeScene("save", "KEEPER'S SAVE", strike, flight, reach, parry, clear)
}

func crossScene() matchScene {
	base := func() *sceneCanvas {
		c := pitchWithGoal(true)
		c.stamp(27, figureRow, sprPlayer...)
		return c
	}

	wind := base()
	wind.stamp(3, figureRow, sprRunner...)
	wind.stamp(31, figureRow, sprPlayer...)
	wind.put(7, groundBallRow, '*')

	lift := base()
	lift.stamp(3, figureRow, sprPlayer...)
	lift.stamp(31, figureRow, sprPlayer...)
	lift.label(9, 2, "~~*")
	lift.put(8, 3, '/')

	hang := base()
	hang.stamp(3, figureRow, sprPlayer...)
	hang.stamp(31, figureRow, sprPlayer...)
	hang.label(15, 1, "~~~~~*")

	drop := base()
	drop.stamp(3, figureRow, sprPlayer...)
	drop.stamp(31, figureRow, sprPlayer...)
	drop.label(24, 1, "~~~")
	drop.put(28, 2, '*')

	header := base()
	header.stamp(3, figureRow, sprPlayer...)
	header.stamp(30, 2, sprJumper...)
	header.put(34, 1, '*')
	header.put(28, 1, '!')

	return composeScene("cross", "WIDE DELIVERY", wind, lift, hang, drop, header)
}

func cutbackScene() matchScene {
	base := func() *sceneCanvas {
		c := pitchWithGoal(true)
		c.stamp(33, figureRow, sprPlayer...)
		return c
	}

	reach := base()
	reach.stamp(16, figureRow, sprRunner...)
	reach.put(32, groundBallRow, '*')

	rollback := base()
	rollback.stamp(20, figureRow, sprRunner...)
	rollback.label(27, groundBallRow, "....*")

	arrive := base()
	arrive.stamp(23, figureRow, sprRunner...)
	arrive.label(27, groundBallRow, "*..")

	strike := base()
	strike.stamp(24, figureRow, sprPlayer...)
	strike.label(29, ballRow, "--*")
	strike.put(28, groundBallRow, '\'')

	return composeScene("cutback", "CUT-BACK", reach, rollback, arrive, strike)
}

func throughScene() matchScene {
	base := func(keeperUp bool) *sceneCanvas {
		c := pitchWithGoal(keeperUp)
		c.stamp(20, figureRow, sprPlayer...)
		c.stamp(26, figureRow, sprPlayer...)
		c.stamp(6, figureRow, sprPlayer...)
		return c
	}

	thread := base(true)
	thread.stamp(13, figureRow, sprRunner...)
	thread.put(10, groundBallRow, '*')

	slip := base(true)
	slip.stamp(16, figureRow, sprRunner...)
	slip.put(24, ballRow, '*')

	burst := base(true)
	burst.stamp(23, figureRow, sprRunner...)
	burst.put(30, groundBallRow, '*')

	clear := base(false)
	clear.stamp(34, figureRow, sprKeeper...)
	clear.stamp(29, figureRow, sprRunner...)
	clear.put(33, groundBallRow, '*')
	clear.label(25, 2, "..")

	return composeScene("through", "THROUGH BALL", thread, slip, burst, clear)
}

func longshotScene() matchScene {
	base := func() *sceneCanvas {
		c := pitchWithGoal(true)
		c.stamp(24, figureRow, sprPlayer...)
		c.stamp(29, figureRow, sprPlayer...)
		return c
	}

	setup := base()
	setup.stamp(13, figureRow, sprRunner...)
	setup.put(17, groundBallRow, '*')

	launch := base()
	launch.stamp(13, figureRow, sprPlayer...)
	launch.put(20, 3, '*')
	launch.put(18, 4, '/')

	soar := base()
	soar.stamp(13, figureRow, sprPlayer...)
	soar.label(24, 1, "~~~*")

	stretch := pitchWithGoal(false)
	stretch.stamp(24, figureRow, sprPlayer...)
	stretch.stamp(29, figureRow, sprPlayer...)
	stretch.stamp(13, figureRow, sprPlayer...)
	stretch.stamp(38, 2, sprJumper...)
	stretch.label(32, 1, "~~~")
	stretch.put(36, 2, '*')

	return composeScene("longshot", "FROM RANGE", setup, launch, soar, stretch)
}

func setpieceScene() matchScene {
	base := func() *sceneCanvas {
		c := pitchWithGoal(true)
		c.stamp(18, figureRow, sprPlayer...)
		c.stamp(21, figureRow, sprPlayer...)
		c.stamp(24, figureRow, sprPlayer...)
		c.stamp(30, figureRow, sprPlayer...)
		c.stamp(33, figureRow, sprPlayer...)
		return c
	}

	spot := base()
	spot.stamp(5, figureRow, sprRunner...)
	spot.put(9, groundBallRow, '*')

	strike := base()
	strike.stamp(5, figureRow, sprPlayer...)
	strike.put(14, 2, '*')
	strike.put(12, 3, '/')

	overWall := base()
	overWall.stamp(5, figureRow, sprPlayer...)
	overWall.label(17, 1, "~~~*")

	dropZone := base()
	dropZone.stamp(5, figureRow, sprPlayer...)
	dropZone.label(24, 1, "~~")
	dropZone.put(27, 2, '*')

	melee := base()
	melee.stamp(5, figureRow, sprPlayer...)
	melee.stamp(30, 2, sprJumper...)
	melee.put(33, 1, '*')
	melee.put(35, 2, '!')

	return composeScene("setpiece", "SET PIECE", spot, strike, overWall, dropZone, melee)
}

func counterScene() matchScene {
	frame := func(step int) *sceneCanvas {
		c := newSceneCanvas()
		c.ground()
		shift := step * 3
		lead := 24 + shift
		c.stamp(4+shift, figureRow, sprRunner...)
		c.stamp(14+shift, figureRow, sprRunner...)
		c.stamp(lead, figureRow, sprRunner...)
		c.put(lead+4, ballRow, '*')
		c.label(4+shift-3, ballRow, "..")
		c.label(14+shift-3, ballRow, "..")
		c.label(lead-3, ballRow, "..")
		c.stamp(38, figureRow, sprPlayer...)
		c.stamp(43, figureRow, sprPlayer...)
		return c
	}
	return composeScene("counter", "COUNTER ATTACK", frame(0), frame(1), frame(2), frame(3))
}

func scrambleScene() matchScene {
	base := func() *sceneCanvas {
		c := pitchWithGoal(true)
		c.stamp(25, figureRow, sprPlayer...)
		c.stamp(29, figureRow, sprPlayer...)
		c.stamp(33, figureRow, sprPlayer...)
		c.stamp(20, groundBallRow, "_o~")
		return c
	}

	first := base()
	first.put(28, ballRow, '*')
	first.put(30, 1, '!')

	second := base()
	second.put(32, 2, '*')
	second.put(27, 1, '?')

	third := base()
	third.put(26, 2, '*')
	third.put(32, 1, '!')

	fourth := base()
	fourth.put(36, ballRow, '*')
	fourth.label(28, 1, "!?")

	return composeScene("scramble", "SIX-YARD SCRAMBLE", first, second, third, fourth)
}

func dribbleScene() matchScene {
	base := func() *sceneCanvas {
		c := newSceneCanvas()
		c.ground()
		c.stamp(38, figureRow, sprPlayer...)
		return c
	}

	approach := base()
	approach.stamp(20, figureRow, sprPlayer...)
	approach.stamp(9, figureRow, sprRunner...)
	approach.put(13, ballRow, '*')

	feint := base()
	feint.stamp(20, figureRow, " o ", "/|\\", "/--")
	feint.stamp(15, figureRow, sprRunner...)
	feint.put(19, 5, '*')

	past := base()
	past.stamp(20, figureRow, sprPlayer...)
	past.put(21, 2, '?')
	past.stamp(24, figureRow, sprRunner...)
	past.put(28, ballRow, '*')
	past.label(21, ballRow, "..")

	away := base()
	away.stamp(20, figureRow, sprPlayer...)
	away.stamp(31, figureRow, sprRunner...)
	away.put(35, ballRow, '*')
	away.label(27, ballRow, "...")

	return composeScene("dribble", "DRIBBLE", approach, feint, past, away)
}

func cardScene() matchScene {
	base := func() *sceneCanvas {
		c := newSceneCanvas()
		c.ground()
		c.stamp(16, figureRow, sprPlayer...)
		c.stamp(30, figureRow, sprPlayer...)
		return c
	}

	raise := base()
	raise.stamp(23, figureRow, sprPlayer...)
	raise.label(26, 2, "[]")

	shown := base()
	shown.stamp(23, figureRow, " o/", "/| ", "/ \\")
	shown.label(26, 1, "[]")
	shown.put(18, 2, '!')

	protest := base()
	protest.stamp(23, figureRow, " o/", "/| ", "/ \\")
	protest.label(26, 1, "[]")
	protest.label(17, 1, "?!")

	return composeScene("card", "REFEREE'S BOOK", raise, shown, protest)
}

func injuryScene() matchScene {
	base := func() *sceneCanvas {
		c := newSceneCanvas()
		c.ground()
		c.stamp(8, figureRow, sprPlayer...)
		c.stamp(41, figureRow, sprPlayer...)
		c.stamp(24, 5, "~o~")
		c.stamp(19, 4, "o ", "|\\")
		return c
	}

	down := base()
	down.put(28, 1, '+')

	treat := base()
	treat.label(27, 1, "(+)")
	treat.put(25, 3, '!')

	carry := base()
	carry.put(28, 1, '+')
	carry.stamp(31, 4, "[==]")
	carry.stamp(36, figureRow, sprRunner...)

	return composeScene("injury", "STOPPAGE", down, treat, carry)
}

func subScene() matchScene {
	frame := func(step int) *sceneCanvas {
		c := newSceneCanvas()
		c.ground()
		c.label(8, 1, "OFF")
		c.label(37, 1, "ON")
		c.stamp(21, 1, " ___ ", "|< >|")
		c.stamp(8-step*2, figureRow, sprPlayer...)
		c.label(13-step*2, ballRow, "<-")
		c.stamp(36-step*2, figureRow, sprRunner...)
		c.label(41-step*2, ballRow, "<-")
		return c
	}
	return composeScene("sub", "TECHNICAL AREA", frame(0), frame(1), frame(2))
}

func chanceScene() matchScene {
	carry := pitchWithGoal(true)
	carry.stamp(8, figureRow, sprRunner...)
	carry.stamp(22, figureRow, sprPlayer...)
	carry.put(12, groundBallRow, '*')

	strike := pitchWithGoal(true)
	strike.stamp(14, figureRow, sprPlayer...)
	strike.stamp(22, figureRow, sprPlayer...)
	strike.label(18, ballRow, "-*")

	flight := pitchWithGoal(true)
	flight.stamp(14, figureRow, sprPlayer...)
	flight.stamp(22, figureRow, sprPlayer...)
	flight.label(25, 2, "---*")

	wide := pitchWithGoal(true)
	wide.stamp(14, figureRow, sprPlayer...)
	wide.stamp(22, figureRow, sprPlayer...)
	wide.label(31, 1, "---")
	wide.put(35, 0, '*')

	return composeScene("chance", "CHANCE", carry, strike, flight, wide)
}

func buildUpScene() matchScene {
	frame := func(ballAt int) *sceneCanvas {
		c := newSceneCanvas()
		c.ground()
		for _, y := range []int{0, 2, 4} {
			c.put(24, y, '.')
		}
		c.stamp(8, figureRow, sprPlayer...)
		c.stamp(19, figureRow, sprPlayer...)
		c.stamp(33, figureRow, sprPlayer...)
		positions := []int{12, 24, 37}
		c.put(positions[ballAt], groundBallRow, '*')
		return c
	}
	return composeScene("build", "BUILD-UP", frame(0), frame(1), frame(2))
}

func matchSceneFromLive(mv LiveMatchView, line string) matchScene {
	if line != "" {
		// Prefer the prose shape over the marker so goal cut-backs, counters, and crosses
		// can show the specific action frame instead of a generic goal frame.
		return matchSceneFromLine(line, nil)
	}
	if len(mv.Markers) == 0 {
		return matchSceneFromLine("", nil)
	}
	return matchSceneFromLine("", &mv.Markers[len(mv.Markers)-1])
}

// Every goal template quotes the new score with an en-dash; the only other
// commentary carrying one is the interval/final/shootout bookkeeping.
var commentScorePattern = regexp.MustCompile(`\d+–\d+`)

// goalProse reports whether a commentary line announces a scored goal.
// Both the scene classifier and the modal goal banners rely on it, so a
// patterned goal that plays its action scene still carries a goal signal.
func goalProse(line string) bool {
	if line == "" {
		return false
	}
	lower := strings.ToLower(line)
	// Interval, final-whistle, and shootout bookkeeping quotes scores and may
	// mention goals without one being scored on this beat.
	if containsAny(lower, "half time", "full time", "after 90", "penalties",
		"전반 종료", "경기 종료", "90분 종료", "승부차기") {
		return false
	}
	if commentScorePattern.MatchString(line) {
		return true
	}
	return containsAny(lower,
		"goal!", "scores", "scored", "finds the net", "it's in",
		"strikes!", "lashes it home", "ruthless finish", "buries it", "smashes",
		"finish, noise", "rolls it home", "bundles it",
		"득점", "골!", "골망", "들어갔", "꽂아 넣", "냉정한 마무리", "첫 터치, 슛",
		"밀어 넣", "굴려 넣")
}

func matchSceneFromLine(line string, marker *LiveMarker) matchScene {
	kind := ""
	if marker != nil {
		kind = strings.ToUpper(marker.Kind)
	}
	lower := strings.ToLower(line)
	switch {
	case kind == "GOAL" || goalProse(line):
		// A scored move keeps its action scene — the goal banner rendered by
		// the live and replay modals announces the goal — so cut-back,
		// counter, and cross goals play their specific frames. Prose without
		// an action shape gets the generic goal celebration.
		if action := actionSceneKind(lower); action != "" {
			return sceneByKind(action)
		}
		return sceneByKind("goal")
	case kind == "CARD" || containsAny(lower, "booked", "red card", "yellow", "경고", "퇴장", "카드"):
		return sceneByKind("card")
	case kind == "INJURY" || containsAny(lower, "injury", "treatment", "is down", "stays down", "goes down", "쓰러", "치료", "부상"):
		return sceneByKind("injury")
	case kind == "SUB" || containsAny(lower, "replaces", "fresh legs", "make a change", "comes on", "교체", "투입"):
		return sceneByKind("sub")
	case kind == "CHANCE":
		return sceneByKind("chance")
	case containsAny(lower,
		"fine stop", "strong hand", "gets down sharply", "clawed out", "palm it away",
		"tip it away", "makes the stop", "strong save", "somehow blocks", "keeper stays big",
		"throws up a strong hand", "goalkeeper reacts", "goalkeeper gets down", "keeper claws through bodies to save",
		"선방", "강한 손", "쳐냅니다", "쳐냅", "낮게 몸을 던져", "빠르게 반응해", "걷어 올려집니다",
		"골키퍼가 크게 버티며", "손끝으로 밀어냅니다", "몸들 사이로 걷어냅니다",
		"골키퍼가 어떻게든", "골키퍼가 읽고 나와"):
		return sceneByKind("save")
	default:
		if action := actionSceneKind(lower); action != "" {
			return sceneByKind(action)
		}
		if containsAny(lower, "shoot", "shot", "chance", "effort", "finish", "fizz wide", "skids wide", "dragged wide", "슛", "슈팅", "기회", "마무리") {
			return sceneByKind("chance")
		}
		return sceneByKind("build")
	}
}

// actionSceneKind classifies attacking-move prose (crosses, cut-backs,
// through balls, long shots, set pieces, counters, scrambles, dribbles).
// It returns "" when the line has no recognizable action shape.
func actionSceneKind(lower string) string {
	switch {
	// Set pieces outrank crosses: a dead-ball delivery headed home should
	// play the set-piece frame even though the prose mentions the header.
	case containsAny(lower, "set piece", "set-piece", "dead ball", "dead-ball", "corner", "free kick", "세트피스", "데드볼", "코너", "프리킥"):
		return "setpiece"
	case containsAny(lower, "wide channel", "far post", "far-post", "delivery hangs", "rises above", "a header", "the header", "header loops", "powers the header", "teasing cross", "swing it in", "glancing it", "크로스", "측면", "먼 포스트", "헤더") || containsWordAny(lower, "cross", "crosses", "crossing", "crossed"):
		return "cross"
	case containsAny(lower, "cut-back", "cutback", "pull-back", "byline", "컷백", "골라인", "뒤로 내줍"):
		return "cutback"
	case containsAny(lower, "through ball", "threaded pass", "split the defence", "clean through", "slice through", "스루패스", "수비 라인", "일대일", "침투 각도", "단숨에 찢"):
		return "through"
	case containsAny(lower, "from range", "from distance", "from long distance", "long-distance", "distance strike", "long-range", "long shot", "lets fly", "thunderous", "at range", "strike whistles", "crowd urges", "중거리", "먼 거리", "거리에서"):
		return "longshot"
	case containsAny(lower, "counter", "on the break", "the break is", "burst forward", "races clear", "grass ahead", "역습", "넓은 공간"):
		return "counter"
	case containsAny(lower, "scramble", "ricochet", "loose ball", "six-yard", "chaos", "nobody clears", "keep the chance alive", "혼전", "튕", "흐른 공", "걷어내지 못한", "기회를 살려"):
		return "scramble"
	case containsAny(lower, "dribble", "darts between", "holds off", "파고", "돌파", "제쳐", "버텨"):
		return "dribble"
	default:
		return ""
	}
}
