# Console (TUI) Design

Design for the Console — the human-facing TUI client (Viewer Mode / Admin Mode, see [00-glossary.md](00-glossary.md)). Centerpiece: the **responsive layout system**. All size thresholds are *(placeholder — tune with real usage)*.

## 1. Design principles

1. **Responsive by contract.** The Console adapts to terminal size on a fixed ladder of **Layout Tiers**. What appears at which tier is *specified, not improvised* — every screen has an explicit tier matrix (§4).
2. **Smaller reduces simultaneity first, capability second.** From tier S upward, every *core* action (watch a match, read news, browse tables, run admin ops) is reachable; shrinking primarily collapses side-by-side panes into hotkey-switched tabs. Enhanced features (comparisons, multi-match tickers, momentum graphs) are genuinely tier-gated and appear only on larger terminals.
3. **Live reflow.** Resizing the terminal re-tiers the UI immediately, preserving context (current screen, selection, scroll position). No restart, no flicker of state.
4. **CM heritage.** Match broadcasts keep classic CM's useful furniture — commentary stream + score/clock header + compact panels ([90 §2](90-reference-cm-fm.md)) — but open from the fixtures/results list as focused pop-ups instead of a separate screen.
5. **Keyboard-only, discoverable.** Every action has a key; a help overlay (`?`) lists context-relevant keys at every tier.

## 2. Layout Tiers

Tier is computed from **columns** (primary) and **rows** (modifier):

| Tier | Columns | Baseline experience |
|------|---------|---------------------|
| **XS** | < 60 | Not playable: centered notice showing current size and the minimum (60×16), nothing else. |
| **S — Compact** | 60–99 | Single pane. One screen at a time, full width; panels become hotkey tabs. The 80×24 default terminal lands here and must be fully usable for all core actions. |
| **M — Standard** | 100–139 | Main pane + one context side pane (toggleable). |
| **L — Wide** | 140–179 | Three panes: navigation/list + main + context. |
| **XL — Dashboard** | ≥ 180 | Multi-column dashboard; persistent extras (league ticker, pinned table, comparisons). |

**Row modifiers** (applied within any tier):

| Rows | Effect |
|------|--------|
| < 16 | Treated as XS. |
| 16–27 (**short**) | Header collapses to one line; no persistent bottom ticker; lists shorten. |
| ≥ 28 (**tall**) | Two-line header (world · clock · tempo · focus context) + persistent bottom ticker (live scores / breaking news). |
| ≥ 42 (**extra-tall**) | Expanded panels: longer commentary backlog, inline sparklines, more table rows. |

Chrome at every tier ≥ S: a full-terminal **app frame** with the product title centered in the top border, a **header** (world name, game date/time, current tempo, viewed subject), tab row, active content pane, and a **contextual footer key bar**. The footer shows the active screen or match pop-up controls on the left and reserves the right edge for the global quit key, so compact terminals and double-width locales never truncate the escape route. Tab navigation stays in the tab row instead of being redundantly repeated in the footer. Tall adds the bottom ticker. A lightweight graphical overlay lane can place a small Agentic FC mascot over the frame with a speech bubble for spectator alerts such as fresh press stories or new live match windows; overlays are decorative read-only presentation, never world state.

Mouse support is enabled where the terminal supports it: click tabs to switch screens, click table/list rows to select news, clubs, fixtures/results, and players, and use the wheel/PageUp/PageDown to move through article bodies and replay text.

## 3. Screen inventory

**Viewer Mode:** Media desk · League Tables · Club view · Fixtures/Results with live/replay match pop-ups · World overview (browse/switch focus freely — never pinned to one Manager).
**Admin Mode adds:** World Init wizard · Settings · Managers & Tokens · World Status (ops/health).

## 4. Tier matrix per screen

### 4.1 Fixtures / Results And Match Broadcasts

The Fixtures/Results screen is the match hub on every layout tier. It first
presents a single list of past results, live fixtures, and upcoming fixtures,
with live rows labeled in place. Selecting a live fixture opens a centered
broadcast pop-up over the list; selecting a finished fixture opens the replay
version of the same pop-up, with PgUp/PgDn and left/right rewind/advance
controls.

