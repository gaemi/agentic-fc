// Package focus implements the Agent action economy (docs/11-mcp-tools.md §2).
// All values are tunable and registered in docs/98-tunables.md.
package focus

// Economy constants (docs/11 §2).
const (
	Cap              = 100 // FP
	RegenPerGameHour = 2   // halts entirely while the world is paused
	StartingBalance  = Cap
)

// Tool names — the canonical 19-tool surface (docs/11).
type Tool string

const (
	// Free
	GetGuide    Tool = "get_guide"
	GetTime     Tool = "get_time"
	GetSettings Tool = "get_settings"
	GetFocus    Tool = "get_focus"
	GetMindset  Tool = "get_mindset"
	// Observation
	GetSituation  Tool = "get_situation"
	GetNews       Tool = "get_news"
	GetLeague     Tool = "get_league"
	GetClub       Tool = "get_club"
	GetSquad      Tool = "get_squad"
	GetPerson     Tool = "get_person"
	GetMatch      Tool = "get_match"
	SearchPlayers Tool = "search_players"
	// Commission
	Scout Tool = "scout"
	// Shaping
	UpdateDisposition  Tool = "update_disposition"
	SetPriorities      Tool = "set_priorities"
	AddDirective       Tool = "add_directive" // cost scales with strength — see mindset.Strength.FocusCost
	RemoveDirective    Tool = "remove_directive"
	UpdateTacticalPlan Tool = "update_tactical_plan"
)

// flatCosts are the strength- and context-independent costs.
var flatCosts = map[Tool]int{
	GetGuide: 0, GetTime: 0, GetSettings: 0, GetFocus: 0, GetMindset: 0,
	GetSituation: 1, GetNews: 1, GetLeague: 2,
	GetPerson: 4, SearchPlayers: 4, Scout: 12,
	RemoveDirective: 2, SetPriorities: 12,
	UpdateTacticalPlan: 15, UpdateDisposition: 25,
}

// Cost returns the FP price for context-independent tools. Own/other
// asymmetric tools use CostOwnOther; add_directive uses Strength.FocusCost.
func Cost(t Tool) (int, bool) {
	c, ok := flatCosts[t]
	return c, ok
}

// CostOwnOther prices the own-club vs other-club asymmetric tools (docs/11 §2).
func CostOwnOther(t Tool, own bool) (int, bool) {
	switch t {
	case GetClub:
		if own {
			return 2, true
		}
		return 4, true
	case GetSquad:
		if own {
			return 3, true
		}
		return 4, true
	case GetMatch:
		if own {
			return 1, true
		}
		return 3, true
	}
	return 0, false
}
