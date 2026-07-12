# Match Model

Agentic FC does not have a public CM/FM formula to copy. The design target is
therefore an original, explainable football model: physical facts, player
attributes, tactical intent, match state, and dice all contribute in bounded
ways. The engine should feel like football without pretending to be a continuous
physics simulation.

The point is not to make outcomes "random enough"; it is to build a game that
can be studied. A strong Agent should be able to learn that a direct, wide team
needs different bodies and attributes than a narrow pressing team, then still
live with the match-to-match variance that makes football dramatic.

This document is normative for the current match model. The implemented code
contains the wider attribute taxonomy, weak-foot cost/surface, body-expression
modifiers, tactical chance-type distribution, event-specific chance resolution,
derived factor helpers, tactical-role selection bias, public match diagnostics,
and a deterministic calibration harness. Remaining work is deeper event
families, position-aware replacement/eligibility, published per-event weight
tables, and long-run balance tuning.

## 0. Public Reference Basis

The exact CM/FM match formula is not public. Public material is still useful if
it is treated with the right confidence level:

| Grade | Source type | What it can decide here | What it cannot decide |
|-------|-------------|-------------------------|-----------------------|
| A | Official/directly observable FM manuals and UI behavior | Attribute groups, 1-20 scale, attribute-combination principle, position familiarity as a modifier, height + jumping reach relationship, scouting ranges | Exact weights, exact random rolls, internal match-engine formulas |
| B | CM 01/02 guides, editor-visible databases, official editor-derived community tables | Candidate visible/hidden stat names, CA/PA-style budget shape, position-specific ability-cost patterns, consistency/important-match style volatility, personality descriptor pattern | Exact modern FM behavior, match-event coefficients, legal/canonical formulas |
| C | Black-box experiments such as FM-Arena season batches | Calibration warnings: which attributes can dominate if over-applied, expected importance of pace/acceleration/reach/consistency in some engines | Authoritative coefficients for Agentic FC |

Design rule: grade A/B evidence can shape the model taxonomy and invariants.
Grade C evidence can shape calibration tests and anti-degenerate constraints,
but not direct coefficients. Agentic FC must be explainable on its own terms.

Public takeaways imported into this model:

- Attributes are not isolated buttons. FM's public manual describes attributes
  combining with each other, player position, pressure, surface, body profile,
  and decision context. Agentic FC formulas therefore combine several inputs per
  event and avoid one-stat resolution.
- Position familiarity is a real modifier, especially to decision and
  positioning quality. Agentic FC should treat role/position fit as a first
  class input in actor selection and event execution.
- FM exposes or allows reconstruction of **Current Ability cost weights** via
  its official editor tooling. That is useful for Agentic FC's Ability Pool
  economy, not for match-event resolution. Ability cost and event usefulness
  must be separate concepts.
- Body facts matter through football expression. Height helps reach, but
  Heading/Jumping Reach/timing decide what the player does with the ball.
  Weight helps contact expression, but Strength/Balance/Agility decide whether
  the body is useful.
- Weak-foot proficiency is not cosmetic. It changes which actions are safe and
  which roles are valuable; it should be costed and surfaced as a scouting
  descriptor/band.
- CM/FM-style hidden attributes are useful because they explain variance and
  careers: Consistency, Important Matches/Pressure, Injury Proneness,
  Professionalism, Ambition, Loyalty, Temperament, Sportsmanship/Dirtiness,
  Versatility, and Reputation all have clear simulation roles.
- Community tests repeatedly warn that if speed/reach/dribbling are useful in
  every event, they become the whole game. Agentic FC should let those
  attributes be decisive only in the event families where football says they
  should be decisive.

## 1. Design Principles

1. **Football causality first.** A player wins a header because reach, timing,
   bravery, positioning, body contact, cross quality, defensive pressure, and
   randomness all lined up. No raw stat should be a magic button.
2. **Correlation is not identity.** Height correlates with aerial reach, weight
   correlates with mass in duels, and body mass can trade off with agility or
   acceleration. These are probability biases, not hard truths.
