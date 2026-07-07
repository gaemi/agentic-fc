package worldgen

import "github.com/gaemi/agentic-fc/internal/sim"

// The season calendar is Gregorian-like for familiarity (docs/09 §3) but
// fixed at 365 days — no leap days. The world epoch
// (GameTime 0) is July 1 of season 1, 00:00: worlds generate pre-season.
const (
	DaysPerSeason = 365
)

// monthDays starts at July (index 0) — the season-year runs Jul 1 → Jun 30.
var monthDays = [12]int{31, 31, 30, 31, 30, 31, 31, 28, 31, 30, 31, 30}

// dayOfSeason converts a calendar month (1–12) and day to the 0-based day
// offset from July 1. Months January–June belong to the second half.
func dayOfSeason(month, day int) int {
	idx := month - 7 // July → 0
	if idx < 0 {
		idx += 12 // Jan → 6 … Jun → 11
	}
	d := 0
	for i := 0; i < idx; i++ {
		d += monthDays[i]
	}
	return d + day - 1
}

// gameTimeAt returns the GameTime for a (season, month, day, hh:mm).
// Season is 1-based; season 1 day July 1 00:00 == GameTime 0.
func gameTimeAt(season, month, day, hour, minute int) sim.GameTime {
	days := int64(season-1)*DaysPerSeason + int64(dayOfSeason(month, day))
	return sim.GameTime(days*sim.MinutesPerDay + int64(hour*60+minute))
}

// GameDate is a calendar view of a GameTime (the inverse of gameTimeAt).
type GameDate struct {
	Season int `json:"season"` // 1-based
	Month  int `json:"month"`  // calendar month 1–12
	Day    int `json:"day"`
	Hour   int `json:"hour"`
	Minute int `json:"minute"`
}

// DateOf converts a GameTime to its calendar date for display layers.
func DateOf(t sim.GameTime) GameDate {
	totalDays := int64(t) / sim.MinutesPerDay
	minute := int(int64(t) % sim.MinutesPerDay)
	d := GameDate{
		Season: int(totalDays/DaysPerSeason) + 1,
		Hour:   minute / 60,
		Minute: minute % 60,
	}
	dayInSeason := int(totalDays % DaysPerSeason)
	idx := 0
	for dayInSeason >= monthDays[idx] {
		dayInSeason -= monthDays[idx]
		idx++
	}
	d.Month = idx + 7 // index 0 = July
	if d.Month > 12 {
		d.Month -= 12
	}
	d.Day = dayInSeason + 1
	return d
}

// TransferWindowOpenAt reports whether a transfer window is open at game time t
// The summer window runs from season start (Jul 1) through Aug 31,
// the winter window Jan 1–31. Derived from the calendar — no persisted flag — so
// it is always correct after a snapshot load, at any game time (docs/09 §3).
func TransferWindowOpenAt(t sim.GameTime) bool {
	dayInSeason := int(int64(t) / sim.MinutesPerDay % DaysPerSeason)
	return dayInSeason <= daySummerWindowClose ||
		(dayInSeason >= dayWinterWindowOpen && dayInSeason <= dayWinterWindowClose)
}

// Named calendar anchors (docs/09 §3), as day-of-season offsets.
var (
	daySummerWindowClose = dayOfSeason(8, 31) // Aug 31
	dayWinterWindowOpen  = dayOfSeason(1, 1)  // Jan 1
	dayWinterWindowClose = dayOfSeason(1, 31) // Jan 31
	dayFirstLeagueRound  = dayOfSeason(8, 16) // mid-August kickoff
	dayLastLeagueRound   = dayOfSeason(5, 24) // late May
	dayCongestionStart   = dayOfSeason(12, 1) // midweek insertion window
	dayCongestionEnd     = dayOfSeason(2, 28) //
	dayFirstCupRound     = dayOfSeason(9, 8)  // first cup Tuesday
	dayYouthIntakeStart  = dayOfSeason(3, 1)  // spring intake window
	dayYouthIntakeEnd    = dayOfSeason(4, 30) //
)

// Kickoff times: weekend rounds at 15:00 (the classic 3pm feel, docs/09 §3),
// midweek league and cup rounds at 19:30.
const (
	weekendKickoffMinute = 15 * 60
	midweekKickoffMinute = 19*60 + 30
)

// leagueRoundTimes places R league rounds on the season calendar: weekly
// weekend slots from mid-August, spread evenly across the season when R is
// small, with midweek rounds inserted in the December–February congestion
// window when R outgrows the weekends (docs/09 §3).
func leagueRoundTimes(rounds int) []sim.GameTime {
	var weekends []int
	for d := dayFirstLeagueRound; d <= dayLastLeagueRound; d += 7 {
		weekends = append(weekends, d)
	}
	var days []int
	var midweek map[int]bool
	switch {
	case rounds <= len(weekends):
		// Spread evenly so a short season still runs August → May.
		for i := 0; i < rounds; i++ {
			idx := i * (len(weekends) - 1) / max(rounds-1, 1)
			days = append(days, weekends[idx])
		}
	default:
		days = append(days, weekends...)
		midweek = map[int]bool{}
		deficit := rounds - len(weekends)
		for _, w := range weekends {
			if deficit == 0 {
				break
			}
			if w >= dayCongestionStart && w+4 <= dayCongestionEnd {
				days = append(days, w+4)
				midweek[w+4] = true
				deficit--
			}
		}
	}
	sortInts(days)
	times := make([]sim.GameTime, len(days))
	for i, d := range days {
		kick := weekendKickoffMinute
		if midweek[d] {
			kick = midweekKickoffMinute
		}
		times[i] = sim.GameTime(int64(d)*sim.MinutesPerDay + int64(kick))
	}
	return times
}

// cupRoundTimes places the cup rounds on roughly monthly midweek dates from
// early September, nudged off any league day (docs/09 §3).
func cupRoundTimes(rounds int, leagueDays map[int]bool) []sim.GameTime {
	times := make([]sim.GameTime, rounds)
	for i := 0; i < rounds; i++ {
		d := dayFirstCupRound + i*30
		for leagueDays[d] {
			d++
		}
		times[i] = sim.GameTime(int64(d)*sim.MinutesPerDay + int64(midweekKickoffMinute))
	}
	return times
}

func sortInts(s []int) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
