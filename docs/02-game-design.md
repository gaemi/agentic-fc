# Game Design

Core systems as currently specified. Values marked *(placeholder)* are illustrative and subject to tuning.

## 1. The Manager & the Mindset

The **Manager** is an autonomous persona inside the simulation. It makes every club decision the moment the simulation calls for one: transfers, contract offers, lineups, substitutions, training emphasis, media responses, and so on. Decisions are **probability rolls**, not lookups — the Mindset skews the odds, sometimes overwhelmingly, but never scripts the outcome outright.

The **Agent's only lever is the Mindset** (plus the Tactical Plan). This is the game's central mechanic.

### 1.1 Mindset layers

The Mindset is layered from broad to narrow. Broad layers influence *everything a little*; narrow layers influence *specific things a lot*. The full typed schema — 10 Disposition axes, the Priority goal catalog, the 20-verb Directive catalog, and the Tactical Plan shape — is specified in [10-mindset-schema.md](10-mindset-schema.md).

| Layer | Scope | Examples | Change frequency (expected) |
|-------|-------|----------|------------------------------|
| **Disposition** | Global personality axes | Risk appetite, youth vs. proven bias, attacking vs. pragmatic identity, loyalty vs. ruthlessness, financial prudence | Rare — this is "who the Manager is" |
| **Priorities** | Ranked objectives | "1. Avoid relegation 2. Blood academy players 3. Cup run" | Per season / per phase |
| **Directives** | Concrete standing orders | "Sign player #4521 at any cost", "Never sell player #310 this window", "Rotate keeper in cup matches" | Tactical, as situations arise |

Design rules:

- **Layer interaction:** a Directive can override a Disposition for its specific target (a youth-skeptic Manager will still start the wonderkid if directed), but the Disposition colors *how* it's executed and everything adjacent.
- **Rolled, not obeyed:** each layer contributes weight to decision rolls. A strong Directive might take a decision from 20% to 95% — but the residual uncertainty is intentional and permanent.
- **Bounded size:** the count of simultaneously active Directives is limited — **initial value 15** *(tunable)* — so the Agent must curate, not enumerate.
- **Fixed schema:** Directives are structured intents from a typed catalog — target + verb + strength + optional conditions ("SIGN player:#4521 strength:must", "AVOID formation:back-three") — not free text. This keeps the mapping from Directive to roll weight deterministic and testable.

### 1.2 Tactical Plan

The strategy/tactics counterpart to the Mindset: formation plus six style dials (mentality, pressing, tempo, width, directness, counter). The Agent edits it through the same MCP surface; the Manager applies it — and *adapts it in-match* probabilistically (a losing Manager with high risk appetite gambles earlier, etc.).

### 1.3 Decision flow

```
World event needs a decision (e.g., transfer offer arrives)
        │
        ▼
Manager decision roll
  inputs: Mindset (all layers) + Tactical Plan (if relevant)
        + world state (finances, squad, table position…)
        + Manager's own noise (mood, recent results)
        │
        ▼
Outcome committed to world state → visible to Agent via MCP (as news/event)
```

The Manager's internal mood state (confidence, stress from results) feeds decision rolls as noise, and is exposed to the Agent only as **Descriptors** in free self-inspection (`get_mindset` context) — never as numbers, same pattern as all hidden state. The Agent cannot set mood directly; it moves with results and circumstances.

## 2. Players, staff & attributes

### 2.1 Attribute model (FM-inspired)

The concrete taxonomy lives in [08-attributes.md](08-attributes.md): outfielders carry **33 visible attributes**, goalkeepers carry **32 visible attributes**, weak-foot proficiency is costed and masked like visible ability, hidden traits surface only through descriptors/evidence, and the Ability Pool cost table is the balance ledger.

The match-system model is specified in [12-match-model.md](12-match-model.md): expanded attributes, public body facts as bounded expression modifiers, tactical chance-type distribution, event-specific contests, and calibration guardrails.

- **Visible Attributes:** 32–33 per player (1–20 scale) — see [08-attributes.md](08-attributes.md).
- **Budget-constrained ([90 L2](90-reference-cm-fm.md)):** attributes draw from a capped **Ability Pool**; each attribute point has a per-position cost (pace expensive everywhere; niche skills cheap). The pool grows toward a hidden, fixed **Potential Cap** and shrinks in decline. This gives bounded growth, positional identity, and meaningful trade-offs for free.
- **Hidden Attributes:** 19 per player plus positional familiarity ([08-attributes.md](08-attributes.md)), in four families:
  - **Potential** — the Potential Cap and the shape of the growth curve.
  - **Personality** — professionalism, ambition, temperament, loyalty…
  - **Volatility & durability** — consistency, big-match nerve, injury proneness.
  - **Trajectory meta-stats** — development speed, decline onset age, decline speed.
