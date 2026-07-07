package mcpserver

import (
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gaemi/agentic-fc/internal/engine"
	"github.com/gaemi/agentic-fc/internal/focus"
	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/narrative"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// PR-A tool surface: the free tools and the five shaping tools (docs/11
// §3/§6) — enough for an Agent to play. The observation layer (§4/§5)
// lands in the follow-up PR alongside the news store and the knowledge
// model.

type emptyIn struct{}

func (g *Gateway) registerTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        string(focus.GetGuide),
		Description: "Free. Start here: game premise, first steps, strategy loop, and valid Mindset/Tactical vocabulary.",
	}, handle(g, g.getGuide))
	mcp.AddTool(s, appTool(&mcp.Tool{
		Name:        string(focus.GetTime),
		Description: "Free. Game date-time, tempo, run profile, speed, next match window, season phase.",
	}), handleUI(g, g.getTime, timeCard))
	mcp.AddTool(s, appTool(&mcp.Tool{
		Name:        string(focus.GetSettings),
		Description: "Free. Non-seed world settings and pacing table: league shape, run profile, economy/quality, current speed, and real-time estimates.",
	}), handleUI(g, g.getSettings, settingsCard))
	mcp.AddTool(s, &mcp.Tool{
		Name:        string(focus.GetFocus),
		Description: "Free. Focus balance, cap, regen rate, and recent spend history.",
	}, handle(g, g.getFocus))
	mcp.AddTool(s, &mcp.Tool{
		Name:        string(focus.GetMindset),
		Description: "Free. The full Mindset + Tactical Plan, plus manager self-state.",
	}, handle(g, g.getMindset))
	g.registerAlertTools(s)

	mcp.AddTool(s, appTool(&mcp.Tool{
		Name:        string(focus.UpdateDisposition),
		Description: "25 FP. Set disposition axis targets (max 3 per call); deltas ≤2 apply instantly, larger ones drift ~2 pts/game-week.",
	}), handleUI(g, g.updateDisposition, dispositionCard))
	mcp.AddTool(s, appTool(&mcp.Tool{
		Name:        string(focus.SetPriorities),
		Description: "12 FP. Replace the full ranked priority list (max 5 goals, no duplicates).",
	}), handleUI(g, g.setPriorities, prioritiesCard))
	mcp.AddTool(s, appTool(&mcp.Tool{
		Name:        string(focus.AddDirective),
		Description: "6/10/18 FP by strength. Add a standing directive; direct contradictions are rejected.",
	}), handleUI(g, g.addDirective, addDirectiveCard))
	mcp.AddTool(s, appTool(&mcp.Tool{
		Name:        string(focus.RemoveDirective),
		Description: "2 FP. Remove a directive by id.",
	}), handleUI(g, g.removeDirective, removeDirectiveCard))
	mcp.AddTool(s, appTool(&mcp.Tool{
		Name:        string(focus.UpdateTacticalPlan),
		Description: "15 FP. Partially patch the tactical plan; patches violating a FORBID directive are rejected.",
	}), handleUI(g, g.updateTacticalPlan, tacticalCard))

	g.registerObservationTools(s)
}

func flatCost(t focus.Tool) func(*callCtx) int {
	return func(*callCtx) int {
		c, _ := focus.Cost(t)
		return c
	}
}

// ---- Free tools (docs/11 §3) ----

func (g *Gateway) getGuide(mid int64, sid string, _ emptyIn) map[string]any {
	return g.run(mid, sid, focus.GetGuide, nil, flatCost(focus.GetGuide),
		func(cc *callCtx) (any, *apiError) {
			return guideData(), nil
		})
}

func (g *Gateway) getTime(mid int64, sid string, _ emptyIn) map[string]any {
	return g.run(mid, sid, focus.GetTime, nil, flatCost(focus.GetTime),
		func(cc *callCtx) (any, *apiError) {
			w := g.Host.World()
			data := map[string]any{
				"game_time":              gameTimeISO(cc.now),
				"tempo":                  g.tempoString(cc),
				"run_profile":            runProfileName(w.Config),
				"game_speed":             int(w.Config.GameSpeed),
				"idle_acceleration":      w.Config.IdleAcceleration,
				"offseason_acceleration": w.Config.OffseasonAccel,
				"season_phase":           seasonPhase(w, cc.now),
			}
			if k, f := nextKickoff(w, cc.now, cc.manager.ClubID); k > 0 {
				window := map[string]any{"kickoff": gameTimeISO(k)}
				if f != nil {
					window["own_fixture"] = fixtureRef(w, f)
				}
				if g.Host.Paused() {
					window["real_until_kickoff"] = map[string]any{"paused": true}
				} else {
					d := g.Host.RealUntil(k)
					window["real_until_kickoff"] = durationBlock(d)
				}
				data["next_match_window"] = window
			}
			return data, nil
		})
}

