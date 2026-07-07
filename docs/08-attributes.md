# Attribute Taxonomy

This is the current code contract for Agentic FC's player model. The old
15-visible-attribute surface has been replaced by a wider CM/FM-inspired model
because the compact version hid too much football causality: a cross, a
through-ball, a header, a first touch under pressure, and a keeper's box claim
need different inputs.

The model deliberately separates three concepts:

- **Ability Pool cost**: how expensive a generated/developed profile is.
- **Event weights**: when a profile matters in a match.
- **Hidden traits**: volatility, durability, personality, career pressure, and
  social behavior. Raw hidden values never cross MCP/Console surfaces.

The public reference basis and confidence grades are recorded in
[91-reference-fm-player-model.md](91-reference-fm-player-model.md). Agentic FC
does not clone a closed FM formula; it uses the public taxonomy and
Current-Ability-style budget pattern to build its own explainable model.

## 1. Scales

| Thing | Scale | Notes |
|-------|-------|-------|
| Visible attributes | 1-20 integer | Football abilities used by event formulas. |
| Weak-foot proficiency | 1-20 integer | Stored like visible ability, costed separately, surfaced as exact/range plus descriptor by knowledge. |
| Hidden attributes | 1-20 integer unless stated | Descriptor/evidence only. |
| Ability Pool / Potential Cap | 0-200 integer | Stored as round(ProfilePoolCost). |
| Reputation | 0-10,000 | Descriptor-only on the wire. |
| Condition / Sharpness | 0-100% | State, not attributes. |
| Height / Weight | cm / kg public facts | Not Ability Pool; bounded expression modifiers. |
| Match rating | x10 integer, displayed as one decimal | Practical band is controlled in `worldgen.LiveRatingsX10`. |

## 2. Visible Attributes

Outfielders carry 33 visible attributes: 11 technical, 14 mental, and 8
physical. Goalkeepers replace the 11 outfield technical rows with 10
goalkeeping rows, while keeping the shared mental and physical rows.

### 2.1 Outfield Technical

| Attribute | Meaning |
|-----------|---------|
| Finishing | Shot placement and scoring execution. |
| Long Shots | Shooting threat outside the box. |
| First Touch | Receiving and setting the next action under pressure. |
| Passing | Ground passing, tempo retention, and through-ball quality. |
| Crossing | Wide delivery and early balls. |
| Dribbling | Beating a player or carrying through pressure. |
| Technique | Clean execution of hard actions. |
| Heading | Direction and power of headers after contact. |
| Tackling | Ball-winning challenge quality. |
| Marking | Tracking and denying space/opponents. |
| Set Pieces | Corners, free kicks, penalties, and long dead-ball delivery. |

### 2.2 Shared Mental

| Attribute | Meaning |
|-----------|---------|
| Aggression | Willingness to engage in duels, presses, and challenges. Not dirtiness by itself. |
| Vision | Seeing progressive or creative options. |
| Decisions | Choosing the right action. |
| Composure | Executing under pressure. |
| Concentration | Avoiding lapses across repeated actions. |
| Positioning | Defensive positioning and rest-defense reading. |
| Off Ball | Attacking movement and space finding. |
| Anticipation | Reading passes, rebounds, and second balls early. |
| Work Rate | Repeated effort and pressing appetite. |
| Bravery | Willingness in contested headers, blocks, and tackles. |
| Teamwork | Following team structure and supporting teammates. |
| Leadership | On-pitch organization. |
| Determination | Persistence under adversity. |
| Flair | Appetite and ability to try creative, risky actions. |

### 2.3 Shared Physical

| Attribute | Meaning |
|-----------|---------|
| Acceleration | First steps and separation. |
| Pace | Top speed over space. |
| Agility | Turning and body control. |
| Balance | Staying stable under contact or pressure. |
| Strength | Contact strength and shielding. |
| Stamina | Sustained output. |
| Natural Fitness | Recovery between matches and resistance to physical decline. |
| Jumping Reach | Reach in aerial contests, including leap timing. |

### 2.4 Goalkeeping

| Attribute | Meaning |
|-----------|---------|
| Reflexes | Reaction saves and unpredictable close-range events. |
| One on Ones | Judgement and execution when isolated against a finisher. |
| Handling | Catching and spill resistance. |
| Aerial Reach | Keeper's physical reach in the air. |
| Command of Area | Decision and authority to claim/hold/organize in the box. |
| Communication | Organizing defenders and box calls. |
| Distribution | Kicks, throws, and build-up quality. |
| Sweeping | Starting position and rushing-out actions. |
| Eccentricity | Tendency to attempt unusual keeper actions. High is not always better. |
| Punching | Tendency to punch rather than catch. High is not always better. |

