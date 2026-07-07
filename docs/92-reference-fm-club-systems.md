# Reference: FM Club Management & Meta Systems

Research reference on Football Manager's management layer. Part of the CM/FM reference set — see [90-reference-cm-fm.md](90-reference-cm-fm.md) for the overview and design lessons.

## 1. Manager career system

**Creation — two orthogonal levers:**
- **Past playing experience** (None → Sunday League → Amateur/Semi-Pro → Professional → International): sets **starting reputation** plus small mental bonuses.
- **Coaching qualifications** (None → National C/B/A → Continental C/B/A → Continental Pro): set **coaching ability** (a CA-like point pool across coaching attribute categories).
- Community experiments confirm the split: *badges give coaching ability, experience gives reputation*. High-rep/low-badge managers get interviews but underperform on the training ground; low-rep managers can't get big-club interviews at all. "Sunday League, no badges" is canonical hard mode.

**Reputation:** hidden 0–10,000, surfaced as descriptors (obscure → local → regional → national → continental → worldwide). Grows with overachievement vs. media prediction, trophies, promotions; decays with sackings and idle unemployment. Gates job interviews, player respect (team-talk effectiveness), and board receptiveness.

**Jobs & interviews:** a Job Centre lists clubs with incumbent-manager security status (Untouchable / Secure / Okay / Insecure / Precarious / Very Insecure / Under Review). You can **Apply** to vacancies or **Declare Interest** in occupied seats (only effective with strong reputation; can read as unprofessional). Interviews (FM19+) are multi-question dialogues — style of play, youth use, transfer approach, expected finish — and your answers become **tracked promises/expectations** feeding the Club Vision. Objectives can be negotiated up/down, trading budget and patience.

**Sacking:** driven by aggregated board confidence (below). Trajectory: confidence slips → private warning/board meeting → public vote of confidence or **ultimatum** ("X points from next Y games") → dismissal. Catastrophic runs or takeovers can skip steps. Sacking cuts reputation; contract compensation is paid.

**Unemployment & re-entry:** wait for vacancies, watch the security column, apply widely to farm interview exposure, drop divisions to rebuild reputation. Unemployment slowly erodes reputation.

**The merry-go-round:** AI managers live in the same system — reputation, style, ambitions. AI boards sack on the same confidence logic, shortlist by reputation/style fit/wage demands, and appoint — cascading vacancies down the pyramid mid-season. AI managers also resign, get poached, take national jobs, retire, and are replaced by **newly generated managers**, so the pool churns for decades. The human competes in this same market and can be head-hunted while employed.

## 2. Board & fans

**Board confidence:** per-area satisfaction (Competitions, Finances, Match results, Transfers, Squad performance) plus an overall descriptor (Delighted → … → Precarious).

**Club Vision (FM20+), three strands:**
- **Club culture** — persistent philosophies ("develop through the academy", "play attractive football", "sign under-24s", "maintain financial stability"). Weighted almost as heavily as results.
- **Season objectives** — per-competition targets with importance levels, negotiated at hire and season review.
- **Five-year plan** — staged milestones (promote → consolidate → establish → European places).

**Board requests:** budgets, facility upgrades, stadium expansion, affiliates, scouting range. Rejections can be escalated to an **ultimatum** — board caves, refuses, or sacks you.

**Takeovers** (random, non-triggerable): Internal reshuffle / Fan group / Local consortium / **Foreign tycoon** (huge money, huge expectations). Freeze transfers during due diligence; may replace the vision wholesale; new owners commonly sack the incumbent or hand out fresh budgets.

**Supporter Confidence (FM23+):** each club's fanbase is a mix of six profiles — **Hardcore, Core, Family, Fair Weather, Corporate, Casual** — each with its own patience and priorities (lower-league clubs skew Hardcore/patient; growing clubs accumulate impatient Fair Weather fans). Fans rate the manager on Match Performance, Transfer Activity, Tactics/identity, Squad; the board watches objectives and finances, fans watch identity and performances. Selling a fan darling tanks fan confidence.

## 3. Finances