func (g *Gateway) getSettings(mid int64, sid string, _ emptyIn) map[string]any {
	return g.run(mid, sid, focus.GetSettings, nil, flatCost(focus.GetSettings),
		func(cc *callCtx) (any, *apiError) {
			w := g.Host.World()
			tempo := sim.TempoPaused
			if !g.Host.Paused() {
				tempo = g.Host.Engine().TempoAt(cc.now)
			}
			data := map[string]any{
				"world": map[string]any{
					"name":                 w.Config.Name,
					"divisions":            w.Config.Divisions,
					"clubs_per_division":   w.Config.ClubsPerDivision,
					"total_clubs":          w.Config.TotalClubs(),
					"run_profile":          runProfileName(w.Config),
					"quality":              string(w.Config.Quality),
					"economy":              string(w.Config.Economy),
					"squad_size_target":    w.Config.SquadSizeTarget,
					"youth_intake_batch":   w.Config.YouthIntakeBatch,
					"promotion_slots":      w.Derived.PromotionSlots,
					"relegation_slots":     w.Derived.PromotionSlots,
					"league_rounds":        w.Derived.Rounds,
					"cup_rounds":           w.Derived.CupRounds,
					"cup_bracket_size":     w.Derived.CupBracketSize,
					"season_phase":         seasonPhase(w, cc.now),
					"current_game_time":    gameTimeISO(cc.now),
					"current_tempo":        tempo.String(),
					"seed":                 "redacted",
					"seed_redacted_reason": "Seed and other replay-randomness inputs are intentionally not exposed through MCP settings.",
				},
				"pacing": pacingBlock(w, tempo),
				"focus": map[string]any{
					"cap":                 focus.Cap,
					"regen_per_game_hour": focus.RegenPerGameHour,
					"regen_real_time":     focusRegenRealBlock(w, tempo),
				},
			}
			if k, f := nextKickoff(w, cc.now, cc.manager.ClubID); k > 0 {
				next := map[string]any{"kickoff": gameTimeISO(k)}
				if f != nil {
					next["own_fixture"] = fixtureRef(w, f)
				}
				if g.Host.Paused() {
					next["real_until_kickoff"] = map[string]any{"paused": true}
				} else {
					next["real_until_kickoff"] = durationBlock(g.Host.RealUntil(k))
				}
				data["next_match_window"] = next
			}
			return data, nil
		})
}

func (g *Gateway) getFocus(mid int64, sid string, _ emptyIn) map[string]any {
	return g.run(mid, sid, focus.GetFocus, nil, flatCost(focus.GetFocus),
		func(cc *callCtx) (any, *apiError) {
			spends := make([]map[string]any, 0, len(cc.manager.FocusSpends))
			for _, s := range cc.manager.FocusSpends {
				spends = append(spends, map[string]any{
					"tool": s.Tool, "cost": s.Cost, "game_time": gameTimeISO(s.GameTime),
				})
			}
			return map[string]any{
				"balance":             cc.manager.FocusBalance,
				"cap":                 focus.Cap,
				"regen_per_game_hour": focus.RegenPerGameHour,
				"spends":              spends,
			}, nil
		})
}

func pacingBlock(w *worldgen.World, current sim.Tempo) map[string]any {
	cfg := w.Config
	rows := map[string]any{}
	for _, tempo := range []sim.Tempo{sim.TempoMatch, sim.TempoIdle, sim.TempoOffseason} {
		rows[tempo.String()] = tempoRateBlock(cfg, tempo)
	}
	return map[string]any{
		"current_tempo":           current.String(),
		"run_profile":             runProfileName(cfg),
		"base_game_speed":         int(cfg.GameSpeed),
		"meaning":                 "base_game_speed means N game minutes per real minute during MATCH tempo",
		"idle_acceleration":       cfg.IdleAcceleration,
		"offseason_acceleration":  cfg.OffseasonAccel,
		"current_effective_speed": effectiveSpeed(cfg, current),
		"match_window_game_min":   engine.MatchWindowMinutes,
		"match_window_real_time":  durationBlock(time.Duration(engine.MatchWindowMinutes) * pacer(cfg).RealPerGameMinute(sim.TempoMatch)),
		"tempo_rates":             rows,
	}
}

