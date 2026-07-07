# Simulation Engine

The Simulation Core is the authoritative, real-time-paced, probability-driven engine that advances the world. Its defining pattern is **roll-and-reschedule** over a discrete-event queue.

## 1. Roll-and-Reschedule

The engine is a **discrete-event simulation (DES)**, not a fixed-tick loop:

```
┌─────────────────────────────────────────────────────┐
│                  Event Queue (by game time)          │
│  [t+2h: Player A stat roll] [t+6h: finance roll] …  │
└──────────────────────┬──────────────────────────────┘
                       │  pop next due event (paced by
                       │  Game Speed / Adaptive Tempo)
                       ▼
                 ┌──────────┐
                 │   ROLL   │  inputs: entity attributes,
                 └────┬─────┘  environment, Mindset (if a
                      │        Manager decision)
        ┌─────────────┼─────────────────┐
        ▼             ▼                 ▼
  mutate state   emit events      SCHEDULE next roll
  (attributes,   (news, injury,   for this entity
   finances…)    decision made)   (interval is itself
                                   an outcome of the roll)
```

Key properties:

- **Sparse evaluation.** An entity is only touched when its scheduled event comes due. A stable veteran might be rolled every few game-weeks; a volatile teenager in a crisis club, every few game-days.
- **Self-scheduling.** Each roll's outputs include *when this entity rolls next*. Turbulent outcomes shorten the interval; calm ones lengthen it.
- **Cascade.** A roll may enqueue events for *other* entities (an injury roll for Player A enqueues a Manager decision event: "replace him in the XI?").
- **State-conditioned evolution.** The event label and RNG seed provide
  reproducibility, but the current world state provides meaning. A transfer
  roll, injury roll, media story, development tick, or board reaction must read
  the relevant current facts before choosing an outcome.

## 2. The probability model

Every roll follows the same shape:

```
P(outcome) = f( base rate,
                subject attributes (visible + hidden),
                environment modifiers (club, teammates, league…),
                Mindset weights (Manager decisions only),
                temporal context (season phase, fixture congestion…) )
```

Design constraints:

1. **Many axes, small weights.** No single stat should dominate an outcome. Unpredictability emerges from the *interaction* of many modest modifiers — this is what guarantees "the same thing never happens twice."
2. **Feedback loop.** Roll outcomes mutate attributes; mutated attributes change future roll probabilities *and* future roll timing. The system is a stochastic dynamical system, not a lookup table.
3. **Tendency + dice.** Persistent attributes are the tendency; the roll is the dice. Both are always present. (Pillar: no pure RNG, no pure determinism.)
4. **Legible modulation over opaque noise ([90 L3](90-reference-cm-fm.md)).** Where variance is applied, prefer simple, tunable mechanics in the shape of FM's consistency model ("full ability in x out of N matches") over unstructured noise injection. Effective attributes are derived per event from base attributes × volatility rolls × context — each factor individually explainable in the roll audit trail.
5. **No context-free mutation.** Randomness cannot directly teleport the world
   to an unrelated state. It may only choose among outcomes made plausible by
   the current state: a cash-poor club cannot randomly buy like a rich one; a
   player without minutes should not develop as if he is starting every week; a
   press rumour should point to an actual contract, vacancy, squad need, agent
   directive, or recent event.

## 3. Event taxonomy (initial)

| Category | Examples | Typical reschedule horizon |
|----------|----------|---------------------------|
| **Attribute drift** | Growth, decline, form/sharpness changes | days–weeks |
| **Condition** | Injury onset, recovery progress, fatigue | hours–days |
| **Career** | Retirement decisions, youth intake, contract expiry | weeks–months |
| **Club** | Finance updates, board mood, fan sentiment | days–weeks |
| **Manager decision points** | Transfer responses, lineup selection, contract offers | triggered by other events + fixture calendar |
| **Match** | Kickoff, in-match event stream, full-time result | fixture calendar |
| **World** | Season rollover, transfer window open/close, awards | calendar |

**Match engine granularity (decision):** the match engine uses **key-moment sampling**, not a continuous minute-by-minute physical simulation. It rolls a sequence of significant passages (chances, momentum shifts, cards, injuries, set pieces, Manager decision points) whose density and character derive from tactics, player effective attributes, and match state — enough resolution to (a) produce commentary-grade event streams with connective tissue lines, (b) drive per-player ratings, condition drain, and stats, and (c) give the Manager in-match decision events. Classic CM proved the text surface needs curated moments, not physics ([90 §2](90-reference-cm-fm.md)).

The next match-engine rebuild keeps key-moment sampling but replaces the
generic "chance" shortcut with an event grammar: tactics and personnel choose
the event family (cross, cutback, through ball, counter, set piece, scramble,
etc.), then event-specific contests resolve it. The normative model is
[12-match-model.md](12-match-model.md).

## 4. Time integration

- The event queue is ordered by **game time**; the pacer converts game time to real time using the current **Adaptive Tempo** (match window → base Game Speed; in-season idle window → accelerated; off-season window → heavily accelerated).
- Changing tempo never changes *what* happens — only how fast the queue drains in real-world terms. Simulation outcomes must be independent of the chosen Game Speed.

## 5. Determinism & reproducibility

Settled ([05 A3](05-architecture.md)) — for testability, debugging, and agent-scale exercise:

- All randomness flows through a **seeded RNG**, with per-entity or per-stream sub-seeds so event *ordering* doesn't perturb unrelated outcomes.
- **Replay contract:** a run is defined by *(seed, world config, ordered external-input log)*. The log contains **every accepted MCP tool call — including free and read-only calls — plus every admin operation**: reads spend Focus and `get_news` advances per-session cursors, so mutations alone cannot reproduce a run. Each entry records session id, tool, params, result/error, Focus charge, game time, and a monotonic **`ingress_seq`**; replay re-applies the log by `(game_time, ingress_seq)` — never by wall-clock arrival.
- **Seed is not fate:** see [09 §1.1](09-world-generation.md) for the generated-sandbox contract. The seed fixes the initial condition; the input log and state-conditioned rolls produce the lived timeline.
- **Queue determinism:** events due at the same game time drain in a total order — `(game_time, priority class, entity kind, entity id, schedule seq)`. Priority classes drain **World < Match < Decision < Condition < Drift**; entity kinds **World < Match < Club < Manager < Player**. Tempo changes and pauses re-pace the queue but never reorder it.
- Every roll is **logged** (inputs, weights, outcome, next-roll schedule) to an audit trail — invaluable for balancing, and a candidate data source for "why did this happen?" style agent queries later.

## 6. Performance envelope *(early, to be validated)*

- World size: up to a few thousand players (league scale dependent) × dozens of clubs.
- Sparse scheduling keeps steady-state event throughput low (roughly: total entities / average reschedule interval), so the engine should comfortably sustain the maximum configurable tempo (60× base × 64× idle = 3840× effective; 60× base × 240× off-season = 14400× effective — NFR-4) on commodity hardware. Match windows are the densest periods.
- The engine must run **headless and unattended** — the world doesn't wait for the Agent.
