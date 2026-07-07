package worldgen

import (
	"math/rand/v2"

	"github.com/gaemi/agentic-fc/internal/focus"
	"github.com/gaemi/agentic-fc/internal/mindset"
)

// Stage 3 — managers: one per club plus the unemployed pool, each running a
// predefined Mindset from an archetype (docs/09 §4.3). The exact archetype
// → Disposition tables were left for implementation (docs/09 §6); they are
// authored here as initial values: 10 axes each, ±2 rolled
// jitter, a Priorities template, and a complete default Tactical Plan
// (autonomous Managers need one from day 0). Directives start empty.

type managerArchetype struct {
	Name       string
	Axes       map[mindset.Axis]int
	Priorities []mindset.Priority
	Tactical   mindset.TacticalPlan
}

var managerArchetypes = []managerArchetype{
	{
		Name: "The Idealist", // attacking, youth-first, loyal, patient
		Axes: map[mindset.Axis]int{
			mindset.AxisRiskAppetite: 4, mindset.AxisYouthPreference: 8,
			mindset.AxisPlayingIdentity: 7, mindset.AxisFinancialPrudence: 0,
			mindset.AxisLoyalty: 7, mindset.AxisDiscipline: -2,
			mindset.AxisHorizon: 8, mindset.AxisMediaPosture: 0,
			mindset.AxisTacticalFlexibility: -4, mindset.AxisPersonalAmbition: -2,
		},
		Priorities: []mindset.Priority{
			{Rank: 1, Goal: mindset.GoalDevelopYouth},
			{Rank: 2, Goal: mindset.GoalEstablishIdentity},
			{Rank: 3, Goal: mindset.GoalBuildSquadValue},
		},
		Tactical: mindset.TacticalPlan{Formation: "4-3-3", Mentality: "ATTACKING",
			Pressing: "HIGH", Tempo: "FAST", Width: "WIDE", Directness: "SHORT"},
	},
	{
		Name: "The Pragmatist", // results-first, flexible, risk-averse
		Axes: map[mindset.Axis]int{
			mindset.AxisRiskAppetite: -5, mindset.AxisYouthPreference: -2,
			mindset.AxisPlayingIdentity: -6, mindset.AxisFinancialPrudence: 3,
			mindset.AxisLoyalty: -2, mindset.AxisDiscipline: 2,
			mindset.AxisHorizon: -3, mindset.AxisMediaPosture: -2,
			mindset.AxisTacticalFlexibility: 7, mindset.AxisPersonalAmbition: 2,
		},
		Priorities: []mindset.Priority{
			{Rank: 1, Goal: mindset.GoalFinishTopN, Params: map[string]any{"n": float64(8)}},
			{Rank: 2, Goal: mindset.GoalProtectJob},
			{Rank: 3, Goal: mindset.GoalFinancialHealth},
		},
		Tactical: mindset.TacticalPlan{Formation: "4-4-2", Mentality: "BALANCED",
			Pressing: "MID", Tempo: "MIXED", Width: "MIXED", Directness: "MIXED", Counter: true},
	},
	{
		Name: "The Firefighter", // survival specialist; defensive, short-horizon
		Axes: map[mindset.Axis]int{
			mindset.AxisRiskAppetite: -7, mindset.AxisYouthPreference: -5,
			mindset.AxisPlayingIdentity: -8, mindset.AxisFinancialPrudence: 5,
			mindset.AxisLoyalty: -4, mindset.AxisDiscipline: 4,
			mindset.AxisHorizon: -9, mindset.AxisMediaPosture: -3,
			mindset.AxisTacticalFlexibility: 3, mindset.AxisPersonalAmbition: 1,
		},
		Priorities: []mindset.Priority{
			{Rank: 1, Goal: mindset.GoalAvoidRelegation},
			{Rank: 2, Goal: mindset.GoalProtectJob},
		},
		Tactical: mindset.TacticalPlan{Formation: "5-4-1", Mentality: "VERY_DEFENSIVE",
			Pressing: "LOW", Tempo: "SLOW", Width: "NARROW", Directness: "DIRECT", Counter: true},
	},
	{
		Name: "The Trader", // market-driven; buys low, sells anyone
		Axes: map[mindset.Axis]int{
			mindset.AxisRiskAppetite: 1, mindset.AxisYouthPreference: 2,
			mindset.AxisPlayingIdentity: -1, mindset.AxisFinancialPrudence: 6,
			mindset.AxisLoyalty: -8, mindset.AxisDiscipline: 0,
			mindset.AxisHorizon: 2, mindset.AxisMediaPosture: 1,
			mindset.AxisTacticalFlexibility: 2, mindset.AxisPersonalAmbition: 4,
		},
		Priorities: []mindset.Priority{
			{Rank: 1, Goal: mindset.GoalBuildSquadValue},
			{Rank: 2, Goal: mindset.GoalFinancialHealth},
			{Rank: 3, Goal: mindset.GoalFinishTopN, Params: map[string]any{"n": float64(10)}},
		},
		Tactical: mindset.TacticalPlan{Formation: "4-2-3-1", Mentality: "BALANCED",
			Pressing: "MID", Tempo: "MIXED", Width: "MIXED", Directness: "MIXED"},
	},
	{
		Name: "The Professor", // system-obsessed; stubborn tactics, analytical
		Axes: map[mindset.Axis]int{
			mindset.AxisRiskAppetite: 0, mindset.AxisYouthPreference: 3,
			mindset.AxisPlayingIdentity: 4, mindset.AxisFinancialPrudence: 2,
			mindset.AxisLoyalty: 2, mindset.AxisDiscipline: 3,
			mindset.AxisHorizon: 6, mindset.AxisMediaPosture: -5,
			mindset.AxisTacticalFlexibility: -9, mindset.AxisPersonalAmbition: 0,
		},
		Priorities: []mindset.Priority{
			{Rank: 1, Goal: mindset.GoalEstablishIdentity},
			{Rank: 2, Goal: mindset.GoalDevelopYouth},
			{Rank: 3, Goal: mindset.GoalFinishTopN, Params: map[string]any{"n": float64(6)}},
		},
		Tactical: mindset.TacticalPlan{Formation: "4-2-3-1", Mentality: "ATTACKING",
			Pressing: "HIGH", Tempo: "MIXED", Width: "WIDE", Directness: "SHORT"},
	},
	{
		Name: "The Motivator", // man-management first; chemistry over talent
		Axes: map[mindset.Axis]int{
			mindset.AxisRiskAppetite: 1, mindset.AxisYouthPreference: 1,
			mindset.AxisPlayingIdentity: 2, mindset.AxisFinancialPrudence: 0,
			mindset.AxisLoyalty: 8, mindset.AxisDiscipline: -5,
			mindset.AxisHorizon: 3, mindset.AxisMediaPosture: 2,
			mindset.AxisTacticalFlexibility: 0, mindset.AxisPersonalAmbition: -1,
		},
		Priorities: []mindset.Priority{
			{Rank: 1, Goal: mindset.GoalEstablishIdentity},
			{Rank: 2, Goal: mindset.GoalFinishTopN, Params: map[string]any{"n": float64(8)}},
			{Rank: 3, Goal: mindset.GoalCupRun},
		},
		Tactical: mindset.TacticalPlan{Formation: "4-4-2", Mentality: "BALANCED",
			Pressing: "MID", Tempo: "FAST", Width: "MIXED", Directness: "MIXED", Counter: true},
	},
	{
		Name: "The Tyrant", // discipline, high demands, volatile relationships
		Axes: map[mindset.Axis]int{
			mindset.AxisRiskAppetite: 2, mindset.AxisYouthPreference: -3,
			mindset.AxisPlayingIdentity: -3, mindset.AxisFinancialPrudence: 1,
			mindset.AxisLoyalty: -6, mindset.AxisDiscipline: 9,
			mindset.AxisHorizon: -2, mindset.AxisMediaPosture: 5,
			mindset.AxisTacticalFlexibility: -3, mindset.AxisPersonalAmbition: 5,
		},
		Priorities: []mindset.Priority{
			{Rank: 1, Goal: mindset.GoalFinishTopN, Params: map[string]any{"n": float64(4)}},
			{Rank: 2, Goal: mindset.GoalCupRun},
		},
		Tactical: mindset.TacticalPlan{Formation: "4-1-4-1", Mentality: "BALANCED",
			Pressing: "HIGH", Tempo: "FAST", Width: "NARROW", Directness: "DIRECT", Counter: true},
	},
	{
		Name: "The Gambler", // high risk in every dimension
		Axes: map[mindset.Axis]int{
			mindset.AxisRiskAppetite: 9, mindset.AxisYouthPreference: 4,
			mindset.AxisPlayingIdentity: 6, mindset.AxisFinancialPrudence: -7,
			mindset.AxisLoyalty: -3, mindset.AxisDiscipline: -4,
			mindset.AxisHorizon: -4, mindset.AxisMediaPosture: 6,
			mindset.AxisTacticalFlexibility: 5, mindset.AxisPersonalAmbition: 6,
		},
		Priorities: []mindset.Priority{
			{Rank: 1, Goal: mindset.GoalWinLeague},
			{Rank: 2, Goal: mindset.GoalCupRun},
		},
		Tactical: mindset.TacticalPlan{Formation: "3-4-3", Mentality: "VERY_ATTACKING",
			Pressing: "HIGH", Tempo: "FAST", Width: "WIDE", Directness: "DIRECT"},
	},
}

