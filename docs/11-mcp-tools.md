# MCP Tool Specification (v1)

The canonical MCP surface: 22 tools, their parameters, response shapes, and Focus costs. [04-agent-interface.md](04-agent-interface.md) explains the higher-level design principles. The alert/watch subset is specified in more detail in [14-agent-alerts.md](14-agent-alerts.md).

## 1. Conventions

### 1.1 Response envelope

Every tool returns:

```json
{
  "ok": true,
  "data": { },
  "meta": {
    "game_time": "1926-03-07T14:30",
    "tempo": "IDLE",
    "tool": "get_squad",
    "focus": {"spent": 4, "balance": 63, "cap": 100, "regen_per_game_hour": 2},
    "mindset_version": 41
  }
}
```

`meta` rides along free on every call — the Agent never pays to know the clock, its balance, or its Mindset version.

### 1.2 Errors

```json
{"ok": false, "error": {"code": "INSUFFICIENT_FOCUS", "message_key": "err.focus.insufficient",
  "message": "Needs 10 FP, balance is 4.",
  "details": {"required": 10, "balance": 4, "affordable_at": {"game": "1926-03-07T17:30", "real_seconds": 180}}}}
```

| Code | When |
|------|------|
| `AUTH_REQUIRED` | No Manager Token presented |
| `INVALID_TOKEN` | Unknown or regenerated-away token — sessions on a replaced token fail their next call with this |
| `MANAGER_RETIRED` | The bound Manager has retired (FR-14e); the token is permanently in `RETIRED` state |
| `INSUFFICIENT_FOCUS` | Balance < cost; includes time-to-afford (FR-27) |
| `NOT_FOUND` | Unknown id |
| `INVALID_TARGET` | Target type doesn't fit the verb/tool |
| `CONFLICT` | Directive contradiction (FR-16d) — details name the conflicting directive |
| `CAP_EXCEEDED` | 16th Directive / 6th Priority / 4th axis in one disposition call |
| `UNEMPLOYED_SCOPE` | Club-internal view requested while unemployed (FR-20d) |
| `VALIDATION` | Malformed params |

Failed calls (including `INSUFFICIENT_FOCUS`) cost nothing.

### 1.3 Paused worlds

While the world is paused (Admin maintenance — FR-34b): tools still respond against frozen state at normal costs, **Focus regen halts**, and `affordable_at` in `INSUFFICIENT_FOCUS` errors returns `{"paused": true}` instead of a real-seconds estimate. `meta.tempo` reads `PAUSED`. (Console-side: SSE streams stay open with heartbeats plus `world.paused` / `world.resumed` events — [05 A11](05-architecture.md).)

### 1.4 Data conventions

- **IDs**: opaque integers, globally unique per entity kind; refs are typed objects: `{"player": 4521}`, `{"club": 12}`, `{"manager": 7}`, `{"match": 88031}`.
- **Player-facing visibility**: MCP is the game-playing surface. It exposes football-facing data, scouting uncertainty, and controls required to manage a club; it does **not** expose private engine internals, seeded randomness inputs, raw private traits, or exact resolution formulas.
- **Attribute masking (FR-22a)**: visible attributes come as exact ints for fully-known players (own squad), else `[lo, hi]` ranges that narrow with knowledge. Qualitative reads appear as `descriptors[]` and `evidence[]` (prose with a `confidence` tag and source) instead of private raw values.
- **Money**: integer minor units of Crowns plus display string (`{"amount": 2400000, "display": "cr2.4M"}`).
- **Narrative fields**: `{"key": "news.transfer.completed", "params": {…}, "text": "…"}` — structured + text is rendered in **English** on the MCP surface; locale rendering belongs to the human Console surface (currently `en`/`ko` catalogs). Keys + params keep the contract language-neutral.
- **Time**: ISO-like game timestamps; `since` cursors are opaque strings from previous responses.

## 2. Focus economy — initial constants

| Constant | Initial value |
|----------|---------------|
| Cap | **100 FP** |
| Regen | **2 FP per game-hour** (≈ 48/game-day; regen rides Adaptive Tempo, so faster idle/off-season profiles refill faster in real time; halts entirely while the world is paused) |
| Starting balance | 100 FP (full) |

Sizing intuition: a game-week yields ~336 FP — comfortable for daily situation checks, a handful of deep reads, and a few meaningful changes; never enough to micromanage everything. (Tuning target per [04 §3](04-agent-interface.md).)

