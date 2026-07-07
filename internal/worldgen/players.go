package worldgen

import (
	"math"
	"math/rand/v2"
	"sort"

	"github.com/gaemi/agentic-fc/internal/attr"
)

// Stage 4 — players: squads on the position template, attributes bought
// from the Ability Pool through the docs/08 cost table according to a rolled
// player archetype, hidden attributes on soft bell curves, contracts, youth
// prospects, and free agents (docs/09 §4.2).

// Concrete positions (docs/08 §2 familiarity map keys).
const (
	posGK = "GK"
	posDR = "DR"
	posDC = "DC"
	posDL = "DL"
	posDM = "DM"
	posMC = "MC"
	posMR = "MR"
	posML = "ML"
	posAM = "AM"
	posWR = "WR"
	posWL = "WL"
	posST = "ST"
)

func posGroup(pos string) attr.PositionGroup {
	switch pos {
	case posGK:
		return attr.GK
	case posDR, posDC, posDL:
		return attr.DF
	case posDM, posMC, posMR, posML, posAM:
		return attr.MF
	default:
		return attr.FW
	}
}

// Squad template: GK 3 fixed, the rest split DF 8 : MF 8 : FW 5 by largest
// remainder when the target isn't 24 (docs/09 §4.2, tunable).
const (
	squadGKCount     = 3
	playerIDBase     = 10000
	freeAgentShare   = 0.08 // of world squad population (docs/09 §4.2)
	freeAgentPoolCut = 0.75
	academyMin       = 3  // initial prospects per club (docs/09 §4.2)
	academySpan      = 3  // 3–5
	youthAcademyCap  = 24 // per-club youth ceiling; intake throttles here (youth intake, docs/98)
	marqueeChance    = 0.22
)

var (
	dfFill = []string{posDC, posDC, posDR, posDL, posDC, posDC, posDR, posDL, posDC, posDC}
	mfFill = []string{posMC, posDM, posMC, posMR, posML, posDM, posAM, posMC, posMC, posDM, posMR, posML}
	fwFill = []string{posST, posST, posST, posWR, posWL, posST, posWR, posWL}
)

// buildSquadSlots returns the concrete-position slot list for a squad size.
func buildSquadSlots(target int) []string {
	rest := target - squadGKCount
	ratios := [3]int{8, 8, 5} // DF : MF : FW
	sum := 21
	counts := [3]int{}
	rem := [3]int{}
	used := 0
	for i, w := range ratios {
		counts[i] = rest * w / sum
		rem[i] = rest * w % sum
		used += counts[i]
	}
	for used < rest { // largest remainder
		best := 0
		for i := 1; i < 3; i++ {
			if rem[i] > rem[best] {
				best = i
			}
		}
		counts[best]++
		rem[best] = -1
		used++
	}
	slots := make([]string, 0, target)
	for i := 0; i < squadGKCount; i++ {
		slots = append(slots, posGK)
	}
	slots = append(slots, dfFill[:counts[0]]...)
	slots = append(slots, mfFill[:counts[1]]...)
	slots = append(slots, fwFill[:counts[2]]...)
	return slots
}

// ---- Player archetypes (docs/09 §4.2: legible identities, not stat mush) ----

type playerArchetype struct {
	Name         string
	Pref         map[attr.Visible]float64 // spending preference; default 1.0
	EarlyDecline bool                     // speedsters decline earlier
}