// archetypeWeight biases assignment by club character (docs/09 §4.3): a
// high-Youth-Emphasis club more often employs an Idealist, a poor impatient
// board attracts Firefighters, and so on. Weights are initial values.
func archetypeWeight(name string, t Tendencies) int {
	w := 10
	switch name {
	case "The Idealist":
		w += t.YouthEmphasis
	case "The Pragmatist":
		w += 5 + t.BoardPatience/3
	case "The Firefighter":
		w += (20-t.BoardPatience)/2 + (20-t.Wealth)/2
	case "The Trader":
		w += (20-t.Wealth)/2 + t.BoardAmbition/3
	case "The Professor":
		w += t.TrainingFacilities / 2
	case "The Motivator":
		w += t.FanPassion / 2
	case "The Tyrant":
		w += (20-t.FanPatience)/3 + t.BoardAmbition/3
	case "The Gambler":
		w += t.BoardAmbition / 2
	}
	return w
}

// Manager generation constants (initial values; docs/09 §4.3).
const (
	managerIDBase        = 1000
	managerAgeMin        = 33
	managerRepTopCenter  = 6000 // Reputation center for tier 1
	managerRepTierStep   = 1500 // center drop per tier
	managerRepSpread     = 1500 // ± roll
	managerRepFloor      = 100
	managerRepCeil       = 9500
	unemployedRepPenalty = 0.7 // pool managers arrive with dented reputations
	dispositionJitter    = 2   // ± on every axis
)