### Cost summary

| Tool | Cost (FP) |
|------|-----------|
| `get_guide` / `get_time` / `get_settings` / `get_focus` / `get_mindset` | 0 |
| `configure_alerts` / `ack_alerts` | 0 |
| `get_alerts` | 1 |
| `get_situation` | 1 |
| `get_news` | 1 |
| `get_match` | 1 own club · 3 other |
| `get_league` | 2 |
| `get_club` | 2 own · 4 other |
| `get_squad` | 3 own · 4 other (public fidelity) |
| `get_person` | 4 |
| `search_players` | 4 |
| `scout` | 12 |
| `remove_directive` | 2 |
| `add_directive` | 6 LEAN · 10 INSIST · 18 ABSOLUTE |
| `set_priorities` | 12 |
| `update_tactical_plan` | 15 |
| `update_disposition` | 25 |

## 3. Free tools

### `get_guide` — 0 FP
No params. → the onboarding guide an Agent should read before playing: the game premise, first-session checklist, strategy loop, common pitfalls, recommended opening pattern for a title challenge, valid vocabularies (`goals`, `directive_verbs`, `strengths`, `formations`, `position_groups`, tactical dials, disposition axes), target-shape hints for `add_directive`, and valid example payloads for `set_priorities`, `update_tactical_plan`, and `add_directive`.

This tool exists so Agents do not have to infer the game model or guess enum values. It also teaches long-running harnesses the Agent Alert loop: call `configure_alerts`, call `get_alerts` to discover the manager-specific alert resource URI, subscribe to that URI when the MCP host supports resource subscriptions, wake on `notifications/resources/updated`, call `get_alerts`, inspect detail with normal tools, then `ack_alerts`. It is also named in the MCP `initialize.instructions` hint so clients can route a fresh session to it before the first meaningful action. It is free, but still accepted/logged like every other MCP read (NFR-2 replay contract).

### `get_time` — 0 FP
No params. → game date-time, tempo (MATCH/IDLE/OFFSEASON/PAUSED), run profile, Game Speed, idle/off-season acceleration, next match window (kickoff time, own fixture if any), real-time estimate until that kickoff when the world is running, season phase (pre-season/season/window-open/off-season).

### `get_settings` — 0 FP
No params. → non-secret world settings and pacing table: world name, league shape, total clubs, run profile, quality/economy scale, squad/youth sizing, promotion/relegation slots, league/cup structure, current game time/tempo, base Game Speed, idle/off-season acceleration, effective speed by tempo, match-window real duration, game-day real duration by tempo, and current real-time Focus regen estimate.

The world seed and equivalent replay-randomness inputs are intentionally not exposed. The response includes a redacted seed marker so Agents know omission is deliberate, not an unavailable field.

### `get_focus` — 0 FP
No params. → balance, cap, regen rate, last 20 spend entries `{tool, cost, game_time}`.

### `get_mindset` — 0 FP
No params. → the full Mindset + Tactical Plan per [10 §7](10-mindset-schema.md): disposition (current/target/drift ETA), priorities, directives, tactical plan, version, archetype origin — plus manager self-state: employment status, club ref, reputation Descriptor, mood **Descriptor** (FR-20e), active board expectations summary.

### `configure_alerts` — 0 FP
Replaces the authenticated Manager's Agent Alert watch configuration. Params:
`enabled` and up to 32 `watches` for `NEWS`, `MATCH`, `CALENDAR`, and `FOCUS`
conditions. This manages wake signals only; it never performs an in-world action.
See [14-agent-alerts.md](14-agent-alerts.md).

### `get_alerts` — 1 FP
Returns the authenticated Manager's concrete alert resource URI, for example
`agenticfc://manager/7/alerts`, current watch configuration, pending alert
summaries, and the next acknowledgement cursor. Subscribe/read requests for a
different Manager id fail as resource-not-found. It does not acknowledge alerts
and does not replace `get_news`, `get_situation`, or `get_match` for detail.

### `ack_alerts` — 0 FP
Acknowledges pending Agent Alerts through an inclusive numeric cursor. Values
above the current highest issued alert id fail with `VALIDATION`; values at or
below the current acknowledgement cursor are no-ops. Acknowledgement is an
accepted MCP input so replay preserves the Agent's alert-handling timeline.

## 4. Observation tools

