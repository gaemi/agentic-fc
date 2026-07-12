# Mindset Schema (v1)

The concrete, typed schema of the Mindset and Tactical Plan — the Agent's entire write surface (FR-15, FR-16a). Every element maps deterministically to roll weights; nothing here is free text. Numeric values are initial and tunable.

## 1. Layer overview

```
Mindset
├── Disposition   10 bipolar axes, −10…+10        (who the Manager is)
├── Priorities    ranked objectives, max 5         (what matters this phase)
└── Directives    typed standing orders, max 15    (specific instructions)
Tactical Plan     formation + dials + constraints  (how the team plays)
```

A **version counter** increments on every accepted change; each Manager decision in the audit trail records the Mindset version it rolled under — "why did this happen" is always answerable.

## 2. Disposition — 10 axes

Integer **−10 … +10** per axis (0 = neutral). Named poles keep signs readable. Archetypes ([09 §4.3](09-world-generation.md)) are presets over these axes.

| # | Axis | −10 pole | +10 pole | Chiefly weighs on |
|---|------|----------|----------|-------------------|
| D1 | Risk Appetite | cautious | daring | In-match gambles, transfer punts, rotation risks |
| D2 | Youth Preference | proven veterans | youth-first | Selection, signings, development attention |
| D3 | Playing Identity | pragmatic | expressive | Tactical style bias, fan/board identity interplay |
| D4 | Financial Prudence | spendthrift | frugal | Budget usage, wage discipline, sell decisions |
| D5 | Loyalty | ruthless | loyal | Squad churn, selling favourites, keeping staff |
| D6 | Discipline | laissez-faire | authoritarian | Misconduct response, squad hierarchy handling |
| D7 | Horizon | this weekend | five-year project | Development vs. results, panic thresholds |
| D8 | Media Posture | guarded | provocative | Press tone, mind games |
| D9 | Tactical Flexibility | system purist | chameleon | In-match adaptation, opponent-specific tweaks |
| D10 | Personal Ambition | content | ladder-climbing | Job market behaviour, big-club approaches |

