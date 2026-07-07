# World Generation & Configuration

What gets decided at world creation, by whom (operator config vs. derivation vs. seeded rolls), and the generation pipeline. Runs from the Console's **World Init wizard** (Admin Mode). All numeric defaults are initial values, tunable.

## 1. The three layers of world creation

| Layer | Decided by | Examples |
|-------|-----------|----------|
| **Configured** | The operator, in the init wizard | League shape, run profile, initial game speed, world quality, name culture mix |
| **Derived** | Deterministic rules over the config | Promotion/relegation slots, season calendar, cup bracket, budget bands, board objectives |
| **Rolled** | The seeded RNG | Names, club tendencies, managers, squads, fixtures, rivalries |

Same config + same seed ⇒ identical world (NFR-2).

### 1.1 Seeded living-world contract

World generation fixes the **initial condition**, not a prewritten timeline. A
seeded Agentic FC world should feel like a generated sandbox: the same seed and
config create the same clubs, squads, managers, calendar, rivalries, economy,
and opening queue, but the world then keeps changing through state-dependent
simulation.

After generation, every future state is a deterministic function of:

```
current world state
+ current event queue
+ ordered external input log
+ labelled RNG streams for due rolls
```

This means:

- same seed + config + same ordered inputs ⇒ identical history;
- same seed + config + different Agent choices/admin operations ⇒ a divergent
  but still reproducible history;
- a roll never ignores the world around it. Transfers depend on squad shape,
  budgets, contracts, reputation, manager mindset, and market state; media
  stories depend on actual appointments, form, injuries, and rumours; player
  development depends on age, facilities, minutes, injuries, personality, and
  current ability.

The seed is therefore a world-root, not fate. The timeline is earned by the
state transitions that happen inside the generated world.

## 2. Operator configuration (the World Config)

### 2.1 Core settings (wizard page 1)

| Setting | Type / range | Default | Notes |
|---------|-------------|---------|-------|
| World name | string | generated (e.g. "Alderton League") | Display only |
| Seed | uint64 | random | Shown after creation for reproducibility |
| **Divisions** | 1–5 | 2 | The pyramid depth |
| **Clubs per division** | 8–24 (even) | 16 | Uniform across divisions at v1 |
| **Run profile** | default / fast / slow / custom | default | Launch preset for match, idle, and off-season pacing |
| **Game Speed** | 5× / 15× / 30× / 60× *(tier set now fixed)* | 15× | Initial real:game ratio during match windows; Admin Settings may adjust runtime pacing later without changing world generation. |
| **World quality** | Amateur / Semi-Pro / Professional / Elite | Professional | Scales the Ability Pool bands of every division (§4.2) |
| **Economy scale** | Austerity / Standard / Flush | Standard | Scales all money in the world (budgets, wages, fees) |
| Custom club names | ordered list, max club count | none | Optional display-name overrides; applies to clubs in tier-by-tier generation order, partial lists allowed |
| Custom manager names | ordered list, max clubs + unemployed pool | none | Optional display-name overrides; applies to club managers in club order, then unemployed pool, partial lists allowed |

Presets for one-key setup: **Compact** (1×12), **Classic** (2×16), **Deep** (3×16), **Sprawling** (4×20).

### 2.2 Advanced settings (wizard page 2, defaults fine)

| Setting | Type / range | Default | Notes |
|---------|-------------|---------|-------|
| Name culture mix | 4 weights, sum 100 | Anglo 40 / Latin 25 / Continental 25 / East Asian 10 | Applies to player, manager, club, place names |
| Idle acceleration | 2×–64× of base | 16× | In-season Adaptive Tempo fast-forward factor |
| Off-season acceleration | 2×–240× of base | 96× | Fixtureless pre/post-season fast-forward factor |
| Squad size target | 20–30 | 24 | Generation target; in-play squads may drift within min/max rules |
| Youth intake batch | 3–8 per club per season | 5 | Population sustain rate |
| Start state | ready / running | **ready** | "ready" = world fully generated but clock stopped, so the operator can distribute Manager Tokens before kickoff; Admin Mode issues `start` |

Custom name lists are deterministic inputs. Empty names, duplicates within the
same list, names longer than 64 characters, or lists longer than the generated
entity count are rejected before generation. Providing only a few names is valid:
the supplied names fill the first slots and the remaining clubs/managers keep
generated names. Generated identities still roll all supporting facts
(culture, region, stadium, archetype, reputation) from the seeded pipeline.
Club short names are derived from the override display name after common football
tokens such as `FC`, `AFC`, and `United` are ignored.

