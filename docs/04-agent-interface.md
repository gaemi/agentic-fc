# Agent Interface (MCP)

The Agent plays Agentic FC exclusively through an **MCP server**. There are two verbs — **observe** and **shape** — and one currency: **Focus**.

## 0. Sessions & authentication

- **Every Manager has a Manager Token**, issued **when the Manager entity is created** — the initial pool at world creation; caretakers and newgen backfills at spawn. All tokens (caretakers included) are viewable from the Console (Admin Mode).
- An MCP session presents a Manager Token to bind that Manager as the Agent's **Avatar**. All tools then operate in that Manager's context — its club, its Mindset, its Focus pool.
- **The binding is to the Manager as a person, not to a club.** If the Manager is sacked, changes clubs, or sits unemployed, the token — and the Agent's control — follows the Manager through it all (see [02 §3](02-game-design.md)). Getting sacked is a setback, not a game over.
- **Zero to many Agents per world.** Unbound Managers simply run on their predefined Mindsets; each bound Manager is independently controlled by its own Agent.
- **Disconnect changes nothing in-world.** The simulation never pauses; the Avatar keeps managing with the last Mindset it was given. Reconnecting with the same token resumes control (Focus kept regenerating to its cap in the meantime).
- **Concurrent sessions on one token are allowed** — no exclusive lock. All sessions share the Manager's single Focus pool and Mindset; edits apply in arrival order (last write wins). There is no interference to protect against beyond what the Focus economy already prices.
- **Token reissue:** a Manager Token can be regenerated from the Console (Admin Mode); the old token is invalidated immediately — sessions on it fail their next call with `INVALID_TOKEN`. No expiry, no scopes at v1.
- **Retirement:** Managers bound to an Agent are exempt from retirement rolls while the binding is live; the exemption lapses after 2 game-years with no session on the token (FR-14e). When a Manager retires, the token enters a **`RETIRED`** state: all calls fail with `MANAGER_RETIRED`, Focus regen stops, and the operator obtains a different Manager's token from the Admin to continue playing in that world.
- **While unemployed**, the Avatar's observation narrows to public information (tables, results, news — no club-internal views) at normal Focus prices; shaping tools still work on the Mindset (including job-hunt Directives).
- **Long-running harnesses may subscribe to manager-scoped alerts.** Alerts are MCP resource update signals for events the Agent asked to watch; they do not run the Agent, and they do not reveal detail beyond the normal MCP visibility rules. See [14-agent-alerts.md](14-agent-alerts.md).

## 1. Interface principles

