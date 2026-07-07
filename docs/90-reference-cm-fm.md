# Reference: Championship Manager / Football Manager

Research reference on the CM/FM series — the genre ancestor of Agentic FC. This doc covers series history, the text match presentation (our most important heritage), pacing, and the **design lessons** we draw. Companion detail docs:

- [91-reference-fm-player-model.md](91-reference-fm-player-model.md) — attributes, personality, development, youth intake
- [92-reference-fm-club-systems.md](92-reference-fm-club-systems.md) — manager careers, board/fans, finances, transfers, scouting, tactics, dynamics, media, staff

## 1. Series lineage (condensed)

| Year | Milestone |
|------|-----------|
| 1992 | **Championship Manager** (Domark; Collyer brothers → Sports Interactive). All text/menus, generated player names, per-match & average player ratings from day one. |
| 1993 | CM 93: engine rewritten in C; **first real-player database** — the breakout. |
| 1995 | **CM2**: match shown one commentary line at a time; optional CD voice commentary (Clive Tyldesley) — quickly abandoned; the last audio commentary in the series. |
| 1999 | **CM3**: ground-up rebuild, massive attribute database, record sales. |
| 2001 | **CM: Season 01/02** — the beloved classic. Bosman frees, **attribute masking ("fog of war" scouting)**. Freeware since Dec 2008; community-patched to this day (champman0102). |
| 2003 | **CM4**: first 2D top-down match view (alongside text). |
| 2003–04 | **The split**: SI kept the code (database + match engine) → *Football Manager* under SEGA; Eidos kept the CM brand, which withered and died (services ended 2018). Lesson: **the engine and database were the product, not the brand.** |
| 2004–24 | FM era: team talks (FM06), Match Flow (FM08), **3D engine + decimal ratings + press conferences (FM09)**, touchline shouts (FM10), Dynamics (FM18), Club Vision (FM20), Supporter Confidence (FM23). FM24: 19M+ players, most-played entry ever. |
| 2025–26 | FM25 cancelled after Unity-rebuild delays; **FM26** (Nov 2025) ships on Unity. |

## 2. The text match engine (classic CM) — our most important heritage

### How it presented a match

- **One line of text at a time** in a single commentary box, **coloured by the team in possession** — an instant, glanceable "who's attacking" signal with zero graphics.
- **Event color/emphasis coding:** background flashes for cards; **goals in ALL CAPS with the box flashing rapidly**.
- **Variable line timing as drama** (the crucial insight): different lines display for different durations — a penalty is deliberately held on screen longer to build dread. Retrospective verdict: every chance "comes at the end of some kind of sequence of tension being ratcheted up and down… an exquisite sense of rhythm which makes every one feel like it was written rather than generated."
  **→ The drama came from the *pacing of text delivery*, not the text itself.**
- **Screen furniture** (CM 01/02): score, upward-counting match clock, possession bar, and tabs: Overview (goals/injuries/cards log), full commentary log, match stats, **live player ratings updating during the match**, latest scores from other grounds, your tactics (subs/changes), opponent tactics, and a **per-match commentary speed slider**.
- **Line vocabulary:** build-up passes, crosses, blocked shots, chances, saves, defenders "hacking the ball away", goals, cards, injuries, subs, penalty sequences. CM3 deliberately expanded commentary so managers could **diagnose their losses from key lines + stats** — text as tactical feedback channel, not just flavour.
- **No highlight filtering** in the classic era — an always-on "key moments + connective tissue" feed. The engine simulated everything; the text was a curated surface over it.

### Why CM 01/02 is beloved (retrospective consensus)

1. **Imagination is the renderer.** Text + stats let the player's head render the match; the 2D/3D engines are framed by retrospectives as *removing* this, not improving it.
2. **Speed.** ~**4 hours per season** in CM 01/02 vs 40+ in modern FM. "The right compromise between complexity and time per season."
3. **Mythology from the sim.** The legendary players (To Madeira, Tsigalko…) became folk heroes because stats + imagination create legends — no visuals required.

