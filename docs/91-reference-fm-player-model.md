# Reference: FM Player Model & Development

Research reference on Football Manager's player modeling. Part of the CM/FM reference set — see [90-reference-cm-fm.md](90-reference-cm-fm.md) for the overview and design lessons. Facts marked *(community)* are reverse-engineered by the FM community rather than officially documented by Sports Interactive; treat as strong-but-unofficial.

## 0. Evidence grade

This document is a reference input, not a formula clone. Use these grades when
moving facts into normative Agentic FC docs:

| Grade | Evidence | Use in Agentic FC |
|-------|----------|-------------------|
| A | Sports Interactive manual pages and directly observable FM UI concepts | Taxonomy, scale, masking pattern, combination principle, position/body relationships |
| B | CM 01/02 guides, editor-visible databases, official editor-derived community tables | Candidate hidden attributes, CA/PA budget concept, position-specific ability-cost patterns, personality/descriptors, classic text-match expectations |
| C | Black-box test batches and meta guides | Calibration warnings and regression targets, not direct coefficients |

Key reference pages checked for the match-model redesign:

- Sports Interactive FM24 manual: player attributes use the 1-20 scale; are
  grouped into Technical/Mental/Physical with GK-specific ratings; combine with
  other attributes, position familiarity, match situations, and body facts such
  as height in aerial play; position familiarity directly modifies ability;
  heading combines with Jumping Reach, Height, and Strength; attributes and
  position/foot competence feed Current Ability. <https://community.sports-interactive.com/sigames-manual/football-manager-2024/players-r4958/>
- CM 01/02 GameFAQs guides: visible/hidden attributes existed on the same 1-20
  tradition; Current/Potential Ability used a 0-200 scale; hidden mental
  characteristics included Adaptability, Ambition, Determination, Loyalty,
  Pressure, Professionalism, Sportsmanship, and Temperament; player ability
  fields included Consistency, Important Matches, Injury Proneness, Natural
  Fitness, Versatility, footedness, set pieces, and Vision. <https://gamefaqs.gamespot.com/pc/563063-championship-manager-season-01-02/faqs/28485> and <https://gamefaqs.gamespot.com/pc/563063-championship-manager-season-01-02/faqs/18827>
- FM-Arena attribute-testing tables: black-box experiments repeatedly show high
  sensitivity around pace/acceleration/reach/dribbling/consistency in specific
  FM versions. Treat this as an anti-degenerate warning: Agentic FC should make
  those attributes powerful in their football contexts, not universal keys.
  <https://fm-arena.com/table/26-player-attributes-testing/>
- Official editor / in-game editor surfaces: FM's match formula is not exposed,
  but Current Ability weighting is visible or reconstructable through editor
  tooling (`recommended current ability`, pre-game editor attribute weights).
  Use this only for Ability Pool design, not match resolution.
  <https://community.sports-interactive.com/forums/topic/308816-current-ability-cost-of-of-attributes-position-breakdown/>
  and <https://www.fmscout.com/a-guide-to-current-ability-in-football-manager.html>

## 1. Visible attributes (1–20 scale)

All visible attributes are integers 1–20 (higher = better; internal progress is fractional — only the display is integer *(community)*). The set has been stable from ~FM12 through FM26.

- **Technical (14):** Corners, Crossing, Dribbling, Finishing, First Touch, Free Kick Taking, Heading, Long Shots, Long Throws, Marking, Passing, Penalty Taking, Tackling, Technique.
- **Mental (14):** Aggression, Anticipation, Bravery, Composure, Concentration, Decisions, Determination, Flair, Leadership, Off the Ball, Positioning, Teamwork, Vision, Work Rate. (Determination is the only personality attribute that is publicly visible.)
- **Physical (8):** Acceleration, Agility, Balance, Jumping Reach, Natural Fitness, Pace, Stamina, Strength.
- **Goalkeeping (13, replaces Technical for GKs):** Aerial Reach, Command of Area, Communication, Eccentricity, First Touch, Handling, Kicking, One on Ones, Passing, Punching (Tendency), Reflexes, Rushing Out (Tendency), Throwing. Punching and Rushing Out are *tendencies*, not quality ratings — higher isn't strictly better.
- **Footedness:** left/right foot ratings stored numerically, surfaced only as descriptors (Very Weak → Very Strong); weak-foot proficiency matters in the match engine.

### Agentic FC import

Agentic FC v1 compressed this into 15 visible attributes for readability. The
the match model should widen again where compression hides a football cause:
Crossing vs. Passing, Heading vs. Jumping Reach, Acceleration vs. Pace,
Positioning vs. Marking, Composure vs. Decisions vs. Concentration, and Reflexes
vs. One-on-Ones vs. Handling for keepers. Exact FM naming is not required; the
important part is preserving distinct tactical questions. The model should
also stop treating footedness as a cosmetic profile field: weak-foot proficiency
needs its own cost and event impact.

