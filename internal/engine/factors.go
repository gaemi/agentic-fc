package engine

import (
	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Football factors are fixed-point-ish integer summaries used by the
// match-model event grammar. They are derived, never stored as truth.

func factorReach(p *worldgen.Player) int {
	return bodyReach(p)
}

func factorDuelPower(p *worldgen.Player) int {
	return bodyStrength(p) + effective(p, attr.Balance) + effective(p, attr.Aggression) + effective(p, attr.Bravery)
}

func factorHeaderQuality(p *worldgen.Player) int {
	return factorReach(p) + effective(p, attr.Heading) + effective(p, attr.Technique) + effective(p, attr.Composure)
}

func factorSeparation(p *worldgen.Player) int {
	return effective(p, attr.Acceleration) + effective(p, attr.Pace) + effective(p, attr.Agility) + effective(p, attr.OffBall)
}

func factorBallSecurity(p *worldgen.Player) int {
	return effective(p, attr.FirstTouch) + effective(p, attr.Technique) + effective(p, attr.Dribbling) + effective(p, attr.Balance) + effective(p, attr.Composure)
}

func factorDeliveryQuality(p *worldgen.Player, delivery attr.Visible) int {
	return effective(p, delivery) + effective(p, attr.Vision) + effective(p, attr.Technique) + effective(p, attr.Decisions)
}

func factorShotQuality(p *worldgen.Player, shot attr.Visible) int {
	return effective(p, shot) + effective(p, attr.Technique) + effective(p, attr.Composure) + weakFootExpression(p)/2
}

func factorDefensiveRead(p *worldgen.Player) int {
	return effective(p, attr.Positioning) + effective(p, attr.Marking) + effective(p, attr.Anticipation) + effective(p, attr.Concentration) + effective(p, attr.Decisions)
}

func factorPressImpact(p *worldgen.Player) int {
	return effective(p, attr.WorkRate) + effective(p, attr.Acceleration) + effective(p, attr.Stamina) + effective(p, attr.Aggression)
}
