# Architecture

High-level shape of the system.

Guiding investment principle ([90 L13](90-reference-cm-fm.md)): when CM/FM split in 2003, the side holding the **engine + database** thrived and the side holding the brand + interface died. The Simulation Core and world generation are the asset; every interface (MCP schemas, Console, future Web) is a replaceable skin over them.

## 1. Component overview

```
      ┌────────────────────────────┐      ┌──────────────────────────────┐
      │      AI Agent(s) (0..N)     │      │        Humans (0..N)          │
      │    MCP-capable clients      │      │  spectators · administrators  │
      └─────────────┬──────────────┘      └──────────────┬───────────────┘
                    │ MCP + Manager Token                 │
                    │ (stdio / HTTP)          ┌───────────▼───────────────┐
                    │                         │       Console (TUI)        │
                    │                         │ Viewer Mode: open, text    │
                    │                         │  feeds + match commentary  │
                    │                         │ Admin Mode: Admin Token —  │
                    │                         │  init/settings/tokens      │
                    │                         └───────────┬───────────────┘
                    │                                     │ Console API
      ┌─────────────▼──────────────┐      ┌──────────────▼───────────────┐
      │        MCP Gateway          │      │      Console API server       │
      │  tool schemas · token bind  │      │  view streams (feed, match,   │
      │  Focus accounting & limits  │      │  tables) · admin ops (gated)  │
      │  read model / query shaping │      └──────────────▲───────────────┘
      └───────┬─────────────▲──────┘                      │ (read + admin,
              │             │                             │  never gameplay)
                     intents    │             │  state views, news
                (Mindset edits) │             │
┌───────────────────────────────▼─────────────┴───────────────────────┐
│                          SIMULATION CORE                             │
│                                                                       │
│  ┌────────────────┐  ┌──────────────────┐  ┌──────────────────────┐  │
│  │ Event Scheduler │  │ Roll Resolver    │  │ Manager Decision     │  │
│  │ (DES queue,     │──│ (probability     │──│ Module               │  │
│  │  adaptive pacer)│  │  model, seeded   │  │ (Mindset + state →   │  │
│  └────────────────┘  │  RNG streams)    │  │  weighted decisions) │  │
│                       └──────────────────┘  └──────────────────────┘  │
│  ┌────────────────┐  ┌──────────────────┐  ┌──────────────────────┐  │
│  │ Match Engine    │  │ World Systems    │  │ News/Event Feed      │  │
│  │ (match windows, │  │ (development,    │  │ (append-only log of  │  │
│  │  event stream)  │  │  finance, intake,│  │  observable events)  │  │
│  └────────────────┘  │  careers…)       │  └──────────┬───────────┘  │
│                       └──────────────────┘             │              │
│                                            ┌───────────▼──────────┐  │
│                                            │ Narrative Renderer   │  │
│                                            │ (events → text lines:│  │
│                                            │  commentary, press,  │  │
│                                            │  reactions; i18n keys)│  │
│                                            └──────────────────────┘  │
└───────────────────────────────┬──────────────────────────────────────┘
                                │
                  ┌─────────────▼──────────────┐
                  │       Persistence           │
                  │  world state · event log ·  │
                  │  roll audit trail · saves   │
                  └────────────────────────────┘
```

## 2. Component responsibilities

| Component | Responsibilities |
|-----------|------------------|
| **MCP Gateway** | Exposes observation/shaping/free tools; binds sessions to Managers via **Manager Tokens**; enforces the Focus economy (balance, costs, cap, regen); translates world state into breadth×depth-shaped views; never mutates the world directly — shaping tools write only to Mindset/Tactical Plan. |
| **Console API server** | The core's second interface. Serves **view streams** (news feed, live match commentary, tables, club summaries) to any client, and **admin operations** (world init, settings, world status, Manager/token listing) gated by the **Admin Token**. Never exposes gameplay verbs — shaping stays MCP-only. Designed as the reuse point for future Web clients. |
| **Console (TUI client)** | Separate binary. **Viewer Mode** (no auth): scrolling all-text spectate experience — CM-style match commentary, press articles, board/fan reactions, league tables. **Admin Mode** (Admin Token): world lifecycle, settings, token inspection, plus everything Viewer Mode shows. **Responsive** across Layout Tiers (XS–XL) with per-screen tier matrices — see [07-console-design.md](07-console-design.md). Tiering is purely presentation; the Console API is tier-agnostic. |
| **Narrative Renderer** | Turns feed events into human-readable text via message-key templates: match commentary lines, press articles, board/fan reaction quotes, news blurbs. **Emits cadence, not just content**: each line carries display-timing/density hints so clients can reproduce classic CM's tension pacing ([90 L1](90-reference-cm-fm.md)). Sole text-producing layer (i18n seam). Consumed richly by the Console, in condensed form by `get_news`. |
| **Event Scheduler** | The DES priority queue keyed by game time; the **adaptive pacer** drains it against real time per Game Speed + tempo (match, in-season idle, off-season). |
| **Roll Resolver** | Single home of the probability model: composes base rates, attributes, environment, Mindset weights; draws from per-stream seeded RNG; writes every roll to the audit trail. |
| **Manager Decision Module** | Turns decision events into outcomes using the Mindset (all layers) + Tactical Plan + world context, via the Roll Resolver. Also runs rival clubs' non-agent Managers with static/generated Mindsets. |
| **Match Engine** | Simulates matches during match windows; emits an observable in-match event stream; feeds results back into world systems. |
| **World Systems** | Domain logic bundles: player development/decline, injuries, finances, board/fans, youth intake, careers/retirement, season calendar. Each defines its event types, roll shapes, and reschedule policies. |
| **News Store / Media Desk** | Append-only, timestamped `World.News` ring of public and manager-private news items; the source for MCP `get_news` and the TUI Media tab. It is the CM-style inbox/press surface (appointments, transfers, injuries, contracts, results, board pressure), not the raw engine event stream. Hidden information never enters public items. |
| **Persistence** | Authoritative world state, event queue snapshot, feed, roll audit trail; save/load; crash recovery (the world must resume, not restart). |