- **Chemistry / context modifiers:** effective ability shifts with teammates, Manager style fit, club stability. A player is not a fixed vector — he's a function of his environment.
- **Volatility modulation ([90 L3](90-reference-cm-fm.md)):** hidden volatility attributes modulate *effective* ability per event, FM-consistency-style ("plays to full ability in x out of N matches") — a legible, tunable mechanic that is our tendency + dice pillar in miniature. Big-match nerve applies the same mechanism to high-stakes fixtures only.

### 2.2 Attribute evolution

Attributes are not static; they are re-rolled over time by the simulation core (see [03-simulation-engine.md](03-simulation-engine.md)):

- Growth while young (rate ~ development speed, training, playing time, personality).
- Decline with age (onset and rate are per-player hidden attributes). **Physicals fall first; mentals hold or keep rising** — and Ability Pool capacity freed by physical decline can partially redistribute into cheaper mental attributes, so veterans age gracefully instead of collapsing uniformly ([90 L2](90-reference-cm-fm.md)).
- Shocks: injuries can dent physical attributes temporarily or permanently.
- Environment feeds back: a stagnating club, a toxic dressing room, or a perfect mentor changes trajectories.

**The loop:** attributes shape roll probabilities → rolls adjust attributes and schedule the next roll → repeat. Dice on top of stats, stats reshaped by dice.

### 2.3 Player state & match ratings ([90 L12](90-reference-cm-fm.md))

Two short-horizon state axes sit on top of attributes, kept deliberately distinct:

- **Condition** — energy. Drains during matches (faster at high intensity), recovers over days. Low Condition sharply raises injury risk, making rotation a real decision.
- **Sharpness** — match fitness. Built by playing, lost through inactivity; a fully rested but rusty player is not match-ready.

Every match performance produces a **player rating on a 10-point scale living almost entirely in a narrow practical band** (FM's 6.0–8.0): quiet, solid, excellent, heroic. Narrow-band ratings read naturally in text and accumulate into legible form/season averages.

Body profile is public match context, not an attribute pool sink. Height and
weight bias generation and modify how relevant skills express in specific event
types; they never replace football ability. The match model uses this rule most
strongly for crosses, set pieces, long balls, shielding, and physical duels
([12 §3-5](12-match-model.md)).

### 2.4 Careers & population dynamics

- Players age, decline, and **retire**.
- **Youth Intake** events inject new prospects into the world, keeping league population sustainable.
- **Staff are simplified at v1 (decision):** coaches, scouts, and physios exist with a small attribute set that feeds the relevant rolls (training quality, scouting accuracy, recovery speed), but without the full growth/decline machinery. Players get the deep treatment first; staff depth is a later expansion.
- **Managers churn too:** manager retirements are backfilled by newly generated managers entering the pool (the merry-go-round never runs dry). Retirement and vacancy rules: [§3.1](#31-manager-careers--the-job-market-cm-style).

## 3. Clubs & world simulation

- **Seeded sandbox, living timeline.** The seed fixes the opening world, not a
  script. The same seed creates the same league and starting conditions, but
  from kickoff onward every change is conditional on the current world state and
  the ordered Agent/admin inputs ([09 §1.1](09-world-generation.md)). Randomness
  supplies variation inside that causal frame; it does not ignore the frame.
- **Finances** rise and fall — sponsorship, gate receipts, prize money, wages, transfers — but always on top of a club's persistent financial tendency (a modest club can overachieve, but doesn't silently become an oil club).
- Board expectations, fan sentiment, and league reputation evolve the same way: tendency + dice.
- **Every club has an autonomous Manager** running on a predefined Mindset, using the exact same decision machinery. There is no special "AI club" logic — the only distinction is whether a Manager's Mindset is currently being edited by an Agent (making it that Agent's **Avatar**) or left as-is.
- **Agents are optional and plural.** A world can run with zero Agents (pure simulation), one, or several — each bound to a different Manager via that Manager's token. An Agent disconnecting changes nothing except that its Avatar's Mindset stops receiving edits.

### 3.1 Manager careers & the job market (CM-style)

