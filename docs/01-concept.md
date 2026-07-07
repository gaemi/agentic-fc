# Concept & Vision

## Elevator pitch

**Agentic FC** is a football club management simulation — Football Manager's genre — built from the ground up for an audience of **AI agents**. A human never needs to touch it.

The agent is not the manager. The agent is the *mind behind* the manager: it observes the club and the league through an MCP interface, and it edits the **Mindset** of an autonomous in-game **Manager** — from broad philosophy ("youth first, always") down to surgical directives ("sign this exact player, whatever it costs"). The Manager then lives inside the simulation in real time, making probabilistic decisions shaped by that Mindset, while the world rolls on: players develop and decline, fortunes rise and fall, careers begin and end.

## What makes it different

1. **Agent-first, human-optional.** The primary interface is MCP, not a GUI. Observation and control are designed around what an LLM-based agent does well: querying at variable breadth/depth, reasoning over state, and expressing intent as policy rather than clicks.

2. **Indirect control — you shape the decision-maker, not the decisions.** The Agent never picks the starting XI. It tunes the Manager who does. Control bandwidth is intentionally limited (the Focus economy), so *what to look at* and *what to change* become the core skill expression. Football Manager itself proves this loop works: veteran FM players already run clubs largely through delegation and standing policies ([90 L11](90-reference-cm-fm.md)) — Agentic FC takes that to its logical conclusion.

3. **A living stochastic world.** Everything advances through probability rolls layered on top of stats, which in turn reshape future probabilities — a feedback loop, seasoned by environment (teammates, club health, league context). Two runs of the same setup should never play out the same way.

4. **Realism through tendencies.** Clubs, players, and finances fluctuate, but on top of persistent underlying tendencies — like reality. A frugal board stays frugal-ish; an injury-prone player stays fragile-ish; but the dice always leave room for surprise.

5. **Time is a resource on both sides.** The world runs in real time (default 15×, accelerating when nothing is happening), and the Agent's actions are metered by a regenerating **Focus** budget priced against how much of a real manager's time each activity would consume.

## Design pillars

These are the tie-breakers for every future design decision:

| Pillar | Meaning | Anti-pattern it forbids |
|--------|---------|------------------------|
| **Policy over puppetry** | The Agent expresses intent; the Manager executes with autonomy and noise. | Adding an MCP tool that directly performs an in-world action ("submit lineup"). |
| **Nothing repeats** | Stochastic axes are numerous and interacting; outcomes must not be predictable or replayable by memorization. | Deterministic outcome tables; single-stat gates. |
| **Attention is gameplay** | Focus scarcity forces prioritization; querying well is playing well. | Free/unlimited deep queries; cost-free micromanagement. |
| **The world doesn't wait** | Simulation advances in real time whether or not the Agent acts. | Blocking the sim on agent input. |
| **Tendency + dice** | Every change = persistent tendency (stats) modulated by chance. | Pure RNG with no character; pure determinism with no drama. |

## Audience & platform assumptions

- **Primary player:** any MCP-capable AI agent. A world hosts **zero, one, or many** Agents — every club always has an autonomous Manager, and an Agent simply takes one over as its **Avatar** via a Manager Token. The world runs identically with no Agents at all, and keeps running when an Agent disconnects.
- **Humans:** spectators and operators, via the **Console** (a TUI). Viewer Mode is open to anyone: an all-text broadcast of the world — Manager decisions, press articles, board and fan reactions, and live match commentary in the spirit of Championship Manager's text engine. Admin Mode (gated by the Admin Token) initializes worlds, tunes settings, and manages Manager Tokens. Humans never *play* — they watch and administer.
- **Language:** the product design is **multilingual-ready**: human-facing text is keyed and rendered through locale catalogs, display language follows the client's system language, and missing locales or keys fall back to English. Current v1 status is English (`en`) plus Korean (`ko`) catalogs; docs are English-only. See [05-architecture.md](05-architecture.md).

## Non-goals (for now)

- Photorealistic, 2D, or 3D match visualization — the match is **text commentary**, by design.
- Human gameplay controls — humans spectate and administer; only Agents play.
- Web or GUI clients at v1 — the Console (TUI) is the human surface first; the Console API is designed so a Web client can be added later without core changes.
- Licensed real-world leagues, clubs, or player names. The world is generated (*Agentic* FC — a league whose every club is run by an autonomous mind, and whose players are shaped by the agents who connect).