## 3. Architecture Rules

| Rule | Current design |
|------|----------------|
| Single-writer core | All world mutations flow through the simulation engine and event queue. MCP writes change intent state such as Mindset and Tactical Plan, not arbitrary world state. |
| Discrete-event scheduling | The engine uses roll-and-reschedule events rather than fixed ticks. |
| Deterministic randomness | World-affecting randomness uses labelled internal RNG streams, total queue ordering, and ordered external input logs. |
| Strict interface roles | MCP is the gameplay protocol for agents. Console API/TUI is the human spectator and operator surface. |
| Game-time Focus regen | Focus accrues over game time, so accelerated quiet periods refill faster in real time. |
| Message-key text | Human-facing text is rendered from message keys and catalogs. |
| Token access | Admin Token gates operator control. Manager Tokens bind MCP sessions to a Manager. Viewer Mode is unauthenticated read-only. |
| One daemon per world | Each `agenticfc` process owns one world and one persistence directory. |
| Append-only play | Worlds persist and resume. Operators can back up files, but there is no player-facing rollback mechanic. |
| Maintenance pause | Admin pause freezes the pacer and Focus regen while tools continue to read frozen state. |

## 4. Internationalization (i18n)

The architecture target is **multilingual readiness**, not a permanently bilingual product. Human-facing prose is separated from game data so additional display locales can be added as catalog/content work without changing simulation or API contracts. Current v1 status: English (`en`, default/fallback) and Korean (`ko`) catalogs ship together.

- **Separation of data and prose.** Every MCP response and feed entry carries structured fields plus a **message key** + parameters; catalogs render the text. Adding further locales = adding catalogs, zero contract changes.
- **Locale resolution:** display language follows the client's **system language**. The Console detects it from the client environment (`LC_ALL`/`LC_MESSAGES`/`LANG`) and passes it on Console API requests; MCP sessions may set a locale at bind time (default `en`). Any unsupported locale, missing catalog, or missing key falls back to **English** — never an error, never a blank.
- **Rendering is server-side** in the Narrative Renderer (the sole text producer, FR-35): clients receive final text + keys, so the Console and future Web stay catalog-free.
- **Generated names** (players, clubs, competitions) come from pluggable, culture-aware name generators. The current generators cover Anglo, Latin (Iberian/South American), Continental (Germanic/Nordic), and East Asian romanized output. Distribution mix per world is a generation parameter. Name cultures are world content and do **not** vary by display locale.
- **Units & formats** (dates, currency) rendered through the locale layer, stored canonically (ISO dates, integer minor currency units).
- Docs remain English-only; localization of docs is out of scope.

## 5. Tech stack

Agentic FC is a single Go module. Core runtime, MCP gateway, Console API, TUI,
and calibration tooling all build from the same module.

Current stack:

| Area | Technology |
|------|------------|
| Language | Go |
| TUI | Bubble Tea, Lip Gloss |
| MCP | `modelcontextprotocol/go-sdk` |
| Persistence | JSON snapshots plus append-only logs |
| Console transport | HTTP JSON plus SSE |

Repo shape:

```
agentic-fc/
├── cmd/
│   ├── agenticfc/             # core daemon: sim + MCP gateway + Console API
│   ├── agenticfc-console/     # TUI client
│   └── agenticfc-calibrate/   # match calibration CLI
├── internal/
│   ├── sim/               # scheduler and game time
│   ├── engine/            # simulation systems and match engine
│   ├── worldgen/          # world config and generated state
│   ├── mindset/           # Mindset & Tactical Plan model
│   ├── mcpserver/         # MCP tools + Focus economy
│   ├── consoleapi/        # view streams + admin ops
│   ├── narrative/         # renderer + message catalogs (i18n seam)
│   └── store/             # persistence
└── docs/
```

The Console API uses HTTP JSON for reads and SSE for live streams. Future human
clients can reuse this API without touching the simulation core.