func runProfileName(cfg worldgen.WorldConfig) string {
	if cfg.RunProfile == "" {
		return "custom"
	}
	return cfg.RunProfile
}

func tempoRateBlock(cfg worldgen.WorldConfig, tempo sim.Tempo) map[string]any {
	p := pacer(cfg)
	perMinute := p.RealPerGameMinute(tempo)
	perDay := time.Duration(sim.MinutesPerDay) * perMinute
	return map[string]any{
		"effective_speed":              effectiveSpeed(cfg, tempo),
		"real_per_game_minute_ms":      perMinute.Milliseconds(),
		"real_per_game_minute":         perMinute.String(),
		"real_per_game_day_seconds":    int(perDay.Seconds()),
		"real_per_game_day":            perDay.String(),
		"game_minutes_per_real_minute": effectiveSpeed(cfg, tempo),
	}
}

func focusRegenRealBlock(w *worldgen.World, tempo sim.Tempo) map[string]any {
	if tempo == sim.TempoPaused {
		return map[string]any{"paused": true}
	}
	d := time.Duration(worldgen.FocusMinutesPerFP) * pacer(w.Config).RealPerGameMinute(tempo)
	return map[string]any{
		"one_focus_point_every_game_minutes": worldgen.FocusMinutesPerFP,
		"current_real_time_per_focus_point":  durationBlock(d),
	}
}

func effectiveSpeed(cfg worldgen.WorldConfig, tempo sim.Tempo) int {
	base := int(cfg.GameSpeed)
	if base == 0 {
		base = int(sim.Speed15)
	}
	switch tempo {
	case sim.TempoMatch:
		return base
	case sim.TempoIdle:
		return base * cfg.IdleAcceleration
	case sim.TempoOffseason:
		return base * cfg.OffseasonAccel
	default:
		return 0
	}
}

func pacer(cfg worldgen.WorldConfig) engine.Pacer {
	return engine.Pacer{
		Speed:                 cfg.GameSpeed,
		IdleAcceleration:      cfg.IdleAcceleration,
		OffseasonAcceleration: cfg.OffseasonAccel,
	}
}

func durationBlock(d time.Duration) map[string]any {
	if d < 0 {
		d = 0
	}
	return map[string]any{
		"seconds": int(d.Seconds()),
		"human":   d.Round(time.Second).String(),
	}
}

func (g *Gateway) getMindset(mid int64, sid string, _ emptyIn) map[string]any {
	return g.run(mid, sid, focus.GetMindset, nil, flatCost(focus.GetMindset),
		func(cc *callCtx) (any, *apiError) {
			m := cc.manager
			data := map[string]any{
				"mindset":    m.Mindset,
				"employment": g.employment(m),
				"reputation": g.descriptor("desc.reputation." + reputationBand(m.Reputation)),
				// The descriptor surface is stable even before richer morale and
				// mood systems are modeled (FR-20e).
				"mood": g.descriptor("desc.mood.STEADY"),
			}
			// Drift ETAs per axis with an active target (docs/11 §3).
			if len(m.Mindset.Disposition.Target) > 0 {
				etas := map[string]any{}
				for _, a := range mindset.AllAxes {
					tgt, ok := m.Mindset.Disposition.Target[a]
					if !ok {
						continue
					}
					diff := tgt - m.Mindset.Disposition.Current[a]
					if diff < 0 {
						diff = -diff
					}
					etas[string(a)] = gameTimeISO(driftETA(cc.now, diff))
				}
				data["drift_eta"] = etas
			}
			if club, ok := g.clubOf(m); ok {
				data["board"] = map[string]any{
					"objective_finish":    club.BoardObjectiveFinish,
					"predicted_finish":    club.PredictedFinish,
					"confidence_baseline": club.ConfidenceBaseline,
				}
			}
			return data, nil
		})
}

