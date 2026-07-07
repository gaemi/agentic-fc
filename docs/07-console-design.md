# Console (TUI) Design

Design for the Console — the human-facing TUI client (Viewer Mode / Admin Mode, see [00-glossary.md](00-glossary.md)). Centerpiece: the **responsive layout system**. All size thresholds are *(placeholder — tune with real usage)*.

## 1. Design principles

1. **Responsive by contract.** The Console adapts to terminal size on a fixed ladder of **Layout Tiers**. What appears at which tier is *specified, not improvised* — every screen has an explicit tier matrix (§4).
2. **Smaller reduces simultaneity first, capability second.** From tier S upward, every *core* action (watch a match, read news, browse tables, run admin ops) is reachable; shrinking primarily collapses side-by-side panes into hotkey-switched tabs. Enhanced features (comparisons, multi-match tickers, momentum graphs) are genuinely tier-gated and appear only on larger terminals.
3. **Live reflow.** Resizing the terminal re-tiers the UI immediately, preserving context (current screen, selection, scroll position). No restart, no flicker of state.
4. **CM heritage.** The match screen is a direct descendant of classic CM's furniture — commentary stream + score/clock header + tabbed panels ([90 §2](90-reference-cm-fm.md)). Small terminals get CM's *actual* layout (one tab at a time); large terminals promote those tabs into simultaneous panes.
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

Chrome at every tier ≥ S: a full-terminal **app frame** with the product title centered in the top border, a **header** (world name, game date/time, current tempo, viewed subject), tab row, active content pane, and a **footer key bar**. Tall adds the bottom ticker. A lightweight graphical overlay lane can place a small Agentic FC mascot over the frame with a speech bubble for spectator alerts such as fresh press stories or new live match windows; overlays are decorative read-only presentation, never world state.

Mouse support is enabled where the terminal supports it: click tabs to switch screens, click table/list rows to select news, clubs, fixtures/results, and players, and use the wheel/PageUp/PageDown to move through article bodies and replay text.

## 3. Screen inventory

**Viewer Mode:** Media desk · League Tables · Club view · Live Match · Fixtures/Results · World overview (browse/switch focus freely — never pinned to one Manager).
**Admin Mode adds:** World Init wizard · Settings · Managers & Tokens · World Status (ops/health).

## 4. Tier matrix per screen

### 4.1 Live Match (the flagship screen)

| Element | S | M | L | XL |
|---------|---|---|---|----|
| Score + clock header (with possession bar) | ✔ (bar omitted if short) | ✔ | ✔ | ✔ |
| Big scoreboard + goal flash | — | — | ✔ | ✔ |
| **ASCII pitch** (event markers, [FR-35a](06-requirements.md)) | — *(commentary-only)* | compact strip *(toggle)* | ✔ band above commentary | ✔ larger band above commentary |
| Commentary stream (cadence-paced, [FR-35a](06-requirements.md)) | ✔ full width | ✔ main pane | ✔ center pane | ✔ center pane |
| Match stats/diagnostics panel | tab (hotkey) | side pane (toggle with ratings) | ✔ left pane | ✔ left pane |
| Live player ratings (both teams) | tab | side pane (toggle) | ✔ right pane | ✔ right pane |
| Lineups & subs log | tab | tab | tab | ✔ collapsible section |
| Momentum sparkline | — | — | ✔ under header | ✔ under header |
| Other-grounds ticker ("Latest Scores") | — | — | tall only | ✔ persistent column |
| Commentary speed control | ✔ | ✔ | ✔ | ✔ |

**The ASCII pitch** (§1.4 CM heritage, made spatial): the TUI Match tab draws
live fixtures from `GET /v1/matches/live` with goal/chance/card/injury/sub/
shootout markers. S stays pure commentary, M offers a compact toggleable pitch
strip, and L/XL render the full field above commentary. L/XL also use the
available space for a big scoreboard, latest-goal flash, momentum sparkline,
ratings, public diagnostics, and other-grounds ticker. These elements are
presentation over persisted live match facts; they add no simulation state and
nothing to the world hash. The pitch stays deliberately abstract because the
match engine samples key moments, not continuous ball position.

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
Match kick-off and full-time items are grouped into matchday preview and
round-up articles, with fixture lists, result blocks, table notes, and a short
storyline, so the Media tab does not become a repetitive list of one-line score
alerts.
Player attribute drift is therefore not shown as global breaking news. It is
ordinary development and belongs on player/training-style views, not the media
desk.

### 4.3 League Tables / Fixtures

| Element | S | M | L | XL |
|---------|---|---|---|----|
| Single division table (scroll) | ✔ | ✔ | ✔ | ✔ |
| Form / recent-results columns | — | ✔ | ✔ | ✔ |
| Fixtures/results browser | ✔ stacked | ✔ stacked | ✔ split list/detail | ✔ split list/detail |
| Finished match detail / replay log | ✔ | ✔ | ✔ | ✔ |
| Selected club mini-card | — | side pane | ✔ context pane | ✔ context pane |
| Multiple divisions side by side | — | — | — | ✔ |

The Fixtures/Results tab is a spectator archive as well as a forward schedule: it mixes finished results, archived results, and upcoming fixtures in one selectable list. The Console API accepts a bounded history `limit` (the TUI asks for a large spectator cap, currently 1000 rows) so a viewer can move through prior seasons instead of only the latest page. Finished current-season matches open a detail pane with score, shots, scorers, cards, substitutions, top ratings, and the preserved commentary log as a text replay; archived past-season results show the permanent factual ledger, with commentary honestly absent because season archival deliberately drops prose.

### 4.4 Club / Manager / Player views

| Element | S | M | L | XL |
|---------|---|---|---|----|
| Core profile (identity, key facts) | ✔ | ✔ main | ✔ main | ✔ main |
| Secondary sections (squad list, finances summary, board/fan mood) | ✔ stacked | ✔ stacked | ✔ context pane | ✔ context pane |
| Related-entity navigator (club list → selected club) | ↑/↓ | ↑/↓ | ✔ left list pane | ✔ left list pane |
| Side-by-side comparison (two players/clubs) | — | — | — | ✔ |

Club views include a deterministic ASCII club badge generated from the club name and a selected-player dossier. Player dossiers show the public squad facts the viewer already receives — name, age, body profile (height/weight), preferred foot, weak-foot descriptor, position/unit, familiarity, contract season, academy flag, and visible attributes with bar graphs. The body profile comes from the simulation model and feeds small match modifiers for aerial reach and physical duels.

### 4.5 Admin screens (Admin Mode)

| Element | S | M | L | XL |
|---------|---|---|---|----|
| World Init | wizard, one step per screen | form + live summary pane | form + summary + validation pane | same as L |
| Settings | grouped list, edit in place | list + detail pane | list + detail + effective-config pane | same as L |
| Managers & Tokens | list; token revealed on demand | list + detail (token, club, binding status) | + Agent session info pane | + world status column |
| World Status | key metrics page | metrics + queue/tempo pane | + event-rate panel | + live log tail |

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
- **Locale:** the Console resolves the client's system language (`LC_ALL` → `LC_MESSAGES` → `LANG`) and passes it on Console API requests; the server renders text through the locale catalog layer with English fallback (FR-35c). Current v1 catalogs are `en` and `ko`. The Console itself stays catalog-free.
- TUI graphics use a display-width-aware surface layer for tables and frame lines so mixed-width localized rows stay aligned. Ordered text overlays are presentation-only width-1 TUI art layers for ASCII badges, characters, help panels, or match graphics; they never add game state.