- **Two budgets** set by the board (revised on takeover/promotion/renegotiation): **transfer** and **wage**, with a **conversion slider** (board-set exchange rate; locked after use, unavailable while over budget). Board sets a **% of sale revenue** returned for re-spending (healthier finances = higher %).
- **Revenue:** gate/season tickets, TV, prize money, sponsorship, merchandising, player sales, loan fees. **Expenditure:** wages (largest recurring line), fees & instalments, agent fees, bonuses, staff, maintenance, youth setup, debt service.
- **Financial state ladder:** Rich → Secure → Okay → Insecure → bank-controlled spending → Insolvent → **Administration** (points deductions, embargoes, forced sales). State controls board generosity.
- **FFP:** UEFA-style break-even where licensed; FM26 adds **Squad Cost Ratio** (wages + amortized fees + agent fees capped ~70% of revenue; breaches → fines, points deductions, embargoes). Academy/infrastructure spend exempt.
- **Benefactor owner types** *(database flag)*: **Foreground** (bankrolls everything), **Background** (releases funds as rewards for growth), **Underwriter** (clears debts then hands off), **Underwriter expecting return** (recoups investment later). Owner-departure risk is the classic downfall of benefactor saves.

## 4. Transfer system

**Flow:** enquiry → offer → accept/reject/counter → contract talks with player+agent → medical/permit → done. Delegable to a DoF. Scout reports show an **estimated cost range** distinct from profile "Value"; rival bids set the floor; deadline pressure hardens sellers.

**Offer structure:** up-front fee + instalments + conditional fees (per appearance, after N appearances/caps/goals, after promotion) + clauses (**sell-on %**, buy-back, friendly, loan-back). Conditional money is discounted by the AI — unrealistic conditions are valued at zero. Current-season cost vs. future commitments is the standard trick for spending beyond this year's pot. Part-exchange accepted only if the AI wants that player.

**Contracts (negotiated with the agent):** agent personality axes — Patience (tolerates ~2–10 rounds), Personal Gain (agent fee), Client Gain (player terms); failed talks lock renegotiation ~2 weeks. Terms: **squad status** (Star Player → … → Not Needed; drives wage demands and playing-time promises), length, wage, signing-on, **loyalty bonus**, agent fee, performance bonuses, and clauses: **minimum fee release** (with foreign/domestic/higher-division/relegation variants), yearly rise %, promotion/relegation wage changes, one-year auto-extension after N games, optional club extension. Playing-time expectations become tracked **promises**; breaking them cascades.

**Windows/loans/frees:** windows per league calendar (deals agreed early with delayed dates; deadline-day drama modeled). Loans: wage split, monthly fee, recall clause, playing-time guarantee, option/obligation to buy. **Bosman:** pre-contracts from 6 months before expiry (foreign clubs; domestic restricted until final weeks); under-24 tribunal compensation.

**AI valuation:** reputation, own-scout-judged CA/PA, age curve, contract time remaining, positional need, style fit, club transfer policy. Low remaining contract craters price; AI bids opportunistically for unhappy/listed players.

## 5. Scouting & recruitment

- **Knowledge model:** per-staff, per-nation scouting knowledge (100% home; built by working/scouting there; decays when neglected). Club knowledge = best single source among staff + affiliates; low knowledge hides low-reputation players from search entirely.
- **Assignments:** player (one-off or ongoing), match/competition, nation/region, or **recruitment focus** (position/age/budget/star thresholds, ongoing with periodic scouting meetings). Chief Scout can run everything. Scouting range is board-restricted at small clubs; scouting budget caps travel.
- **Reports:** knowledge % governs fidelity — attribute **ranges** narrow with knowledge; CA/PA **star ratings relative to your squad** with dark-star uncertainty bands; pros/cons; role suitability vs. current options; estimated cost & wage demands; personality/media handling (knowledge-gated); injury susceptibility. Reports are dated snapshots — re-scout to refresh. Accuracy scales with the scout's judging attributes; different scouts disagree.

## 6. Tactics framework

- **Structure:** formation (~40–50 base shapes) → per-slot **role + duty** → team instructions → per-player instructions. Up to 3 tactics train simultaneously.
- **Roles & duties:** FM24 has **45 roles** × Defend/Support/Attack duties ≈ 90+ combos (Sweeper Keeper, Ball-Playing Defender, Inverted Wing-Back, Deep-Lying Playmaker, Mezzala, Shadow Striker, False Nine, Target Man…). FM26: **72 roles** under a new In-Possession/Out-of-Possession dual-role system. Role suitability shown per player.
- **Team instructions:** global **Mentality** (7 levels FM19+: Very Defensive → Very Attacking) is a risk dial shifting tempo, width, line heights, directness, and every player's individual mentality. TIs grouped: **In Possession** (directness, tempo, width, creative freedom…), **In Transition** (counter vs. regroup, counter-press, GK distribution), **Out of Possession** (line height, engagement line, pressing intensity, traps, offside trap). Preset styles: Gegenpress, Tiki-Taka, Route One, Catenaccio, Park the Bus…
- **Tactical familiarity:** per-tactic bar from Awkward to Fluid across components; raised by training and match use; partially reset by changes; new signings start unfamiliar. Low familiarity = mispositioning and slow decisions.
- **Opposition instructions:** per-opponent orders (tight-mark, always close down, tackle harder, show onto weak foot). Delegable.

