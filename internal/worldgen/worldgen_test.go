package worldgen

import (
	"math"
	"sync"
	"testing"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/sim"
)

var (
	classicOnce sync.Once
	classic     *Result
)

// classicWorld generates one Classic (2×16) world shared across tests.
func classicWorld(t *testing.T) *Result {
	t.Helper()
	classicOnce.Do(func() {
		res, err := Generate(PresetClassic(42), WithTokenReader(&counterReader{}))
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		classic = res
	})
	return classic
}

func TestClubs(t *testing.T) {
	w := classicWorld(t).World
	if len(w.Clubs) != w.Config.TotalClubs() {
		t.Fatalf("clubs = %d, want %d", len(w.Clubs), w.Config.TotalClubs())
	}
	names := map[string]bool{}
	kits := map[int]map[Colors]bool{}
	for _, c := range w.Clubs {
		if names[c.Name] {
			t.Errorf("duplicate club name %q", c.Name)
		}
		names[c.Name] = true
		if kits[c.DivisionTier] == nil {
			kits[c.DivisionTier] = map[Colors]bool{}
		}
		if kits[c.DivisionTier][c.Colors] {
			t.Errorf("kit clash in division %d: %+v", c.DivisionTier, c.Colors)
		}
		kits[c.DivisionTier][c.Colors] = true

		for name, v := range map[string]int{
			"wealth": c.Tendencies.Wealth, "board_patience": c.Tendencies.BoardPatience,
			"board_ambition": c.Tendencies.BoardAmbition, "fan_patience": c.Tendencies.FanPatience,
			"fan_passion": c.Tendencies.FanPassion, "youth_emphasis": c.Tendencies.YouthEmphasis,
			"training_facilities": c.Tendencies.TrainingFacilities,
			"youth_facilities":    c.Tendencies.YouthFacilities,
		} {
			if v < 1 || v > 20 {
				t.Errorf("club %d tendency %s = %d out of 1–20", c.ID, name, v)
			}
		}
		if c.Stadium.Capacity <= 0 || c.Stadium.Name == "" {
			t.Errorf("club %d has a broken stadium %+v", c.ID, c.Stadium)
		}
		if c.RegionID < 1 || int(c.RegionID) > len(w.Regions) {
			t.Errorf("club %d region %d out of range", c.ID, c.RegionID)
		}
	}
}

func TestManagers(t *testing.T) {
	w := classicWorld(t).World
	wantPool := unemployedPoolSize(w.Config.TotalClubs())
	if len(w.Managers) != len(w.Clubs)+wantPool {
		t.Fatalf("managers = %d, want %d clubs + %d pool", len(w.Managers), len(w.Clubs), wantPool)
	}
	archNames := map[string]bool{}
	for _, a := range managerArchetypes {
		archNames[a.Name] = true
	}
	if len(managerArchetypes) != 8 {
		t.Fatalf("archetype catalog = %d, docs/09 §4.3 says 8", len(managerArchetypes))
	}
	clubManager := map[int64]int{}
	for _, m := range w.Managers {
		if !archNames[m.Archetype] {
			t.Errorf("manager %d has unknown archetype %q", m.ID, m.Archetype)
		}
		if m.ClubID != 0 {
			clubManager[m.ClubID]++
		}
		if m.Age < 33 || m.Age > 68 {
			t.Errorf("manager %d age %d out of 33–68", m.ID, m.Age)
		}
		if m.Reputation < 0 || m.Reputation > 10000 {
			t.Errorf("manager %d reputation %d out of range", m.ID, m.Reputation)
		}
		for _, axis := range mindset.AllAxes {
			v, ok := m.Mindset.Disposition.Current[axis]
			if !ok {
				t.Errorf("manager %d missing axis %s (no dead axes)", m.ID, axis)
			}
			if v < mindset.AxisMin || v > mindset.AxisMax {
				t.Errorf("manager %d axis %s = %d out of range", m.ID, axis, v)
			}
		}
		if len(m.Mindset.Priorities) == 0 || len(m.Mindset.Priorities) > mindset.MaxPriorities {
			t.Errorf("manager %d has %d priorities", m.ID, len(m.Mindset.Priorities))
		}
		if err := m.Mindset.Tactical.Validate(); err != nil {
			t.Errorf("manager %d tactical plan invalid: %v", m.ID, err)
		}
		if m.Mindset.Tactical.Formation == "" || m.Mindset.Tactical.Mentality == "" {
			t.Errorf("manager %d tactical plan incomplete (autonomous Managers need one)", m.ID)
		}
		if len(m.Mindset.Directives) != 0 {
			t.Errorf("manager %d starts with directives; docs/09 §4.3 says empty", m.ID)
		}
	}
	// club.manager never null (FR-14d starts holding at generation).
	for _, c := range w.Clubs {
		if clubManager[c.ID] != 1 {
			t.Errorf("club %d has %d managers, want exactly 1", c.ID, clubManager[c.ID])
		}
	}
}