func guideData() map[string]any {
	dials := mindset.DialValues()
	return map[string]any{
		"title": "Agentic FC Guide",
		"premise": []string{
			"You are not the in-world manager. You are the agent shaping an autonomous Manager.",
			"The world keeps running in real game time. Clubs, matches, injuries, transfers, board confidence, careers, youth intake, contracts, and seasons progress without waiting for you.",
			"You do not submit lineups or force outcomes. You observe, spend Focus, and edit Mindset/Tactical Plan intent; the Manager executes probabilistically.",
			"MCP is the play surface: it gives football-facing data and controls, not private engine details or exact resolution rules.",
		},
		"first_session": []string{
			"Call get_guide first when you are unfamiliar with the game.",
			"Call get_time, get_focus, get_situation, and get_mindset.",
			"Call get_club for your own club, get_squad with detail=attributes, and get_league for the current division.",
			"Decide whether the board goal is realistic. Do not promise WIN_LEAGUE blindly if the squad is weak.",
			"Use set_priorities only with goals from vocabularies.goals.",
			"Use update_tactical_plan for broad style, then add_directive for specific standing orders.",
			"For long-running play, call configure_alerts and subscribe to the alert resource returned by get_alerts if your MCP host supports resource subscriptions.",
			"After shaping, wait/observe: get_news, get_situation, get_match around fixtures, then adjust.",
		},
		"strategy_loop": []string{
			"Observe wide: get_situation, get_league, get_news.",
			"Inspect narrow: get_squad, get_club, get_person, get_match.",
			"Plan: choose at most 5 priorities and a tactical plan that matches the squad.",
			"Shape: disposition changes are slow personality pressure; priorities set strategic goals; directives are concrete standing orders.",
			"Review: the Manager may not obey perfectly. Stronger directives cost more Focus but still do not guarantee certainty.",
		},
		"long_running_alerts": []string{
			"Agentic FC does not run your agent. Your harness controls its own loop.",
			"Use configure_alerts to watch for NEWS, MATCH, CALENDAR, and FOCUS wake signals.",
			"Call get_alerts to read the manager-specific resource URI, then subscribe to it when your MCP host supports resources/subscribe.",
			"When notifications/resources/updated arrives, call get_alerts, inspect detail through normal tools such as get_news or get_match, then call ack_alerts through the highest handled id.",
			"If your host cannot subscribe, poll get_alerts sparingly; it costs Focus like other attention reads.",
		},
		"common_pitfalls": []string{
			"Do not invent enum values. Use the vocabularies in this guide.",
			"set_priorities replaces the full priority list; include every priority you want to keep.",
			"update_tactical_plan accepts only partial tactical fields; omit fields you do not want to change.",
			"add_directive target shape depends on the verb. For example SIGN needs target.player, ROTATE needs target.position_group, FORBID needs target.formation or target.scope.",
			"Focus is scarce. Read cheaply first, then make a few high-confidence changes.",
			"Other clubs' players may be uncertain. Scout, read match evidence, and follow news before treating a player profile as complete.",
		},
		"recommended_opening_for_title_challenge": []string{
			"If the user asks to win the league, first confirm squad quality and board expectation with get_situation, get_club, get_squad, and get_league.",
			"If the squad looks competitive, set WIN_LEAGUE as rank 1. If not, use FINISH_TOP_N or ESTABLISH_IDENTITY while recruiting toward a later title push.",
			"Prefer tactical changes that suit known squad strengths. Add player-specific START/KEEP/SIGN/RENEW directives only after inspecting the relevant players.",
		},
		"vocabularies": map[string]any{
			"goals":            stringsOfGoals(mindset.AllGoals),
			"directive_verbs":  stringsOfVerbs(mindset.AllVerbs),
			"strengths":        stringsOfStrengths(mindset.AllStrengths),
			"formations":       append([]string{}, mindset.FormationCatalog...),
			"position_groups":  []string{"GK", "DF", "MF", "FW"},
			"age_groups":       []string{"U21"},
			"tactical_dials":   dials,
			"disposition_axes": dispositionGuide(),
		},
		"target_shapes": map[string]any{
			"player_verbs": map[string]any{
				"verbs":  []string{"START", "BENCH", "EXCLUDE", "CAPTAIN", "SIGN", "SELL", "KEEP", "LOAN_OUT", "RENEW", "RELEASE", "DEVELOP", "RETRAIN_POSITION"},
				"target": map[string]any{"player": 10001},
			},
			"give_minutes": []map[string]any{
				{"verb": "GIVE_MINUTES", "target": map[string]any{"player": 10001}},
				{"verb": "GIVE_MINUTES", "target": map[string]any{"age_group": "U21"}},
			},
			"rotate_or_target_profile": []map[string]any{
				{"verb": "ROTATE", "target": map[string]any{"position_group": "MF"}},
				{"verb": "TARGET_PROFILE", "target": map[string]any{"position_group": "FW"}},
			},
			"wage_cap": []map[string]any{
				{"verb": "WAGE_CAP", "target": map[string]any{"scope": "new_signings"}},
				{"verb": "WAGE_CAP", "target": map[string]any{"scope": "renewals"}},
				{"verb": "WAGE_CAP", "target": map[string]any{"scope": "all"}},
			},
			"forbid": []map[string]any{
				{"verb": "FORBID", "target": map[string]any{"formation": "3-5-2"}},
				{"verb": "FORBID", "target": map[string]any{"scope": "pressing:HIGH"}},
			},
			"pursue_job": []map[string]any{
				{"verb": "PURSUE_JOB", "target": map[string]any{"club": 1}},
				{"verb": "PURSUE_JOB", "target": map[string]any{"division_tier": 1}},
				{"verb": "PURSUE_JOB", "target": map[string]any{"scope": "ANY"}},
			},
			"reject_job": []map[string]any{
				{"verb": "REJECT_JOB", "target": map[string]any{"club": 1}},
				{"verb": "REJECT_JOB", "target": map[string]any{"division_tier": 1}},
			},
			"push_board": []map[string]any{
				{"verb": "PUSH_BOARD", "target": map[string]any{"scope": "budget"}},
				{"verb": "PUSH_BOARD", "target": map[string]any{"scope": "training_facilities"}},
				{"verb": "PUSH_BOARD", "target": map[string]any{"scope": "youth_facilities"}},
			},
		},
		"examples": map[string]any{
			"set_priorities": map[string]any{
				"priorities": []map[string]any{
					{"rank": 1, "goal": "WIN_LEAGUE"},
					{"rank": 2, "goal": "ESTABLISH_IDENTITY"},
					{"rank": 3, "goal": "FINANCIAL_HEALTH"},
				},
			},
			"update_tactical_plan": map[string]any{
				"formation": "4-3-3", "mentality": "ATTACKING", "pressing": "MID",
				"tempo": "FAST", "width": "WIDE", "directness": "MIXED", "counter": true,
			},
			"add_directive": map[string]any{
				"verb": "KEEP", "target": map[string]any{"player": 10001}, "strength": "INSIST",
			},
		},
	}
}