### `get_situation` — 1 FP
The wide-shallow dashboard; the intended cheap heartbeat call.
**Params**: none.
**Returns**: league position & last 3 results; next fixture; urgent items (injuries, expiring contracts, active offers awaiting Manager decision, board mood flag, ultimatum if any); 5 newest headline refs; window status. **Unemployed variant**: reputation summary, open vacancies list, active job approaches, headline refs.

### `get_news` — 1 FP
**Params**: `since` (cursor; **per-session state** — a session's first call without `since` defaults to the start of the current game-day; pass explicit cursors thereafter), `categories?` (transfer/match/injury/board/media/decision/career/youth/contract), `scope?` (own | league | world; default own), `limit?` (≤ 100, default 50). Session identity is part of the replay input log ([03 §5](03-simulation-engine.md)), so cursor state replays deterministically.
**Returns**: condensed narrative items `{id, game_time, category, headline{key,params,text}, article{source,title,deck,body}, refs[]}` + next cursor. `headline` remains the compact machine-readable news fact; `article` is the human press clipping used by MCP Apps cards, rendered only from the same masked params. Manager decisions appear here with their Mindset version (FR-16e).

### `get_league` — 2 FP
**Params**: `division?` (default: own), `sections?` (table | results | fixtures | managers; default table+results).
**Returns**: table rows (pos, club ref, P/W/D/L/GF/GA/Pts, form string); recent results; upcoming fixtures; `managers` section: per-club manager ref, tenure, security Descriptor (public — feeds the job market view).

### `get_club` — 2 FP own / 4 FP other
**Params**: `club` (default own).
**Returns (own)**: finances summary (balance, wage bill, budgets remaining), board expectation & confidence Descriptor, fan mood Descriptor, facilities Descriptors, squad list (shallow rows: name, age, position, condition%, morale Descriptor).
**Returns (other)**: public profile — division, table position, stadium, manager ref, reputation Descriptor, known headline players (shallow, masked). Internal finances/board detail never included. **Unemployed**: other-club form only (`UNEMPLOYED_SCOPE` guards own-style detail).

### `get_squad` — 3 FP own / 4 FP other
**Params**: `club` (default own), `detail?` (attributes | condition | contracts; default attributes).
**Returns**: per player: public body profile (height/weight), preferred foot, weak-foot profile (exact/range by knowledge + Descriptor), visible attributes (exact for own; ranges for others by knowledge), positional familiarity Descriptors, condition/sharpness %, season stats (apps, goals, rating avg), contract summary (own club only), form.

When the match model expands further ([12-match-model.md](12-match-model.md)),
`get_squad` should also expose public tactical-fit summaries derived from
known football evidence: aerial target, wide creator, press runner, transition
threat, set-piece threat, and similar role-readable tags. These summaries must
stay player-facing and must not expose private raw values or formula weights.

### `get_person` — 4 FP
The narrow-deep single-entity view.
**Params**: `ref` (player | manager | staff).
**Returns (player)**: full visible profile (masked per knowledge), age/body/foot/weak-foot/positions, career history, season stats, contract (own club: full; else: expiry year only), injury history, `descriptors[]` (personality etc. per knowledge), `evidence[]` (scout impressions with confidence + date). **(manager)**: career record, reputation Descriptor, style summary (public reads on their football), current club & security. **Unemployed Agent**: public fidelity only.

### `get_match` — 1 FP own / 3 FP other
**Params**: `match` (id) or `fixture` (club + date); live matches allowed.
**Returns (finished)**: score, scorers, cards, stats table (`home_shots`, `away_shots`, `match_patterns[]`, `shot_quality[]`, `aerial_duels[]`, `aerial_wins[]`, `press_turnovers[]`, `set_piece_threat[]`, `tactical_tilt[]`), lineups, substitutions, player ratings, key-moment commentary log (condensed).
**Returns (live)**: current score/clock, last ~20 commentary lines + cursor, stats snapshot (`home_shots`, `away_shots`, `match_patterns[]`, public diagnostic rows), own-team state (condition, bookings), Manager's in-match adjustments so far. `match_patterns[]` is a public tactical read such as crosses, cutbacks, or counters — useful for play, not a formula dump. Diagnostics are observed match facts: quality bands, aerial counts, press turnovers, set-piece threat, and tilt families. Polling a live match costs per call — watching closely is a real Focus decision for *other* matches, cheap for your own.
**Returns (archived)**: a PAST season's fixture serves the finished view from the permanent ledger, flagged `archived: true` with its `season` — every fact (score, lineups, subs + reasons, scorers, cards, ratings, adjustments, shots, match-pattern mix, public diagnostics) kept, the commentary honestly empty (prose isn't archived). Own/other billing follows the archived sides. Seasons finished before the ledger existed remain `NOT_FOUND`.