### 2.3 Daemon config (not part of the world — launch flags/file)

Data directory, MCP bind address/port, Console API bind address/port, backup cadence. Changing these never touches world state.

### 2.4 Fixed at v1 (not configurable)

Competition structure (league + one national cup + two windows — FR-4a), closed ecosystem (FR-4b), currency, calendar shape, Focus economy constants, Directive cap.

## 3. Derived structure

Deterministic functions of the config — no dice involved:

- **Promotion/relegation:** `slots = max(2, round(clubs_per_division × 0.15))` between adjacent divisions (16-club divisions → 2 up / 2 down, automatic; no playoffs at v1). Top division promotes nobody in; bottom division relegates nobody out.
- **Season calendar** (Gregorian-like, for familiarity):
  - **Season year:** July 1 → June 30. League rounds = `2 × (clubs_per_division − 1)`, played on weekly match days from mid-August; winter adds midweek congestion if rounds demand it.
  - **National cup:** single-elimination, all clubs, seeded entry (lower divisions enter earlier rounds); rounds on midweek dates roughly monthly.
  - **Transfer windows:** June 15 – August 31 (summer), January 1 – 31 (winter).
  - **Youth intake day:** one per club, rolled within a fixed spring window (March–April), per club.
  - **Off-season:** June — board reviews, contract expiries, retirements announced.
- **Match days:** all fixtures of a match day kick off **simultaneously** (classic 3pm feel — keeps the other-grounds ticker coherent; viewers hop between grounds mid-match), consistent with league-wide match windows (FR-6a).
- **Division economy bands:** division d's revenue/budget band = top-division band × `decay^(d−1)` (decay ≈ 0.45), all scaled by Economy scale.
- **Media predictions & board objectives:** after squads are generated, each club's predicted finish = rank of squad strength within its division (with small noise); the board objective derives from prediction adjusted by Board Ambition (§4.3), and sets the season's confidence baseline.

## 4. Generation pipeline (seeded rolls)

Stages run in order; each stage consumes its own RNG stream (stream-split — see [03 §5](03-simulation-engine.md)) so a change in one stage never perturbs another.

```
0 validate config → derive structure (§3)
1 world skeleton   calendar, competitions, region map
2 clubs            names, colors, stadiums, tendencies
3 managers         one per club + unemployed pool, Mindset archetypes
4 players          squads, attributes, contracts; free agents; youth
5 history seeding  last-season table, rivalries
6 schedule         fixture lists, cup draw
7 economy init     balances, wage bills, first budgets
8 credentials      Manager Tokens for the generated pool, world manifest
                   (Admin Token already exists — daemon first launch,
                   [05 A7]; later-spawned Managers get tokens at spawn)
9 queue priming    first rolls staggered for every entity; kickoff events
→ world enters "ready" (or "running") state
```

### 4.1 Clubs (stage 2)

Each club rolls:

- **Identity:** name (culture from mix; pattern pools like "«Place» Athletic", "Real «Place»", "«Place» 1899"…), short name, colors (TUI-safe palette, clash-checked within division), stadium name + capacity (division band × wealth), region tag (from a generated region map — feeds rivalries and place names).
- **Tendencies (1–20 unless noted), the club's persistent character:**

| Tendency | Drives |
|----------|--------|
| Wealth | Budget percentile within division band; benefactor odds |
| Board Patience | Confidence decay rate; ultimatum threshold |
| Board Ambition | Objective adjustment; spending appetite |
| Fan Patience | Fan mood decay |
| Fan Passion | Attendance sensitivity, pressure amplitude, derby weight |
| Youth Emphasis | Board's youth-pipeline expectations; intake investment |
| Training Facilities | Development rolls |
| Youth Facilities | Intake quality rolls |

### 4.2 Player generation (stage 4) & World quality

Per club: squad of ~24 on a position template (**GK 3 / DF 8 / MF 8 / FW 5**), ages 17–35 (weighted to 22–29).

- **Ability Pool** sampled from the division band; the sample is the spending budget — the *stored* pool is the materialized spend, so `pool == round(ProfilePoolCost)` holds from generation on. **World quality** shifts all bands:

| Quality | Division 1 pool band | Each lower division |
|---------|---------------------|---------------------|
| Amateur | 40–90 | −15 |
| Semi-Pro | 60–110 | −18 |
| Professional | 90–150 | −22 |
| Elite | 120–180 | −25 |

- **Potential Cap** = current pool + headroom that shrinks with age (17-year-old: up to +60; 30-year-old: ≈ 0), so every squad carries a few genuine prospects.
- **Visible attributes + weak foot:** the pool is spent through the cost table ([08 §4](08-attributes.md)) according to a rolled **player archetype** per position (e.g. FW: poacher / target man / wide speedster) plus noise — archetypes create legible identities instead of uniform stat mush.
- **Hidden attributes:** personality rolled on soft bell curves; volatility/durability independent; trajectory correlated with archetype (speedsters decline earlier).
- **Contracts:** length 1–4y (young prospects longer), wage = f(pool, division band, Reputation, Ambition noise); a few clubs roll a marquee earner.
- **Free agents:** ≈ 8% of world population, biased older/flawed.
- **Initial youth:** each club starts with 3–5 academy prospects (ages 15–17) so the first intake day isn't the world's first youth.

### 4.3 Managers & Mindset archetypes (stage 3)

One Manager per club + an unemployed pool (≈ 10% of club count, min 2). Each rolls: name/age (33–68), Reputation (division-scaled), Coaching, Man-Management ([08 §6](08-attributes.md)) — and a **predefined Mindset from an archetype**, which is what autonomous Managers run on until/unless an Agent takes over:

| Archetype | Disposition sketch |
|-----------|--------------------|
| The Idealist | Attacking, youth-first, loyal, patient |
| The Pragmatist | Results-first, flexible, risk-averse |
| The Firefighter | Survival specialist; defensive, short-horizon |
| The Trader | Market-driven; buys low, sells anyone |
| The Professor | System-obsessed; stubborn tactics, analytical recruitment |
| The Motivator | Man-management first; chemistry over talent |
| The Tyrant | Discipline, high demands, volatile relationships |
| The Gambler | High risk in every dimension |

Archetype assignment is weighted by club character (a high-Youth-Emphasis club more often employs an Idealist). Archetypes set Disposition axes + a Priorities template; Directives start empty.

### 4.4 History seeding (stage 5) — deliberately thin

- **Last-season table** per division (plausible standings consistent with squad strengths, with upsets) — seeds media predictions, board mood, and "form" narrative so day one doesn't feel blank.
- **Rivalries:** 1–2 per club, derived from shared region + last-season proximity; rivalry weight feeds Fan Passion effects and derby fixtures.
- No deeper fake history at v1 (no honours lists, no legendary ex-players). The world's mythology should be *earned in play* — that's the CM lesson ([90 §2](90-reference-cm-fm.md)).

### 4.5 Currency

Fictional currency: **Crowns**, notation `cr` — "cr2.4M", "cr180k/wk". Stored as integer minor units; rendering via the locale layer (NFR-5).

## 5. Planned init wizard flow (Console, Admin Mode)

World bootstrap currently exposes these settings through daemon CLI flags and
`WorldConfig`. A future Console Admin Mode wizard should mirror the same
contract. It authenticates with the **Admin Token issued at daemon first
launch** (printed to the daemon's output) — Admin Mode therefore works before
any world exists.

```
1 Core settings     preset pick or custom (§2.1)
2 Names             optional world, club, and manager display-name overrides
3 Advanced          culture mix, tempo, squad/youth sizes, start state (§2.2)
4 Review            full config + derivations preview (divisions, calendar,
                    pro/rel, budget bands) before any generation
5 Generate          pipeline §4 with stage progress; abort = nothing written
6 Handover          shows world manifest: seed, Manager list
                    with club, archetype, Reputation — and Manager Tokens
                    (copyable). Ready worlds sit stopped until `start`;
                    worlds configured `running` begin immediately.
```

Tier behavior per [07 §4.5](07-console-design.md): S = one step per screen; M+ adds live summary pane.

## 6. Current Limitations

- Whether divisions can have differing sizes.
- Region generation is intentionally abstract. Regions feed rivalries and place
  names, not a map with geometry.
- Name pools and Manager archetype tables are implementation data in
  `internal/worldgen`; their balance can evolve without changing the public
  generation contract.