func stringsOfGoals(in []mindset.Goal) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, string(v))
	}
	return out
}

func stringsOfVerbs(in []mindset.Verb) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, string(v))
	}
	return out
}

func stringsOfStrengths(in []mindset.Strength) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, string(v))
	}
	return out
}

func dispositionGuide() []map[string]string {
	return []map[string]string{
		{"axis": "D1", "name": "risk_appetite", "low": "cautious", "high": "daring"},
		{"axis": "D2", "name": "youth_preference", "low": "proven veterans", "high": "youth-first"},
		{"axis": "D3", "name": "playing_identity", "low": "pragmatic", "high": "expressive"},
		{"axis": "D4", "name": "financial_prudence", "low": "spendthrift", "high": "frugal"},
		{"axis": "D5", "name": "loyalty", "low": "ruthless", "high": "loyal"},
		{"axis": "D6", "name": "discipline", "low": "laissez-faire", "high": "authoritarian"},
		{"axis": "D7", "name": "horizon", "low": "this weekend", "high": "five-year project"},
		{"axis": "D8", "name": "media_posture", "low": "guarded", "high": "provocative"},
		{"axis": "D9", "name": "tactical_flexibility", "low": "system purist", "high": "chameleon"},
		{"axis": "D10", "name": "personal_ambition", "low": "content", "high": "ladder-climbing"},
	}
}

// ---- Shaping tools (docs/11 §6) ----

type updateDispositionIn struct {
	// Targets maps axis id (D1…D10) to a target value in −10…+10.
	Targets map[string]int `json:"targets"`
}

func (g *Gateway) updateDisposition(mid int64, sid string, in updateDispositionIn) map[string]any {
	return g.run(mid, sid, focus.UpdateDisposition, in, flatCost(focus.UpdateDisposition),
		func(cc *callCtx) (any, *apiError) {
			targets := make(map[mindset.Axis]int, len(in.Targets))
			for k, v := range in.Targets {
				targets[mindset.Axis(k)] = v
			}
			// Validate BEFORE touching anything: a rejected call must
			// mutate nothing (docs/11 §1.2 — not even accrued drift).
			if err := mindset.ValidateDispositionTargets(targets); err != nil {
				return nil, mapMindsetErr(err)
			}
			// Apply drift accrued under the OLD targets first — re-targeting
			// must never erase earned progress (also re-anchors at now).
			engine.ApplyDispositionDrift(cc.manager, cc.now)
			applied, drifting, err := cc.manager.Mindset.SetDispositionTargets(targets)
			if err != nil {
				return nil, mapMindsetErr(err) // unreachable: validated above
			}

			axes := map[string]any{}
			for _, a := range applied {
				axes[string(a)] = map[string]any{
					"current": cc.manager.Mindset.Disposition.Current[a],
				}
			}
			for _, a := range drifting {
				cur := cc.manager.Mindset.Disposition.Current[a]
				tgt := cc.manager.Mindset.Disposition.Target[a]
				diff := tgt - cur
				if diff < 0 {
					diff = -diff
				}
				axes[string(a)] = map[string]any{
					"current": cur,
					"target":  tgt,
					"eta":     gameTimeISO(driftETA(cc.now, diff)),
				}
			}
			return map[string]any{
				"axes":            axes,
				"mindset_version": cc.manager.Mindset.Version,
			}, nil
		})
}