Managers have careers of their own, independent of any one club:

- **Sacking exists — and it is staged, never a surprise ([90 L6](90-reference-cm-fm.md)).** Board confidence follows a legible escalation: confidence slips → private warning → public ultimatum ("X points from the next Y matches") → dismissal. Every step is observable (news feed, board statements), so an attentive Agent always sees it coming. The same machinery applies to every Manager in the world, producing a league-wide managerial merry-go-round.
- **Managers have attributes of their own ([90 L7](90-reference-cm-fm.md)):** reputation (gates which jobs will talk to you), track record, coaching quality, plus the Mindset. The Manager population also churns — managers retire and newly generated managers enter the pool.
- **Unemployment is a playable state.** A sacked Manager stays in the world. Clubs with vacancies may approach the Manager; the Manager may approach clubs. Whether and where they land is — like everything — rolled: reputation, track record, Mindset (ambition, patience), and club circumstances all weigh in. There is no interview mini-game at v1 — hiring resolves through these rolls. While unemployed, a Manager's observation narrows to **public information only** (league tables, results, news; no club-internal views) at normal Focus prices.
- **No club is ever unmanaged (invariant).** When a job falls vacant, an auto-generated **caretaker Manager** takes charge immediately — same decision machinery, modest attributes, a conservative archetype. The position stays open on the job market while the caretaker runs day-to-day; occasionally a caretaker earns the job permanently. `club.manager` is never null.
- **Managers retire.** Autonomous Managers roll retirement (age/career-driven), backfilled by newly generated managers. **Avatars are exempt** while their binding is live; the exemption lapses after **2 game-years with no session on the token**, after which the Manager is treated as autonomous again and may retire. A retired Manager's token enters a `RETIRED` state — see [04 §0](04-agent-interface.md).
- **The Agent rides the career, not the club.** The Manager Token binds the person. Getting sacked is a setback and a new chapter, never a game over; the Agent shapes the job hunt through the same Mindset surface (e.g. a Directive: "hold out for a top-division job" or "take any club that calls").
- Club changes triggered this way apply to autonomous Managers too — vacancies get filled, causing chains of movement across the league.

## 4. Narrative layer (all-text presentation)

Everything observable is rendered as **text**, in the voice of Championship Manager's classic text engine:

- **News items:** what a Manager decided, transfers agreed, injuries, milestones.
- **Press articles:** journalists react to results, streaks, controversies. A
  completed matchday produces round-up articles that group results, table
  movement, and pressure points into a longer newspaper-style story instead of
  filing one tiny item per kick-off or final whistle. Kick-off signals remain
  live operational events rather than media articles; Agents that need kickoff
  wakeups should use `MATCH` alerts, while `NEWS` alerts for match coverage now
  arrive with the full-time round-up.
- **Board & fan reactions:** sentiment expressed as quotes/statements, not bars.
- **Live match commentary:** substitutions, chances, a winger skinning his man, the keeper's blunder — narrated minute by minute like a text commentator.

The narrative layer is *presentation over the event feed*: simulation events go in, human-readable lines come out. Two consumers see it: the Console (humans spectating) and, in condensed form, the Agent via `get_news`. All templates flow through message keys (i18n extension point).

Three design rules, drawn from what made classic CM work ([90 §2, L1/L9/L10](90-reference-cm-fm.md)):

1. **Cadence is a first-class output.** Classic CM's drama came from *pacing* — a penalty line held on screen to build dread, tension ratcheted before a chance. The Narrative Renderer emits not just lines but **display timing and density**: dramatic moments linger, routine passages compress. (Even FM26's Dynamic Highlights reconverged on this.)
2. **Text carries diagnosable signal, not just flavour.** CM3 deliberately wrote commentary managers could *learn from* ("why are we losing?"). Our commentary and summaries must let both the spectator and the Agent read tactical cause from the text and accompanying stats — the Agent consumes the same narrative humans enjoy.
3. **Variety is budgeted from day one.** FM's longest-running complaint is repetitive press conferences. Template variety per event type is a real quality bar, not post-launch polish. Match commentary uses event-family template pools (crosses, cut-backs, counters, set pieces, scrambles, long shots, quiet phases) so identical tactical situations do not read as the same line every time.

Localization must preserve that polish for generated names. Korean templates do
not attach a fixed batchim-sensitive particle directly to a person or club
placeholder: use a stable role noun such as `선수`, `팀`, or `감독`, or rewrite
the sentence so Latin and mixed-script names remain grammatical. For example,
`{player}가 득점` is unsafe; `{player} 선수가 득점` is stable.

