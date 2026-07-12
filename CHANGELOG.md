# Changelog

This project follows a simple human-readable changelog. The current release
version is tracked in `VERSION`; each release must have a matching dated
section below.

## Unreleased

- Replay pop-ups now animate their match scenes: browsing a finished match
  plays each beat's terminal animation (goals, saves, counters, ceremonies)
  exactly like the live broadcast instead of freezing on the first frame,
  restarting cleanly when you step between beats, with the same `Space`
  pause/resume and no extra polling while the pop-up is closed.
- Match commentary for bookings, injuries, and the kickoff whistle no longer
  repeats a single line: yellow cards rotate six localized voices, straight
  reds four, second yellows up to three, and the opening whistle six — all
  chosen from public match state with no extra RNG, so existing seeds replay
  identical football. Injury calls pick their voice from the severity band
  (mild knocks never arrive on a stretcher), and the "quick succession"
  second yellow only speaks when the bookings really were close together.

## 0.2.0 - 2026-07-12

- Agentic FC now installs with Homebrew: `brew install gaemi/tap/agentic-fc`
  puts the daemon, the spectator console, and the calibration CLI on the
  `PATH`, and README/docs/13 gained install and quick-start guidance for the
  packaged binaries.
- Connecting an AI agent is now one command: the new `agenticfc -mcp-config`
  flag lists the world's Managers and prints a ready-to-paste `claude mcp add`
  command plus a generic JSON config with the chosen Manager token filled in
  (`-mcp-manager <id>` picks one), and the daemon's startup banner points at
  the exact helper invocation for its own launch flags.
- New docs/16 Agent Playbooks: structured opening plans for the four goals
  agents pursue most — title challenge, survival, youth development, and
  financial rebuild — each with priorities, tactical plan guidance, standing
  directives, alert watches, a weekly Focus budget, and adjustment triggers,
  every example payload valid against the real MCP schemas.
- The standings tab gained an honours board on the `h` key: every completed
  season's champion and runner-up per division plus the cup winner, newest
  first, served by the new `GET /v1/history` endpoint.
- Goalkeepers no longer poach open-play goals: the scorer draw gives keepers
  zero weight for every open-play pattern and an eighth of their weight at
  set-piece headers, so the beloved stoppage-time keeper header stays
  possible but drops from ~2% of all goals to a genuine rarity.
- The media desk now files a Team of the Week after every completed league
  matchday: a per-division 1-4-3-3 best XI picked deterministically from the
  published ratings, with the round's top performer headlined as Player of
  the Round — full article with rotating voices in English and Korean.
- Formations finally reach the pitch: squad selection now fills the XI in
  the shape of the manager's tactical plan (all twelve catalog shapes —
  "4-2-3-1" reads as 4 defenders, 5 midfielders, 1 forward), so an agent's
  formation choice changes who actually plays. Depleted bands fall back to
  the strongest leftovers so the XI never shrinks, and the bench now always
  reserves its first seat for a spare keeper when one exists.
- The standings table earned its depth columns: goal difference (signed,
  always shown) and — on medium-and-wider layouts, as docs/07 promised — a
  last-five form strip (승/무/패 in Korean, W/D/L in English, oldest to
  newest). `/v1/tables` rows now carry `gd` and `form` (stable W/D/L enums).
- Red cards now carry consequences beyond the final whistle: the player is
  banned for the club's next fixture (straight red or second yellow, tunable
  count), squad selection skips banned players exactly like injured ones,
  and the ban counts down as the club's fixtures complete. The ban is
  announced as news in both languages, the agent dashboard lists suspensions
  under urgent items and `get_person` shows the remaining matches, and the
  console's squad table marks banned players.
- Finished matches now carry a story-of-the-match report: the replay pop-up
  (and `/v1/matches/{id}` as `story`) opens with a result frame, at most one
  "how it was won" edge read from the public diagnostics and credited to the
  side that got the result (pressing, aerials, set pieces, chance quality,
  or the winner's chance-pattern identity; either side may narrate a genuine
  draw), and at most one story beat read from the scorer ledger (hat-trick,
  two-goal comeback, late winner detected from the decisive goal; shootout
  ties read as wins) — rendered server-side in English and Korean,
  deterministic per fixture, from already-public facts only.