### Presentation evolution & the text mode that never died

- CM4 (2D, 2003) → FM09 (3D) → FM26 Unity "Broadcast Mode".
- **"Commentary Only" has remained an official mode ever since** — the ladder is Commentary Only → Key → Extended → Comprehensive → Full Match. The engine always simulates the full 90; the level only chooses what renders.
- FM26's **Dynamic Highlights** adapt clip frequency to match state (tight game near full-time → denser highlights). Note the convergence: even the 3D flagship is drifting back toward *editorially paced* presentation — what CM2 did with text line timing in 1995.
- FM26's **momentum graph + xG story** exist explicitly for managers watching fewer highlights — FM's answer to "convey game state without showing the game," which is exactly our problem.

## 3. In-match management (for our Manager's in-match decision surface)

- Classic CM: subs (3), formation/instruction changes anytime; the match waits while you fiddle. Purely mechanical.
- FM2006+: team talks (pre/HT/FT); FM2008 Match Flow (continuous matchday, no hard pause for tactics); FM2010 touchline shouts (one-click preset adjustments); modern FM: 5 subs in 3 windows, auto-pause at KO/HT/FT, per-player talk feedback.
- FM's matchday is a **pause-and-decide loop**: the sim streams highlights; managerial intervention freezes the world; three scripted pause points carry the psychological layer.
- **Contrast for us:** our world never pauses. The Manager makes these calls autonomously (rolled against Mindset + Tactical Plan); the Agent pre-loads intent instead of intervening live.

## 4. Match ratings & player state

- Ratings out of 10; classic CM used integers, FM09 introduced decimals. **Practical band is 6.0–8.0** (6.6–6.9 quiet, 7.0–7.4 solid, 7.5+ excellent, 9+ heroics). Event-driven: goals, assists, key actions, errors; position-weighted.
- **Condition** (energy %) drains in-match, faster at high tempo; low condition sharply raises injury odds (community test: 8 in-match injuries starting at 100% condition vs **87** at 60%, same sample). **Match sharpness** is a separate axis (FM21+); ~33% win-rate difference measured between 90% and 100% sharpness. Natural Fitness governs recovery.
- Momentum graphs (recent FMs) summarize "who's on top" continuously.

## 5. The Continue-button time model (what we're inverting)

- CM/FM is **a turn-based event queue wearing a calendar**: time advances only on "Continue"; the **inbox is the pacing engine** (news, scout reports, required responses gate the loop; matchdays are the anchor events).
- **Holiday mode** = built-in fast-forward (assistant runs everything under a policy; re-enter anytime). **Instant Result** exists officially only on Touch/Console — and is one of the most popular skin mods on desktop, which says something about demand for pace.
- Season length as a design axis: CM 01/02 ≈ 4h; modern FM ≈ 40h+; FM13's "FM Classic" mode targeted ~8–10h. Retrospectives repeatedly name CM 01/02's pace as *the* reason it's still played.
- **In CM/FM the world is frozen by default and the player spends time. In Agentic FC the world runs by default and the Agent spends attention (Focus).** FM's pacing tools map to real-time analogues: inbox → news feed; highlight levels → commentary depth; holiday mode/instant result → Adaptive Tempo; the pause-and-decide loop → Mindset pre-loading.

## 6. Design lessons for Agentic FC

> **Status: all lessons below have been folded into the design docs** (the "Where it lands" column links to the sections that now carry them). This table remains as the rationale record.