type priorityIn struct {
	Rank   int            `json:"rank"`
	Goal   string         `json:"goal"`
	Params map[string]any `json:"params,omitempty"`
}

type setPrioritiesIn struct {
	Priorities []priorityIn `json:"priorities"`
}

func (g *Gateway) setPriorities(mid int64, sid string, in setPrioritiesIn) map[string]any {
	return g.run(mid, sid, focus.SetPriorities, in, flatCost(focus.SetPriorities),
		func(cc *callCtx) (any, *apiError) {
			ps := make([]mindset.Priority, 0, len(in.Priorities))
			for _, p := range in.Priorities {
				goal := mindset.Goal(p.Goal)
				if !goalKnown(goal) {
					return nil, errFor(ErrValidation, "err.validation",
						map[string]any{"detail": "unknown goal " + p.Goal},
						map[string]any{"goal": p.Goal})
				}
				ps = append(ps, mindset.Priority{Rank: p.Rank, Goal: goal, Params: p.Params})
			}
			if err := cc.manager.Mindset.SetPriorities(ps); err != nil {
				return nil, mapMindsetErr(err)
			}
			return map[string]any{
				"priorities":      cc.manager.Mindset.Priorities,
				"mindset_version": cc.manager.Mindset.Version,
			}, nil
		})
}

type addDirectiveIn struct {
	Verb       string            `json:"verb"`
	Target     mindset.Target    `json:"target"`
	Strength   string            `json:"strength"`
	Params     map[string]any    `json:"params,omitempty"`
	Conditions map[string]string `json:"conditions,omitempty"`
	Expiry     string            `json:"expiry,omitempty"`
}

func (g *Gateway) addDirective(mid int64, sid string, in addDirectiveIn) map[string]any {
	strength := mindset.Strength(in.Strength)
	cost := func(*callCtx) int { return strength.FocusCost() }
	return g.run(mid, sid, focus.AddDirective, in, cost,
		func(cc *callCtx) (any, *apiError) {
			if !verbKnown(mindset.Verb(in.Verb)) {
				return nil, errFor(ErrValidation, "err.validation",
					map[string]any{"detail": "unknown verb " + in.Verb},
					map[string]any{"verb": in.Verb})
			}
			if strength.FocusCost() == 0 {
				return nil, errFor(ErrValidation, "err.validation",
					map[string]any{"detail": "unknown strength " + in.Strength},
					map[string]any{"strength": in.Strength})
			}
			// A manager can only list one of their OWN players for sale: a SELL on
			// a player they don't own is meaningless and, worse, would spring active
			// the instant they later signed that player, re-listing the new signing
			// (the transfer-poach case). Reject it up front; the engine also
			// clears any stale SELL on a player it acquires as defence in depth.
			if mindset.Verb(in.Verb) == mindset.VerbSell {
				// Club 0 is BOTH the free-agent marker (Player) and the unemployed
				// pool (Manager), so an unemployed manager must be rejected before
				// the ownership compare — otherwise 0 == 0 would let them "list" a
				// free agent they don't own. Youth are never on the market.
				p := g.playerByID(in.Target.Player)
				if cc.manager.ClubID == 0 || p == nil || p.ClubID != cc.manager.ClubID || p.Youth {
					return nil, errFor(ErrValidation, "err.validation",
						map[string]any{"detail": "can only SELL a senior player at your own club"},
						map[string]any{"verb": "SELL", "player": in.Target.Player})
				}
			}
			d, err := cc.manager.Mindset.AddDirective(mindset.Directive{
				Verb:       mindset.Verb(in.Verb),
				Target:     in.Target,
				Strength:   strength,
				Params:     in.Params,
				Conditions: in.Conditions,
				Expiry:     in.Expiry,
			})
			if err != nil {
				aerr := mapMindsetErr(err)
				if aerr.Code == ErrCapExceeded {
					// docs/11 §6: the cap error includes the full active
					// list so the Agent can pick what to drop.
					aerr.Details = map[string]any{
						"active_directives": cc.manager.Mindset.Directives,
					}
				}
				return nil, aerr
			}
			return map[string]any{
				"directive":         d,
				"active_directives": len(cc.manager.Mindset.Directives),
				"mindset_version":   cc.manager.Mindset.Version,
			}, nil
		})
}

