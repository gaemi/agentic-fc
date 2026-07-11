# Changelog

This project follows a simple human-readable changelog. The current release
version is tracked in `VERSION`; each release must have a matching dated
section below.

## Unreleased

- Friendlier first launch of the daemon:
  - Listen addresses are bound before any world data is touched, so a busy
    port (for example a second daemon on the defaults) fails fast with a hint
    naming the flag to change — instead of generating a world first and
    leaving the corrected relaunch to silently resume it.
  - The startup banner prints the actually bound addresses, making
    `-console-addr 127.0.0.1:0` (random free port) usable.
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