### `search_players` — 4 FP
**Params**: filters — `position?`, `age_min?`/`age_max?`, `max_wage?`, `division?`, `contract_status?` (expiring | listed | free_agent), `sort?` (value | age; default value), `limit` ≤ 30. `max_fee?` and `contract_status=listed` depend on the transfer/market engine (roadmap 5): until it lands, `max_fee` is inert and `listed` matches no one.
**Returns**: shallow rows (name, age, position, club, foot, weak-foot profile, headline attribute ranges per knowledge, value band, status flags). Fidelity follows scouting knowledge — searching doesn't reveal, it *finds*; `scout` reveals. The default `value` sort ranks by the **masked** value bucket (id tie-break), so it never leaks exact intra-bucket ordering for unscouted players.

## 5. Commission tool

### `scout` — 12 FP
**Params**: `target` (player ref) **or** `profile` (position + filter set, like search filters).
**Behavior**: starts an in-world scouting process (duration ~1–2 game-weeks, quality scaled by scout staff Judgement). Results arrive asynchronously: a scout-report news item + permanently enriched `evidence[]`/narrowed ranges on `get_person`. Re-scouting deepens. **Requires employment** (no club, no scouts → `UNEMPLOYED_SCOPE`). This is the documented exception to "no tool performs an in-world act" ([04 §1](04-agent-interface.md)): the process consumes scout time but mutates nothing except the Agent's own knowledge.

## 6. Shaping tools

Payload schemas per [10-mindset-schema.md](10-mindset-schema.md); all return the new Mindset version + a diff summary.

### `update_disposition` — 25 FP
**Params**: `targets`: map of axis → target value, **max 3 axes per call**.
**Behavior**: deltas ≤ 2 apply instantly; larger drift at ~2 pts/game-week (FR-16b). Returns current/target/ETA per axis.

### `set_priorities` — 12 FP
**Params**: full ranked list (≤ 5, catalog goals, no duplicate goals). Full replace.

### `add_directive` — 6 / 10 / 18 FP by strength
**Params**: `{verb, target, strength, params?, conditions?, expiry?}`.
**Errors**: `CONFLICT` (names the contradicting directive), `CAP_EXCEEDED` (includes full active list), `INVALID_TARGET`.

### `remove_directive` — 2 FP
**Params**: `id`.

### `update_tactical_plan` — 15 FP
**Params**: partial patch of the plan (`formation?`, dials…). Validated against active `FORBID` directives — a patch that violates one is rejected with `CONFLICT` (remove the fence first; the Mindset never contradicts itself).

## 7. Unemployed scope matrix (FR-20d)

| Tool | Employed | Unemployed |
|------|----------|------------|
| Free tools, `get_situation`, `get_news`, `get_league`, `get_match`, `search_players` | full | full (public fidelity) |
| `get_person` | full per knowledge | public fidelity |
| `get_club` / `get_squad` | own full / others public | others public only (no "own") |
| `scout` | ✔ | ✖ `UNEMPLOYED_SCOPE` |
| Shaping tools | ✔ | ✔ (job-hunt directives especially) |

## 8. Worked example

```json
// add_directive request
{"verb": "SIGN", "target": {"player": 8802}, "strength": "INSIST",
 "params": {"max_fee": 1200000}, "expiry": "END_OF_WINDOW"}

// response
{"ok": true,
 "data": {"directive": {"id": "dir_0012", "verb": "SIGN", "target": {"player": 8802},
          "strength": "INSIST", "params": {"max_fee": {"amount": 1200000, "display": "cr1.2M"}},
          "expiry": "END_OF_WINDOW"},
          "mindset_version": 42,
          "active_directives": 9},
 "meta": {"game_time": "1926-01-04T09:00", "tempo": "IDLE",
          "focus": {"spent": 10, "balance": 41, "cap": 100, "regen_per_game_hour": 2},
          "mindset_version": 42}}
```

What happens next is the game: the Manager, weighted hard toward signing #8802, opens negotiations when the simulation gives it a decision point — and the Agent reads how it went in `get_news`.