func TestPlayers(t *testing.T) {
	w := classicWorld(t).World
	byClub := map[int64][]*Player{}
	var freeAgents, youth int
	for i := range w.Players {
		p := &w.Players[i]
		if p.ClubID == 0 {
			freeAgents++
			if p.Contract != nil {
				t.Errorf("free agent %d has a contract", p.ID)
			}
		} else {
			byClub[p.ClubID] = append(byClub[p.ClubID], p)
			if p.Contract == nil {
				t.Errorf("squad player %d has no contract", p.ID)
			}
		}
		if p.Youth {
			youth++
			if p.Age < 15 || p.Age > 17 {
				t.Errorf("youth %d age %d out of 15–17", p.ID, p.Age)
			}
		} else if p.Age < 17 || p.Age > 35 {
			t.Errorf("player %d age %d out of 17–35", p.ID, p.Age)
		}
		checkAttributes(t, p)
		if p.PotentialCap < p.AbilityPool || p.PotentialCap > attr.PoolMax {
			t.Errorf("player %d potential %d vs pool %d invalid", p.ID, p.PotentialCap, p.AbilityPool)
		}
		if fam := p.Familiarity[p.Position]; fam < 18 {
			t.Errorf("player %d not Natural (%d) at primary position %s", p.ID, fam, p.Position)
		}
	}

	target := w.Config.SquadSizeTarget
	for _, c := range w.Clubs {
		var seniors, gks, academy int
		for _, p := range byClub[c.ID] {
			if p.Youth {
				academy++
				continue
			}
			seniors++
			if p.Group == attr.GK {
				gks++
			}
		}
		if seniors != target {
			t.Errorf("club %d squad = %d, want %d", c.ID, seniors, target)
		}
		if gks != squadGKCount {
			t.Errorf("club %d has %d GKs, want %d", c.ID, gks, squadGKCount)
		}
		if academy < academyMin || academy >= academyMin+academySpan {
			t.Errorf("club %d academy = %d, want 3–5", c.ID, academy)
		}
	}

	wantFA := int(float64(w.Config.TotalClubs()*target)*freeAgentShare + 0.5)
	if freeAgents != wantFA {
		t.Errorf("free agents = %d, want %d (≈8%%)", freeAgents, wantFA)
	}
}

