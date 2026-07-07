# Requirements

Current functional and non-functional requirements. **FR** = functional, **NFR** = non-functional. Requirements marked *(placeholder)* or *(tunable)* carry initial values pending tuning; current public priorities live in [99-roadmap.md](99-roadmap.md).

## Functional requirements

### World setup

- **FR-1** At world creation, the operator chooses the **league shape**: divisions (1–5) × clubs per division (8–24, default 16), with presets. Promotion/relegation slots derive from division size (~15%, min 2). Full config surface: [09-world-generation.md](09-world-generation.md).
- **FR-2** At world creation, the operator chooses a base **Game Speed** from the fixed tier set **5× / 15× / 30× / 60×** (default 15× — 1 game day = 96 real minutes).
- **FR-2a** Additional world config: **Run profile** (default/fast/slow/custom pacing), **World quality** preset (Amateur/Semi-Pro/Professional/Elite — scales talent bands), **Economy scale** preset (Austerity/Standard/Flush — scales all money), **name culture mix** weights, and advanced settings (in-season idle acceleration, off-season acceleration, squad size target, youth intake batch, start state).
- **FR-3** World content (players, clubs, competitions, names) is procedurally generated; no licensed real-world data. Currency is fictional (**Crowns**, `cr`), stored in integer minor units.
- **FR-4** World generation accepts a **seed** for reproducible worlds; generation stages consume stream-split RNG so stages don't perturb each other.
- **FR-4c** A newly generated world enters a **ready** state by default (fully generated, clock stopped) so the operator can distribute Manager Tokens before Admin Mode starts it; the operator may opt into auto-start via the `start state = running` config. Generation is atomic — an aborted init writes nothing.
- **FR-4a** v1 competition structure is fixed: the league pyramid + **one national cup** (knockout, all divisions) + **two transfer windows** (summer/winter); promotion/relegation counts derive from league scale.
- **FR-4b** The v1 world is a **closed ecosystem**: no foreign leagues or international duty; players enter via Youth Intake and leave via retirement. World generation keeps an extension seam for external leagues.

### Simulation

