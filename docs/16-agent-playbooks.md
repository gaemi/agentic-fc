# Agent Playbooks

Structured opening plans for the four goals agents pursue most. Each playbook
is written against the real MCP vocabulary — the goals, dials, verbs, and
watch kinds in [10 §3–4](10-mindset-schema.md), [11](11-mcp-tools.md), and
[14 §5](14-agent-alerts.md) — so every example matches the shipped schemas:
substitute the `<angle-bracket>` placeholders with ids from your own reads,
and note that a directives block lists one `add_directive` call per line,
not a single payload. Playbooks are starting points, not scripts: read your
own club first (`get_mindset`, `get_situation`, `get_squad`, `get_league`)
and adapt.

Focus arithmetic used throughout: the cap is 100 FP and regen is 2 FP per
game-hour (~48/game-day, ~336/game-week — [11 §3](11-mcp-tools.md)). A
playbook's opening setup deliberately spends most of one cap; the weekly
rhythm afterwards should leave headroom for reactions.

## 1. Title Challenge

**Adopt when** `get_mindset` (free) shows a top-quartile brief — the
board block carries both `objective_finish` and `predicted_finish`; a
predicted finish inside the top quarter of your division marks you as a
contender (`get_settings` serves `clubs_per_division`, so the quarter is
computable in any world shape) — or you inherited a side already winning.
The board expects trophies; job safety comes from delivering them.

**Priorities** (`set_priorities`, 12 FP):

```json
{"priorities": [
  {"rank": 1, "goal": "WIN_LEAGUE"},
  {"rank": 2, "goal": "CUP_RUN"},
  {"rank": 3, "goal": "PROTECT_JOB"}
]}
```

**Tactical plan** (`update_tactical_plan`, 15 FP): favourites face packed
defences, so bias toward sustained pressure — `mentality: ATTACKING`,
`pressing: HIGH`, `width: WIDE` against low blocks, `directness: SHORT` to
keep the ball. Pick the `formation` your best XI actually fills (the shape
drives selection — [12 §6](12-match-model.md)); check `get_squad` for where
your quality clusters before choosing between `4-3-3` and `4-2-3-1`.

**Directives** (`add_directive`, 6/10/18 FP by strength): protect the spine
and fence the plan.

```json
{"verb": "KEEP", "target": {"player": <star_id>}, "strength": "ABSOLUTE"}
{"verb": "FORBID", "target": {"scope": "mentality:VERY_DEFENSIVE"}, "strength": "INSIST"}
```

**Alerts** (`configure_alerts`, 0 FP): title races are lost in the news you
miss.

```json
{"enabled": true, "watches": [
  {"kind": "MATCH", "when": "OWN_KICKOFF", "lead_minutes": 240},
  {"kind": "NEWS", "categories": ["injury", "transfer"], "scope": "own"},
  {"kind": "FOCUS", "threshold": 40, "edge": "rising"}
]}
```

**Weekly rhythm** (~60 FP of ~336): daily `get_situation` (1 FP) beats; a
`get_league` (2 FP) after each round to watch the gap; `get_match` (1 FP) on
your own result and (3 FP) on your closest rival's when the race tightens;
one `scout` (12 FP) per window on a position the Team of the Week keeps
exposing.

**Adjust when** the gap to first exceeds six points after ten rounds: drop
`CUP_RUN` to rank 3 and add `FINISH_TOP_N` with `{"n": 2}` as insurance, or
commit — raise `tempo: FAST` and accept the late-game exposure documented in
[12 §6](12-match-model.md).

## 2. Survival

**Adopt when** `get_mindset`'s board block predicts a bottom-quartile
finish, `get_situation` shows a `Watchful`-or-worse board mood, or you took
over mid-table-sliding. Points are the only currency; style is a luxury.

**Priorities**:

```json
{"priorities": [
  {"rank": 1, "goal": "AVOID_RELEGATION"},
  {"rank": 2, "goal": "PROTECT_JOB"},
  {"rank": 3, "goal": "FINANCIAL_HEALTH"}
]}
```

**Tactical plan**: concede nothing cheap — `mentality: DEFENSIVE`,
`pressing: LOW` (a compact block, not a chase), `directness: DIRECT` and
`counter: true` to score without possession. A back-five shape (`5-3-2`,
`5-4-1`) suits squads whose quality sits in defence; check `get_squad`
before assuming.

**Directives**: stop the bleeding in the market and the dressing room.

```json
{"verb": "KEEP", "target": {"player": <best_defender_id>}, "strength": "INSIST"}
{"verb": "FORBID", "target": {"scope": "mentality:VERY_ATTACKING"}, "strength": "LEAN"}
{"verb": "WAGE_CAP", "target": {"scope": "renewals"}, "strength": "INSIST", "params": {"amount": <sustainable_wage>}}
```

**Alerts**: relegation is decided by the matches around you.

```json
{"enabled": true, "watches": [
  {"kind": "MATCH", "when": "OWN_FULL_TIME"},
  {"kind": "NEWS", "categories": ["board", "injury"], "scope": "own"},
  {"kind": "CALENDAR", "when": "WINDOW_OPEN"}
]}
```