var playerArchetypes = map[string]playerArchetype{
	"Shot-Stopper":   {Name: "Shot-Stopper", Pref: map[attr.Visible]float64{attr.Reflexes: 3, attr.OneOnOnes: 2, attr.Handling: 2, attr.Agility: 2, attr.Concentration: 1.8}},
	"Sweeper-Keeper": {Name: "Sweeper-Keeper", Pref: map[attr.Visible]float64{attr.Sweeping: 3, attr.Distribution: 2.5, attr.Decisions: 2, attr.Composure: 1.8, attr.Acceleration: 1.4}},
	"Commander":      {Name: "Commander", Pref: map[attr.Visible]float64{attr.AerialReach: 2.5, attr.CommandOfArea: 3, attr.Handling: 1.8, attr.Communication: 2, attr.Leadership: 2, attr.Strength: 1.5}},
	"Stopper":        {Name: "Stopper", Pref: map[attr.Visible]float64{attr.Tackling: 2.5, attr.Marking: 2.4, attr.Strength: 2.2, attr.Heading: 2, attr.JumpingReach: 1.8, attr.Positioning: 1.8}},
	"Ball-Player":    {Name: "Ball-Player", Pref: map[attr.Visible]float64{attr.Passing: 2.5, attr.FirstTouch: 2, attr.Composure: 2, attr.Vision: 1.8, attr.Tackling: 1.3, attr.Marking: 1.2}},
	"Rapid":          {Name: "Rapid", Pref: map[attr.Visible]float64{attr.Acceleration: 3, attr.Pace: 3, attr.Stamina: 1.8, attr.Tackling: 1.5, attr.Dribbling: 1.3}, EarlyDecline: true},
	"Destroyer":      {Name: "Destroyer", Pref: map[attr.Visible]float64{attr.Tackling: 2.5, attr.WorkRate: 2.2, attr.Strength: 1.8, attr.Aggression: 1.8, attr.Positioning: 1.6}},
	"Playmaker":      {Name: "Playmaker", Pref: map[attr.Visible]float64{attr.Passing: 3, attr.Vision: 2.8, attr.Decisions: 2.2, attr.Technique: 2, attr.FirstTouch: 1.8, attr.Composure: 1.8}},
	"Engine":         {Name: "Engine", Pref: map[attr.Visible]float64{attr.Stamina: 2.5, attr.WorkRate: 2.5, attr.Teamwork: 1.8, attr.Passing: 1.4, attr.Positioning: 1.3}},
	"Winger":         {Name: "Winger", Pref: map[attr.Visible]float64{attr.Acceleration: 2.8, attr.Pace: 2.8, attr.Dribbling: 2.5, attr.Crossing: 2.1, attr.Agility: 1.6}, EarlyDecline: true},
	"Poacher":        {Name: "Poacher", Pref: map[attr.Visible]float64{attr.Finishing: 3, attr.OffBall: 2.4, attr.Anticipation: 2.2, attr.Composure: 1.8, attr.Acceleration: 1.5}},
	"Target Man":     {Name: "Target Man", Pref: map[attr.Visible]float64{attr.Strength: 2.8, attr.Heading: 2.6, attr.JumpingReach: 2.4, attr.Bravery: 1.8, attr.Finishing: 1.6}},
	"Wide Speedster": {Name: "Wide Speedster", Pref: map[attr.Visible]float64{attr.Acceleration: 3, attr.Pace: 3, attr.Dribbling: 2.2, attr.Agility: 1.8, attr.Finishing: 1.3}, EarlyDecline: true},
}

type archChoice struct {
	Name   string
	Weight int
}

var archetypesByPosition = map[string][]archChoice{
	posGK: {{"Shot-Stopper", 3}, {"Sweeper-Keeper", 2}, {"Commander", 2}},
	posDC: {{"Stopper", 4}, {"Ball-Player", 3}, {"Rapid", 1}},
	posDR: {{"Rapid", 4}, {"Stopper", 1}, {"Ball-Player", 1}},
	posDL: {{"Rapid", 4}, {"Stopper", 1}, {"Ball-Player", 1}},
	posDM: {{"Destroyer", 4}, {"Playmaker", 2}, {"Engine", 2}},
	posMC: {{"Engine", 3}, {"Playmaker", 3}, {"Destroyer", 2}},
	posMR: {{"Winger", 5}, {"Engine", 1}},
	posML: {{"Winger", 5}, {"Engine", 1}},
	posAM: {{"Playmaker", 4}, {"Winger", 2}},
	posST: {{"Poacher", 3}, {"Target Man", 2}, {"Wide Speedster", 1}},
	posWR: {{"Wide Speedster", 5}, {"Poacher", 1}},
	posWL: {{"Wide Speedster", 5}, {"Poacher", 1}},
}