type removeDirectiveIn struct {
	ID string `json:"id"`
}

func (g *Gateway) removeDirective(mid int64, sid string, in removeDirectiveIn) map[string]any {
	return g.run(mid, sid, focus.RemoveDirective, in, flatCost(focus.RemoveDirective),
		func(cc *callCtx) (any, *apiError) {
			if !cc.manager.Mindset.RemoveDirective(in.ID) {
				return nil, errFor(ErrNotFound, "err.not_found",
					map[string]any{"id": in.ID}, map[string]any{"id": in.ID})
			}
			return map[string]any{
				"removed":           in.ID,
				"active_directives": len(cc.manager.Mindset.Directives),
				"mindset_version":   cc.manager.Mindset.Version,
			}, nil
		})
}

type updateTacticalPlanIn struct {
	Formation  string `json:"formation,omitempty"`
	Mentality  string `json:"mentality,omitempty"`
	Pressing   string `json:"pressing,omitempty"`
	Tempo      string `json:"tempo,omitempty"`
	Width      string `json:"width,omitempty"`
	Directness string `json:"directness,omitempty"`
	Counter    *bool  `json:"counter,omitempty"`
}

func (g *Gateway) updateTacticalPlan(mid int64, sid string, in updateTacticalPlanIn) map[string]any {
	return g.run(mid, sid, focus.UpdateTacticalPlan, in, flatCost(focus.UpdateTacticalPlan),
		func(cc *callCtx) (any, *apiError) {
			// The Mindset never contradicts itself (docs/11 §6): a patch
			// that violates an active FORBID fence is rejected — both
			// formation fences and style-element fences ("dial:VALUE"
			// scopes, docs/10 §4.2).
			if aerr := forbidFenceCheck(cc.manager, in); aerr != nil {
				return nil, aerr
			}
			patch := mindset.TacticalPlan{
				Formation: in.Formation, Mentality: in.Mentality,
				Pressing: in.Pressing, Tempo: in.Tempo,
				Width: in.Width, Directness: in.Directness,
			}
			setCounter := in.Counter != nil
			if setCounter {
				patch.Counter = *in.Counter
			}
			merged, err := cc.manager.Mindset.ApplyTacticalPatch(patch, setCounter)
			if err != nil {
				return nil, mapMindsetErr(err)
			}
			return map[string]any{
				"tactical_plan":   merged,
				"mindset_version": cc.manager.Mindset.Version,
			}, nil
		})
}

// driftETA estimates when a diff-point drift completes (~2 pts/game-week,
// FR-16b; the decision cadence adds jitter, so this is an estimate).
func driftETA(now sim.GameTime, diff int) sim.GameTime {
	return now + sim.GameTime(int64(diff)*7*sim.MinutesPerDay/2)
}

// forbidFenceCheck rejects tactical patches that hit an active FORBID:
// formation fences match Target.Formation; style fences use the
// "dial:VALUE" scope convention (docs/10 §4.2), e.g. "pressing:HIGH".
func forbidFenceCheck(m *worldgen.Manager, in updateTacticalPlanIn) *apiError {
	dials := map[string]string{
		"formation": in.Formation, "mentality": in.Mentality,
		"pressing": in.Pressing, "tempo": in.Tempo,
		"width": in.Width, "directness": in.Directness,
	}
	for _, d := range m.Mindset.Directives {
		if d.Verb != mindset.VerbForbid {
			continue
		}
		if d.Target.Formation != "" && in.Formation != "" && d.Target.Formation == in.Formation {
			return errFor(ErrConflict, "err.conflict",
				map[string]any{"with": d.ID},
				map[string]any{"conflicts_with": d.ID, "forbidden_formation": in.Formation})
		}
		if d.Target.Scope == "" {
			continue
		}
		dial, value, ok := strings.Cut(d.Target.Scope, ":")
		if !ok {
			continue
		}
		if v := dials[strings.ToLower(dial)]; v != "" && strings.EqualFold(v, value) {
			return errFor(ErrConflict, "err.conflict",
				map[string]any{"with": d.ID},
				map[string]any{"conflicts_with": d.ID, "forbidden_style": d.Target.Scope})
		}
	}
	return nil
}