The pop-up carries the score/clock header (with a first/second-half tag),
public match stats and diagnostics,
live ratings, and the rhythmic commentary stream ([FR-35a](06-requirements.md)).
Commentary beats carry their football minute: the live "earlier flow" backlog
and the replay log read as a match report (`27' Goal! …`). The current live
beat stays prose-first; the replay's selected beat carries its minute so
scrubbing always shows where you are. The opening whistle stays unstamped
(there is no 0th minute), and older daemons without minute data fall back to
plain lines.
On tall layouts the live pop-up also draws two at-a-glance strips built from
public data only: a **marker timeline** (home events above, away below, on a
minute ruler with 15-minute ticks and a play head, using the marker legend
glyphs) and a **momentum strip** (the signed ten-minute momentum buckets as
mirrored home/away bars). Replay pop-ups list exact events instead and do not
repeat these strips. Tall live layouts also close the pop-up with a one-line **elsewhere
ticker** — the other live scores, with a `G!`
prefix while a goal there is fresh — and the fixtures list shows the running
minute on live rows, so the screen reads as a whole matchday rather than a
single game.
On wide layouts, every recognized football scene (goal, save, cross, cut-back,
through ball, long-range shot, set piece, counter, scramble, dribble, card,
stoppage, substitution, generic chance, and quiet build-up) plays as a terminal
animation of two to six frames, and ceremony scenes bracket the football:
kick-off, the interval, full time, and penalty shootouts each have their own
frames keyed off the whistle commentary. Scene frames are composed on one fixed-size
cell canvas (single-width runes only, out-of-canvas draws clipped), so every
frame of every scene shares exact dimensions and the art can never render
ragged. Attack scenes are direction-aware: while the
latest fresh marker belongs to the away side, the frame plays mirrored so
away moves attack leftward, with banners re-stamped readable. Ceremonies,
stoppages, and neutral build-up keep one orientation. Replays mirror only
recorded away goals (the scorer ledger carries the side); other replay beats
stay home-directed.
The 180 ms presentation tick exists only
while a live match pop-up is open, restarts when the latest commentary beat
changes (including a new action of the same kind),
and is invalidated immediately on close or full time. Replay
views stay on a stable frame, so the spectator surface does not flicker when
nothing is happening. `Space` pauses the live animation and invalidates its
timer chain; pressing it again starts a fresh chain. Presentation never changes
simulation timing.
Wide replay pop-ups retain the same chance-pattern, shot-quality, aerial,
pressing, and set-piece diagnostics after full time so tactical review does not
lose evidence that was visible live. When side-aware chance-pattern or
shot-quality data exists, both live and replay views separate home (`H`) and
away (`A`) bands, with `?` preserving any legacy unattributed remainder;
compact layouts continue to prioritize score, events, and prose.
It deliberately drops the ASCII pitch: the useful surface is the score/state
board plus commentary. Goal events produce a visible text flash that expires
within four elapsed match minutes, so a quiet or unrelated current scene never
sits beneath a stale goal banner. This does not pretend the model is a
continuous spatial simulation.

### 4.2 Media Desk

| Element | S | M | L | XL |
|---------|---|---|---|----|
| Press list (newest first, source/category) | ✔ below detail | ✔ main | ✔ left pane | ✔ left pane |
| Article detail (source, deck, body) | ✔ primary | ✔ side pane | ✔ right pane | ✔ right pane |
| Filter/source panel (club, category, date) | future overlay | future overlay | future pane | future pane |
| Pinned mini league table | — | — | — | ✔ right column |

The Media tab is intentionally not the raw engine SSE feed. Engine feed lines
are diagnostics/live commentary plumbing; the spectator UX follows the CM-style
inbox/press model and reads the public `World.News` ring through `GET /v1/news`.
Article detail uses a terminal masthead, dateline, section tag, and multi-
paragraph prose so the press desk reads like a living newspaper rather than a
raw event log. Long bodies preserve paragraph rhythm in the TUI and can be
paged with the same wheel/PageUp/PageDown controls used by match replays.
Full-time items are grouped into matchday round-up articles with result blocks,
table notes, and a longer storyline, so the Media tab reads like a sports desk
summary rather than a repetitive list of one-line score alerts. Kick-off and
fixture-start signals remain available through live match surfaces, `MATCH`
alerts, and the fixture list, but they are not filed as standalone media
articles or `NEWS` alert items.
Player attribute drift is therefore not shown as global breaking news. It is
ordinary development and belongs on player/training-style views, not the media
desk.

### 4.3 League Tables / Fixtures

| Element | S | M | L | XL |
|---------|---|---|---|----|
| Single division table (scroll) | ✔ | ✔ | ✔ | ✔ |
| Form / recent-results columns | — | ✔ | ✔ | ✔ |
| Fixtures/results browser | ✔ full list | ✔ full list | ✔ full list | ✔ full list |
| Finished match detail / replay log | pop-up | pop-up | pop-up | pop-up |
| Selected club mini-card | — | side pane | ✔ context pane | ✔ context pane |
| Multiple divisions side by side | — | — | — | ✔ |