1. **No direct action.** No tool performs an in-world act. The Agent reads the world and writes intent (Mindset / Tactical Plan). The Manager does the acting. *(Pillar: policy over puppetry.)* One documented exception on the observation side: `scout` starts an in-world **knowledge-gathering** process — it changes nothing in the world except the Agent's own evidence, so it stays on the observation ledger, not the action one.
2. **Breadth × depth querying.** Observation tools support the full range from *wide-and-shallow* (league table, squad summary) to *narrow-and-deep* (one player's full visible profile, scouting history, chemistry web). How the Agent spends its limited Focus across this range **is** the skill.
3. **Costs mirror reality.** Each tool's Focus Cost approximates the real-world time a human manager would spend on the equivalent activity. Glancing at the table is cheap; deep-scouting a target is expensive; reorganizing your entire philosophy is very expensive.
4. **Private internals stay private — evidence has explicit precision.** MCP returns only what a manager could reasonably know: public facts, own-club detail, scouting ranges, and qualitative reports. It never returns private raw traits, seeded randomness inputs, or exact resolution formulas. Observation yields *evidence*, following FM's proven masking pattern ([90 L4/L5](90-reference-cm-fm.md)):
   - **Visible attributes of unfamiliar players come as ranges** (e.g. Finishing 9–15) that **narrow as observation is invested** — knowledge precision literally scales with Focus spent (scouting assignments, watching matches, squad familiarity). Your own squad is always fully known.
   - **Qualitative traits surface only as Descriptors and impressions** — threshold-derived labels ("ruthless perfectionist", "fragile under pressure") and scout report prose, never numbers.
   - **Assessments carry explicit uncertainty and frame of reference** (FM's dark-star pattern): a potential estimate is a band, not a point, and is relative to a stated context (your squad, your division).

## 2. Tool families (draft)

> **The canonical tool-by-tool specification (params, response shapes, error codes, exact costs) is [11-mcp-tools.md](11-mcp-tools.md).** This section keeps the conceptual families; where the tables disagree, doc 11 wins.

### 2.1 Observation tools (cost: low → high with depth)

| Tool (draft) | Breadth/Depth | Returns |
|--------------|---------------|---------|
| `get_situation` | widest / shallowest | Dashboard: date, next fixture, league position, urgent items, recent headlines |
| `get_league` | wide / shallow | Tables, fixtures, results across divisions |
| `get_club` | medium | Own club: finances summary, board mood, squad list with headline attributes |
| `get_squad` | medium | Full squad with visible attributes, condition, morale |
| `get_person` | narrow / deep | One player/staff: full visible profile, history, contract, scouting evidence |
| `scout` | narrow / deepest | Commission focused scouting on a target → richer evidence over time (an in-world process, results arrive later) |
| `get_news` | wide / shallow | Inbox/press-room feed via per-session cursor: article-style items for decisions, appointments, injuries, results, transfers, scout reports… |

### 2.2 Shaping tools (cost: high)

Payload schemas for all shaping tools are defined in [10-mindset-schema.md](10-mindset-schema.md).

| Tool (draft) | Edits |
|--------------|-------|
| `update_disposition` | Mindset — Disposition axis targets (rare, most expensive; large changes drift in over game-weeks) |
| `set_priorities` | Mindset — ranked goal list (≤ 5, full replace) |
| `add_directive` / `remove_directive` | Mindset — standing orders (≤ 15 active; cost scales with strength: LEAN < INSIST < ABSOLUTE) |
| `update_tactical_plan` | Tactical Plan (formation + dials, partial patch) |

### 2.3 Free tools (cost: zero)

| Tool (draft) | Returns |
|--------------|---------|
| `get_mindset` | The Agent's own current Mindset + Tactical Plan, verbatim |
| `get_focus` | Current FP balance, regen rate, cap, recent spend log |
| `get_time` | Current game time, run profile, Game Speed, current tempo, next match window, real-time ETA |
| `get_settings` | Non-seed world settings and pacing table |
| `configure_alerts` / `get_alerts` / `ack_alerts` | Manager-scoped alert watches and pending wake signals for long-running harnesses |

Self-knowledge and meta-state are always free: the Agent must never have to pay to know what it already decided or what it can afford.

## 3. The Focus economy

- **Regeneration (decision):** FP regenerates continuously over **game time**, up to a hard **cap** — so fast-forward periods refill Focus faster in real terms ("the manager has more free time between matches"). No infinite stockpiling: use it or lose the regen.
- **Rate limiting by design:** the cap and regen rate together bound actions-per-game-hour. There is no separate rate limiter — the economy *is* the limiter.
- **Costs:** the canonical per-tool table (initial values: cap 100 FP, regen 2 FP/game-hour, reads 1–4, scouting 12, shaping 6–25) lives in [11 §2](11-mcp-tools.md).

- **Tuning target** *(placeholder)*: a full FP pool should feel like "one focused working day" of managerial attention; regen should comfortably support routine observation plus a few meaningful changes per game-week, but never continuous micromanagement.

## 4. Interaction contract

- **Non-blocking world:** tools return immediately against current state; the simulation never pauses for the Agent.
- **Asynchronous consequences:** shaping tools change the Mindset *now*, but effects surface only as future Manager decisions. `get_news` is how the Agent learns what its shaping wrought.
- **Alert notifications are wake signals, not gameplay data dumps.** A subscribed harness may receive `notifications/resources/updated` for the manager-specific alert resource returned by `get_alerts`, then should call `get_alerts` and normal observation tools to inspect the reason.
- **Insufficient FP:** the tool call fails cleanly with balance info and time-to-afford. Nothing queues.
- **Multiple agents:** one Agent ↔ one Manager (via Manager Token); a world hosts any number of such bindings, including none. Information isolation between Avatars follows the same rule as everything else: you see what your Manager could see.

## 5. i18n note

Response envelopes carry structured data + message keys + rendered text. **The MCP surface is English-only** (decision, 2026-07-04): agents are AIs — locale negotiation adds nothing, and a fixed language keeps prompts and replays reproducible. The locale catalog system serves the human surface (Console API → TUI) exclusively; message keys stay in every envelope, so nothing about the contract is language-bound. Current v1 human-surface catalogs are `en` and `ko`. See [05-architecture.md](05-architecture.md#4-internationalization-i18n).