### 2.5 Public Profile Facts

| Fact | Visibility | Match role |
|------|------------|------------|
| Preferred Foot | public | Profile/context for role fit. |
| Weak Foot | masked like visible ability | Cutbacks, first-time finishes, turns, and two-sided play. |
| Height | public | Bounded modifier to `JumpingReach` / `AerialReach` expression. |
| Weight | public | Bounded modifier to `Strength` expression. |
| Position Familiarity | descriptor by position | Future out-of-position penalties; primary position generated as Natural. |

Height and weight express ability; they do not replace it. A tall player with
poor Heading is still a poor header. A heavier player with low Strength is not
automatically dominant. Generation biases body and archetype together, then the
match model applies only small deterministic modifiers.

## 3. Hidden Attributes

All raw hidden values are private. MCP/Console surfaces may show only
Descriptors, evidence phrases, or public consequences such as injuries, cards,
contract behavior, or media behavior.

### 3.1 Trajectory

| Attribute | Meaning |
|-----------|---------|
| Potential Cap | 0-200 long-run ceiling. |
| Development Speed | How quickly growth converts opportunity into ability. |
| Decline Onset | When decline tends to begin. |
| Decline Speed | How sharply decline expresses. |

### 3.2 Personality and Social

| Attribute | Meaning |
|-----------|---------|
| Professionalism | Training seriousness, self-care, decline resistance. |
| Ambition | Career drive, transfer desire, development hunger. |
| Loyalty | Attachment to club/manager/squad. |
| Temperament | Emotional control. |
| Pressure | Stress response in high-expectation contexts. |
| Adaptability | Settling after transfers or tactical/cultural change. |
| Sportsmanship | Fair-play tendency; reluctance to exploit dark arts. |
| Controversy | Media friction and disruptive behavior risk. |
| Sociability | Chemistry formation and dressing-room mood spread. |
| Influence | Dressing-room weight and captaincy gravity. |

### 3.3 Volatility and Durability

| Attribute | Meaning |
|-----------|---------|
| Consistency | Reliability of visible ability expression. |
| Big Match Nerve | High-stakes reliability. |
| Injury Proneness | Knock and lay-off risk. |
| Recovery | Injury duration and condition recovery. |
| Discipline | Scouting evidence trait; card rolls now primarily read Aggression + Temperament + Sportsmanship. |
| Versatility | Position learning and future out-of-position penalty size. |
| Reputation | 0-10,000 market/fan/board pull; descriptor-only. |

The current personality descriptor precedence is implemented in
`attr.PersonalityDescriptor`. `Controversy >= 16` can now produce a Volatile
read even if Temperament itself is not low.

## 4. Ability Pool Cost

Pool consumed = sum((attribute - 1) * position weight) + weak-foot cost.
Weights are stored as 0.1-grained values in `internal/attr.PoolCosts`, and the
stored `Player.AbilityPool` is `round(ProfilePoolCost)`.

The table is an economics surface, not a match formula. Cheap Set Pieces can
still decide a set piece; expensive Acceleration must not decide stationary
headers. Hidden/personality attributes do not spend Ability Pool.

### 4.1 Outfield Technical Costs

| Attribute | DF | MF | FW |
|-----------|----|----|----|
| Finishing | 0.3 | 0.7 | 1.8 |
| Long Shots | 0.2 | 0.6 | 0.8 |
| First Touch | 0.8 | 1.4 | 1.2 |
| Passing | 0.9 | 1.5 | 1.0 |
| Crossing | 0.5 | 0.8 | 0.8 |
| Dribbling | 0.5 | 1.2 | 1.4 |
| Technique | 0.7 | 1.3 | 1.2 |
| Heading | 1.1 | 0.4 | 1.0 |
| Tackling | 1.8 | 0.9 | 0.2 |
| Marking | 1.7 | 0.9 | 0.2 |
| Set Pieces | 0.3 | 0.4 | 0.4 |

### 4.2 Shared Mental Costs