// checkAttributes verifies the attribute contract: right attribute set per
// group, 1–20 range, and the pool actually spent through the cost table.
func checkAttributes(t *testing.T, p *Player) {
	t.Helper()
	wantAttrs := len(attr.PoolCosts[p.Group])
	if len(p.Visible) != wantAttrs {
		t.Errorf("player %d has %d visible attributes, want %d", p.ID, len(p.Visible), wantAttrs)
	}
	_, hasGKAttr := p.Visible[attr.Reflexes]
	_, hasOutfield := p.Visible[attr.Finishing]
	if (p.Group == attr.GK) != hasGKAttr || (p.Group == attr.GK) == hasOutfield {
		t.Errorf("player %d (%s) has the wrong attribute set", p.ID, p.Group)
	}
	for a, v := range p.Visible {
		if v < attr.ScaleMin || v > attr.ScaleMax {
			t.Errorf("player %d %s = %d out of 1–20", p.ID, a, v)
		}
	}
	if p.WeakFoot < attr.ScaleMin || p.WeakFoot > attr.ScaleMax {
		t.Errorf("player %d weak foot %d out of 1–20", p.ID, p.WeakFoot)
	}
	// The stored pool is the materialized spend: pool == round(ProfilePoolCost).
	spent := attr.ProfilePoolCost(p.Group, p.Visible, p.WeakFoot)
	if math.Abs(spent-float64(p.AbilityPool)) > 0.5 {
		t.Errorf("player %d pool %d out of sync with cost %.1f", p.ID, p.AbilityPool, spent)
	}
	if p.HeightCm < 160 || p.HeightCm > 205 {
		t.Errorf("player %d height %dcm out of range", p.ID, p.HeightCm)
	}
	if p.WeightKg < 58 || p.WeightKg > 108 {
		t.Errorf("player %d weight %dkg out of range", p.ID, p.WeightKg)
	}
	for _, h := range hiddenBellAttrs {
		v, ok := p.Hidden[h]
		if !ok || v < attr.ScaleMin || v > attr.ScaleMax {
			t.Errorf("player %d hidden %s = %d invalid", p.ID, h, v)
		}
	}
}

func TestHistoryAndPredictions(t *testing.T) {
	w := classicWorld(t).World
	n := w.Config.ClubsPerDivision
	games := 2 * (n - 1)
	if len(w.LastSeason) != w.Config.Divisions {
		t.Fatalf("last-season tables = %d, want %d", len(w.LastSeason), w.Config.Divisions)
	}
	for tier, table := range w.LastSeason {
		if len(table) != n {
			t.Fatalf("tier %d table has %d rows, want %d", tier+1, len(table), n)
		}
		for i, row := range table {
			if row.Pos != i+1 {
				t.Errorf("tier %d row %d has pos %d", tier+1, i, row.Pos)
			}
			if row.Played != games || row.Won+row.Drawn+row.Lost != games {
				t.Errorf("tier %d club %d W/D/L don't sum to %d", tier+1, row.ClubID, games)
			}
			if row.Points != 3*row.Won+row.Drawn {
				t.Errorf("tier %d club %d points mismatch", tier+1, row.ClubID)
			}
			if i > 0 && table[i-1].Points < row.Points {
				t.Errorf("tier %d table not sorted at pos %d", tier+1, row.Pos)
			}
		}
	}
	for _, c := range w.Clubs {
		if c.PredictedFinish < 1 || c.PredictedFinish > n {
			t.Errorf("club %d predicted finish %d out of range", c.ID, c.PredictedFinish)
		}
		if c.BoardObjectiveFinish < 1 || c.BoardObjectiveFinish > n {
			t.Errorf("club %d board objective %d out of range", c.ID, c.BoardObjectiveFinish)
		}
		if c.ConfidenceBaseline < 20 || c.ConfidenceBaseline > 95 {
			t.Errorf("club %d confidence %d out of range", c.ID, c.ConfidenceBaseline)
		}
	}
}

func TestRivalries(t *testing.T) {
	w := classicWorld(t).World
	involved := map[int64]int{}
	seen := map[[2]int64]bool{}
	for _, r := range w.Rivalries {
		if r.ClubA >= r.ClubB {
			t.Errorf("rivalry pair not ordered: %+v", r)
		}
		key := [2]int64{r.ClubA, r.ClubB}
		if seen[key] {
			t.Errorf("duplicate rivalry %+v", r)
		}
		seen[key] = true
		if r.Weight < 1 || r.Weight > 3 {
			t.Errorf("rivalry weight %d out of 1–3", r.Weight)
		}
		involved[r.ClubA]++
		involved[r.ClubB]++
	}
	for _, c := range w.Clubs {
		if involved[c.ID] == 0 {
			t.Errorf("club %d has no rivalry (docs/09 §4.4 says 1–2 per club)", c.ID)
		}
	}
}

