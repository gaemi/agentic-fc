package engine

import (
	"math"
	"math/rand/v2"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Player drift (docs/03 §3 "Attribute drift"): growth toward the Potential
// Cap while young, physicals-first decline with pool redistribution once
// aging starts (docs/02 §2.2). Every factor is a named, legible multiplier
// in the audit trail (docs/03 §2 constraint 4). Initial values, registered
// in docs/98.

const (
	driftGrowthBaseYouth = 0.35 // age < 21
	driftGrowthBaseEarly = 0.20 // age 21–23
	driftGrowthBasePrime = 0.10 // age 24+
	driftDeclineBase     = 0.10 // + DeclineSpeed/100, × professionalism resistance

	// declineAge = 28 + (DeclineOnset−10)/3, clamped (docs/08 trajectory).
	declineAgeBase = 28
	declineAgeMin  = 24
	declineAgeMax  = 34

	// Reschedule horizons in game days (docs/03 §3: days–weeks); a changed
	// outcome shortens the next interval (turbulence).
	driftIntervalYouthDays   = 7
	driftIntervalPrimeDays   = 14
	driftIntervalDeclineDays = 10
	driftIntervalJitterDays  = 4
	driftTurbulenceFactor    = 0.6
)

// Fixed orders — never iterate attribute maps directly (determinism).
var (
	physicalAttrs = []attr.Visible{attr.Acceleration, attr.Pace, attr.Agility, attr.Balance, attr.Strength, attr.Stamina, attr.NaturalFitness, attr.JumpingReach}
	mentalAttrs   = []attr.Visible{attr.Aggression, attr.Vision, attr.Decisions, attr.Composure, attr.Concentration, attr.Positioning, attr.OffBall, attr.Anticipation, attr.WorkRate, attr.Bravery, attr.Teamwork, attr.Leadership, attr.Determination, attr.Flair}
)

func declineAge(p *worldgen.Player) int {
	a := declineAgeBase + (p.Hidden[attr.DeclineOnset]-10)/3
	if a < declineAgeMin {
		return declineAgeMin
	}
	if a > declineAgeMax {
		return declineAgeMax
	}
	return a
}

func (e *Engine) handlePlayerDrift(ev *sim.Event) error {
	p, ok := e.players[ev.EntityID]
	if !ok {
		return e.log(ev, "drift", nil, "unknown_player", 0, 0)
	}
	// A retired player's tick ends the chain: no reschedule, and no mutation at
	// all — the guard sits BEFORE the recovery writes below, or a retiree would
	// keep "resting" forever (player lifecycle; mirrors the manager RETIRED
	// guard in handleDecisionRoll).
	if p.Retired {
		return e.log(ev, "drift", nil, "retired", 0, 0)
	}
	// Match recovery: the player tick doubles as rest — condition
	// climbs back toward full between matches; sharpness eases off with idleness.
	p.Condition = clampInt(p.Condition+conditionRecoverTick, 0, worldgen.ConditionMax)
	if p.Sharpness > worldgen.ConditionMax {
		p.Sharpness = worldgen.ConditionMax
	}
	r := e.rollStream(ev)

	headroom := p.PotentialCap - p.AbilityPool
	declining := p.Age >= declineAge(p)
	changed := false
	outcome := "hold"
	factors := map[string]any{
		"age":         p.Age,
		"pool":        p.AbilityPool,
		"headroom":    headroom,
		"decline_age": declineAge(p),
	}

	switch {
	case !declining && headroom > 0:
		base := driftGrowthBasePrime
		switch {
		case p.Age < 21:
			base = driftGrowthBaseYouth
		case p.Age < 24:
			base = driftGrowthBaseEarly
		}
		dev := 0.5 + float64(p.Hidden[attr.DevelopmentSpeed])/20.0
		prof := 0.7 + float64(p.Hidden[attr.Professionalism])/40.0
		pGrow := math.Min(base*dev*prof, 0.9)
		factors["p_grow"] = round3(pGrow)
		factors["dev_mult"] = round3(dev)
		factors["prof_mult"] = round3(prof)
		if r.Float64() < pGrow {
			if a, ok := e.growAttribute(r, p); ok {
				outcome = "grew_" + string(a)
				changed = true
				e.emitDrift(ev.Due, p, FeedDriftGrew, string(a), p.Visible[a]-1, p.Visible[a])
			}
		}
	case declining:
		resist := 1.2 - float64(p.Hidden[attr.Professionalism])/50.0
		pDecl := (driftDeclineBase + float64(p.Hidden[attr.DeclineSpeed])/100.0) * resist
		factors["p_decline"] = round3(pDecl)
		factors["prof_resist"] = round3(resist)
		if r.Float64() < pDecl {
			if a, ok := declineAttribute(r, p); ok {
				outcome = "declined_" + string(a)
				changed = true
				e.emitDrift(ev.Due, p, FeedDriftDeclined, string(a), p.Visible[a]+1, p.Visible[a])
				// Pool redistribution (docs/02 §2.2): experience partly
				// converts to craft, professionalism-scaled.
				if r.Float64() < 0.2+float64(p.Hidden[attr.Professionalism])/40.0 {
					if m, ok := e.redistributeToMental(r, p); ok {
						outcome += "_gained_" + string(m)
					}
				}
			}
		}
	}

	if changed {
		p.AbilityPool = int(math.Round(attr.ProfilePoolCost(p.Group, p.Visible, p.WeakFoot)))
	}

	interval := driftIntervalPrimeDays
	switch {
	case p.Age < 21:
		interval = driftIntervalYouthDays
	case declining:
		interval = driftIntervalDeclineDays
	}
	days := float64(interval)
	if changed {
		days *= driftTurbulenceFactor // turbulent outcomes roll again sooner
	}
	next := ev.Due + sim.GameTime(int64(days)*sim.MinutesPerDay) +
		sim.GameTime(r.Int64N(int64(driftIntervalJitterDays)*sim.MinutesPerDay))
	e.reschedule(ev, next)
	return e.log(ev, "drift", factors, outcome, next, 0)
}

// growAttribute buys +1 on a value-weighted visible attribute (identity
// persists: strengths grow first), respecting the 20 cap and the Potential
// Cap on total pool cost.
func (e *Engine) growAttribute(r *rand.Rand, p *worldgen.Player) (attr.Visible, bool) {
	costs := attr.PoolCosts[p.Group]
	cost := attr.ProfilePoolCost(p.Group, p.Visible, p.WeakFoot)
	var candidates []attr.Visible
	var weights []float64
	for _, a := range visibleOrder(p.Group) {
		v := p.Visible[a]
		if v >= attr.ScaleMax {
			continue
		}
		if int(math.Round(cost+costs[a])) > p.PotentialCap {
			continue
		}
		candidates = append(candidates, a)
		weights = append(weights, float64(v))
	}
	a, ok := weightedPick(r, candidates, weights)
	if !ok {
		return "", false
	}
	p.Visible[a]++
	return a, true
}

// declineAttribute takes −1 from a value-weighted physical (physicals fade
// first, and the strongest physical has the most to lose).
func declineAttribute(r *rand.Rand, p *worldgen.Player) (attr.Visible, bool) {
	var candidates []attr.Visible
	var weights []float64
	for _, a := range physicalAttrs {
		if v := p.Visible[a]; v > attr.ScaleMin {
			candidates = append(candidates, a)
			weights = append(weights, float64(v))
		}
	}
	a, ok := weightedPick(r, candidates, weights)
	if !ok {
		return "", false
	}
	p.Visible[a]--
	return a, true
}

// redistributeToMental converts part of a decline into craft: +1 on a
// mental attribute, kept inside the Potential Cap.
func (e *Engine) redistributeToMental(r *rand.Rand, p *worldgen.Player) (attr.Visible, bool) {
	costs := attr.PoolCosts[p.Group]
	cost := attr.ProfilePoolCost(p.Group, p.Visible, p.WeakFoot)
	var candidates []attr.Visible
	var weights []float64
	for _, a := range mentalAttrs {
		v := p.Visible[a]
		if v >= attr.ScaleMax || int(math.Round(cost+costs[a])) > p.PotentialCap {
			continue
		}
		candidates = append(candidates, a)
		weights = append(weights, float64(v))
	}
	a, ok := weightedPick(r, candidates, weights)
	if !ok {
		return "", false
	}
	p.Visible[a]++
	return a, true
}

// visibleOrder returns the group's attributes in a fixed order.
func visibleOrder(group attr.PositionGroup) []attr.Visible {
	if group == attr.GK {
		return append([]attr.Visible{
			attr.Reflexes, attr.OneOnOnes, attr.Handling, attr.AerialReach,
			attr.CommandOfArea, attr.Communication, attr.Distribution,
			attr.Sweeping, attr.Eccentricity, attr.Punching,
		}, append(append([]attr.Visible{}, mentalAttrs...), physicalAttrs...)...)
	}
	return append([]attr.Visible{
		attr.Finishing, attr.LongShots, attr.FirstTouch, attr.Passing,
		attr.Crossing, attr.Dribbling, attr.Technique, attr.Heading,
		attr.Tackling, attr.Marking, attr.SetPieces,
	}, append(append([]attr.Visible{}, mentalAttrs...), physicalAttrs...)...)
}

func weightedPick(r *rand.Rand, candidates []attr.Visible, weights []float64) (attr.Visible, bool) {
	total := 0.0
	for _, w := range weights {
		total += w
	}
	if len(candidates) == 0 || total <= 0 {
		return "", false
	}
	roll := r.Float64() * total
	for i, a := range candidates {
		roll -= weights[i]
		if roll < 0 {
			return a, true
		}
	}
	return candidates[len(candidates)-1], true
}

func round3(f float64) float64 { return math.Round(f*1000) / 1000 }