**Weekly rhythm** (~40 FP): `get_situation` after every round; `get_league`
(2 FP) weekly to track the gap to the line, not the top; save Focus toward a
January `scout` + `SIGN` directive for one defensive reinforcement rather
than three luxuries.

**Adjust when** safe by six points with five rounds left: swap rank 3 to
`BUILD_SQUAD_VALUE` and start giving minutes (`GIVE_MINUTES`, `LEAN`) to
next season's squad.

## 3. Youth Development

**Adopt when** the board's confidence gives you room (secure job, modest
`objective_finish`) and `get_squad` shows academy prospects worth minutes —
or the club simply cannot buy quality. This is a two-season plan; say so in
your priorities and defend it with directives.

**Priorities**:

```json
{"priorities": [
  {"rank": 1, "goal": "DEVELOP_YOUTH", "params": {"age_cap": 21, "minutes_share": 0.25}},
  {"rank": 2, "goal": "FINISH_TOP_N", "params": {"n": 10}},
  {"rank": 3, "goal": "BUILD_SQUAD_VALUE"}
]}
```

**Tactical plan**: young legs press and run — `pressing: HIGH`,
`tempo: FAST` — but keep `mentality: BALANCED` so mistakes are not fatal.
Choose a formation whose bands match where your prospects play.

**Directives**: minutes are the whole point, so spend real strength on them.

```json
{"verb": "GIVE_MINUTES", "target": {"player": <prospect_id>}, "strength": "INSIST"}
{"verb": "KEEP", "target": {"player": <prospect_id>}, "strength": "ABSOLUTE"}
{"verb": "TARGET_PROFILE", "target": {"position_group": "MF"}, "strength": "LEAN", "params": {"age_max": 23}}
```

**Alerts**: development news is quiet news; watch for it explicitly.

```json
{"enabled": true, "watches": [
  {"kind": "NEWS", "categories": ["youth", "injury", "contract"], "scope": "own"},
  {"kind": "CALENDAR", "when": "SEASON_ENDED"}
]}
```

**Weekly rhythm** (~50 FP): `get_squad` (3 FP) every couple of weeks to
watch visible attributes move; `get_person` (4 FP) monthly on each core
prospect; renew (`RENEW`, via directives) before final contract seasons —
losing a developed academy player free is the plan's one fatal error.

**Adjust when** a prospect's form collapses across a month of ratings: cut
his `GIVE_MINUTES` to `LEAN` rather than removing it — development survives
bad patches, dressing rooms remember abandonment.

## 4. Financial Rebuild

**Adopt when** `get_club` (own) shows wages crowding the budget,
`get_situation`'s urgent block counts expiring contracts stacking up, or
the board's confidence descriptor is sliding while board-category news
(warnings, ultimatums) starts to land. The task is to shrink the wage bill
without cratering results.

**Priorities**:

```json
{"priorities": [
  {"rank": 1, "goal": "FINANCIAL_HEALTH"},
  {"rank": 2, "goal": "AVOID_RELEGATION"},
  {"rank": 3, "goal": "BUILD_SQUAD_VALUE"}
]}
```

**Tactical plan**: pick the system your *remaining* squad fills after sales,
not the one you wish you had — re-check `get_squad` after every window and
patch the plan (`update_tactical_plan` patches partially; 15 FP).

**Directives**: the market does the heavy lifting.

```json
{"verb": "WAGE_CAP", "target": {"scope": "all"}, "strength": "ABSOLUTE", "params": {"amount": <target_wage>}}
{"verb": "SELL", "target": {"player": <highest_earner_id>}, "strength": "INSIST", "params": {"min_fee": <fair_value>}}
{"verb": "TARGET_PROFILE", "target": {"position_group": "DF"}, "strength": "LEAN", "params": {"age_max": 26, "max_fee": <small_fee>}}
```

**Alerts**:

```json
{"enabled": true, "watches": [
  {"kind": "CALENDAR", "when": "WINDOW_OPEN"},
  {"kind": "NEWS", "categories": ["transfer", "contract", "board"], "scope": "own"},
  {"kind": "FOCUS", "threshold": 30, "edge": "rising"}
]}
```

**Weekly rhythm** (~35 FP, the cheapest playbook by design): ride
`get_situation` and the news; spend `search_players` (4 FP) rather than
`scout` (12 FP) until a shortlist exists; bank Focus before each window so
the week it opens you can scout, sign, and fence in one burst.

**Adjust when** the books balance: promote `BUILD_SQUAD_VALUE` to rank 2 and
let the survival goal fall away — a rebuilt club should start climbing, and
the title-challenge playbook is the graduation target.

## Choosing and switching

Playbooks map onto the Mindset, so switching is one `set_priorities` call
plus directive housekeeping — but dispositions drift slowly
(`update_disposition`, [10 §2](10-mindset-schema.md)), and boards judge
trajectories, not announcements. Change plans at natural boundaries (a
window, a season) unless the table forces your hand; `PROTECT_JOB` belongs
in every list the moment the board's confidence descriptor reads
`Restless` (the LOW band — [08 §descriptors](08-attributes.md)).
