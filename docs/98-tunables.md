# Tunables Registry

Every gameplay value marked *(tunable)* or *(initial)* in the design docs, with its single code location. **Changing a tunable means changing the code constant and this row in the same commit.** Tests pin important values and should fail loudly on silent drift.

| Tunable | Initial value | Code location | Doc source |
|---------|---------------|---------------|------------|
| Focus cap | 100 FP | `internal/focus` `Cap` | [11 §2](11-mcp-tools.md) |
| Focus regen | 2 FP/game-hour | `internal/focus` `RegenPerGameHour` | [11 §2](11-mcp-tools.md) |
| Tool costs (flat) | table | `internal/focus` `flatCosts` | [11 §2](11-mcp-tools.md) |
| Agent Alert tool costs | configure/ack 0 FP · get_alerts 1 FP | `internal/focus` `flatCosts` | [11 §2](11-mcp-tools.md), [14](14-agent-alerts.md) |
| Agent Alert pending cap | 512 per Manager | `internal/worldgen` alert state | [14 §7](14-agent-alerts.md) |
| Agent Alert Focus sub-cap / hysteresis | 128 pending Focus alerts · 2 FP hysteresis | `internal/worldgen` alert state | [14 §5](14-agent-alerts.md) |
| Tool costs (own/other) | 2/4 · 3/4 · 1/3 | `internal/focus` `CostOwnOther` | [11 §2](11-mcp-tools.md) |
| Directive costs by strength | 6 / 10 / 18 FP | `internal/mindset` `Strength.FocusCost` | [11 §2](11-mcp-tools.md) |
| Directive odds multipliers | ×2 / ×6 / ×20 | `internal/mindset` `Strength.OddsMultiplier` | [10 §4.1](10-mindset-schema.md) |
| Directive cap | 15 active | `internal/mindset` `MaxDirectives` | FR-19 |
| Priority cap / rank weights | 5 · 1.0/0.6/0.4/0.25/0.15 | `internal/mindset` `MaxPriorities`, `RankWeights` | FR-16c, [10 §3](10-mindset-schema.md) |
| Disposition drift | instant ≤2 · 2 pts/game-week | `internal/mindset` `InstantDelta`, `DriftPerGameWeek` | FR-16b |
| Formation catalog | 12 shapes | `internal/mindset` `FormationCatalog` | [10 §5](10-mindset-schema.md) |
| Speed tiers | 5/15/30/60× | `internal/sim` `Speed*` | FR-2 |
| Runtime pacing settings | Game Speed 5/15/30/60× · idle 2–64× · off-season 2–240× | `internal/consoleapi` admin settings validation, `internal/engine` `Runner.SetPacer` | [02 §5.2](02-game-design.md), [13](13-operations.md) |
| Idle acceleration | 16× base | `internal/sim` `DefaultIdleAcceleration` | [02 §5.2](02-game-design.md) |
| Off-season acceleration | 96× base | `internal/sim` `DefaultOffseasonAcceleration` | [02 §5.2](02-game-design.md) |
| Run profile presets | default 15/16/96 · fast 30/32/192 · slow 15/6/36 | `cmd/agenticfc` `baseRunProfile` | [02 §5.2](02-game-design.md) |
| Ability Pool cost table | per-position attribute weights | `internal/attr` `PoolCosts` | [08 §4](08-attributes.md) |
| Weak-foot Pool cost | GK 0.2 · DF 0.5 · MF 0.8 · FW 1.0 per point above 1 | `internal/attr` `WeakFootCosts` | [08 §4.5](08-attributes.md) |
| Familiarity descriptor thresholds | 18/13/7 | `internal/attr` `FamiliarityDescriptor` | [08 §2](08-attributes.md) |
| Default culture mix | 40/25/25/10 | `internal/worldgen` `DefaultCultureMix` | [09 §2.2](09-world-generation.md) |
| Custom name override length | 64 characters | `internal/worldgen` `maxCustomNameLen` | [09 §2.2](09-world-generation.md) |
| World quality pool bands | D1 band + per-div decrement table | `internal/worldgen` `qualityBands` | [09 §4.2](09-world-generation.md) |
| Pro/rel slots | 15% of division, min 2 | `internal/worldgen` `promotionSlotShare` | [09 §3](09-world-generation.md) |
| Unemployed manager pool | 10% of clubs, min 2 | `internal/worldgen` `unemployedPoolShare` | [09 §4.3](09-world-generation.md) |
| Manager reputation bands | 6000 center, −1500/tier, ±1500 | `internal/worldgen` `managerRep*` | [09 §4.3](09-world-generation.md) |
| Archetype → Disposition tables | 8 archetypes × 10 axes, ±2 jitter | `internal/worldgen` `managerArchetypes` | [09 §4.3](09-world-generation.md) |
| Squad position template | GK 3 / DF 8 : MF 8 : FW 5 | `internal/worldgen` `buildSquadSlots` | [09 §4.2](09-world-generation.md) |
| Potential headroom by age | +60 at 17 → 0 at 30 | `internal/worldgen` `headroomAt17`, `headroomEndAge` | [09 §4.2](09-world-generation.md) |
| Free-agent share | 8% of squad population, ×0.75 pool | `internal/worldgen` `freeAgentShare`, `freeAgentPoolCut` | [09 §4.2](09-world-generation.md) |
| Initial academy prospects | 3–5 per club, ages 15–17 | `internal/worldgen` `academyMin`, `academySpan` | [09 §4.2](09-world-generation.md) |
| Marquee earner chance | 22% per club, ×1.7–2.2 wage | `internal/worldgen` `marqueeChance` | [09 §4.2](09-world-generation.md) |
| Division economy decay | 0.45 per tier | `internal/worldgen` `divisionEconomyDecay` | [09 §3](09-world-generation.md) |
| Top-division revenue base | cr 30M/season | `internal/worldgen` `revenueTopDivBaseMinor` | [09 §3](09-world-generation.md) |
| Economy scale factors | 0.6 / 1.0 / 1.8 | `internal/worldgen` `economyScaleFactor` | [09 §2.1](09-world-generation.md) |
| Quality revenue factors | 0.05 / 0.25 / 1.0 / 2.5 | `internal/worldgen` `qualityRevenueFactor` | [09 §2.1](09-world-generation.md) |
| Wage model | share 0.55, pool exponent 1.8 | `internal/worldgen` `wageRevenueShare`, `wagePoolExponent` | [09 §4.2](09-world-generation.md) |
| Season budget rule | wage budget = wage share of weekly tier revenue, floored at bill + 5%; transfer budget = balance × (0.2 + BoardAmbition/20 × 0.4), clamped ≥ 0 | `internal/worldgen` `deriveClubBudgets` (shared: generation stage 7 + season rollover) | [02 §3](02-game-design.md), [09 §4](09-world-generation.md) |
| Transfer valuation | cr 0.05 × pool² (fee, engine-internal) | `internal/worldgen` `transferValuationPerPoolSq` | [02 §2.3](02-game-design.md) |
| Transfer acceptance | free 0.45/0.75/0.95 · listed 0.60/0.85/0.97 | `internal/engine` `signAcceptBase`, `sellListedAccept` | [02 §2.3](02-game-design.md) |
| Caretaker manager | rep ×0.5, coaching/man-management capped 12, archetype "The Pragmatist" | `internal/worldgen` `caretakerRepFactor`, `caretakerAttrCap`, `caretakerArchetypeName` | [02 §3.1](02-game-design.md), FR-14d |
| Board confidence movement | favorite gap 4 places; Δ by [win/draw/loss] fav 2/−2/−5 · even 4/1/−4 · underdog 6/3/−1 | `internal/engine` `confidenceFavoriteGap`, `confidenceDelta` | [02 §3.1](02-game-design.md) |
| Media-prediction noise | Gaussian ×3 on squad strength | `internal/worldgen` `predictionNoise` | [09 §3](09-world-generation.md) |
| Sacking thresholds | warn ≤30, ultimatum ≤20, reprieve ≥45 (confidence); ultimatum 42 days / gain 7 pts; caretaker honeymoon 55 | `internal/engine` `sackWarnThreshold`, `sackUltimatumThreshold`, `sackRecoverThreshold`, `ultimatumDays`, `ultimatumPointsTarget`, `caretakerHoneymoon` | [02 §3.1](02-game-design.md), FR-14b |
| Confidence / security bands | HIGH/SECURE ≥70, MODERATE/STABLE ≥45, else LOW/UNDER_PRESSURE | `internal/mcpserver` `confidenceBand`, `securityBand` | [02 §3.1](02-game-design.md), FR-22a |
| Manager hiring | 25% chance a caretaker review settles the vacancy; candidate reputation floor 3000 at tier 1, −700 per tier down | `internal/engine` `hireReviewChance`, `hireRepFloorTop`, `hireRepFloorStep` | [02 §3.1](02-game-design.md), FR-14a |
| Manager retirement / newgen | never retires below age 60, certain at 72 (linear between); avatar exemption = 2 game-years since the last authenticated call; unemployed pool topped back up to the generation target each season | `internal/engine` `retireAgeFloor`, `retireAgeCeil`, `retirementExemptionWindow`; `internal/worldgen` `UnemployedPoolTarget` | [02 §3.1](02-game-design.md), FR-14e |
| Youth graduation age | 18 — an academy youth turns senior at the season boundary | `internal/engine` `youthGraduationAge` | [02 §2.3](02-game-design.md) |
| Player retirement | never retires below age 32, certain at 40 (linear between) | `internal/engine` `playerRetireAgeFloor`, `playerRetireAgeCeil` | [02 §2.3](02-game-design.md) |
| Contract renewal length | ≤21: 2–3y · ≤29: 1–3y · 30+: 1–2y (per-player stream); youth auto-extend exactly 1 season at the academy wage | `internal/engine` `renewalYears` | [02 §2.3](02-game-design.md) |
| Injury lay-off | 3 + 0–13 days, +1/point of InjuryProne over 10, −1/2 points of Recovery over 10, floor 1; bands: <7 DAYS · <21 WEEKS · else MONTH | `internal/engine` `injuryDaysBase`, `injuryDaysSpan`, `injuryBand` | [02 §2.3](02-game-design.md), [08 §3.4](08-attributes.md) |
| Substitutions | 3 per side per match; bench of 5 picked beside the XI (position-blind) | `internal/engine` `subsMax`, `benchSize` | [12 §7](12-match-model.md) |
| Discretionary substitutions | window opens 60' · fatigue when derived in-match condition <30 (dice-free) · fresh-legs 0.35/moment when a strictly stronger bench outfielder exists · last sub reserved for injuries until 75' | `internal/engine` `tacticalSubFromMinute`, `fatigueSubThreshold`, `tacticalSubProb`, `subReserveUntil` | [12 §7](12-match-model.md) |
| Player form | rolling last-5 ratings; band on the ×10 integer average (truncating division): ≥70 IN_FORM · ≥64 STEADY · else OUT_OF_FORM; UNKNOWN under 3 samples | `internal/engine` `formWindow`; `internal/mcpserver` `formBand`, `formMinSamples`, `formInThreshold`, `formOkThreshold` | [02 §2.3](02-game-design.md), [11 §4](11-mcp-tools.md) |
| Youth intake | `YouthIntakeBatch` (config, 3–8, default 5) prospects per club each spring, throttled by a per-club academy soft cap of 24 (no deletion) | `internal/worldgen` `YouthIntakeBatch` (config), `youthAcademyCap` | [09 §4.2](09-world-generation.md) |
| Stadium capacity bands | 22k base + 40k span, ×0.55/tier | `internal/worldgen` `capacity*` | [09 §4.1](09-world-generation.md) |
| Calendar anchors | Aug 16 first round, monthly cup, spring intake | `internal/worldgen` `day*` (calendar.go) | [09 §3](09-world-generation.md) |
| Match window length | 120 game-minutes | `internal/engine` `MatchWindowMinutes` | [02 §5.2](02-game-design.md) |
| Match sampling | 90-min full time · 18 key moments | `internal/engine` `matchFullTimeMinutes`, `matchMoments` | [03 §3](03-simulation-engine.md) |
| Match chance model | chance 0.55 · convert 0.20 · event-score divisor 140 · probability band 0.03-0.58 · finishing wobble 0.40 · home edge ×1.15 | `internal/engine` `chanceBaseRate`/`conversionBase`/`conversionScoreDivisor`/`conversionMin`/`conversionMax`/`chanceAttackScore`/`chanceDefenseScore`/`volatilitySpread`/`homeAdvantage` | [03 §3](03-simulation-engine.md), [08 §5](08-attributes.md), [12 §5](12-match-model.md) |
| Match chance-type mix | base weights 12/10/11/8/7/6/8 for cross-header/cutback/through-ball/long-shot/set-piece-header/scramble/counter; tactical dials add bounded integer biases | `internal/engine` `chooseChanceType` | [12 §5](12-match-model.md) |
| Match tactical role fit | chance actor role bonuses 20/18/16/12/10/8 by role-event fit; selection fit bonuses 18/16/14/12/8 by tactical dial-role fit | `internal/engine` `roleChance*`, `selectionFit*` | [12 §6](12-match-model.md) |
| Match public diagnostics | shot-quality band from attack-vs-defense delta: HIGH ≥25, LOW ≤−15, else MEDIUM | `internal/engine` `shotQualityHighDelta`, `shotQualityLowDelta` | [12 §8](12-match-model.md), [11 §4](11-mcp-tools.md) |
| Match cards / injuries | card 0.06/moment · 10% red · injury 0.01/moment · knock −35 cond | `internal/engine` `cardRatePerMoment`/`redCardShare`/`injuryRatePerMoment`/`injuryConditionHit` | [03 §3](03-simulation-engine.md) |
| Condition model | drain 22/match (floor 5) · recover 14/drift tick · sharpness +8/match | `internal/engine` `conditionDrainPlay`/`conditionFloorPlay`/`conditionRecoverTick`/`sharpnessGainPlay` | [02 §2.3](02-game-design.md) |
| Player rating band | base 6.5, clamp 6.0–8.0 · goal +0.8 · win/loss ±0.3 · clean sheet +0.5 · yellow −0.3 · red −1.3 | `internal/worldgen` `Rating*X10` (shared: engine full-time + Console live pane via `LiveRatingsX10`) | [02 §2.3](02-game-design.md) |
| In-match decisions | from 55' · max +2 mentality · base 0.15 + Risk Appetite weight 0.55 | `internal/engine` `adjustFromMinute`/`maxMentalityShift`/`adjustBaseProb`/`adjustRiskWeight` | [02 §2.1](02-game-design.md) |
| In-match adjustment commentary | 4 attacking-push variants, cycled independently per club without RNG | `internal/engine` `adjustmentCommentaryKeys` / `adjustmentCommentaryKey` | [02 §4](02-game-design.md) |
| Cup shootout | best-of-5 then bounded sudden death · flat convert 0.75 (winner advances; score cosmetic) | `internal/engine` `shootoutKicks`/`shootoutConvert`/`shootoutSuddenDeathMax` | [09 §3](09-world-generation.md) |
| Live commentary window | last 20 lines per fresh poll | `internal/mcpserver` `liveCommentaryWindow` | [11 §4](11-mcp-tools.md) |
| News article perspective pools | 3 paired deck/body variants for injury and matchday-result articles; stable public News-ID mix | `internal/narrative` `articleVariantCounts` / `ArticleTemplateKey` | [02 §4](02-game-design.md), [11 §4](11-mcp-tools.md) |
| Console UI catalog refresh | 30 seconds; retry every 2-second poll while empty | `internal/tui` `uiRefreshInterval` / `pollInterval` | [07 §6](07-console-design.md) |
| Drift growth base rates | 0.35 / 0.20 / 0.10 (youth/early/prime) | `internal/engine` `driftGrowthBase*` | [03 §3](03-simulation-engine.md) |
| Drift decline model | base 0.10 + DeclineSpeed/100, × prof resistance | `internal/engine` `driftDeclineBase` | [03 §3](03-simulation-engine.md) |
| Decline age formula | 28 + (DeclineOnset−10)/3, clamp 24–34 | `internal/engine` `declineAge*` | [08 §3](08-attributes.md) |
| Drift reschedule horizons | 7/14/10 days + jitter, ×0.6 on change | `internal/engine` `driftInterval*` | [03 §3](03-simulation-engine.md) |
| Finance tick cadence | weekly, ±10% revenue variance | `internal/engine` `financeTickDays` | [03 §3](03-simulation-engine.md) |
| Decision roll cadence | 2–6 game-days | `internal/engine` `decisionInterval*` | [03 §3](03-simulation-engine.md) |
| Reputation descriptor bands | 1500/3000/5000/7000/8500 | `internal/mcpserver` `reputationBand` | FR-22a, [08 §3](08-attributes.md) |
| Disposition drift pace | 1 pt per half game-week (≈2/wk) | `internal/engine` `driftMinutesPerPoint` | FR-16b |
| News ring size | 2000 items | `internal/worldgen` `NewsCap` | [11 §4](11-mcp-tools.md) |
| Player body profile range | 160–205 cm · 58–108 kg, position/archetype bases | `internal/worldgen` `rollBodyProfile` | [08 §1–2](08-attributes.md) |
| Body expression modifiers | height: 5 cm/pt, -3…+4 reach (`JumpingReach`/`AerialReach`); weight: 8 kg/pt, -2…+3 Strength | `internal/engine` body profile constants, `heightBonus`, `massBonus`, `bodyReach`, `bodyStrength` | [08 §2](08-attributes.md) |
| Knowledge masking buckets | 5/3/2/1 by scout level | `internal/mcpserver` `knowledgeBuckets` | FR-22a, [11 §4](11-mcp-tools.md) |
| Scout duration / judgement | 7–14 days · baseline 12 | `internal/engine` `scoutDuration*`, `scoutJudgementBaseline` | [11 §5](11-mcp-tools.md) |
| Market value curve | 5000 minor × pool² · ±25% band, pool quantized to the knowledge bucket | `internal/mcpserver` `valuePerPoolSquaredMinor`, `valueBand` | [11 §4](11-mcp-tools.md) |
| Confidence/security/facility bands | 70/45 · 16/11/6 thresholds | `internal/mcpserver` `confidenceBand`/`securityBand`/`facilityBand` | FR-22a |
| Layout Tier thresholds | 60/100/140/180 cols · 16/28/42 rows | `internal/layout` (constant table) | [07 §2](07-console-design.md) |
| SSE heartbeat interval | 15s | `internal/consoleapi` `Server.HeartbeatInterval` | [11 §1.3](11-mcp-tools.md), A11 |
| Feed cadence hints | 1500/2500/4000 ms by density | `internal/consoleapi` `cadenceFor` | FR-35a |

Future gameplay constants should be added to this table in the same change that
introduces or changes them.