## 2. Hidden attributes

### Current Ability / Potential Ability (0–200)

- **CA is a weighted budget** constraining the attribute set. Each attribute has a per-position CA cost (editor-exposed since FM21). *(community)* examples: +1 Acceleration ≈ 2.5 CA vs +1 Teamwork ≈ 0.3 CA; Acceleration/Pace/Agility are the most expensive in nearly all positions; Decisions is heavily weighted for GKs and centre-backs; set-piece attributes are cheapest. **Redistribution within CA**: a player can improve cheap attributes while an expensive one declines, with no CA change.
- **PA is fixed for the whole career** once a save starts. Database entries can hold negative PA, rolled to a random value in a 30-point band at game start *(community table)*: −10 → 170–200, −9 → 150–180, −8 → 130–160, −7 → 110–140, −6 → 90–120, −5 → 70–100, … −1 → 0–20 (with intermediate −x5 values since FM16).

### Editor-exposed CA weights: what is actually formula-like

The most useful public formula-adjacent FM material is the ability-budget
layer, not the match engine:

- SI's in-game editor exposes "recommended current ability" after changing
  attributes, letting community analysts infer per-position attribute costs.
- Since FM21, community guides report that the pre-game editor includes
  attribute weights for positions.
- The consistent public lesson is structural: PoolCost is position-specific;
  physicals such as Pace/Acceleration/Agility are expensive; Decisions is
  expensive for keepers/central defenders; set pieces are cheap; weak-foot
  proficiency costs ability; several tendencies such as Aggression,
  Determination, Flair, Natural Fitness, Eccentricity, and keeper punch/rush
  tendencies are reported as low/no CA cost.
- Hidden/personality attributes do not spend CA in the same way; they affect
  expression, development, morale, discipline, and availability.

Agentic FC import: use this as the blueprint for Ability Pool economics. Do not
turn it into match-event coefficients. A cheap set-piece attribute can still be
decisive on a set piece; an expensive acceleration point should not decide a
stationary defensive header.

### Other hidden attributes (1–20)

| Attribute | Effect *(community models)* |
|-----------|------------------------------|
| **Consistency** | Rating x ⇒ plays x out of 25 matches at full CA; in off matches, technical + mental attributes are randomly marked down (up to ~20 points spread); physicals unaffected. |
| **Important Matches** | Same mechanism applied to high-stakes fixtures (derbies, finals). |
| **Injury Proneness** | Likelihood of injury. |
| **Versatility** | Speed of learning new positions; size of out-of-position penalty. |
| **Dirtiness** | Fouling/diving/card tendency. |
| **Reputation** (current/home/world) | 0–10,000 scale; drives transfers and morale rather than match performance. |

### Agentic FC import

The hidden layer should stay broad because it is what makes a world feel alive
without exposing raw dice. The next model should preserve at least these
simulation roles:

- **volatility:** Consistency, Big Match Nerve / Important Matches, Pressure;
- **durability:** Injury Proneness, Recovery / Natural Fitness interaction;
- **career:** Potential Cap, Development Speed, Decline Onset, Decline Speed;
- **personality:** Professionalism, Ambition, Loyalty, Adaptability,
  Temperament, Sportsmanship, Controversy;
- **utility/social:** Versatility, Influence, Sociability, Reputation.

The raw numbers remain hidden. Public surfaces get descriptors, scout evidence,
media behavior, transfer behavior, training/development outcomes, cards,
injuries, and form volatility.

## 3. Personality system

Eight hidden personality attributes (1–20): **Adaptability, Ambition, Controversy, Loyalty, Pressure, Professionalism, Sportsmanship, Temperament** — plus visible **Determination**. Adaptability (settling abroad) and Controversy (outbursts, contract agitation) don't feed the personality descriptor; Controversy drives **Media Handling** instead.

**The key pattern:** hidden attributes are never shown; the game derives a **personality descriptor** from threshold combinations, evaluated with precedence. Selected rows of the *(community)* table:

| Descriptor | Requirements |
|------------|--------------|
| Model Citizen | Det 14–20, Prof 15–20, Amb 12–20, Loy 15–20, Pres 14–20, Spor 15–20, Temp 15–20 |
| Model Professional | Prof 20, Temp 10–20 |
| Perfectionist | Det 14–20, Prof 14–20, Amb 14–20 |
| Driven | Det 18–20, Amb 12–20 |
| Mercenary | Loy 1–3, Amb 16–20 |
| Born Leader | Leadership 20, Det 20 |
| Temperamental | Temp 1–4, Prof 1–10 (overrides most others) |
| Balanced | anything not matching a rule (also masks deliberately negative data) |

(Full table: ~35 descriptors — Model Citizen > leader types > professionalism types > determination/ambition/loyalty/pressure/sportsmanship types > Balanced.)