- **FR-5** The simulation advances in **real time**, continuously and unattended; it never blocks on Agent input.
- **FR-6** **Adaptive Tempo:** match windows run at base Game Speed; in-season idle windows run at an accelerated multiple; fixtureless off-season windows run at a larger accelerated multiple. Tempo changes must not alter simulation outcomes, only real-time pacing.
- **FR-6a** Match windows are **league-wide match days**: while any fixture is in play the world runs at base speed, and all fixtures of a match day run concurrently. The same rule applies in zero-agent worlds.
- **FR-6b** The match engine simulates via **key-moment sampling** (rolled significant passages), sufficient to produce commentary-grade event streams, per-player ratings/condition/stats, and in-match Manager decision events — not a continuous physical simulation.
- **FR-7** All world change flows through **probability rolls** combining: base rates, subject attributes (visible + hidden), environment modifiers, and (for Manager decisions) Mindset weights.
- **FR-8** The engine uses **roll-and-reschedule** discrete-event scheduling: each roll determines when that entity next rolls; entities are never evaluated on a fixed global tick.
- **FR-9** Players carry the expanded visible Attribute surface from [08-attributes.md](08-attributes.md): 33 visible rows for outfielders, 32 for goalkeepers, weak-foot proficiency, public body facts, hidden traits, and positional familiarity.
- **FR-9a** Attributes are **budget-constrained**: they draw from a capped **Ability Pool** with per-position attribute costs; the pool grows toward a hidden fixed **Potential Cap** and shrinks in decline, with freed capacity partially redistributable into cheaper attributes.
- **FR-10** Player effective ability is **context-dependent** (chemistry with teammates, Manager style fit, club situation) and **volatility-modulated** per event (consistency/big-match hidden attributes, FM's x-out-of-N pattern).
- **FR-11** Attributes evolve over time: growth, aging decline (per-player onset/rate), injuries, form — each via rolls influenced by trajectory meta-stats and environment. Decline is **physicals-first**; mentals hold or rise.
- **FR-11a** Players carry two distinct short-horizon state axes: **Condition** (energy; drains in matches, low values raise injury risk) and **Sharpness** (match fitness; built by playing, lost through inactivity).
- **FR-11b** Each match appearance yields a **player rating on a 10-point scale** whose values live almost entirely in a narrow practical band (quiet ↔ heroic), accumulating into form and season averages.
- **FR-12** The world has **population dynamics**: retirements remove players; periodic **Youth Intake** introduces new prospects.
- **FR-13** Club **finances and sentiment** (board, fans) fluctuate stochastically on top of persistent per-club tendencies.
- **FR-14** **Every club** is run by an autonomous Manager with a predefined Mindset, using one shared decision machinery. A world is fully playable-by-nobody: it runs with zero Agents connected.
- **FR-14a** **Manager careers:** boards can sack Managers; sacked Managers remain in the world, unemployed. Vacancies are filled through a two-way job market (clubs approach Managers; Managers approach clubs), resolved by rolls weighing reputation, track record, Mindset, and club circumstances. This applies equally to Avatars and autonomous Managers.
- **FR-14b** Sacking follows a **staged, observable escalation** (confidence decline → private warning → public ultimatum → dismissal), with each stage surfaced through the news feed — never an unheralded roll.
- **FR-14c** Managers carry their own attributes (reputation gating the job market, track record, coaching quality) in addition to the Mindset.
- **FR-14d** **No club is ever unmanaged:** a vacancy immediately installs an auto-generated **caretaker Manager** (same decision machinery, modest attributes, conservative archetype) while the position remains open on the job market; caretakers are occasionally appointed permanently. `club.manager` is never null.
- **FR-14e** Autonomous Managers roll **retirement** (age/career-driven), backfilled by newly generated managers. Managers bound to an Agent are **exempt while the binding is live**; the exemption lapses after 2 game-years with no session on the token. On retirement the Manager Token enters a `RETIRED` state: calls fail with `MANAGER_RETIRED` and Focus regen stops.

### Control model

- **FR-15** The Agent's only write surface is the **Mindset** (Disposition / Priorities / Directives) and the **Tactical Plan**. No tool directly performs an in-world action — with one documented observation-side exception: `scout` commissions an in-world knowledge-gathering process whose only output is the Agent's own evidence ([11 §5](11-mcp-tools.md)).
- **FR-16** The Mindset spans broad-to-narrow control: personality axes through ranked objectives down to target-specific standing orders ("sign player X at any cost").
- **FR-16a** Directives use a **fixed, typed schema** (target + verb + strength + optional conditions); free-text directives are not part of the public interface. Full schema: [10-mindset-schema.md](10-mindset-schema.md).
- **FR-16b** The Disposition consists of **10 bipolar axes** (−10…+10); large changes apply as **gradual drift** (~2 points per game-week) rather than instantly.
- **FR-16c** Priorities are a ranked list of **at most 5** goals from a fixed catalog; rank determines weight.
- **FR-16d** Directives that directly contradict an active Directive are **rejected** (with the conflict identified), never merged; dangling targets auto-expire with a news item.
- **FR-16e** The Mindset carries a **version counter**; every Manager decision records the Mindset version it rolled under.
- **FR-17** The Manager makes all in-world decisions via rolls **heavily weighted** by the Mindset — strong directives shift odds dramatically but never to certainty.
- **FR-18** Mindset changes take effect immediately on all *future* decision rolls — with one defined exception: Disposition deltas beyond ±2 set the **target** immediately but the value **drifts** (FR-16b), and rolls always consume the *current* (drifting) value.
- **FR-19** The number of simultaneously active Directives is bounded — initial value **15** *(tunable)*.

### Agent interface (MCP)

- **FR-20** The game exposes an **MCP server** as its only gameplay protocol. Sessions bind to a Manager by presenting that Manager's **Manager Token**; the bound Manager becomes the Agent's **Avatar**.
- **FR-20a** A world supports **0..N concurrently bound Agents**, each on a different Manager. Agent disconnect never pauses or alters the simulation; reconnection with the same token resumes control.
- **FR-20b** Multiple concurrent sessions on the same Manager Token are permitted — no exclusive lock. They share the Manager's single Focus pool and Mindset; conflicting edits resolve last-write-wins.
- **FR-20c** The Agent binding follows the **Manager**, not the club: sacking, unemployment, and hiring by a new club all preserve the binding and the token.
- **FR-20d** While the Manager is unemployed, observation narrows to **public information** (tables, results, news) at normal Focus prices; shaping tools remain available (including job-hunt Directives).
- **FR-20e** The Manager's mood state is exposed to the Agent only as **Descriptors** (never numbers) and cannot be set directly.
- **FR-21** Observation tools cover the full **breadth × depth** range: from onboarding (`get_guide`) and world settings (`get_settings`) to wide/shallow (situation dashboard, league tables) to narrow/deep (single-player deep profile, commissioned scouting). The canonical 19-tool surface, envelopes, and error codes are specified in [11-mcp-tools.md](11-mcp-tools.md).
- **FR-21a** Every response carries a free `meta` block (game time, tempo, Focus state, Mindset version); failed calls cost no Focus.
- **FR-22** Hidden Attributes are **never returned** by any tool; deep observation yields evidence (scouting impressions, behavioral history) only.
- **FR-22a** Evidence has **explicit precision**: unfamiliar players' visible attributes are returned as ranges that narrow with invested observation (own squad always fully known); hidden attributes surface only as threshold-derived **Descriptors** and report prose; ability/potential assessments carry uncertainty bands and a stated frame of reference.
- **FR-23** An append-only **news/event feed** exposes everything observable that happened, queryable via **per-session cursors** (a session's first call defaults to the start of the current game-day — [11 §4](11-mcp-tools.md)).
- **FR-24** Every non-free tool call costs **Focus Points**; costs differ by action class (reads < writes; shallow < deep), priced against equivalent real-world managerial time.
- **FR-25** Focus **regenerates over game time up to a hard cap** — no unbounded accrual. The cap + regen rate bound the Agent's actions per game hour (fast-forward periods therefore refill Focus faster in real terms).
- **FR-26** **Free actions** exist at zero cost: at minimum, reading one's own Mindset/Tactical Plan, Focus balance, and game clock/tempo state.
- **FR-27** Calls with insufficient FP fail cleanly, reporting balance and time-to-afford; they are not queued.

### Console & Console API

- **FR-30** The core exposes a **Console API** — a second interface strictly for viewing and administration. It never exposes gameplay verbs (shaping stays MCP-only).
- **FR-31** A **Console (TUI)** client ships with the game, in the same language/stack as the core.
- **FR-31a** The Console is **responsive** on a fixed ladder of **Layout Tiers** (XS/S/M/L/XL by terminal columns, with row modifiers): smaller terminals show less content and fewer features, larger ones more. The 80×24 default terminal must support all core actions (tier S); below the minimum (60×16) the Console shows only a resize notice.
- **FR-31b** Per-screen content/feature availability at each tier is **explicitly specified** (the tier matrices in [07-console-design.md](07-console-design.md)); resizing re-tiers the UI live, preserving screen, selection, and scroll context.
- **FR-32** Console **Viewer Mode** requires no authentication — including on publicly hosted worlds: an all-text spectate experience — news, press articles, board/fan reaction quotes, league tables, and **live CM-style match commentary** (substitutions, chances, key moments narrated as text).
- **FR-32a** Viewers can freely switch their focus between any Managers, clubs, and matches in the world; a viewing session is never pinned to a single Manager.
- **FR-33** Console **Admin Mode** requires the world's **Admin Token** and additionally provides: world initialization (league scale, Game Speed, seed), settings inspection/adjustment, world status, and the list of Managers with their **Manager Tokens**. Admin Mode includes all Viewer Mode capabilities.
- **FR-34** The **Admin Token** is generated at daemon first launch and printed to the daemon output (authenticating Admin Mode before any world exists); a **Manager Token** is issued **whenever a Manager entity is created** — the initial pool at world creation, caretakers and newgen backfills at spawn.
- **FR-34a** Manager Tokens can be **regenerated from Admin Mode** (old token invalidated immediately). No expiry or scoping at v1.
- **FR-34b** Admin Mode can **pause/resume the world** for maintenance (nothing advances while paused). Agents have no pause capability of any kind.
- **FR-35** All human-readable text (news, commentary, press, reactions) is produced by a single **Narrative Renderer** over the event feed, via message-key templates (the i18n seam). The Console consumes it fully; `get_news` consumes it in condensed form.
- **FR-35a** The Narrative Renderer emits **cadence metadata** with match commentary (display duration / density hints) so clients reproduce tension pacing — dramatic moments linger, routine passages compress.
- **FR-35b** Commentary and match summaries must carry **diagnosable tactical signal** (readable cause of dominance/collapse) alongside flavour, for spectators and Agents alike.
- **FR-35c** The Narrative Renderer is **multilingual-ready**: text renders in the requester's locale, Console API requests carry a locale resolved from the client's system language, and MCP sessions may set a locale at bind (default English). Any unsupported locale, missing key, or missing catalog **falls back to English** — never an error. Current v1 catalogs are English and Korean.

### Persistence & operations

- **FR-28** World state, event queue, feed, and roll audit trail are persisted; a crashed or stopped world **resumes** where it left off.
- **FR-28a** The world is **append-only**: no player-facing save slots, rollback, or time travel. Operators may take filesystem-level backups; each world runs as its own daemon process with its own persistence directory.
- **FR-29** Every roll is logged with inputs, weights, outcome, and next-roll schedule (audit trail for balancing/explainability).

## Non-functional requirements

- **NFR-1 Unattended reliability.** The sim runs headless for long real-time stretches (days) without human intervention.
- **NFR-2 Reproducibility.** Same seed + same setup + same **ordered external-input log** ⇒ identical run. Determinism rests on stream-split seeded RNG, `(game_time, ingress_seq)` input ordering, and total queue tie-breakers — see [03 §5](03-simulation-engine.md).
- **NFR-3 Emergent variety.** No single attribute may dominate an outcome class; unpredictability must emerge from many interacting modifiers. Two runs from different seeds should diverge meaningfully. *(Balancing acceptance criteria TBD.)*
- **NFR-4 Performance.** Steady-state event throughput must sustain the **maximum configurable tempo** — 60× base with 64× in-season idle acceleration ⇒ 3840× effective, and 240× off-season acceleration ⇒ 14400× effective — at maximum league scale on commodity hardware; match windows are the density sizing case, max idle/off-season tempo the throughput sizing case.
- **NFR-5 i18n.** All player-facing strings flow through message keys + parameter catalogs; names use pluggable generators; internal units stay canonical. Adding further display locales requires catalog/content additions, not contract changes. English (`en`) is the fallback, and current v1 catalogs are `en` and `ko`. Narrative variety (NFR-8) applies **per locale**.
- **NFR-6 Observability.** Operators can inspect world health (queue depth, event rates, FP economy stats) without disturbing the sim.
- **NFR-7 Security/isolation.** Access is token-scoped: Manager Tokens bind MCP sessions to exactly one Manager's context; an Avatar's session can never read another club's private state. Admin operations require the Admin Token. Lifecycle is settled: regeneration from Admin Mode invalidates the old token immediately; concurrent sessions share state last-write-wins (FR-20b); retirement freezes the token (FR-14e).
- **NFR-8 Narrative variety.** Repeated event types must not produce immersion-breaking repetition (FM's press conferences are the cautionary tale); template variety per event type is a release-quality requirement.
- **NFR-9 Spectator pace.** Adaptive Tempo must keep spectating **dense** — no dead air between meaningful events. Match windows should stay readable, in-season gaps should pass briskly, and fixtureless off-season stretches should clear aggressively. The CM lesson ([90 L8](90-reference-cm-fm.md)) is about *felt* pace — compressed idle, punchy matches — not literal 4-hour seasons, which a real-time world intentionally does not target.

## Explicit non-requirements (v1)

- Human gameplay controls — humans spectate (Viewer Mode) and administer (Admin Mode); only Agents play.
- Web/GUI clients (the Console API is designed for a later Web client; TUI-first at v1).
- 2D/3D match visualization (the match is text commentary by design).
- Licensed real-world content.
- Additional locale catalogs beyond the current v1 English and Korean set.