| Attribute | GK | DF | MF | FW |
|-----------|----|----|----|----|
| Aggression | 0.1 | 0.2 | 0.2 | 0.2 |
| Vision | 0.4 | 0.7 | 1.4 | 1.1 |
| Decisions | 1.6 | 1.3 | 1.4 | 1.1 |
| Composure | 1.0 | 1.0 | 1.1 | 1.2 |
| Concentration | 1.4 | 1.4 | 1.0 | 0.8 |
| Positioning | 1.0 | 1.6 | 1.2 | 0.6 |
| Off Ball | 0.1 | 0.4 | 1.0 | 1.4 |
| Anticipation | 1.2 | 1.4 | 1.2 | 1.2 |
| Work Rate | 0.3 | 0.8 | 1.0 | 0.8 |
| Bravery | 0.4 | 0.7 | 0.4 | 0.6 |
| Teamwork | 0.4 | 0.5 | 0.8 | 0.6 |
| Leadership | 0.4 | 0.5 | 0.5 | 0.4 |
| Determination | 0.1 | 0.1 | 0.1 | 0.1 |
| Flair | 0.1 | 0.1 | 0.4 | 0.5 |

### 4.3 Shared Physical Costs

| Attribute | GK | DF | MF | FW |
|-----------|----|----|----|----|
| Acceleration | 0.4 | 2.0 | 2.0 | 2.2 |
| Pace | 0.5 | 2.0 | 2.0 | 2.2 |
| Agility | 1.2 | 1.0 | 1.1 | 1.3 |
| Balance | 0.8 | 0.9 | 1.0 | 1.0 |
| Strength | 0.7 | 1.3 | 1.0 | 1.2 |
| Stamina | 0.4 | 1.0 | 1.2 | 1.0 |
| Natural Fitness | 0.1 | 0.1 | 0.1 | 0.1 |
| Jumping Reach | 0.5 | 1.2 | 0.5 | 0.8 |

### 4.4 Goalkeeping Costs

| Attribute | GK |
|-----------|----|
| Reflexes | 2.0 |
| One on Ones | 1.6 |
| Handling | 1.6 |
| Aerial Reach | 1.2 |
| Command of Area | 1.2 |
| Communication | 0.8 |
| Distribution | 0.9 |
| Sweeping | 0.8 |
| Eccentricity | 0.1 |
| Punching | 0.1 |

### 4.5 Weak-Foot Costs

Weak-foot cost = `(weak_foot - 1) * position_weight`.

| Position group | Weight |
|----------------|--------|
| GK | 0.2 |
| DF | 0.5 |
| MF | 0.8 |
| FW | 1.0 |

## 5. Match Model Hooks

The match model keeps the existing deterministic key-moment loop but replaces
the generic chance shortcut with event families:

- `CROSS_HEADER`
- `CUTBACK`
- `THROUGH_BALL`
- `LONG_SHOT`
- `SET_PIECE_HEADER`
- `SCRAMBLE`
- `COUNTER`

Tactical width, directness, tempo, pressing, and counter intent bias the event
distribution. Each event then reads a different attribute mix. Examples:

- Cross headers: team Crossing delivery, receiver reach, Heading, Bravery,
  Off Ball, and defensive/keeper aerial response.
- Cutbacks: wide delivery, Finishing, Composure, Off Ball, and weak-foot
  expression.
- Through balls: team Passing/Vision/Technique/Decisions, receiver
  Acceleration, Off Ball, First Touch, and Finishing.
- Counters: press impact, Pace/Acceleration, Decisions, Finishing, Sweeping,
  and One on Ones.

Finished and live matches persist `chance_types` internally plus public
`MatchDiagnostics` (shot-quality bands, aerial attempts/wins, press turnovers,
set-piece threat, tactical tilt). MCP renders the chance mix as player-facing
`match_patterns` and exposes only those observed diagnostic rows, while the
Console/TUI uses the richer spectator chance-mix and diagnostics view. Both
surfaces can show what kind of problems a tactic is creating without exposing
resolution weights or private traits.

## 6. Descriptor Rules

| Surface | Rule |
|---------|------|
| Attribute values | Own/full-scout exact; otherwise quantized ranges. |
| Weak foot | Same knowledge ladder as visible attributes, plus descriptor: Strong / Useful / Limited / One-footed. |
| Positional familiarity | Natural / Accomplished / Competent / Awkward. |
| Personality | Descriptor/evidence only, never raw hidden values. |
| Reputation | Descriptor only. |

## 7. Tuning Notes

- `PoolCosts` is the balance ledger. Match event formulas may use different
  weights and must stay event-specific.
- Speed and reach are intentionally expensive and useful, but they should not
  become universal meta inputs. Regression work should measure event dominance
  over season batches.
- Body modifiers are small by design: they bias expression without replacing
  football skill.
- Weak foot is costed because it expands usable actions, especially for
  forwards, wide players, and playmakers.