func TestSchedule(t *testing.T) {
	w := classicWorld(t).World
	n := w.Config.ClubsPerDivision
	rounds := w.Derived.Rounds

	type meeting struct{ home, away int64 }
	perDivision := map[int]int{}
	kickoffByRound := map[[2]int]sim.GameTime{} // {tier, round}
	meetings := map[meeting]int{}
	var cupR1 int
	for _, f := range w.Fixtures {
		switch f.Competition {
		case CompetitionLeague:
			perDivision[f.DivisionTier]++
			meetings[meeting{f.HomeID, f.AwayID}]++
			key := [2]int{f.DivisionTier, f.Round}
			if prev, ok := kickoffByRound[key]; ok && prev != f.Kickoff {
				t.Errorf("round %v has multiple kickoff times (FR-6a)", key)
			}
			kickoffByRound[key] = f.Kickoff
		case CompetitionCup:
			if f.Round == 1 {
				cupR1++
			}
		}
	}
	for tier := 1; tier <= w.Config.Divisions; tier++ {
		if perDivision[tier] != rounds*n/2 {
			t.Errorf("tier %d has %d league fixtures, want %d", tier, perDivision[tier], rounds*n/2)
		}
	}
	// Double round-robin: every ordered pair exactly once.
	for m, count := range meetings {
		if count != 1 {
			t.Errorf("pair %v meets %d times at home, want 1", m, count)
		}
		if meetings[meeting{m.away, m.home}] != 1 {
			t.Errorf("pair %v missing the return fixture", m)
		}
	}
	// League-wide match days: same round ⇒ same kickoff across divisions.
	for round := 1; round <= rounds; round++ {
		var t0 sim.GameTime
		for tier := 1; tier <= w.Config.Divisions; tier++ {
			k := kickoffByRound[[2]int{tier, round}]
			if tier == 1 {
				t0 = k
			} else if k != t0 {
				t.Errorf("round %d kicks off at different times across divisions", round)
			}
		}
	}

	clubs := w.Config.TotalClubs()
	bracket := w.Derived.CupBracketSize
	if bracket < clubs || bracket >= 2*clubs {
		t.Errorf("cup bracket %d wrong for %d clubs", bracket, clubs)
	}
	if want := (clubs - w.Derived.CupByes) / 2; cupR1 != want {
		t.Errorf("cup round 1 has %d ties, want %d", cupR1, want)
	}
	if len(w.CupByes) != w.Derived.CupByes {
		t.Errorf("bye list = %d, want %d", len(w.CupByes), w.Derived.CupByes)
	}
	for _, c := range w.Clubs {
		if c.YouthIntakeDay < dayYouthIntakeStart || c.YouthIntakeDay > dayYouthIntakeEnd {
			t.Errorf("club %d youth intake day %d outside the spring window", c.ID, c.YouthIntakeDay)
		}
	}
}

func TestEconomy(t *testing.T) {
	w := classicWorld(t).World
	for _, c := range w.Clubs {
		var bill int64
		for i := range w.Players {
			p := &w.Players[i]
			if p.ClubID == c.ID && p.Contract != nil {
				bill += p.Contract.WageWeeklyMinor
			}
		}
		if c.WageBillWeeklyMinor != bill {
			t.Errorf("club %d wage bill %d ≠ contract sum %d", c.ID, c.WageBillWeeklyMinor, bill)
		}
		if c.WageBudgetWeeklyMinor < bill {
			t.Errorf("club %d starts over its wage budget", c.ID)
		}
		if c.BalanceMinor <= 0 || c.TransferBudgetMinor < 0 {
			t.Errorf("club %d has broken finances: balance %d, transfer %d",
				c.ID, c.BalanceMinor, c.TransferBudgetMinor)
		}
	}
	// Division economy decays downward on average (docs/09 §3).
	avgBalance := func(tier int) float64 {
		var sum float64
		var n int
		for _, c := range w.Clubs {
			if c.DivisionTier == tier {
				sum += float64(c.BalanceMinor)
				n++
			}
		}
		return sum / float64(n)
	}
	if avgBalance(1) <= avgBalance(2) {
		t.Error("division 1 is not richer than division 2 on average")
	}
}