3. **Body expresses ability, not replaces it.** Height and weight are public
   facts. They modify how attributes express in relevant events. They do not
   cost Ability Pool and should not turn a poor footballer into a good one.
4. **Event types carry meaning.** "Chance" is too broad. A cross, cutback,
   through ball, set piece, counter, long shot, scramble, and aerial duel should
   use different attribute mixes.
5. **Tactics change event distribution.** Directness, width, tempo, pressing,
   mentality, and formation should change which football problems are created,
   not only add a flat attack/defense bonus.
6. **Bounded stochastic model.** Every outcome is tendency plus dice. The dice
   must never erase player identity; player identity must never erase surprise.
7. **Known weights are part of the game.** Attribute/event weights should be
   documented and tunable. Agents are allowed to learn the system, build
   strategies around it, and chase edges. The guardrail is event-specificity,
   not opacity.
8. **No universal meta attribute.** An attribute may be highly valuable in a
   matching tactical ecosystem. It must not be a high-weight input to unrelated
   events. Pace can break lines; it should not decide stationary headers.
9. **Explainability is a feature.** MCP Agents and TUI spectators should be able
   to infer why a match looked the way it did: crosses, aerial losses, press
   turnovers, chance quality, fatigue, cards, and tactical changes.
10. **Determinism remains absolute.** All randomness flows through labelled
   engine streams. Persisted state and commentary params avoid floats. Derived
   values are reproducible from stored facts.

## 2. Data Layers

The model separates stable facts from football ability and transient match
state.

| Layer | Examples | Visibility | Role |
|-------|----------|------------|------|
| Public facts | age, height, weight, preferred foot, position, club | public | Body and identity context. |
| Visible ability | finishing, heading, passing, tackling, pace, weak-foot proficiency, positional familiarity | masked by knowledge; descriptors/bands where appropriate | Football skill surface. |
| Hidden tendency | consistency, pressure, discipline, injury risk | descriptors/evidence only | Volatility, durability, personality. |
| Tactical intent | formation, width, tempo, pressing, directness | public enough in matches; editable for own manager | Changes event distribution and risk. |
| Match state | score, minute, fatigue, cards, momentum, injuries | public match facts | Changes late-game behavior and event risk. |

Public facts can cross MCP/Console surfaces directly. Hidden tendency never
crosses as raw values.

## 3. Attribute Model Direction

Compact attribute models compress too much football causality. The implemented
surface moves closer to CM/FM's richer stat model while still keeping Agentic
FC names and formulas.
The test for keeping an attribute is simple: does it create a distinct
managerial question, event formula input, or career/story outcome?

### 3.1 Target visible outfield abilities

| Family | Attribute | Primary meaning |
|--------|-----------|-----------------|
| Technical | Finishing | Shot placement and composure in scoring actions. |
| Technical | Long Shots | Shooting threat outside the box. |
| Technical | First Touch | Receiving quality under pressure. |
| Technical | Passing | Ground passing and tempo retention. |
| Technical | Crossing | Wide delivery and early balls. |
| Technical | Dribbling | Beating a man with the ball. |
| Technical | Technique | Clean execution of hard actions. |
| Technical | Heading | Direction and power of headers once contact is made. |
| Technical | Tackling | Ball-winning challenge quality. |
| Technical | Marking | Tracking, denying, and staying connected to an opponent. |
| Technical | Set Pieces | Corners, free kicks, penalties, long throws. |
| Mental | Aggression | Willingness to engage in duels, tackles, presses, and confrontations. Not dirtiness by itself. |
| Mental | Vision | Seeing progressive options. |
| Mental | Decisions | Choosing the right action. |
| Mental | Composure | Executing under pressure. |
| Mental | Concentration | Avoiding lapses across repeated defensive and technical actions. |
| Mental | Positioning | Defensive positioning and rest-defense reading. |
| Mental | Off Ball | Attacking movement and space finding. |
| Mental | Anticipation | Reading second balls, rebounds, and passes early. |
| Mental | Work Rate | Repeated effort and pressing appetite. |
| Mental | Bravery | Willingness in contested headers/tackles. |
| Mental | Teamwork | Following team structure and supporting actions. |
| Mental | Leadership | On-pitch organization. |
| Mental | Determination | Response to adversity and persistence late in matches/seasons. |
| Mental | Flair | Appetite and ability to choose creative, risky actions. |
| Physical | Acceleration | First steps and separation. |
| Physical | Pace | Top speed over space. |
| Physical | Agility | Turning, balance recovery, body control. |
| Physical | Balance | Staying upright and stable under pressure or contact. |
| Physical | Strength | Contact strength and shielding. |
| Physical | Stamina | Sustained output. |
| Physical | Natural Fitness | Recovery between matches and resistance to physical decline. |
| Physical | Jumping Reach | Reach in aerial contests, including leap timing. |