## 7. Training

- **Schedules (FM19+):** up to 3 sessions/day from ~50 session types (General, Match Prep, Physical, Tactical, Technical, Set Pieces, Recovery, Rest…). Delegable.
- **Individual training:** position/role/duty retraining, attribute focus, trait learn/unlearn, per-player intensity, mentoring groups.
- **Intensity & injuries:** high physical load without recovery measurably raises muscle injuries (the canonical pre-season mistake). Sports scientists/physios mitigate.
- **Development role:** playing time (biggest factor) + training quality + personality + mentoring. Weekly training ratings feed morale and flag risers/slackers.

## 8. Squad dynamics & morale

- **Morale** (Abysmal → Superb) drives performance and training; inputs: playing time vs. promised status, team and personal form, contract satisfaction (including wage envy), settledness, manager interactions, promises kept/broken, transfer activity around them.
- **Dynamics (FM18+):** three pillars — Team Cohesion, Dressing Room Atmosphere, Managerial Support. **Hierarchy** (Team Leaders → Highly Influential → Influential → Others) from reputation/tenure/age/personality; upsetting a Team Leader propagates unhappiness (mutiny mechanics). **Social groups** (Core / Secondary / Others) form by tenure, age, nationality/language; cliques of unsettled players breed contagion.
- **Promises:** formal, deadline-tracked commitments from interviews, contract talks, and chats (playing time, signings, facilities, captaincy, letting a player leave). Breaking one hits trust and can cascade through the player's allies.
- **Interactions:** praise/criticize form/training/conduct, team meetings, fines. Every interaction rolls against personality + your reputation + relationship; bad tone backfires — public criticism of an influential player is the classic self-inflicted crisis.

## 9. Media

- **Press conferences:** pre/post-match and event-driven; ~5–8 questions, 5–6 answer options tagged by tone. Answers shift player morale, board perception, journalist relationships, rival managers (mind games), and fan sentiment. Delegable (with risk). **The series' longest-running complaint is repetitive questioning** — a cautionary tale for narrative variety.
- **Other surfaces:** tunnel interviews, written Q&As on rumours, journalist relationships (friendly journos ask softer questions), media prediction table framing season expectations.
- **Rivalries:** database-defined tiers; derbies carry extra morale/pressure weight, fans weigh them disproportionately, and players reject moves to hated rivals.

## 10. Staff

Roles: Assistant Manager (training, talks, pressers, opposition reports — the catch-all delegate), Coaches (per-category quality stars, workload matters), **Director of Football** (transfers/contracts/sales delegation hub), Technical Director (staff hiring), Chief Scout, Scouts, Recruitment/Data Analysts, **Head of Youth Development** (intake quality/personality imprint), Youth/Reserve managers, Physios, Sports Scientists, Performance Analysts.

Everything sits on a single **Staff Responsibilities matrix** (per-duty dropdown, including "yourself"). Veteran advice: keep transfers, contracts, tactics, training, and media in hand; delegate friendlies, listed-player sales, youth admin, scouting logistics. **FM proves that delegation-based management is a complete, playable game loop** — the relevant precedent for a game where the Agent delegates *everything* to the Manager.

## Sources

sortitoutsi.net (badges-vs-experience experiment, unemployed guides, FM23 supporter confidence, press conference guides), SI community forums (sacking, declare-interest, sugar daddy types), fmscout.com (confidence, finances, sugar daddies FM26, role guides), footballmanager.com official (Club Vision, Supporter Confidence, smarter transfers FM26, delegation, perfecting your manager), guidetofm.com via Wayback (transfers, contracts, scouting, morale, staff, mentality), twoplaymakers.com (takeovers, loan clauses), passion4fm.com (star ratings, tactical familiarity, training, staff responsibilities), fm-arena.com (familiarity testing), footballmanagerblog.org (FM18 dynamics, training), givemesport/videogamer/games.gg guides, footballmanager.fandom.com, en.wikipedia.org (series history), pcgamesn/pcgamer (history retrospectives).
