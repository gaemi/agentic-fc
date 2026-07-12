package engine

import (
	"testing"
)

// After a full league matchday, each division files exactly one Team of the
// Week: an 11-row 1-4-3-3 sheet in deterministic order with a named star.
func TestMatchdayFilesTeamOfTheWeek(t *testing.T) {
	e, _ := newEngine(t, 21)
	kickoff := firstKickoff(e)
	if _, err := e.RunUntil(kickoff + day(1)); err != nil {
		t.Fatal(err)
	}

	perDivision := map[int64]int{}
	for _, n := range e.world.News {
		if n.Key != FeedMatchdayTeam {
			continue
		}
		division, ok := newsParamInt64(n.Params["division"])
		if !ok {
			t.Fatalf("totw news lacks a division: %+v", n.Params)
		}
		perDivision[division]++

		rows, ok := n.Params["team"].([]map[string]any)
		if !ok || len(rows) != 11 {
			t.Fatalf("totw team should hold 11 rows: %+v", n.Params["team"])
		}
		bands := map[string]int{}
		for _, row := range rows {
			pos, _ := row["position"].(string)
			switch pos {
			case "GK":
				bands["GK"]++
			case "DR", "DC", "DL":
				bands["DF"]++
			case "DM", "MC", "MR", "ML", "AM":
				bands["MF"]++
			default:
				bands["FW"]++
			}
			if rating, ok := newsParamInt64(row["rating_x10"]); !ok || rating <= 0 {
				t.Fatalf("totw row lacks a rating: %+v", row)
			}
		}
		if bands["GK"] != 1 || bands["DF"] != 4 || bands["MF"] != 3 || bands["FW"] != 3 {
			t.Fatalf("totw sheet shape = %v, want 1-4-3-3", bands)
		}
		if name, _ := n.Params["star"].(string); name == "" {
			t.Fatalf("totw lacks a star: %+v", n.Params)
		}
		if rating, ok := newsParamInt64(n.Params["star_rating_x10"]); !ok || rating <= 0 {
			t.Fatalf("totw star lacks a rating: %+v", n.Params)
		}
		if len(n.ClubIDs) < 12 {
			t.Fatalf("totw news should reference every club in the round, got %d refs", len(n.ClubIDs))
		}
	}
	if len(perDivision) == 0 {
		t.Fatal("no Team of the Week was filed after a full matchday")
	}
	for division, count := range perDivision {
		if count != 1 {
			t.Fatalf("division %d filed %d TOTW articles for one window, want 1", division, count)
		}
	}
}