Set Pieces remains a single visible attribute unless playtesting proves
separate corner/free-kick/penalty/long-throw specialization is worth the extra
surface. Internally, event formulas may still tag set-piece type so future
specialization can split without rewriting the event grammar.

### 3.2 Target goalkeeping abilities

Goalkeepers keep the shared mental/physical surface where relevant. They use a
wider goalkeeping surface than v1 so one keeper can be a reflex shot-stopper,
another a box-commanding sweeper, and another a conservative handler.

| Attribute | Primary meaning |
|-----------|-----------------|
| Reflexes | Reaction saves and unpredictable close-range events. |
| One on Ones | Judgement and execution when isolated against a finisher. |
| Handling | Catching and spill resistance. |
| Aerial Reach | Physical reach in the air. |
| Command of Area | Decision to claim, hold, or organize under aerial pressure. |
| Communication | Organizing defenders, leaving/claiming calls, box calm. |
| Distribution | Kicks, throws, and build-up quality. |
| Sweeping | Starting position and rushing-out actions. |
| Eccentricity | Tendency to attempt unusual keeper actions. High is not strictly better. |
| Punching | Tendency to punch rather than catch. High is not strictly better. |

### 3.3 Body correlation rules

Generation should model plausible correlations without hard-locking outcomes:

| Body fact | Positive correlations | Negative or soft trade-offs | Exceptions must exist |
|-----------|----------------------|-----------------------------|-----------------------|
| Height | Jumping Reach, Aerial Reach, Heading opportunity | Acceleration/Agility at extremes | Short elite headers, tall poor headers |
| Weight | Strength, Balance, shielding, duel stability | Acceleration/Agility/Stamina at extremes | Light strong players, heavy fast players |
| Age | Decisions, Composure, Concentration, Leadership | Acceleration, Pace, Stamina, injury recovery with decline | Late bloomers, early decline |

Implementation rule: body correlations bias generation preferences and derived
event expression. They do not assign final attributes directly.

### 3.4 Target hidden attributes

Hidden attributes are not a second visible stat sheet. They are the private
traits that explain variance, careers, injuries, adaptation, and squad mood.
They surface only through descriptors, evidence, and public consequences.

| Family | Attribute | Primary simulation role |
|--------|-----------|-------------------------|
| Ability budget | Potential Cap | Long-run attribute ceiling. Fixed once generated. |
| Growth/decline | Development Speed | How quickly growth converts opportunity into ability. |
| Growth/decline | Decline Onset | When physical decline tends to begin. |
| Growth/decline | Decline Speed | How sharply decline expresses once started. |
| Volatility | Consistency | How reliably visible ability expresses in ordinary matches. |
| Volatility | Big Match Nerve | Same idea in finals, derbies, promotion/relegation/title stakes. |
| Volatility | Pressure | Stress response in contract, board, crowd, and late-match situations. |
| Durability | Injury Proneness | Likelihood of knocks and severe injuries. |
| Durability | Recovery | Injury duration and condition recovery beyond visible Natural Fitness. |
| Personality | Professionalism | Training seriousness, self-care, decline resistance. |
| Personality | Ambition | Career drive, transfer desire, development hunger. |
| Personality | Loyalty | Attachment to club/manager/squad. |
| Personality | Adaptability | Settling after transfers, tactical/cultural change. |
| Personality | Temperament | Emotional control under provocation. |
| Personality | Sportsmanship | Fair play tendency and reluctance to exploit dark arts. |
| Personality | Controversy | Media friction, disruptive comments, off-pitch drama. |
| Social | Influence | Dressing-room weight and captaincy gravity. |
| Social | Sociability | Chemistry formation and mood spread. |
| Utility | Versatility | Learning positions and reducing out-of-position penalties. |
| Market | Reputation | Pull, wage expectation, fan expectation; descriptor-only on the wire. |