func genManagers(w *World, r *rand.Rand) {
	nextID := int64(managerIDBase)
	for i := range w.Clubs {
		club := &w.Clubs[i]
		arch := pickArchetype(r, club.Tendencies)
		nextID++
		w.Managers = append(w.Managers, rollManager(r, nextID, club.ID, club.DivisionTier, arch, w.Config.CultureMix))
	}
	// Unemployed pool: ≈10% of club count, min 2 (docs/09 §4.3). Archetype
	// is rolled flat; reputation takes the unemployment haircut.
	pool := unemployedPoolSize(w.Config.TotalClubs())
	for i := 0; i < pool; i++ {
		arch := managerArchetypes[r.IntN(len(managerArchetypes))]
		tier := 1 + r.IntN(w.Config.Divisions)
		nextID++
		m := rollManager(r, nextID, 0, tier, arch, w.Config.CultureMix)
		m.Reputation = int(float64(m.Reputation) * unemployedRepPenalty)
		w.Managers = append(w.Managers, m)
	}
	w.NextManagerID = nextID // runtime spawns (caretakers, newgen) continue from here
}

// caretakerArchetypeName is the conservative archetype an auto-installed caretaker
// runs on (docs/02 §3.1: modest, steady). It must name a real managerArchetype.
const caretakerArchetypeName = "The Pragmatist"

// caretakerRepFactor and caretakerAttrCap keep a caretaker's standing modest — a
// stopgap, not a marquee hire (tunable, docs/98).
const (
	caretakerRepFactor = 0.5
	caretakerAttrCap   = 12
)