- Match commentary reads the story, not just the scoreline: a scorer's third
  goal headlines as a hat-trick call (with a late-drama variant), leveling or
  going ahead after trailing by two narrates as a comeback, re-taking the
  lead within five minutes of conceding reads as an instant response, and a
  fourth goal at a three-goal margin narrates as a rout. Quiet passages are
  state-aware too — close games from 75' speak in nervy tension lines, and
  one-sided games from 60' in cruise-control lines — in both English and
  Korean. All of it is presentation-only: the RNG stream and every match
  outcome are byte-identical to before (docs/12 §7).
- The live and replay match pop-ups gained a team-sheet panel on the `l` key:
  home/away lineups read keeper-to-front with position, name, substitution
  minutes (`▲`/`▼`), goal/card markers, and the public rating; players who
  came on follow the starters, and the live view lists the unused bench,
  dimmed. The Console API now serves the underlying rows as
  `home_lineup`/`away_lineup` on `/v1/matches/live` and `/v1/matches/{id}`
  (public facts only — names, positions, ratings, and event minutes).
- The spectator console now explains itself when it cannot reach the daemon:
  instead of raw `ui.*` tokens and empty panes, a guidance panel shows the
  server URL it tried, asks whether the `agenticfc` daemon is running, points
  at the `-server` override, and displays the retry error. The app chrome
  falls back to readable English until the first catalog fetch, and the
  console still reconnects automatically once the daemon answers.
- The daemon's data directory now defaults to the per-user OS data path
  (`~/Library/Application Support/agenticfc` on macOS,
  `$XDG_DATA_HOME/agenticfc` or `~/.local/share/agenticfc` on Linux,
  `%LocalAppData%\agenticfc` on Windows) when `-data` is omitted and the
  working directory has no `./data` holding Agentic FC world state. A
  packaged binary launched from any directory therefore resumes the same
  world; source checkouts with an existing world in `./data` are unaffected,
  and an unrelated project's `data/` folder is never adopted. The startup
  banner prints the resolved directory.
- Friendlier first launch of the daemon:
  - Listen addresses are bound before any world data is touched, so a busy
    port (for example a second daemon on the defaults) fails fast with a hint
    naming the flag to change — instead of generating a world first and
    leaving the corrected relaunch to silently resume it.
  - The startup banner prints the actually bound addresses (wildcard hosts
    rewritten to loopback so the URL is dialable), making
    `-console-addr 127.0.0.1:0` (random free port) usable, and requests that
    arrive while the world is still loading get `503 Service Unavailable`
    with `Retry-After` instead of stalling.
  - A world created without `-start` now prints how to start it (relaunch
    with `-start`, or the exact Console API call).
- Match broadcast overhaul in the spectator console:
  - Live scenes are composed on a fixed cell canvas (no more ragged ASCII),
    animated in two to six frames, and direction-aware — away attacks play
    mirrored with readable banners.
  - New ceremony scenes bracket each match: kick-off, the interval, full
    time, and penalty shootouts.
  - The live pop-up gained a marker timeline (home/away rows with a play
    head), a momentum strip, first/second-half tags, a full-width goal flash
    (also on replay goal beats), and a closing "elsewhere" ticker with fresh
    goal highlights; the fixtures board shows running minutes on live rows.
  - Goal commentary is score-aware (opener, equalizer, late drama) and every
    chance pattern gained extra goal/chance/save lines in both locales — all
    selected without consuming match RNG, so seeds replay identically.
  - The live markers payload is no longer windowed, so the timeline carries
    the full match story.
  - Commentary beats carry their football minute: the live backlog and the
    replay log read as a match report, and the replay's selected beat shows
    where you are while scrubbing.
  - Six crowd-flavor quiet lines per locale widen the most-repeated pool in
    the game, on the same stream-stable draw.
  - Preferred foot and news categories now render localized labels instead
    of raw enum tokens in the Korean console.
  - Replays mirror recorded away goals so a scrubbed goal beat celebrates
    under the correct end, and extra-tall live layouts show the timeline
    glyph legend inline.

## 0.1.0 - 2026-07-09

- Core daemon with seeded world generation, persistent simulation, Console API,
  MCP gateway, and local token authentication.
- Bubble Tea spectator console with media, tables, clubs, fixtures/results,
  live match view, commentary, replay browsing, and public diagnostics.
- AI-agent play surface through MCP with Focus economy, onboarding guide,
  observation tools, and Mindset/Tactical Plan shaping tools.
- Living football world systems: league, cup, careers, board confidence,
  transfers, contracts, youth intake, injuries, substitutions, form, season
  archive, and calibration tooling.
- Automated main-branch draft release packaging for Linux, macOS, and Windows.