| # | CM/FM finding | Application to Agentic FC | Where it lands |
|---|---------------|--------------------------|----------------|
| L1 | Drama came from **variable text pacing** (CM2's held penalty line); FM26's Dynamic Highlights reconverge on editorial pacing | The Narrative Renderer must control **cadence**, not just content: line display duration and density as first-class outputs, tension ratcheting before chances | [05 Narrative Renderer](05-architecture.md), FR-35a |
| L2 | **CA/PA budget model**: attributes cost per-position weights inside a capped budget; decline redistributes freed budget into cheap mental attributes | Adopt a budget-constrained attribute system — gives us bounded growth, positional identity, and "aging gracefully" for free | [02 §2](02-game-design.md), [08](08-attributes.md) |
| L3 | **Consistency = x/25 matches at full ability** — one number modulating effective ability per event | Exactly our "tendency + dice" pillar in miniature; adopt per-match effective-attribute rolls driven by volatility-type hidden attributes | [03 §2](03-simulation-engine.md) |
| L4 | **Attribute masking**: ranges that narrow with scouting knowledge %; star ratings with explicit uncertainty bands, *relative to your squad* | Direct blueprint for FR-22 (hidden stays hidden): observation tools return **ranges/evidence whose precision scales with Focus invested** — knowledge literally costs attention | [04 §2](04-agent-interface.md) |
| L5 | Hidden personality is surfaced only as **derived descriptors** ("Model Professional"), never raw numbers | Same pattern for our evidence layer: expose descriptors and scout impressions derived from hidden attributes via thresholds | [04 §1](04-agent-interface.md) |
| L6 | **Board confidence → warning → ultimatum → sack** trajectory; club culture weighted nearly as heavily as results | Blueprint for our sacking pipeline (FR-14a) and club tendency system: staged, legible, escalating — never a surprise roll from nowhere | [02 §3.1](02-game-design.md) |
| L7 | Manager reputation (0–10,000) gates the job market; **badges vs. experience** are orthogonal manager stats; AI managers churn via the same rules + newgen managers | Manager entities need their own attribute set (reputation, coaching quality…) and the population needs newgen managers to replace retirees | [02 §3.1](02-game-design.md), FR-14c/e |
| L8 | The **inbox is the pacing engine**; holiday mode & Instant Result prove demand for compression; CM 01/02's 4h season is the beloved sweet spot | Validates Adaptive Tempo; gives us a tuning target — a season of Agentic FC should *feel* CM-fast to a spectator, not FM-slow | [02 §5](02-game-design.md) |
| L9 | Text as **tactical feedback channel** (CM3: diagnose losses from commentary + stats), match state without visuals (momentum, xG story) | Commentary and match summaries must carry diagnosable signal for the Agent, not just flavour — the Agent reads the same narrative humans enjoy | [02 §4](02-game-design.md), [04](04-agent-interface.md) |
| L10 | **Press conference repetition** is the series' longest-running complaint | Narrative variety is a real risk, not polish; budget template variety per event type from the start | NFR-8 |
| L11 | FM's **delegation matrix** proves running a club through standing policies + staff is a complete game loop | Our control model (Agent delegates *everything* to the Manager via Mindset) is a proven loop taken to its logical conclusion | [01 pillars](01-concept.md) |
| L12 | Ratings live in a **6.0–8.0 practical band**; condition/sharpness are separate axes with measured injury/performance effects | Adopt: narrow-band ratings read naturally in text; keep energy (short-term) and sharpness (medium-term) as distinct player state | [02 §2](02-game-design.md) |
| L13 | The 2003 split: **engine + database beat brand + interface** | Invest in the Simulation Core and world generation; interfaces (Console, even MCP schemas) are replaceable skins | [05](05-architecture.md) |

## Sources

Wikipedia (Championship Manager, Football Manager, CM2/CM4/FM2006/FM2009), pcgamesn.com & pcgamer.com series histories, superchartisland.com (CM2 presentation analysis), pcinvasion.com (CM3 review), GameFAQs CM 01/02 guides (match screen), techradar.com & arturararipe.nl & fmprojects.substack.com (CM 01/02 retrospectives), segaretro.org FM18 manual (highlight modes), footballmanagerblog.org & fmscout.com (FM26 matchday, FM09 3D), footballmanager.neoseeker.com (Match Flow), passion4fm.com (shouts), fmprojects/operationsports (ratings analysis), realsport101/fm-arena (condition & sharpness testing), footballmanager.fandom.com (holiday), SI forums (continue segmentation), videogamer.com (instant result mods), footballwhispers.com (CM 01/02 legends), steamcommunity.com (2D classic demand in FM26).