func TestManifestAndQueue(t *testing.T) {
	res := classicWorld(t)
	w := res.World

	if res.Manifest.StartState != "ready" {
		t.Errorf("start state %q, want ready", res.Manifest.StartState)
	}
	if len(res.Manifest.Managers) != len(w.Managers) {
		t.Fatalf("credentials = %d, want one per manager (%d)", len(res.Manifest.Managers), len(w.Managers))
	}
	tokens := map[string]bool{}
	for _, mc := range res.Manifest.Managers {
		if mc.Token == "" || tokens[mc.Token] {
			t.Errorf("manager %d token missing or duplicated", mc.ManagerID)
		}
		tokens[mc.Token] = true
	}

	// 4 calendar events, a finance tick and a youth intake per club,
	// a decision roll per manager, a drift per player, and a kickoff per fixture.
	want := 4 + 2*len(w.Clubs) + len(w.Managers) + len(w.Players) + len(w.Fixtures)
	if res.Queue.Len() != want {
		t.Fatalf("primed queue has %d events, want %d", res.Queue.Len(), want)
	}
}

// TestPoolBandsClamp: a deep Amateur pyramid must clamp, never go negative.
func TestPoolBandsClamp(t *testing.T) {
	cfg := DefaultConfig(1)
	cfg.Divisions, cfg.ClubsPerDivision = 5, 8
	cfg.Quality = QualityAmateur
	d := deriveStructure(cfg)
	for tier, band := range d.DivisionPoolBands {
		if band.Min < poolBandFloorMin || band.Max < band.Min+poolBandMinSpread {
			t.Errorf("tier %d band %+v broken", tier+1, band)
		}
		if tier > 0 && band.Min > d.DivisionPoolBands[tier-1].Min {
			t.Errorf("tier %d band rises above tier %d", tier+1, tier)
		}
	}
}

// TestCongestedCalendar: 24-club divisions need midweek rounds; every round
// must still get a distinct, ascending kickoff slot.
func TestCongestedCalendar(t *testing.T) {
	cfg := DefaultConfig(3)
	cfg.ClubsPerDivision = 24
	d := deriveStructure(cfg)
	if len(d.LeagueRoundTimes) != d.Rounds {
		t.Fatalf("round times = %d, want %d", len(d.LeagueRoundTimes), d.Rounds)
	}
	for i := 1; i < len(d.LeagueRoundTimes); i++ {
		if d.LeagueRoundTimes[i] <= d.LeagueRoundTimes[i-1] {
			t.Fatalf("round times not strictly ascending at %d", i)
		}
	}
	last := int64(d.LeagueRoundTimes[len(d.LeagueRoundTimes)-1]) / sim.MinutesPerDay
	if int(last) > dayLastLeagueRound {
		t.Fatalf("season overruns: last round on day %d", last)
	}
}

// TestSquadSlots: the template scales to any legal target size.
func TestSquadSlots(t *testing.T) {
	for target := 20; target <= 30; target++ {
		slots := buildSquadSlots(target)
		if len(slots) != target {
			t.Fatalf("target %d produced %d slots", target, len(slots))
		}
		gk := 0
		for _, pos := range slots {
			if pos == posGK {
				gk++
			}
		}
		if gk != squadGKCount {
			t.Fatalf("target %d produced %d GKs", target, gk)
		}
	}
}

// TestPoolSpendingIdentity: archetype spending must produce legible
// identities — a Poacher out-finishes, a Stopper out-defends (docs/09 §4.2).
func TestPoolSpendingIdentity(t *testing.T) {
	w := classicWorld(t).World
	var poacherFin, stopperFin, n1, n2 float64
	for i := range w.Players {
		p := &w.Players[i]
		if p.Youth || p.ClubID == 0 {
			continue
		}
		switch p.Archetype {
		case "Poacher":
			poacherFin += float64(p.Visible[attr.Finishing])
			n1++
		case "Stopper":
			stopperFin += float64(p.Visible[attr.Finishing])
			n2++
		}
	}
	if n1 == 0 || n2 == 0 {
		t.Skip("world rolled no poachers or stoppers")
	}
	if math.IsNaN(poacherFin/n1) || poacherFin/n1 <= stopperFin/n2 {
		t.Errorf("poachers (%.1f avg Finishing) don't out-finish stoppers (%.1f)",
			poacherFin/n1, stopperFin/n2)
	}
}
