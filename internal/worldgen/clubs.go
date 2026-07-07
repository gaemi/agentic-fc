package worldgen

import "math/rand/v2"

// Stage 1 — world skeleton: world name (if blank) and the region map.
// The calendar and competition structure are pure derivation (derive.go);
// only the named content rolls here (docs/09 §4 stage 1).
//
// The region map is a simple clustered pool: max(2, clubs/4) regions, each
// with a culture drawn from the mix. Clubs draw region → culture → place
// name; regions feed rivalries (docs/09 §6).
func genSkeleton(w *World, r *rand.Rand, names *nameRegistry) {
	if w.Config.Name == "" {
		w.Config.Name = names.claim(func() string { return worldName(r, w.Config.CultureMix) })
	}
	regions := max(2, w.Config.TotalClubs()/4)
	for i := 0; i < regions; i++ {
		culture := pickCulture(r, w.Config.CultureMix)
		w.Regions = append(w.Regions, Region{
			ID:      int64(i + 1),
			Name:    names.claim(func() string { return placeName(r, culture) }),
			Culture: culture,
		})
	}
}

// Capacity bands (initial values): top-division base scaled per tier, by
// world quality, and by the club's Wealth percentile.
const (
	capacityTopDivBase = 22000
	capacityTopDivSpan = 40000 // base + wealth share of span
	capacityTierDecay  = 0.55
	capacityFloor      = 800
)

var capacityQualityFactor = map[Quality]float64{
	QualityAmateur:      0.15,
	QualitySemiPro:      0.4,
	QualityProfessional: 1.0,
	QualityElite:        1.6,
}

// Stage 2 — clubs: identity (region, place, name, colors, stadium) and the
// eight persistent tendencies (docs/09 §4.1). Club IDs are 1-based in
// generation order: tier by tier, then slot.
func genClubs(w *World, r *rand.Rand, names *nameRegistry) {
	// Round-robin region assignment, shuffled, keeps regions evenly loaded.
	total := w.Config.TotalClubs()
	for _, name := range w.Config.NameOverrides.ClubNames {
		names.reserve(name)
	}
	regionOf := make([]int, total)
	for i := range regionOf {
		regionOf[i] = i % len(w.Regions)
	}
	r.Shuffle(total, func(i, j int) { regionOf[i], regionOf[j] = regionOf[j], regionOf[i] })

	usedKits := map[int]map[Colors]bool{} // per tier
	for tier := 1; tier <= w.Config.Divisions; tier++ {
		usedKits[tier] = map[Colors]bool{}
		for slot := 0; slot < w.Config.ClubsPerDivision; slot++ {
			idx := (tier-1)*w.Config.ClubsPerDivision + slot
			region := w.Regions[regionOf[idx]]
			place := names.claim(func() string { return placeName(r, region.Culture) })
			// Always roll and claim the generated club name before applying an
			// override. That keeps stream consumption stable for this config
			// while still letting reserved custom names block generated clones.
			displayName := names.claim(func() string { return clubName(r, region.Culture, place) })
			shortNameSource := place
			if idx < len(w.Config.NameOverrides.ClubNames) {
				displayName = w.Config.NameOverrides.ClubNames[idx]
				shortNameSource = shortNameSourceFromClubOverride(displayName)
			}

			wealth := 1 + r.IntN(20)
			club := Club{
				ID:           int64(idx + 1),
				Name:         displayName,
				ShortName:    shortName(names, shortNameSource),
				Culture:      region.Culture,
				RegionID:     region.ID,
				DivisionTier: tier,
				Colors:       rollKit(r, usedKits[tier]),
				Stadium: Stadium{
					Name:     stadiumName(r, region.Culture, place),
					Capacity: stadiumCapacity(w.Config.Quality, tier, wealth),
				},
				Tendencies: rollTendencies(r, wealth),
			}
			w.Clubs = append(w.Clubs, club)
		}
	}
}

// rollKit picks a primary/secondary pair unused in the division.
func rollKit(r *rand.Rand, used map[Colors]bool) Colors {
	for {
		c := Colors{
			Primary:   kitPalette[r.IntN(len(kitPalette))],
			Secondary: kitPalette[r.IntN(len(kitPalette))],
		}
		if c.Primary == c.Secondary || used[c] {
			continue
		}
		used[c] = true
		return c
	}
}

func stadiumCapacity(q Quality, tier, wealth int) int {
	base := float64(capacityTopDivBase) + float64(capacityTopDivSpan)*float64(wealth-1)/19.0
	base *= capacityQualityFactor[q]
	for t := 1; t < tier; t++ {
		base *= capacityTierDecay
	}
	c := int(base)
	if c < capacityFloor {
		c = capacityFloor
	}
	// Round to something a groundskeeper would quote.
	return c / 100 * 100
}

// rollTendencies rolls the club character. Facilities correlate with Wealth
// (rich clubs train on better pitches); the rest are independent.
func rollTendencies(r *rand.Rand, wealth int) Tendencies {
	facility := func() int {
		return clamp((wealth+1+r.IntN(20))/2+r.IntN(5)-2, 1, 20)
	}
	return Tendencies{
		Wealth:             wealth,
		BoardPatience:      1 + r.IntN(20),
		BoardAmbition:      1 + r.IntN(20),
		FanPatience:        1 + r.IntN(20),
		FanPassion:         1 + r.IntN(20),
		YouthEmphasis:      1 + r.IntN(20),
		TrainingFacilities: facility(),
		YouthFacilities:    facility(),
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