// ---- Shared helpers ----

func (g *Gateway) tempoString(cc *callCtx) string {
	if g.Host.Paused() {
		return sim.TempoPaused.String()
	}
	return g.Host.Engine().TempoAt(cc.now).String()
}

func (g *Gateway) descriptor(key string) map[string]any {
	return map[string]any{
		"key":  key,
		"text": g.Catalogs.Render(narrative.LocaleEN, key, nil),
	}
}

func (g *Gateway) employment(m *worldgen.Manager) map[string]any {
	if m.ClubID == 0 {
		return map[string]any{"status": "UNEMPLOYED"}
	}
	out := map[string]any{"status": "EMPLOYED", "club": map[string]any{"club": m.ClubID}}
	if c, ok := g.clubOf(m); ok {
		out["club_name"] = c.Name
	}
	return out
}

func (g *Gateway) clubOf(m *worldgen.Manager) (*worldgen.Club, bool) {
	if m.ClubID == 0 {
		return nil, false
	}
	w := g.Host.World()
	for i := range w.Clubs {
		if w.Clubs[i].ID == m.ClubID {
			return &w.Clubs[i], true
		}
	}
	return nil, false
}

// reputationBand maps the hidden 0–10,000 scale to its public Descriptor
// band (FR-22a; thresholds are initial values, registered in docs/98).
func reputationBand(rep int) string {
	switch {
	case rep >= 8500:
		return "WORLDWIDE"
	case rep >= 7000:
		return "CONTINENTAL"
	case rep >= 5000:
		return "NATIONAL"
	case rep >= 3000:
		return "REGIONAL"
	case rep >= 1500:
		return "LOCAL"
	default:
		return "OBSCURE"
	}
}

func goalKnown(goal mindset.Goal) bool {
	for _, g := range mindset.AllGoals {
		if g == goal {
			return true
		}
	}
	return false
}

func verbKnown(v mindset.Verb) bool {
	for _, k := range mindset.AllVerbs {
		if k == v {
			return true
		}
	}
	return false
}

// nextKickoff finds the earliest kickoff at or after now, plus the
// manager's own fixture in that window if any.
func nextKickoff(w *worldgen.World, now sim.GameTime, clubID int64) (sim.GameTime, *worldgen.Fixture) {
	best := sim.GameTime(0)
	var own *worldgen.Fixture
	for i := range w.Fixtures {
		f := &w.Fixtures[i]
		if f.Kickoff < now {
			continue
		}
		if best == 0 || f.Kickoff < best {
			best = f.Kickoff
			own = nil
		}
		if f.Kickoff == best && clubID != 0 && (f.HomeID == clubID || f.AwayID == clubID) {
			own = f
		}
	}
	return best, own
}

func fixtureRef(w *worldgen.World, f *worldgen.Fixture) map[string]any {
	names := map[int64]string{}
	for i := range w.Clubs {
		names[w.Clubs[i].ID] = w.Clubs[i].Name
	}
	return map[string]any{
		"fixture":     f.ID,
		"competition": f.Competition,
		"round":       f.Round,
		"home":        map[string]any{"club": f.HomeID, "name": names[f.HomeID]},
		"away":        map[string]any{"club": f.AwayID, "name": names[f.AwayID]},
		"kickoff":     gameTimeISO(f.Kickoff),
	}
}

// seasonPhase derives the coarse phase from the calendar (docs/11 §3).
func seasonPhase(w *worldgen.World, now sim.GameTime) string {
	d := worldgen.DateOf(now)
	inSummerWindow := (d.Month == 6 && d.Day >= 15) || d.Month == 7 || d.Month == 8
	inWinterWindow := d.Month == 1
	if inSummerWindow || inWinterWindow {
		return "WINDOW_OPEN"
	}
	first, last := leagueSpan(w)
	switch {
	case now < first:
		return "PRE_SEASON"
	case now > last:
		return "OFF_SEASON"
	default:
		return "SEASON"
	}
}

func leagueSpan(w *worldgen.World) (first, last sim.GameTime) {
	kicks := make([]sim.GameTime, 0, len(w.Fixtures))
	for i := range w.Fixtures {
		if w.Fixtures[i].Competition == worldgen.CompetitionLeague {
			kicks = append(kicks, w.Fixtures[i].Kickoff)
		}
	}
	if len(kicks) == 0 {
		return 0, 0
	}
	sort.Slice(kicks, func(i, j int) bool { return kicks[i] < kicks[j] })
	return kicks[0], kicks[len(kicks)-1]
}