**Media Handling styles** (mostly Controversy/Temperament/Pressure-driven): Outspoken, Short-Tempered, Volatile, Confrontational, Evasive, Unflappable, Reserved, Level-Headed, Media-Friendly.

Personality can shift over a career — mentoring, dynamics, manager interaction — especially before ~24.

## 4. Attribute masking & scouting knowledge

- With masking on, attributes of insufficiently known players display as **ranges** (e.g. Technique 11–16) or greyed out entirely.
- Range width is tied to **scouting knowledge %** per player (fed by nation/competition knowledge, scout & staff knowledge, affiliates, playing against them, watching them). More knowledge → narrower ranges → exact values at full knowledge.
- A focused individual scouting assignment (~a week, with baseline regional knowledge) returns full attribute disclosure; ongoing scouting adds report text hinting at *hidden* attributes (personality/injury notes).
- Scout star ratings for CA/PA carry explicit uncertainty (dark stars = uncertainty band), scale with the scout's Judging Player Ability/Potential, and are **relative to your current squad/division**, not absolute.

## 5. Development & decline

- **CA grows toward PA**, driven by: training (facilities, coach quality, workload), match experience at an appropriate level, game time, personality, and injuries.
- **Age curve** *(community testing)*: 15–18 is the golden window (peak ~17–18); training dominates when young, match experience dominates later (~50/50 at 18; by 23 match experience matters far more). Measured slowdown nodes at 19 and ~27. Same training schedule yields ~+30.5 CA at 18, ~+22.5 at 20, ~+20 at 23.
- **Attribute-type timing:** physicals develop earliest; technicals through late teens/early 20s; mentals grow latest and can keep rising past 24 through match experience.
- **Personality effects:** Professionalism is the biggest training multiplier; Determination and Ambition also accelerate development. Low-personality youngsters plateau below PA.
- **Mentoring** (group-based since FM19): shifts young players' personality and Determination toward mentors'; can pass on traits; strongest on teenagers, effective to ~24.
- **Injuries:** stall growth during the key window; serious/recurring ones can permanently prevent reaching PA and reduce physical attributes (PA itself never changes).
- **Decline:** onset ~29–32 outfield (GKs later; decline visible after ~31). **Physicals fall first** (Acceleration/Pace/Agility), mentals hold or rise, technicals decay slowly. High Natural Fitness, Professionalism, and continued playing time slow decline; from ~35 CA cannot be held flat even with optimal training *(community test)*. CA freed by physical decline can partially redistribute into cheap mental attributes — the "aging gracefully" mechanic.

## 6. Youth intake (newgens)

- **One intake day per club per season**, in a nation-specific window (England mid-Mar–mid-Apr, Brazil late Sep–late Oct, …). A preview arrives ~4 weeks prior with the HoYD's early assessment, including "golden generation" flags. Players arrive aged 14–16.
- **Quality inputs:**
  - **Nation Youth Rating** (hidden, 0–200): baseline talent ceiling — a maxed academy in a weak nation still trails a mid academy in Brazil/France. Drifts slowly over long saves.
  - Club facilities on 1–20 scales: **Youth Recruitment** (talent attracted), **Youth Facilities** (development environment), **Junior Coaching** (arrival CA).
  - **Head of Youth Development:** judging attributes shape evaluation; his **preferred formation biases generated positions**; newgens have a chance to **inherit his personality** (officially documented).
  - Club reputation/league standing affect pulling power.
- PA is rolled at intake; quality varies season to season with real RNG even at max facilities.

## 7. Retirement

- Players **choose** to retire (can't be forced); usually announced mid-season for end-of-season, occasionally immediate on contract expiry.
- Probability scales with age, modulated by CA/physical collapse, lack of contract or first-team football, accumulated injuries, and reputation (higher-level players hang on longer). Outfield mid-30s typical; GKs notably later (late 30s–40s). Career-ending injuries can force retirement at any age.
- Retired players may enter the staff pool (coaches/scouts/managers). International retirement (~29–33) is separate from club retirement.
- The decision is stochastic, not an age gate — community threads document players retiring young after injuries or right after signing extensions.

## Sources

Official/directly observable: Sports Interactive FM24 manual player page
(attributes, position familiarity, scouting ranges, development inputs), FM
in-game behavior, footballmanager.com Dugout guides (youth intake, youth
development).

Community/reference: GameFAQs CM 01/02 guides (classic visible/hidden
attributes and CA/PA 0-200), Sports Interactive forum CA-weight experiments
using official editor surfaces, champman0102.net (CM 01/02 editor/community
heritage), Steam/SI forum personality tables, fmscout.com / sortitoutsi.net /
passion4fm.com / fminside.net (CA/PA, hidden attributes, personality, star
ratings, intake dates).

Black-box testing: fm-arena.com season-batch attribute testing, age curves, and
fitness tests. Use for calibration warnings only.