**Change model — drift, not teleport**: `update_disposition` sets **target values**. Deltas ≤ 2 apply immediately; larger deltas drift at ~2 points per game-week. Decision rolls always consume the **current** (drifting) value — only the target updates instantly (the defined exception to FR-18's immediate-effect rule). Reshaping who the Manager is takes time, prevents whiplash play, and gives the news feed a story ("the gaffer seems to be loosening the purse strings"). `get_mindset` shows current + target + drift ETA.

## 3. Priorities — ranked objectives, max 5

Ordered list; rank sets weight: **1.0 / 0.6 / 0.4 / 0.25 / 0.15**. Conflicts between priorities resolve by rank. Schema: `{rank, goal, params?}`.

Goal catalog (v1):

| Goal | Params | Example |
|------|--------|---------|
| `AVOID_RELEGATION` | — | Survival first |
| `WIN_LEAGUE` | — | Title push |
| `WIN_PROMOTION` | — | Go up |
| `FINISH_TOP_N` | n | Top-half/European-style targets |
| `CUP_RUN` | min_round \| WIN | "Reach the semis" |
| `DEVELOP_YOUTH` | age_cap (default U21), minutes_share | Blood the academy |
| `FINANCIAL_HEALTH` | — | Stay in budget, trim wages |
| `BUILD_SQUAD_VALUE` | — | Buy low, sell high |
| `ESTABLISH_IDENTITY` | style_ref (Tactical Plan) | Entertain / impose the system |
| `PROTECT_JOB` | — | Manager self-preservation mode |
| `FIND_JOB` | tier_min? | Unemployed / angling for a move |

`set_priorities` replaces the whole list (validated: ≤ 5, no duplicate goals).

## 4. Directives — typed catalog, max 15 active

```
Directive {
  id:         assigned by the engine
  verb:       from the catalog below
  target:     typed reference (validated per verb)
  strength:   LEAN | INSIST | ABSOLUTE
  conditions: optional guards
  expiry:     optional (game date | end of window | end of season)
}
```

### 4.1 Strength semantics

| Strength | Meaning | Weight effect *(placeholder)* | Focus cost *(placeholder)* |
|----------|---------|-------------------------------|---------------------------|
| `LEAN` | Tilts close calls | ~2× odds shift | 6 FP |
| `INSIST` | Overrides most other considerations | ~6× | 10 FP |
| `ABSOLUTE` | Only extreme circumstances override (~95% compliance region) | ~20× | 18 FP |

Never certainty — even `ABSOLUTE` loses to a red-card-and-two-injuries kind of day. (Pillar: rolled, not obeyed.)

### 4.2 Verb catalog (20 verbs)

**Selection & squad**

| Verb | Target | Notes |
|------|--------|-------|
| `START` | player | Optional condition: competition |
| `BENCH` | player | Keep in squad, out of XI |
| `EXCLUDE` | player | Out of matchday squad entirely |
| `ROTATE` | position group | Condition typically competition=cup |
| `GIVE_MINUTES` | player \| age group | Params: minutes/week target |
| `CAPTAIN` | player | |

**Transfers**

| Verb | Target | Notes |
|------|--------|-------|
| `SIGN` | player | Params: max_fee?, deadline (window) |
| `SELL` | player | Params: min_fee? |
| `KEEP` | player | Refuse offers |
| `LOAN_OUT` | player | Params: preference (playing time / division tier) |
| `TARGET_PROFILE` | position | Recruitment focus: attribute emphasis, age band, budget band |

**Contracts**

| Verb | Target | Notes |
|------|--------|-------|
| `RENEW` | player | Params: wage_ceiling? |
| `RELEASE` | player | |
| `WAGE_CAP` | scope (new signings \| renewals \| all) | Params: amount |

**Development**

| Verb | Target | Notes |
|------|--------|-------|
| `DEVELOP` | player | Params: focus (visible attribute \| position craft) |
| `RETRAIN_POSITION` | player | Params: position |

**Tactics guard**

| Verb | Target | Notes |
|------|--------|-------|
| `FORBID` | tactical element (formation \| style dial value) | Constrains the Manager's autonomous in-match/weekly adaptation ("never a back three"). Style fences use the `scope` field with the **`dial:VALUE`** convention (e.g. `pressing:HIGH`, `tempo:SLOW`). |

**Career & board**

| Verb | Target | Notes |
|------|--------|-------|
| `PURSUE_JOB` | club \| division tier \| ANY | Job market ([02 §3.1](02-game-design.md)) |
| `REJECT_JOB` | club \| division tier | |
| `PUSH_BOARD` | request (budget \| training facilities \| youth facilities) | Manager lobbies the board |

### 4.3 Validation rules

- Target type checked per verb; dangling references (sold player) auto-expire the Directive with a news item.
- **Direct contradictions are rejected**, not merged (`START X` vs `EXCLUDE X`): the add fails with the conflicting Directive's id. The Agent must remove first — the Mindset never contains ambiguity.
- 15-active cap (FR-19): adding #16 fails with the full active list.
- Every accepted/rejected/expired Directive is a feed event (observable via `get_news`).

## 5. Tactical Plan (v1 — deliberately small)

```
TacticalPlan {
  formation:   from catalog (~12 shapes: 4-4-2, 4-3-3, 4-2-3-1, 3-5-2, 5-3-2, …)
               — drives matchday selection: the shape's bands set the XI's
               defender/midfielder/forward slot counts (docs/12 §6)
  mentality:   VERY_DEFENSIVE | DEFENSIVE | BALANCED | ATTACKING | VERY_ATTACKING
  pressing:    LOW | MID | HIGH
  tempo:       SLOW | MIXED | FAST
  width:       NARROW | MIXED | WIDE
  directness:  SHORT | MIXED | DIRECT
  counter:     bool
}
```

- No per-slot role assignments: the match engine infers player roles from attributes + archetype ([09 §4.2](09-world-generation.md)).
- The **Manager adapts the plan autonomously** in-match and week-to-week, within bounds: adaptation frequency/magnitude scales with D9 (Tactical Flexibility) and D1 (Risk); `FORBID` Directives are hard fences around the space.
- Dials feed the key-moment sampler as chance frequency/quality modifiers ([03 §3](03-simulation-engine.md)).

## 6. Weight composition (how a decision roll consumes all this)

Illustrative — lineup selection for Saturday:

```
score(player) = base fit (attributes, familiarity, Condition, Sharpness, form)
  × disposition mods   (D2 youth bonus if U21, D1 rotation risk tolerance…)
  × priority mods      (DEVELOP_YOUTH rank-weight × minutes deficit)
  × directive mods     (START/BENCH/EXCLUDE/ROTATE strength multipliers)
  × manager noise      (mood, D9-scaled habit inertia)
→ softmax over candidates → rolled XI
```

Every factor is logged separately in the roll audit trail ([03 §5](03-simulation-engine.md), constraint 4 legibility). The same composition pattern applies to every decision family; the binding contract below says what each family listens to.

### 6.1 Decision-family mapping (initial — magnitudes tunable, wiring contractual)

Sensitivity: **H** = strong multiplier, **M** = moderate, *fence* = hard constraint, not a weight.

**COMPETITION_GOALS** is shorthand for whichever of `AVOID_RELEGATION` / `WIN_LEAGUE` / `WIN_PROMOTION` / `FINISH_TOP_N` / `CUP_RUN` are active in the Priorities.

| Decision family | Disposition axes | Priority goals | Directive verbs |
|-----------------|------------------|----------------|-----------------|
| Lineup & rotation | D1 M, D2 H, D7 M | DEVELOP_YOUTH, COMPETITION_GOALS | START/BENCH/EXCLUDE/ROTATE/GIVE_MINUTES/CAPTAIN — H |
| Transfers in | D1 M, D2 M, D4 H | BUILD_SQUAD_VALUE, DEVELOP_YOUTH, FINANCIAL_HEALTH | SIGN H, TARGET_PROFILE M |
| Transfers out / incoming offers | D4 M, D5 H | FINANCIAL_HEALTH, BUILD_SQUAD_VALUE | SELL H, KEEP H, LOAN_OUT M |
| Contracts | D4 H, D5 M, D7 M | FINANCIAL_HEALTH | RENEW/RELEASE H, WAGE_CAP *fence* |
| In-match management | D1 H, D3 M, D9 H | ESTABLISH_IDENTITY, COMPETITION_GOALS (stakes) | FORBID *fence* |
| Training & development | D2 M, D7 H | DEVELOP_YOUTH | DEVELOP/RETRAIN_POSITION H |
| Squad discipline & morale (misconduct response, hierarchy handling, fines) | D6 H, D5 M | — | — |
| Board interaction | D7 M, D10 M | PROTECT_JOB, FINANCIAL_HEALTH | PUSH_BOARD H |
| Media | D8 H | — | — |
| Job market | D7 M, D10 H | FIND_JOB, PROTECT_JOB | PURSUE_JOB/REJECT_JOB H |

Axes/goals/verbs not listed for a family contribute **nothing** to it — the wiring above is the contract; only the magnitudes are tuning surface. Every Disposition axis appears in at least one family (no dead axes).

## 7. Serialization example

```json
{
  "version": 41,
  "archetype_origin": "FIREFIGHTER",
  "disposition": {
    "current":  {"D1": -4, "D2": 6, "D3": -2, "D4": 5, "D5": 3,
                 "D6": 1, "D7": 7, "D8": -6, "D9": -3, "D10": 2},
    "target":   {"D2": 9},
    "drift_eta": "1926-03-14"
  },
  "priorities": [
    {"rank": 1, "goal": "AVOID_RELEGATION"},
    {"rank": 2, "goal": "DEVELOP_YOUTH", "params": {"age_cap": 21, "minutes_share": 0.25}},
    {"rank": 3, "goal": "FINANCIAL_HEALTH"}
  ],
  "directives": [
    {"id": "dir_0007", "verb": "KEEP", "target": {"player": 4521},
     "strength": "ABSOLUTE"},
    {"id": "dir_0009", "verb": "ROTATE", "target": {"position_group": "GK"},
     "strength": "INSIST", "conditions": {"competition": "CUP"}},
    {"id": "dir_0012", "verb": "SIGN", "target": {"player": 8802},
     "strength": "INSIST", "params": {"max_fee": 1200000},
     "expiry": "END_OF_WINDOW"}
  ],
  "tactical_plan": {
    "formation": "4-4-2", "mentality": "DEFENSIVE", "pressing": "MID",
    "tempo": "FAST", "width": "MIXED", "directness": "DIRECT", "counter": true
  }
}
```

## 8. Future Scope

Free-text Directives (engine-interpreted), per-slot tactical roles, set-piece taker assignments, media-interaction Directives (PRAISE/CRITICIZE), mentoring pair assignments.