Visible Aggression and hidden Sportsmanship/Temperament/Dirtiness-like behavior
must stay separate. A high-aggression player joins duels and presses hard. A
low-sportsmanship or low-temperament player turns that contact load into cheap
fouls, dissent, retaliation, and cards.

### 3.5 Footedness and position familiarity

Preferred foot is a public profile fact. Weak-foot proficiency is a football
ability because it expands the action set available to a player under pressure.
FM's CA-weight discussions repeatedly treat weak foot as part of player ability,
especially for attacking roles; Agentic FC should do the same.

Implementation rules:

- Store weak-foot proficiency internally on the same 1-20 scale as other
  football ability, but surface it as descriptors/bands rather than a raw hidden
  trait. It is not personality.
- Ability Pool cost should be higher for roles that often receive, cross, pass,
  or shoot from either side: wide forwards, attacking midfielders, central
  forwards, wingbacks, and playmakers.
- Event formulas should apply off-foot pressure in concrete places: first-time
  finishes, cutbacks, weak-side crosses, press-resistant turns, through balls
  played across the body, and emergency clearances.
- Role/position familiarity remains a separate 1-20 internal map surfaced as
  descriptors. Low familiarity primarily hurts Decisions, Positioning/Marking,
  Teamwork, and tactical event eligibility, not raw Pace or Strength.

### 3.6 Ability Pool benchmark from FM

The strongest publicly usable formula-like material from FM is not the match
engine. It is the editor-exposed/reconstructable Current Ability cost model:
attributes have different budget costs by position; some tendencies cost little
or nothing; weak foot costs ability; hidden/personality attributes generally do
not consume the visible ability budget.

Agentic FC should benchmark that pattern aggressively while keeping its own
numbers:

1. **Separate budget cost from event weight.** An attribute can be cheap to
   generate but situationally decisive, or expensive in the budget but unused in
   a specific event. Set Pieces are the classic example: cheap in the budget,
   decisive when a set-piece event occurs.
2. **Use position/role-specific PoolCosts.** Pace/Acceleration should be
   expensive for wide and forward roles; Decisions/Positioning/Concentration
   expensive for central defenders and keepers; specialist delivery and set
   pieces cheaper unless the role depends on them.
3. **Treat tendency/profile attributes differently.** Aggression, Determination, Flair,
   Natural Fitness, Eccentricity, Punching/Rush tendencies, and similar
   behavior-shapers should be zero- or low-pool-cost modifiers with bounded
   event roles, not a way to inflate raw football ability.
4. **Keep hidden attributes off the PoolCost.** Consistency, Big Match Nerve,
   Injury Proneness, Professionalism, Ambition, Loyalty, Temperament, and
   Reputation shape expression, growth, health, and market behavior; they do not
   buy more visible ability.
5. **Document multi-position costing.** A player natural in multiple positions
   should use a deterministic blended or max-cost rule. The rule matters because
   multi-position players otherwise become a loophole in the budget.

This is the main place where public FM formula-adjacent data can improve
Agentic FC immediately. Match formulas still remain original.

## 4. Derived Football Factors

The match engine should compute fixed-point integer derived factors from public
facts, visible ability, hidden volatility, current state, and tactical context.
These are not stored as independent truth; they are explainable formulas.

