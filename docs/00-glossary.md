# Glossary

Canonical terminology for Agentic FC. All docs, code, and MCP tool names should use these terms.

## Actors

| Term | Definition |
|------|------------|
| **Agent** | The external AI agent playing the game. Connects via MCP. Never acts inside the world directly — it only observes and edits its Manager's Mindset (and related plans). A world can host **zero, one, or many** Agents. |
| **Manager** | An autonomous in-game persona with a career of its own. **Every club has an acting one** — when a job falls vacant, an auto-generated **caretaker Manager** takes charge until the board appoints a successor, so the decision machinery is never empty even while the position is openly on the market. Managers can be **sacked**, sit unemployed, be hired elsewhere, and eventually **retire** — the person persists across clubs. Managers keep acting whether or not any Agent is connected. |
| **Avatar** | A Manager currently bound to an Agent. Same machinery as every other Manager — the only difference is that its Mindset is being edited by an Agent instead of staying fixed. The binding follows the Manager across sackings, unemployment, and new jobs. If the Agent disconnects, the Avatar simply keeps managing with the last Mindset it was given. |
| **Club** | The football club a Manager runs. Has finances, squad, staff, facilities, board expectations. |

## Control model

| Term | Definition |
|------|------------|
| **Mindset** | The Manager's editable decision-making profile — the single control surface the Agent has over club management. Spans from broad personality to hyper-specific instructions. Decisions are still rolled probabilistically, but Mindset heavily skews the odds. |
| **Disposition** | The broadest Mindset layer: long-lived philosophy axes (risk tolerance, youth vs. proven players, attacking vs. pragmatic, financial prudence…). Slow-moving, wide influence. |
| **Priority** | Ranked mid-term objectives inside the Mindset ("survival first", "develop the academy", "win the cup"). |
| **Directive** | The narrowest Mindset layer: concrete standing instructions ("sign player X at any cost", "never field a back three", "always rotate in cup games"). Strong influence on the specific decisions they target. |
| **Tactical Plan** | The Manager's current strategy/tactics setup: formation + six dials (mentality, pressing, tempo, width, directness, counter). Editable by the Agent like the Mindset; executed and adapted probabilistically by the Manager. |
| **Mindset Archetype** | A pre-built Mindset template (The Idealist, The Firefighter, The Trader…) rolled onto autonomous Managers at world generation. An Avatar's Mindset starts from its Manager's archetype and diverges as the Agent edits it. |

## Simulation

| Term | Definition |
|------|------------|
| **Simulation Core** | The authoritative engine that advances the world: discrete-event, probability-driven, real-time-paced. |
| **Roll** | A single probability resolution ("throwing the dice") for an entity or event. Inputs: relevant attributes + environment + Mindset (for Manager decisions). |
| **Roll-and-Reschedule** | The core loop pattern: when an entity is rolled, the outcome (a) may change state/attributes or trigger an event, and (b) schedules when that entity's next roll happens. Nothing rolls on every tick. |
| **Event** | Anything scheduled or produced by the simulation: attribute drift, an injury, a transfer offer, a youth-intake day, a match kickoff, a Manager decision point. |
| **Attribute** | A visible player/staff stat on the 1–20 scale. Players show **15** (GKs swap the technical five) — see [08-attributes.md](08-attributes.md). |
| **Hidden Attribute** | A stat that exists but is never directly shown (**19 per player**, plus positional familiarity maps): potential, personality traits, consistency, injury proneness, development/decline speed… Observable only through behavior over time. |
| **Ability Pool** | The budget constraining a player's attributes: each attribute point has a per-position cost drawn from a capped pool. Growth expands the pool toward the Potential Cap; decline shrinks it, and freed capacity can partially redistribute into cheaper attributes ("aging gracefully"). Adopted from FM's CA model — see [90 L2](90-reference-cm-fm.md). |
| **Potential Cap** | The hidden, fixed ceiling of a player's Ability Pool, set at generation (FM's PA equivalent). |
| **Condition** | Short-term player energy. Drains during matches (faster at high intensity), recovers over days. Low Condition sharply raises injury risk. |
| **Sharpness** | Medium-term match fitness, built by playing and lost through inactivity. A separate axis from Condition. |
| **Descriptor** | A human-readable label derived from hidden attributes via thresholds (e.g. a personality descriptor like "ruthless perfectionist"). The canonical way hidden state surfaces as observable evidence — raw hidden values are never shown. |
| **Chemistry** | Interpersonal/contextual modifiers — a player's effective ability shifts depending on teammates, Manager style, club situation. |
| **Youth Intake** | Periodic generation of new prospects entering the world (replaces retiring players over time). |

## Time

| Term | Definition |
|------|------------|
| **Game Time** | The in-world clock/calendar. |
| **Game Speed** | The base real-to-game time ratio chosen at setup. Default **15×** (1 game day = 96 real minutes). Several tiers, faster and slower, must be offered. |
| **Adaptive Tempo** | Automatic tempo switching: match windows run at base Game Speed; in-season idle stretches run at an accelerated multiple; fixtureless off-season stretches run at a larger accelerated multiple to keep the game pacey. |
| **Run Profile** | A launch preset for new-world pacing. Default = 15× match / 16× idle / 96× off-season; fast/slow/custom profiles trade wall-clock speed against readability. |

## Access & tooling

| Term | Definition |
|------|------------|
| **Manager Token** | A per-Manager credential issued whenever a Manager entity is created (initial pool at world creation; caretakers/newgen backfills at spawn). An Agent presents a Manager Token over MCP to bind that Manager as its Avatar. |
| **Admin Token** | The world's superuser credential ("super token"), generated by the daemon at **first launch** and printed to its output — so Admin Mode works before any world exists (it's what authenticates the init wizard). Required for the Console's Admin Mode. |
| **Console** | The human-facing TUI utility. Connects to the core via the **Console API**. Two modes: Admin Mode and Viewer Mode. |
| **Admin Mode** | Console mode gated by the Admin Token: initialize worlds, adjust settings, inspect world status, list Managers and their Manager Tokens. Includes everything Viewer Mode can do. |
| **Viewer Mode** | Openly accessible Console mode for spectating — unauthenticated by design, even when hosted publicly. All-text feeds of Manager decisions, player events, press articles, board/fan reactions, and live CM-style match commentary. Viewers freely switch focus between any Managers/clubs; they are never pinned to one. Read-only. |
| **Console API** | The core's second interface (alongside MCP): serves the Console's view streams and admin operations. Future clients (e.g. Web) reuse this same API. |
| **Layout Tier** | The Console's responsive size class, computed from terminal columns/rows: XS (too small) / S Compact / M Standard / L Wide / XL Dashboard. Each screen specifies exactly what it shows per tier — see [07-console-design.md](07-console-design.md). |
| **World Config** | The operator-chosen settings at world creation (league shape, Run Profile, Game Speed, quality/economy presets, culture mix, seed…). Everything else is derived or rolled — see [09-world-generation.md](09-world-generation.md). |

## Agent economy

| Term | Definition |
|------|------------|
| **Focus** | The Agent's regenerating action budget, denominated in **Focus Points (FP)**. Represents the Manager's real-world time/attention. Regenerates over game time up to a hard cap — it can never be stockpiled indefinitely. |
| **Focus Cost** | The FP price of an MCP action. Heuristic: proportional to how much real-world time the equivalent managerial activity would take. Reads are cheaper than writes; shallow reads cheaper than deep ones. |
| **Free Action** | An MCP action with zero Focus Cost — e.g. inspecting your own Mindset, checking the Focus balance. |
