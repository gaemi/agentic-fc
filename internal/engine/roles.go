package engine

import (
	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

const (
	roleShotStopper   = "SHOT_STOPPER"
	roleSweeperKeeper = "SWEEPER_KEEPER"
	roleStopper       = "STOPPER"
	roleWingBack      = "WING_BACK"
	roleDefender      = "DEFENDER"
	rolePlaymaker     = "PLAYMAKER"
	rolePresser       = "PRESSER"
	roleMidfielder    = "MIDFIELDER"
	roleTargetForward = "TARGET_FORWARD"
	roleRunner        = "RUNNER"
	rolePoacher       = "POACHER"

	roleChanceMajor   = 20
	roleChanceStrong  = 18
	roleChancePrimary = 16
	roleChanceMedium  = 12
	roleChanceSupport = 10
	roleChanceMinor   = 8

	selectionFitMajor   = 18
	selectionFitPrimary = 16
	selectionFitPress   = 14
	selectionFitNarrow  = 12
	selectionFitBlock   = 8
)

// playerTacticalRole is a derived match-engine role. It is not persisted and
// does not expose hidden traits; it lets tactical instructions affect selection
// and event contests without adding a new public stat surface.
func playerTacticalRole(p *worldgen.Player) string {
	if p == nil {
		return ""
	}
	switch p.Group {
	case attr.GK:
		sweeper := effective(p, attr.Sweeping) + effective(p, attr.Distribution)
		stopper := effective(p, attr.Reflexes) + effective(p, attr.Handling)
		if sweeper > stopper {
			return roleSweeperKeeper
		}
		return roleShotStopper
	case attr.DF:
		air := factorHeaderQuality(p) + factorDuelPower(p)
		flank := factorSeparation(p) + effective(p, attr.Crossing) + effective(p, attr.WorkRate)
		if air >= flank+8 {
			return roleStopper
		}
		if flank >= air-4 {
			return roleWingBack
		}
		return roleDefender
	case attr.MF:
		play := factorDeliveryQuality(p, attr.Passing) + effective(p, attr.FirstTouch)
		press := factorPressImpact(p) + effective(p, attr.Tackling)
		if play >= press+6 {
			return rolePlaymaker
		}
		if press >= play-4 {
			return rolePresser
		}
		return roleMidfielder
	case attr.FW:
		target := factorHeaderQuality(p) + factorDuelPower(p)
		run := factorSeparation(p) + effective(p, attr.OffBall) + effective(p, attr.Finishing)
		if target >= run+8 {
			return roleTargetForward
		}
		if run >= target-4 {
			return roleRunner
		}
		return rolePoacher
	default:
		return ""
	}
}

func tacticalRoleBonus(role, chanceType string) int {
	switch chanceType {
	case chanceCrossHeader, chanceSetPieceHeader:
		switch role {
		case roleTargetForward:
			return roleChanceMajor
		case roleStopper:
			return roleChanceMedium
		case roleWingBack:
			return roleChanceMinor
		}
	case chanceCutback:
		switch role {
		case rolePoacher:
			return roleChancePrimary
		case rolePlaymaker, roleWingBack:
			return roleChanceSupport
		}
	case chanceThroughBall:
		switch role {
		case roleRunner:
			return roleChanceStrong
		case rolePlaymaker:
			return roleChanceMedium
		case rolePoacher:
			return roleChanceMinor
		}
	case chanceLongShot:
		switch role {
		case rolePlaymaker, roleMidfielder:
			return roleChanceSupport
		}
	case chanceCounter:
		switch role {
		case roleRunner:
			return roleChanceStrong
		case rolePresser:
			return roleChanceMedium
		}
	case chanceScramble:
		switch role {
		case rolePoacher, roleTargetForward:
			return roleChanceMedium
		}
	}
	return 0
}

func selectionScore(p *worldgen.Player, plan mindset.TacticalPlan) int {
	if p == nil {
		return 0
	}
	return p.AbilityPool*10 + tacticFitBonus(playerTacticalRole(p), plan)
}

func tacticFitBonus(role string, plan mindset.TacticalPlan) int {
	score := 0
	switch plan.Width {
	case "WIDE":
		if role == roleWingBack || role == roleTargetForward {
			score += selectionFitMajor
		}
	case "NARROW":
		if role == rolePlaymaker || role == rolePoacher {
			score += selectionFitNarrow
		}
	}
	switch plan.Directness {
	case "DIRECT":
		if role == roleTargetForward || role == roleRunner || role == roleSweeperKeeper {
			score += selectionFitPrimary
		}
	case "SHORT":
		if role == rolePlaymaker || role == rolePoacher {
			score += selectionFitPrimary
		}
	}
	switch plan.Pressing {
	case "HIGH":
		if role == rolePresser || role == roleRunner {
			score += selectionFitPress
		}
	case "LOW":
		if role == roleStopper || role == roleShotStopper {
			score += selectionFitBlock
		}
	}
	if plan.Counter && (role == roleRunner || role == rolePresser || role == roleSweeperKeeper) {
		score += selectionFitPress
	}
	return score
}