The Fixtures/Results tab is a spectator archive as well as a forward schedule:
it mixes finished results, live fixtures, archived results, and upcoming
fixtures in one selectable list. The Console API accepts a bounded history
`limit` (the TUI asks for a large spectator cap, currently 1000 rows) so a
viewer can move through prior seasons instead of only the latest page. The
first rows lead with the next kick-off group followed by the latest completed
matchday, then the remaining forward schedule and older results. This
keeps both anticipation and replay access in the initial viewport even when a
full season of future fixtures is already generated. Finished current-season
matches open a replay pop-up with score, shots, scorers, cards,
substitutions, club-labelled top ratings, and the preserved commentary log; archived
past-season results show the permanent factual ledger, with commentary honestly
absent because season archival deliberately drops prose.

Reopening a finished-match pop-up shows any cached detail immediately and also
refreshes that fixture from the Console API. Match facts do not change, but
localized rendering and presentation may improve after a daemon deploy; a
long-lived Console must not pin the earlier server-rendered prose forever.
Late responses are applied only when their fixture is still the open replay;
live and scheduled fixture flows keep their existing polling behavior.

### 4.4 Club / Manager / Player views

| Element | S | M | L | XL |
|---------|---|---|---|----|
| Core profile (identity, key facts) | ✔ | ✔ main | ✔ main | ✔ main |
| Secondary sections (squad list, finances summary, board/fan mood) | ✔ stacked | ✔ stacked | ✔ context pane | ✔ context pane |
| Related-entity navigator (club list → selected club) | ↑/↓ | ↑/↓ | ✔ left list pane | ✔ left list pane |
| Side-by-side comparison (two players/clubs) | — | — | — | ✔ |

Club views include a deterministic ASCII club badge generated from the club name and a selected-player dossier. On wide layouts, the badge-side identity header owns predicted finish, board objective, confidence, and job security; the context rows below start with fan mood and finances rather than repeating those board facts. Compact layouts retain a single board summary because they do not render the wide identity header. Player dossiers show the public squad facts the viewer already receives — name, age, body profile (height/weight), preferred foot, weak-foot descriptor, position/unit, familiarity, contract season, academy flag, and visible attributes with bar graphs. The body profile comes from the simulation model and feeds small match modifiers for aerial reach and physical duels.

### 4.5 Admin screens (Admin Mode)

| Element | S | M | L | XL |
|---------|---|---|---|----|
| World Init | wizard, one step per screen | form + live summary pane | form + summary + validation pane | same as L |
| Settings | grouped list, edit in place | list + detail pane | list + detail + effective-config pane | same as L |
| Managers & Tokens | list; token revealed on demand | list + detail (token, club, binding status) | + Agent session info pane | + world status column |
| World Status | key metrics page | metrics + queue/tempo pane | + event-rate panel | + live log tail |

The first Settings implementation exposes runtime pacing controls when the
console is launched with `-admin-token`: Game Speed, in-season idle
acceleration, and off-season acceleration. These controls apply immediately via
the Console API and persist in the world snapshot. Generation-shaping settings
such as seed, league shape, quality, economy, and culture mix remain listed as
new-world-only settings rather than editable live controls.

## 5. Degradation & promotion rules

When the terminal shrinks (or grows), elements move along this ladder — content is demoted before it is removed:

```
persistent pane  →  toggleable side pane  →  hotkey tab / overlay  →  (tier-gated: hidden)
```

- **Drop order on shrink:** decorations & sparklines → extra columns (ticker, pinned table) → third pane → second pane → down to single-pane S.
- **Tier-gated features** (hidden below their tier, listed per matrix above): comparisons, multi-division view, momentum sparkline, persistent tickers, live log tail.
- **Never dropped (≥ S):** header, footer key bar, the active screen's primary content, commentary speed control in a match.
- On **grow**, the same ladder runs in reverse; the user's current focus stays where it was.

## 6. Implementation notes (Go / Bubble Tea)

- Bubble Tea delivers `WindowSizeMsg` on every resize → recompute tier, re-render; Lip Gloss handles pane composition per tier.
- Tier thresholds live in one place (a `layout` package constant table) so tuning them is a one-line change; the tier matrix in §4 is the acceptance spec for that table.
- The Console API is tier-agnostic: clients fetch the same streams regardless of tier; tiering is purely presentation. (A future Web client defines its own breakpoints against the same API.)
- **Locale:** the Console resolves the client's system language (`LC_ALL` → `LC_MESSAGES` → `LANG`) and passes it on Console API requests; the server renders text through the locale catalog layer with English fallback (FR-35c). Current v1 catalogs are `en` and `ko`. The Console itself stays catalog-free. A long-running Console refreshes the server-rendered UI catalog every 30 seconds and retries on each normal poll while the catalog is empty or the latest refresh has not yet been applied, so daemon restarts and deployments do not leave raw keys or stale labels pinned until the client restarts.
- TUI graphics use a display-width-aware surface layer for tables and frame lines so mixed-width localized rows stay aligned. Ordered text overlays are presentation-only width-1 TUI art layers for ASCII badges, characters, help panels, or match graphics; they never add game state.