| Derived factor | Inputs | Used by |
|----------------|--------|---------|
| Reach | height, Jumping Reach, timing volatility, fatigue | aerial duels, set pieces, crosses |
| Header Quality | Heading, Technique, Composure, pressure, body angle | headed shots, clearances, knockdowns |
| Duel Power | Strength, weight, Balance, Aggression, Bravery, fatigue | aerial contact, shielding, tackles |
| Separation | Acceleration, Pace, Agility, Off Ball, defender positioning | through balls, counters, pressing escapes |
| Ball Security | First Touch, Technique, Dribbling, Balance, Composure | press resistance, turnovers |
| Delivery Quality | Passing/Crossing/Set Pieces, Vision, Technique, Decisions | chance creation |
| Shot Quality | Finishing/Long Shots/Heading, Composure, angle, pressure | conversion |
| Defensive Read | Positioning, Marking, Anticipation, Concentration, Decisions, Teamwork | blocks, interceptions, marking |
| Press Impact | Work Rate, Acceleration, Aggression, Stamina, tactical pressing | turnovers, fatigue, cards |
| Creative Risk | Flair, Vision, Technique, Decisions, tactical freedom | unusual passes, dribbles, speculative shots |
| Recovery Load | Stamina, Natural Fitness, Recovery, age, schedule density | condition recovery, fatigue injuries |

All factors should be integer or fixed-point. If a logistic curve is needed, use
a deterministic lookup table or fixed-point approximation whose output is an
integer probability band.

Implemented factor helpers live in `internal/engine/factors.go`: reach, duel
power, header quality, separation, ball security, delivery quality, shot
quality, defensive read, and press impact. They are derived from visible
football abilities plus public body expression; private traits may still affect
volatility elsewhere but do not become public factor values.

## 5. Event Grammar

The match remains key-moment sampled. The implemented first slice replaces the
generic chance roll with a two-step grammar:

```
match state + tactics + personnel
        -> event family distribution
        -> event-specific contest
        -> outcome + commentary + stats
```

### 5.1 Event families

| Family | Examples | Main drivers |
|--------|----------|--------------|
| Build-up | circulation, progressive pass, carry, switch | tempo, passing, decisions, pressure |
| Direct play | long ball, target knockdown, channel ball | directness, reach, strength, off-ball runs |
| Wide play | overlap, cross, cutback | width, crossing, dribbling, pace, box occupation |
| Central play | through ball, combination, half-space pass | vision, passing, first touch, off-ball movement |
| Transition | counter, press turnover, recovery run | pressing, counter, pace, work rate, rest defense |
| Set piece | corner, free kick, penalty, long throw | set pieces, reach, heading, marking, keeper command |
| Defensive event | tackle, interception, block, clearance | positioning, tackling, anticipation, strength |
| Discipline/injury | foul, card, knock, fatigue withdrawal | aggression, temperament, sportsmanship, contact load, condition |

### 5.2 Chance types

| Chance type | Creator inputs | Receiver/finisher inputs | Defender/GK inputs |
|-------------|----------------|--------------------------|--------------------|
| Cross header | Crossing, Vision, wide space | Reach, Heading, Bravery, Off Ball | Reach, Marking/Positioning, Strength, Aerial Reach/Command of Area |
| Cutback | Dribbling, Crossing/Passing, Pace | Finishing, Composure, Off Ball | Positioning, Anticipation, Reflexes |
| Through ball | Vision, Passing, Technique | Acceleration, Off Ball, First Touch, Finishing | Positioning, Pace, Sweeping/One on Ones |
| Long shot | Passing support, space | Long Shots, Technique, Composure | Reflexes, Handling, block pressure |
| Set-piece header | Set Pieces | Reach, Heading, Strength, Bravery | Marking/Positioning, Reach, Aerial Reach/Command of Area |
| Scramble | pressure, rebounds, bodies in box | Anticipation, Bravery, Finishing | Handling, Reflexes, blocks, Composure |
| Counter | turnover quality, space | Pace, Decisions, Finishing | Rest defense, Pace, Sweeping/One on Ones |

This is where body data should matter most. Height should be strong in reach
events, mild elsewhere, and irrelevant to ground combinations unless the event
uses body contact.

### 5.3 Event weight shape

Each event family gets an explicit weight table in code and docs/98. The table
should be readable enough that a manager can reason about recruitment and
tactics. The exact numbers are implementation tunables, but every event follows
this shape:

| Weight bucket | Typical share | Purpose |
|---------------|---------------|---------|
| Primary event skill | 35-50% | The football action being tested: Crossing in crosses, Finishing in shots, Tackling in tackles. |
| Supporting football IQ/technique | 20-35% | Decisions, Vision, Technique, Anticipation, Concentration, Composure. |
| Physical/body expression | 5-20% | Pace, Acceleration, Balance, Strength, height/weight expression, fatigue. |
| Tactical/role context | 10-25% | Whether the tactic and role put the player into this event with support. |
| Hidden volatility/personality | capped modifier | Consistency, pressure, temperament, sportsmanship, injury risk. Never a raw visible output. |

The implementation may put these into integer weights such as basis points, but
the shares above are design bounds. A formula outside the bounds should update
this document and the tunables registry.

Strategic consequence: a manager can deliberately build a crossing side,
counter side, pressing side, possession side, or set-piece side. The reward is
strong when the squad, body profiles, and tactical event distribution agree.
The penalty is also strong: playing a target-man plan without delivery/reach, or
a high line without recovery speed and concentration, should fail for legible
reasons.

## 6. Tactical Coupling

Tactics should not be a flat score. They should alter event distributions,
player selection pressure, and risk.

| Dial | Raises | Lowers / risks |
|------|--------|----------------|
| Width high | wide play, crosses, switches | central compactness, transition defense |
| Width narrow | central combinations, counterpress density | crossing volume, wide isolation |
| Directness high | long balls, target duels, channel runs | possession retention, midfield control |
| Directness low | short build-up, cutbacks, central play | vulnerability to high press if poor first touch |
| Tempo high | transitions, fast shots, fatigue, mistakes | composure and pass completion |
| Tempo low | control, patient chances | chance volume when chasing |
| Pressing high | turnovers, field tilt, fatigue, cards | space behind, injury/contact load |
| Pressing low | compact defense, fewer transitions | sustained pressure against |
| Mentality attacking | box occupation, shot volume | rest defense, late-game exposure |
| Mentality defensive | block density, clearances | chance creation, pressure relief |
| Counter on | transition chance share | slower possession recycling |

Formation and roles should decide which players are eligible for which events.
For example, a wide forward and attacking fullback should increase wide
overload chances; a target striker should increase long-ball/cross target
selection, but only if the team style actually supplies those events.

The current implementation derives non-persisted tactical roles at kickoff from
public/visible football evidence: shot stopper or sweeper keeper, stopper or
wing back, playmaker or presser, target forward or runner/poacher. `selectSquad`
uses Ability Pool plus tactical-fit bonuses, so the same squad can select
different profiles for direct/wide, short/narrow, pressing, or counter plans.
Chance actor weighting also receives bounded role bonuses. These roles are an
engine interpretation, not a new hidden stat sheet.

## 7. Probability Pipeline

Each key moment should follow this shape:

1. **State read:** minute, score, cards, fatigue, substitutions, weather if
   added later, current tactical shifts.
2. **Event distribution:** choose an event family from tactics, personnel, and
   match state.
3. **Actor selection:** choose creator, receiver, defender, and keeper from
   role eligibility and on-pitch positions. Selection weights use attributes,
   tactical role fit, fatigue, and current form.
4. **Contest score:** compute attacker and defender fixed-point scores from the
   event-specific formula.
5. **Chance quality:** if the contest creates a shot, derive a chance-quality
   band from space, angle, body part, delivery, pressure, and finisher state.
6. **Resolution:** roll goal/save/block/miss from chance quality, shot quality,
   and keeper/defender response.
7. **Side effects:** fatigue, card risk, injury risk, momentum, ratings, stats,
   commentary, and future substitution decisions.

Presentation variety must not perturb those decisions. Fatigue substitutions
cycle through six localized commentary voices by counting already-persisted
fatigue changes across the match; they consume no RNG, and the six-substitution
match limit prevents a repeated fatigue line in normal play.

Half-time and full-time commentary is likewise deterministic but contextual.
Half-time distinguishes goalless, scoring-level, ordinary lead, and three-goal
command for either side. Full-time distinguishes goalless, scoring-level,
one-goal edge, two-goal win, and three-plus-goal emphatic win. Legacy generic
keys remain renderable for saved matches.