func pickPlayerArchetype(r *rand.Rand, pos string) playerArchetype {
	choices := archetypesByPosition[pos]
	total := 0
	for _, c := range choices {
		total += c.Weight
	}
	roll := r.IntN(total)
	for _, c := range choices {
		if roll < c.Weight {
			return playerArchetypes[c.Name]
		}
		roll -= c.Weight
	}
	return playerArchetypes[choices[0].Name] // unreachable
}

// Age distribution 17–35, weighted to 22–29 (docs/09 §4.2).
var ageWeights = []int{2, 3, 4, 5, 6, 8, 8, 8, 8, 8, 8, 7, 6, 5, 4, 3, 2, 2, 1} // 17…35

func rollAge(r *rand.Rand) int {
	total := 0
	for _, w := range ageWeights {
		total += w
	}
	roll := r.IntN(total)
	for i, w := range ageWeights {
		if roll < w {
			return 17 + i
		}
		roll -= w
	}
	return 26
}

// rollBodyProfile gives each player public physical data. These are not hidden
// attributes: they are observable facts and later feed small aerial/duel
// modifiers in the match model. Ability still lives in attributes; body shape
// only nudges how reach/Strength express.
func rollBodyProfile(r *rand.Rand, pos string, arch playerArchetype) (heightCm, weightKg int) {
	heightBase := 180
	weightBase := 76
	switch pos {
	case posGK:
		heightBase, weightBase = 188, 84
	case posDC:
		heightBase, weightBase = 186, 82
	case posDR, posDL:
		heightBase, weightBase = 178, 74
	case posDM:
		heightBase, weightBase = 181, 78
	case posMC, posAM:
		heightBase, weightBase = 179, 75
	case posMR, posML, posWR, posWL:
		heightBase, weightBase = 176, 72
	case posST:
		heightBase, weightBase = 182, 78
	}
	switch arch.Name {
	case "Commander":
		heightBase += 4
		weightBase += 3
	case "Stopper", "Destroyer":
		heightBase += 2
		weightBase += 4
	case "Target Man":
		heightBase += 6
		weightBase += 6
	case "Rapid", "Winger", "Wide Speedster":
		heightBase -= 3
		weightBase -= 4
	case "Poacher":
		heightBase -= 1
		weightBase -= 1
	}
	heightCm = clamp(heightBase+r.IntN(15)-7, 160, 205)
	// Weight tracks height first, then position/archetype bulk. Keeping this
	// derived avoids impossible 198cm/62kg or 165cm/101kg profiles.
	weightKg = clamp(weightBase+(heightCm-heightBase)/2+r.IntN(13)-6, 58, 108)
	return heightCm, weightKg
}

// Potential headroom shrinks with age: up to +60 at 17, ≈0 at 30
// (docs/09 §4.2, tunable).
const (
	headroomAt17   = 60
	headroomEndAge = 30
)

func rollPotential(r *rand.Rand, pool, age int) int {
	if age >= headroomEndAge {
		return min(attr.PoolMax, pool)
	}
	maxHead := headroomAt17 * (headroomEndAge - age) / (headroomEndAge - 17)
	return min(attr.PoolMax, pool+r.IntN(maxHead+1))
}