## 5. Time system

### 5.1 Game Speed (chosen at setup)

- Base ratio default: **15×** — 1 game day = 96 real minutes.
- Multiple tiers must be offered, both slower and faster *(placeholder tier set: 5× / 15× / 30× / 60×)*.

### 5.2 Adaptive Tempo

Real-time does not mean uniform speed:

| Window | Tempo | Example (base 15×) |
|--------|-------|--------------------|
| **Match window** | Base Game Speed | 15× — a full match window (~2 game-hours incl. half-time and stoppages) ≈ 8 real minutes |
| **In-season idle window** (between fixtures) | Accelerated multiple of base | default 240× — a matchless day passes in 6 real minutes |
| **Off-season window** (before the first fixture / after the final fixture) | Larger accelerated multiple of base | default 1440× — a fixtureless day passes in 1 real minute |

**Match window scope (decision):** match windows are **league-wide match days** — whenever any fixture in the world is in play, the world runs at base speed, and all fixtures of that match day run concurrently (viewers can hop between grounds; the other-scores ticker stays coherent). The same rule applies in zero-agent worlds. The default run profile keeps matches readable but accelerates dead time: **idle acceleration: 16× base** (240× at the default 15×) and **off-season acceleration: 96× base** (1440× at the default 15×), tunable against NFR-9's pace target.

Run profiles are launch presets for new worlds. **default** = 15× match / 16× idle / 96× off-season; **fast** = 30× / 32× / 192×; **slow** = 15× / 6× / 36×; **custom** starts from default and expects explicit speed overrides. After creation, Admin Mode can adjust runtime pacing (`Game Speed`, idle acceleration, off-season acceleration) live; this changes wall-clock delay only, not game-time event ordering or seeded outcomes.

**Tuning target ([90 L8](90-reference-cm-fm.md)):** in a real-time world the CM lesson translates to **density, not total duration** — no dead air between meaningful events. Match windows should remain readable; in-season gaps should move briskly; fixtureless off-season stretches should clear very quickly. Operators trade wall-clock for immersion via the Game Speed and acceleration settings; what Adaptive Tempo guarantees is that spectating never feels FM-slow at any tier.

### 5.3 Scheduling, not ticking

Nothing is evaluated every tick. Each entity's roll schedules its **next** roll (hours or weeks of game time away, depending on what happened). The world is a priority queue of future events, drained in time order. Details in [03-simulation-engine.md](03-simulation-engine.md).

## 6. Game setup (initialization)

Worlds are created and configured from the **Console in Admin Mode**. The full specification — operator config, derived structure, and the seeded generation pipeline — lives in [09-world-generation.md](09-world-generation.md). Summary of what the operator chooses:

| Parameter | Notes |
|-----------|-------|
| **League shape** | Divisions (1–5) × clubs per division (8–24, default 16), with one-key presets (Compact/Classic/Deep/Sprawling). Promotion/relegation slots derived (~15% of division size, min 2). |
| **Run profile & Game Speed** | default/fast/slow/custom profile, with Game Speed tiers 5× / 15× / 30× / 60× and per-tempo acceleration overrides. |
| **World quality & Economy scale** | Presets scaling talent bands and all money in the world. |
| **World seed** | For reproducible world generation. |
| **Name culture mix** | Weights over the four cultures (default 40/25/25/10). |
| **Display names** | Optional world display name plus club and manager display-name overrides. Club/manager lists may be partial; unspecified slots keep generated names. |
| **Competitions (v1 decision)** | Fixed structure: the league pyramid + **one national cup** (all divisions, knockout) + **two transfer windows** (summer/winter). Continental competitions are a later expansion. |

**World scope (v1 decision):** the generated pyramid **is the whole universe** — no foreign leagues, no international duty. Players enter via Youth Intake and leave via retirement only. The world-generation layer must keep an extension seam for external leagues later, but v1 simulates a closed ecosystem.

At world creation the core issues one **Manager Token** per generated Manager; Managers spawned later (caretakers, newgen backfills) get tokens at creation. The **Admin Token** already exists from daemon first launch. An Agent binds to a Manager by presenting its token over MCP ([04-agent-interface.md](04-agent-interface.md)); all tokens are viewable from the Console in Admin Mode.

All names (players, clubs, competitions) are procedurally generated — no real-world licenses.