// SpawnManager creates a new manager at runtime and appends it to the world,
// allocating a fresh monotonic id. It is the shared primitive
// for both an auto-installed caretaker (A) and a newgen pool backfill (C) — the
// same pattern as the transfer engine's executeTransfer. Fully deterministic: the
// caller passes a stream keyed on the spawn event, so a resumed run spawns the
// identical manager (NFR-2). The returned pointer is into w.Managers, which may
// have reallocated — callers holding a manager index must rebuild it.
func SpawnManager(w *World, r *rand.Rand, clubID int64, tier int, caretaker bool) *Manager {
	w.NextManagerID++
	arch := managerArchetypes[r.IntN(len(managerArchetypes))]
	if caretaker {
		arch = archetypeByName(caretakerArchetypeName)
	}
	m := rollManager(r, w.NextManagerID, clubID, tier, arch, w.Config.CultureMix)
	if caretaker {
		m.Caretaker = true
		m.Reputation = int(float64(m.Reputation) * caretakerRepFactor)
		m.Coaching = min(m.Coaching, caretakerAttrCap)
		m.ManManagement = min(m.ManManagement, caretakerAttrCap)
	}
	w.Managers = append(w.Managers, m)
	return &w.Managers[len(w.Managers)-1]
}

// archetypeByName returns the named archetype, falling back to the first if the
// name is unknown (keeps SpawnManager total).
func archetypeByName(name string) managerArchetype {
	for _, a := range managerArchetypes {
		if a.Name == name {
			return a
		}
	}
	return managerArchetypes[0]
}

func pickArchetype(r *rand.Rand, t Tendencies) managerArchetype {
	total := 0
	weights := make([]int, len(managerArchetypes))
	for i, a := range managerArchetypes {
		weights[i] = archetypeWeight(a.Name, t)
		total += weights[i]
	}
	roll := r.IntN(total)
	for i, wt := range weights {
		if roll < wt {
			return managerArchetypes[i]
		}
		roll -= wt
	}
	return managerArchetypes[0] // unreachable
}

func rollManager(r *rand.Rand, id, clubID int64, tier int, arch managerArchetype, mix CultureMix) Manager {
	culture := pickCulture(r, mix)
	repCenter := managerRepTopCenter - managerRepTierStep*(tier-1)
	rep := clamp(repCenter+r.IntN(2*managerRepSpread+1)-managerRepSpread,
		managerRepFloor, managerRepCeil)

	m := Manager{
		ID:      id,
		Name:    personName(r, culture),
		Culture: culture,
		// 33–68, triangular toward the middle.
		Age:           managerAgeMin + r.IntN(18) + r.IntN(19),
		ClubID:        clubID,
		Archetype:     arch.Name,
		Reputation:    rep,
		Coaching:      clamp(5+r.IntN(12)+boolToInt(tier == 1), 1, 20),
		ManManagement: clamp(5+r.IntN(12), 1, 20),
		FocusBalance:  focus.StartingBalance, // docs/11 §2
		Status:        ManagerActive,
	}

	// Mindset from the archetype: jittered axes, template priorities,
	// complete tactical plan, empty directives (docs/09 §4.3).
	disp := map[mindset.Axis]int{}
	for _, axis := range mindset.AllAxes {
		disp[axis] = clamp(arch.Axes[axis]+r.IntN(2*dispositionJitter+1)-dispositionJitter,
			mindset.AxisMin, mindset.AxisMax)
	}
	m.Mindset = mindset.Mindset{
		ArchetypeOrigin: arch.Name,
		Disposition:     mindset.Disposition{Current: disp},
		Priorities:      clonePriorities(arch.Priorities),
		Tactical:        arch.Tactical,
	}
	return m
}

func clonePriorities(ps []mindset.Priority) []mindset.Priority {
	out := make([]mindset.Priority, len(ps))
	copy(out, ps)
	for i := range out {
		if out[i].Params != nil {
			params := make(map[string]any, len(out[i].Params))
			for k, v := range out[i].Params {
				params[k] = v
			}
			out[i].Params = params
		}
	}
	return out
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