// sortedGroupAttrs returns the attributes of a position group in a fixed
// order — never iterate attr.PoolCosts directly (map order, determinism).
func sortedGroupAttrs(group attr.PositionGroup) []attr.Visible {
	out := make([]attr.Visible, 0, len(attr.PoolCosts[group]))
	for a := range attr.PoolCosts[group] {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// spendPool buys attribute points with the sampled pool through the cost
// table, weighted by jittered archetype preferences with diminishing
// returns on already-high values — identity without uniform mush.
func spendPool(r *rand.Rand, group attr.PositionGroup, arch playerArchetype, pool int) map[attr.Visible]int {
	attrs := sortedGroupAttrs(group)
	costs := attr.PoolCosts[group]
	vals := make(map[attr.Visible]int, len(attrs))
	prefs := make(map[attr.Visible]float64, len(attrs))
	key := make(map[attr.Visible]bool, len(attrs))
	for _, a := range attrs {
		vals[a] = attr.ScaleMin
		p := 1.0
		if ap, ok := arch.Pref[a]; ok {
			p = ap
		}
		key[a] = p >= 1.5
		prefs[a] = p * math.Exp(r.NormFloat64()*0.3)
	}

	budget := float64(max(pool, 0))
	weights := make([]float64, len(attrs))
	for {
		total := 0.0
		for i, a := range attrs {
			w := 0.0
			if costs[a] <= budget && vals[a] < attr.ScaleMax {
				w = prefs[a] * growthDamping(vals[a], key[a])
			}
			weights[i] = w
			total += w
		}
		if total == 0 {
			return vals
		}
		roll := r.Float64() * total
		for i, a := range attrs {
			roll -= weights[i]
			if roll < 0 {
				vals[a]++
				budget -= costs[a]
				break
			}
		}
	}
}

// growthDamping makes further points on an already-high attribute less
// likely; key (archetype-preferred) attributes are allowed to run hotter.
func growthDamping(v int, key bool) float64 {
	switch {
	case v < 8:
		return 1.0
	case v < 12:
		if key {
			return 1.0
		}
		return 0.7
	case v < 15:
		if key {
			return 0.8
		}
		return 0.35
	case v < 18:
		if key {
			return 0.5
		}
		return 0.12
	default:
		if key {
			return 0.25
		}
		return 0.04
	}
}

// bell rolls a soft bell curve on the 1–20 scale (docs/09 §4.2).
func bell(r *rand.Rand) int {
	return clamp(int(math.Round(10.5+r.NormFloat64()*3.2)), attr.ScaleMin, attr.ScaleMax)
}

var hiddenBellAttrs = []attr.Hidden{
	attr.DevelopmentSpeed, attr.DeclineOnset, attr.DeclineSpeed,
	attr.Professionalism, attr.Ambition, attr.Loyalty, attr.Temperament,
	attr.Pressure, attr.Adaptability, attr.Sportsmanship, attr.Controversy,
	attr.Sociability, attr.Influence,
	attr.Consistency, attr.BigMatchNerve, attr.InjuryProne, attr.Recovery,
	attr.Discipline, attr.Versatility,
}

func rollHidden(r *rand.Rand, arch playerArchetype) map[attr.Hidden]int {
	h := make(map[attr.Hidden]int, len(hiddenBellAttrs))
	for _, a := range hiddenBellAttrs {
		h[a] = bell(r)
	}
	if arch.EarlyDecline { // trajectory correlates with archetype
		h[attr.DeclineOnset] = clamp(h[attr.DeclineOnset]-3, attr.ScaleMin, attr.ScaleMax)
	}
	return h
}

func rollFoot(r *rand.Rand, pos string) attr.PreferredFoot {
	leftSide := pos == posDL || pos == posML || pos == posWL
	roll := r.IntN(100)
	switch {
	case leftSide && roll < 55, !leftSide && roll < 25:
		return attr.FootLeft
	case roll < 95:
		return attr.FootRight
	default:
		return attr.FootEither
	}
}

func rollWeakFoot(r *rand.Rand, pos string, arch playerArchetype, foot attr.PreferredFoot) int {
	if foot == attr.FootEither {
		return clamp(16+r.IntN(5), attr.ScaleMin, attr.ScaleMax)
	}
	base := 8
	switch pos {
	case posMR, posML, posWR, posWL, posAM, posST:
		base += 2
	case posMC, posDM, posDR, posDL:
		base++
	}
	switch arch.Name {
	case "Playmaker", "Winger", "Wide Speedster", "Poacher":
		base += 2
	case "Target Man", "Stopper", "Commander":
		base--
	}
	return clamp(base+r.IntN(7)-3, attr.ScaleMin, attr.ScaleMax)
}

// familiarityAdjacency: positions a player also half-knows at generation.
var familiarityAdjacency = map[string][]string{
	posDC: {posDM},
	posDR: {posMR, posDC},
	posDL: {posML, posDC},
	posDM: {posMC, posDC},
	posMC: {posDM, posAM},
	posMR: {posWR, posMC},
	posML: {posWL, posMC},
	posAM: {posMC, posST},
	posWR: {posMR, posST},
	posWL: {posML, posST},
	posST: {posAM},
}

func rollFamiliarity(r *rand.Rand, pos string, versatility int) attr.PositionalFamiliarity {
	f := attr.PositionalFamiliarity{pos: 18 + r.IntN(3)}
	for _, adj := range familiarityAdjacency[pos] {
		f[adj] = clamp(7+r.IntN(7)+(versatility-10)/3, 1, 17)
	}
	return f
}

// Contract length 1–4 years, young prospects longer (docs/09 §4.2).
// ExpirySeasonYear 1 = end of the current season.
func rollContractYears(r *rand.Rand, age int) int {
	switch {
	case age <= 21:
		return 2 + r.IntN(3)
	case age <= 29:
		return 1 + r.IntN(3)
	default:
		return 1 + r.IntN(2)
	}
}

const (
	wagePoolExponent = 1.8   // wage = f(pool …) curve (docs/09 §4.2)
	wageRounding     = 5000  // minor units: quote wages in cr50 steps
	wageFloorMinor   = 5000  // cr50/wk
	youthWageBase    = 10000 // cr100/wk + roll
)

func rollWage(r *rand.Rand, avgWageMinor int64, band PoolBand, pool, reputation int) int64 {
	mid := float64(band.Min+band.Max) / 2
	f := math.Pow(float64(pool)/mid, wagePoolExponent)
	f *= math.Exp(r.NormFloat64() * 0.18)      // ambition/negotiation noise
	f *= 0.9 + float64(reputation)/10000.0*0.4 // reputation premium
	w := int64(float64(avgWageMinor) * f)
	w = w / wageRounding * wageRounding
	if w < wageFloorMinor {
		w = wageFloorMinor
	}
	return w
}

func rollPlayerReputation(r *rand.Rand, pool, divisions, tier int) int {
	return clamp(pool*30+(divisions-tier)*400+r.IntN(801)-400, 0, 10000)
}

func genPlayers(w *World, r *rand.Rand) {
	slots := buildSquadSlots(w.Config.SquadSizeTarget)
	nextID := int64(playerIDBase)

	for ci := range w.Clubs {
		club := &w.Clubs[ci]
		band := w.Derived.DivisionPoolBands[club.DivisionTier-1]
		avgWage := divisionAvgWageMinor(w.Config, club.DivisionTier)

		squadStart := len(w.Players)
		for _, pos := range slots {
			nextID++
			w.Players = append(w.Players, rollPlayer(r, w.Config, nextID, club.ID,
				pos, band, avgWage, w.Config.Divisions, club.DivisionTier, false))
		}
		// A few clubs roll a marquee earner (docs/09 §4.2).
		if r.Float64() < marqueeChance {
			top := squadStart
			for i := squadStart; i < len(w.Players); i++ {
				if w.Players[i].Contract.WageWeeklyMinor > w.Players[top].Contract.WageWeeklyMinor {
					top = i
				}
			}
			boosted := float64(w.Players[top].Contract.WageWeeklyMinor) * (1.7 + r.Float64()*0.5)
			w.Players[top].Contract.WageWeeklyMinor = int64(boosted) / wageRounding * wageRounding
		}
		// Initial academy prospects, ages 15–17 (docs/09 §4.2).
		academy := academyMin + r.IntN(academySpan)
		for i := 0; i < academy; i++ {
			pos := youthPosition(r, slots)
			nextID++
			p := rollYouth(r, w.Config, nextID, club.ID, pos, band, club.DivisionTier)
			w.Players = append(w.Players, p)
		}
	}

	// Free agents: ≈8% of the squad population, biased older and flawed.
	fa := int(float64(w.Config.TotalClubs()*w.Config.SquadSizeTarget)*freeAgentShare + 0.5)
	for i := 0; i < fa; i++ {
		tier := 1 + r.IntN(w.Config.Divisions)
		band := w.Derived.DivisionPoolBands[tier-1]
		pos := slots[r.IntN(len(slots))]
		nextID++
		p := rollPlayer(r, w.Config, nextID, 0, pos, band, 0, w.Config.Divisions, tier, true)
		w.Players = append(w.Players, p)
	}
	w.NextPlayerID = nextID // runtime spawns (youth intake) continue from here
}

// rollPlayer generates one senior player. Free agents (freeAgent=true) take
// an older age roll, a pool haircut, and no contract.
func rollPlayer(r *rand.Rand, cfg WorldConfig, id, clubID int64, pos string,
	band PoolBand, avgWageMinor int64, divisions, tier int, freeAgent bool) Player {

	group := posGroup(pos)
	arch := pickPlayerArchetype(r, pos)
	culture := pickCulture(r, cfg.CultureMix)

	age := rollAge(r)
	pool := band.Min + r.IntN(band.Max-band.Min+1)
	if freeAgent {
		age = 26 + r.IntN(10)
		pool = int(float64(pool) * freeAgentPoolCut)
	}

	hidden := rollHidden(r, arch)
	rep := rollPlayerReputation(r, pool, divisions, tier)
	foot := rollFoot(r, pos)
	weakFoot := rollWeakFoot(r, pos, arch, foot)
	visible := spendPool(r, group, arch, pool-int(math.Round(attr.WeakFootCost(group, weakFoot))))
	p := Player{
		ID:        id,
		Name:      personName(r, culture),
		Culture:   culture,
		Age:       age,
		ClubID:    clubID,
		Position:  pos,
		Group:     group,
		Archetype: arch.Name,
		Foot:      foot,
		WeakFoot:  weakFoot,
		// The stored pool is the materialized spend (sampled pool minus the
		// sub-point remainder), so pool == round(ProfilePoolCost) always
		// holds — the invariant drift maintains from here on.
		AbilityPool:  int(math.Round(attr.ProfilePoolCost(group, visible, weakFoot))),
		PotentialCap: rollPotential(r, pool, age),
		Visible:      visible,
		Hidden:       hidden,
		Reputation:   rep,
		Familiarity:  rollFamiliarity(r, pos, hidden[attr.Versatility]),
	}
	if !freeAgent {
		p.Contract = &Contract{
			WageWeeklyMinor:  rollWage(r, avgWageMinor, band, pool, rep),
			ExpirySeasonYear: rollContractYears(r, age),
		}
	}
	p.HeightCm, p.WeightKg = rollBodyProfile(r, pos, arch)
	return p
}

// rollYouth generates one academy prospect: small current pool, big headroom.
func rollYouth(r *rand.Rand, cfg WorldConfig, id, clubID int64, pos string,
	band PoolBand, tier int) Player {

	group := posGroup(pos)
	arch := pickPlayerArchetype(r, pos)
	culture := pickCulture(r, cfg.CultureMix)

	age := 15 + r.IntN(3)
	pool := band.Min/3 + r.IntN(max(band.Min/3, 1))
	hidden := rollHidden(r, arch)
	foot := rollFoot(r, pos)
	weakFoot := rollWeakFoot(r, pos, arch, foot)
	visible := spendPool(r, group, arch, pool-int(math.Round(attr.WeakFootCost(group, weakFoot))))
	p := Player{
		ID:           id,
		Name:         personName(r, culture),
		Culture:      culture,
		Age:          age,
		ClubID:       clubID,
		Position:     pos,
		Group:        group,
		Archetype:    arch.Name,
		Foot:         foot,
		WeakFoot:     weakFoot,
		AbilityPool:  int(math.Round(attr.ProfilePoolCost(group, visible, weakFoot))),
		PotentialCap: rollPotential(r, pool, age),
		Visible:      visible,
		Hidden:       hidden,
		Reputation:   clamp(pool*10+r.IntN(201), 0, 10000),
		Familiarity:  rollFamiliarity(r, pos, hidden[attr.Versatility]),
		Contract: &Contract{
			WageWeeklyMinor:  int64(youthWageBase + r.IntN(2)*youthWageBase),
			ExpirySeasonYear: 1 + r.IntN(2),
		},
		Youth: true,
	}
	p.HeightCm, p.WeightKg = rollBodyProfile(r, pos, arch)
	return p
}

// youthPosition picks an academy prospect's position: mostly outfield, roughly
// one in ten a keeper (docs/09 §4.2). Shared by the generation-time academy and
// the runtime intake so both draw from the same distribution. The draw order —
// outfield slot first, then the keeper override — matches the original academy
// loop exactly, so extracting it leaves generated worlds byte-identical.
func youthPosition(r *rand.Rand, slots []string) string {
	pos := slots[squadGKCount+r.IntN(len(slots)-squadGKCount)]
	if r.IntN(10) == 0 {
		pos = posGK
	}
	return pos
}

// GenYouthIntake adds one spring's academy prospects to a club, mutating the
// world (youth intake). It allocates fresh monotonic ids from w.NextPlayerID and
// appends to w.Players, returning the new ids so the engine can schedule their
// development and rebuild its index. The batch is throttled by a per-club soft
// cap (youthAcademyCap) so the youth population stays bounded with no deletion
// (news/results reference player ids); graduation drains it each boundary.
// season is the intake's current season year: rollYouth stamps contract expiry
// in season-1-absolute years (right for the generation-time academy), so a
// runtime intake shifts it to the live season — otherwise a season-N prospect
// would be born with an already-expired deal. Fully
// deterministic: the caller passes a stream keyed on the intake event, the
// shift is arithmetic (no extra draws), and the cap count is a pure function of
// replay state, so a resumed run generates the identical intake (NFR-2). Youth
// tendencies (YouthFacilities / YouthEmphasis) don't yet shape the batch —
// handled by maturation rules.
func GenYouthIntake(w *World, r *rand.Rand, club *Club, season int) []int64 {
	current := 0
	for i := range w.Players {
		if w.Players[i].ClubID == club.ID && w.Players[i].Youth {
			current++
		}
	}
	add := w.Config.YouthIntakeBatch
	if room := youthAcademyCap - current; add > room {
		add = room
	}
	if add <= 0 {
		return nil // academy at capacity — nothing this spring
	}

	band := w.Derived.DivisionPoolBands[club.DivisionTier-1]
	slots := buildSquadSlots(w.Config.SquadSizeTarget)
	ids := make([]int64, 0, add)
	for i := 0; i < add; i++ {
		pos := youthPosition(r, slots)
		w.NextPlayerID++
		p := rollYouth(r, w.Config, w.NextPlayerID, club.ID, pos, band, club.DivisionTier)
		// Every generated player is seeded match-fresh by EnsureMatchState, but a
		// runtime intake bypasses it, so seed fitness here for consistency. Otherwise a fresh prospect
		// reads as Condition/Sharpness 0 to get_squad(detail=condition) (Sharpness never
		// recovers without a match), and a future graduate would enter its first squad
		// unfit.
		p.Condition, p.Sharpness = ConditionMax, ConditionMax
		p.Contract.ExpirySeasonYear += season - 1 // rollYouth is season-1-absolute
		w.Players = append(w.Players, p)
		ids = append(ids, p.ID)
		// Keep the club's cached wage bill in step with the new contract, exactly as
		// the transfer engine does — generation counts youth wages in the bill, and
		// finance ticks + transfer affordability read the cache.
		club.WageBillWeeklyMinor += p.Contract.WageWeeklyMinor
	}
	return ids
}