Goal commentary is score-aware on the same no-RNG basis: the opener, an
equalizer, and any goal from the late-drama minute (85', tunable) that levels
or wins the match swap the patterned call for a context call
(`comment.goal.opener/equalizer/late.*`), with the variant rotated on public
match state. The context ladder also reads the persisted scorer ledger: a
scorer's third goal headlines as a hat-trick call (with a late variant)
ahead of every scoreline context; leveling or going ahead after trailing by
two narrates as a comeback; re-taking the lead within five minutes of
conceding reads as an instant response; and a side's fourth goal at a
three-goal margin narrates as a rout
(`comment.goal.hattrick/comeback_level/comeback_ahead/response/rout.*` — all
thresholds tunable, docs/98). Precedence runs specific-to-generic:
hat-trick, then comeback, then instant response, then the late-drama calls,
then opener/equalizer, then rout — so a stoppage-time goal that completes a
two-goal fightback narrates the fightback, and the late keys keep every
ordinary closing-minutes leveler or winner. Ordinary lead-padding goals keep their
chance-pattern call (cross header, cut-back, through ball, long shot, set
piece, counter, scramble — four variants each). The kickoff whistle rotates
three voices by fixture ID.

Quiet beats are state-aware on the same contract: the single legacy-bound
draw is taken exactly as before, but a close game from the tension minute
(75', tunable) maps it into a nervy themed pool, and a three-goal margin
from the cruise minute (60', tunable) maps it into game-management lines
(`comment.quiet.tension/cruise.*`); when a themed pool has no unused line
left, the beat falls back to the broad quiet pool's unused-probe. The RNG
stream seen by cards, injuries, and every other outcome is untouched either
way.

The half-time whistle is its own queue event at exactly 45 minutes; it is no
longer borrowed from the sampled 47' key moment. A match resumed from an older
in-progress snapshot without that event records one 45' fallback line when the
47' moment arrives, without duplicating commentary or changing its moment
roll. A legacy snapshot already past 47' keeps its persisted 47' whistle rather
than rewriting historical world state.

Formula policy:

```
score = base
      + weighted visible abilities
      + bounded body expression
      + bounded tactical/context modifiers
      + hidden-volatility expression
      - pressure/fatigue penalties
```

Then:

```
probability = clamp(base_probability + fixed_point_curve(score_delta), min, max)
```

Do not persist floats. Do not put floats in commentary params. If the engine
uses fixed-point ints internally, tests should lock that no float reaches
world-hash state.

### 7.1 Weight publication and anti-degenerate checks

The match model is allowed to be learnable. Therefore every event-family weight
table, body-expression clamp, fatigue penalty, and volatility cap is a tunable
registered in docs/98 when implemented.

However, learnable must not mean solved by one generic filter. The calibration
harness must include dominance checks:

- **global dominance check:** no single visible outfield attribute should
  improve every archetype/tactic equally across season batches;
- **event dominance check:** a high-weight event attribute must mainly improve
  the event family that uses it;
- **body dominance check:** height/weight can swing aerial/contact contests but
  cannot produce elite delivery, finishing, marking, or decisions by itself;
- **tactic fit check:** the same player profile should change value when the
  tactical event distribution changes;
- **variance floor check:** the better side should not win so reliably that
  upsets disappear, and the weaker side should not win so often that scouting is
  pointless.

This turns public black-box lessons from FM community testing into guardrails:
if our own season-batch output shows one attribute acting like a universal
answer, the formula is wrong even if the aggregate goals/shots look plausible.

## 8. Observability Requirements

The richer model must not become opaque. The spectator and the Agent need
diagnostic surfaces.

MCP/Console expose, at appropriate fidelity:

- side-attributed chance type counts: crosses, cutbacks, through balls,
  counters, set pieces, long shots (`match_patterns` on MCP; localized chance
  mix on Console/TUI). Legacy and upgrade-straddled aggregate counts remain
  explicitly `UNKNOWN` rather than being assigned to a team;
- aerial duels attempted/won;
- press turnovers created;
- set-piece threat counts;
- shot quality bands attributed to home/away, not a real-world xG claim;
- tactical tilt: wide/central/distance/set-piece/transition/scramble profile;
- player role fit and body profile hints where public;
- current own-team condition during a live match, derived from each player's
  minutes on the pitch rather than the unchanged pre-match stored value;
- post-match "why it happened" summaries built from public stats and
  descriptors.

Hidden values remain hidden. Explanations should say "lost too many aerial
duels" or "the press created turnovers", not "hidden Consistency was 6".

## 9. Calibration Targets

The first implementation should include a simulation harness, not only unit
tests. Initial targets are bands, not exact real-world replication:

| Metric | Target use |
|--------|------------|
| Goals per match | League-level sanity band. |
| Shots per match | Chance-volume sanity. |
| Shot conversion | Avoid every match becoming high-score chaos. |
| Header goal share | Validate body/aerial model. |
| Set-piece goal share | Validate specialist value. |
| Cross completion and aerial-duel win rate | Validate wide/direct tactics. |
| Cards and injuries | Validate contact/fatigue risk. |
| Attribute sensitivity | No single attribute dominates all outcomes. |
| Tactical sensitivity | Changing tactics changes event mix before scoreline. |
| Archetype viability | Target men, poachers, pressers, playmakers all matter in different systems. |
| Learnability | A documented tactical/stat edge is visible over season batches. |
| Upset rate | Strong teams win more, but football variance remains alive. |
| Meta resistance | Pace/reach/dribbling/consistency cannot dominate unrelated event families. |

Property tests:

- same seed + config + input log gives identical results;
- tempo/chunking does not change match outcomes;
- commentary params contain no floats;
- hidden attributes never cross viewer/MCP surfaces;
- body facts are public, stable, and not treated as Ability Pool.

Development calibration is available as `engine.RunCalibration` and the
`agenticfc-calibrate` binary. It runs compact worlds across seed batches and
reports integer aggregate metrics: goals/shots per match, conversion rate,
home/draw/away split, upsets, chance-type mix, shot-quality bands, aerial
volume, press turnovers, and set-piece threat. Reports are deterministic for
the same seed list and horizon.

## 10. Implementation Roadmap

This is a large replacement. Build it in slices with tests at each seam.

1. **Attribute model.** ✅ Split the compressed attributes needed by the new
   event grammar, including Heading, Jumping Reach, Crossing, First Touch,
   Acceleration, Long Shots, Decisions, Off Ball, Anticipation, Bravery, and
   Technique. Add weak-foot proficiency, separate Ability Pool cost weights from
   event weights, define position/role-specific PoolCosts, and rework
   generation.
2. **Correlated generation.** ✅ Add body/attribute covariance in player
   generation. Use probabilistic bias, not hard constraints. Keep exceptions.
3. **Derived factor library.** ✅ Implement fixed-point functions for reach, duel
   power, delivery quality, ball security, separation, defensive read, shot
   quality, and press impact.
4. **Event grammar skeleton.** ✅ Replace the generic chance roll with event
   family selection and event-specific contests. Keep commentary tied to
   event-specific templates.
5. **Tactical distribution.** ✅ Make tactical dials alter event mix, actor
   eligibility, risk, and fatigue. Add role/formation hooks where needed.
6. **Stats and observability.** ✅ Persist public event stats, expose them through
   Console/MCP, and teach the TUI replay/match panes to show chance type and
   tactical profile.
7. **Narrative expansion.** Add event-type commentary templates in en+ko.
   Commentary should teach the viewer what the model is doing.
8. **Calibration harness.** ✅ Run season batches across seeds, output aggregate
   metrics, and tune constants through docs/98.
9. **Published weight pass.** Document every implemented event-family weight
   table and dominance guard. This is where the "system can be studied" promise
   becomes concrete.
10. **Retire old shortcuts.** Remove flat attack/defense shortcuts that no
   longer correspond to the event model.

## 11. Non-goals

- No continuous ball physics.
- No real-world xG branding unless calibrated enough to deserve that label.
- No hidden raw values in explanations.
- No exact FM/CM formula claims.
- No backwards compatibility promise for local development snapshots unless
  explicitly requested.
