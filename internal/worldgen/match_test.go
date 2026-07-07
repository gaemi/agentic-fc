package worldgen

import "testing"

// TestLiveMatchSubDerivations locks the derived on-pitch model:
// OnPitch/Participants/SubsUsed are computed from XI + Subs, so an old
// mid-match snapshot with no Subs reads as "starters unchanged", a covered sub
// swaps one body, and an uncovered withdrawal shrinks the side.
func TestLiveMatchSubDerivations(t *testing.T) {
	lm := &LiveMatch{
		HomeID: 1, AwayID: 2,
		HomeXI: []int64{11, 12, 13}, AwayXI: []int64{21, 22, 23},
	}
	// Pre-subs snapshot shape: no Subs at all.
	if got := lm.OnPitch(1); len(got) != 3 {
		t.Fatalf("no-subs on-pitch = %v, want the XI unchanged", got)
	}
	if got := lm.Participants(1); len(got) != 3 {
		t.Fatalf("no-subs participants = %v, want the XI", got)
	}

	lm.Subs = append(lm.Subs, SubEvent{Minute: 50, ClubID: 1, Off: 12, On: 14})
	on := lm.OnPitch(1)
	if len(on) != 3 || on[0] != 11 || on[1] != 13 || on[2] != 14 {
		t.Fatalf("after a sub on-pitch = %v, want [11 13 14]", on)
	}
	if got := lm.Participants(1); len(got) != 4 {
		t.Fatalf("participants = %v, want starters + sub-on", got)
	}
	if lm.SubsUsed(1) != 1 || lm.SubsUsed(2) != 0 {
		t.Fatalf("subs used = %d/%d, want 1/0", lm.SubsUsed(1), lm.SubsUsed(2))
	}

	// The sub-on later goes off injured with nobody left: uncovered withdrawal.
	lm.Subs = append(lm.Subs, SubEvent{Minute: 70, ClubID: 1, Off: 14})
	if got := lm.OnPitch(1); len(got) != 2 {
		t.Fatalf("after an uncovered withdrawal on-pitch = %v, want 2 left", got)
	}
	if lm.SubsUsed(1) != 1 {
		t.Fatal("an uncovered withdrawal must not count as a used sub")
	}
}

// TestArchiveCopyIsDeep locks the ledger's isolation: the
// compact archive form drops ONLY the commentary and shares no backing memory
// with the source — mutating the original after the copy must not reach the
// archive (FinalTables set the precedent; a shallow slice alias would let the
// next season's writes corrupt history).
func TestArchiveCopyIsDeep(t *testing.T) {
	src := MatchResult{
		FixtureID: 7, HomeGoals: 2, AwayGoals: 1,
		HomeXI: []int64{1, 2}, AwayXI: []int64{3, 4},
		Subs:        []SubEvent{{Minute: 60, ClubID: 1, Off: 2, On: 5, Reason: "TACTICAL"}},
		Scorers:     []MatchEvent{{Minute: 30, PlayerID: 1, ClubID: 1}},
		Cards:       []MatchEvent{{Minute: 70, PlayerID: 3, ClubID: 2, Detail: "RED"}},
		RatingsX10:  map[int64]int{1: 74, 3: 60},
		Adjustments: []Adjustment{{Minute: 65, ClubID: 2, Key: "adj.push"}},
		Commentary:  []CommentaryLine{{Minute: 1, Key: "comment.kickoff"}},
		ChanceTypes: map[string]int{"CUTBACK": 2},
		Diagnostics: MatchDiagnostics{
			ShotQuality: map[string]int{"HIGH": 1},
			AerialDuels: map[string]int{"HOME": 2},
			AerialWins:  map[string]int{"HOME": 1},
		},
	}
	arch := src.archiveCopy()
	if arch.Commentary != nil {
		t.Fatal("archive must drop the commentary prose")
	}
	if arch.FixtureID != 7 || arch.HomeGoals != 2 || len(arch.Subs) != 1 ||
		arch.Subs[0].Reason != "TACTICAL" || len(arch.Cards) != 1 || arch.RatingsX10[1] != 74 ||
		arch.ChanceTypes["CUTBACK"] != 2 || arch.Diagnostics.ShotQuality["HIGH"] != 1 ||
		arch.Diagnostics.AerialDuels["HOME"] != 2 || arch.Diagnostics.AerialWins["HOME"] != 1 {
		t.Fatalf("archive lost facts: %+v", arch)
	}
	src.HomeXI[0], src.Subs[0].On, src.Scorers[0].PlayerID = 99, 99, 99
	src.Cards[0].Detail, src.RatingsX10[1], src.Adjustments[0].Key = "YELLOW", 0, "x"
	src.ChanceTypes["CUTBACK"] = 0
	src.Diagnostics.ShotQuality["HIGH"] = 0
	src.Diagnostics.AerialDuels["HOME"] = 0
	if arch.HomeXI[0] == 99 || arch.Subs[0].On == 99 || arch.Scorers[0].PlayerID == 99 ||
		arch.Cards[0].Detail == "YELLOW" || arch.RatingsX10[1] != 74 || arch.Adjustments[0].Key == "x" ||
		arch.ChanceTypes["CUTBACK"] != 2 || arch.Diagnostics.ShotQuality["HIGH"] != 1 ||
		arch.Diagnostics.AerialDuels["HOME"] != 2 {
		t.Fatal("archive aliases the source — deep copy required")
	}
}

// TestOnPitchExcludesSentOff locks red-card ejection: a RED entry
// in the Cards ledger removes its player from the derived on-pitch set — a
// starter and a sub-on alike — with no replacement, while Participants (the
// rated set) keeps them. Yellows eject nobody.
func TestOnPitchExcludesSentOff(t *testing.T) {
	lm := &LiveMatch{
		HomeID: 1, AwayID: 2,
		HomeXI: []int64{11, 12, 13}, AwayXI: []int64{21, 22, 23},
	}
	lm.Cards = append(lm.Cards, MatchEvent{Minute: 30, PlayerID: 12, ClubID: 1, Detail: "YELLOW"})
	if got := lm.OnPitch(1); len(got) != 3 {
		t.Fatalf("a yellow must not eject: on-pitch = %v", got)
	}
	lm.Cards = append(lm.Cards, MatchEvent{Minute: 55, PlayerID: 12, ClubID: 1, Detail: "RED"})
	on := lm.OnPitch(1)
	if len(on) != 2 || on[0] != 11 || on[1] != 13 {
		t.Fatalf("after a red on-pitch = %v, want [11 13]", on)
	}
	if got := lm.Participants(1); len(got) != 3 {
		t.Fatalf("participants = %v — a sent-off starter still played", got)
	}
	if lm.SubsUsed(1) != 0 {
		t.Fatal("an ejection must not consume a substitution")
	}

	// A sub-on who is then sent off leaves too, and the other side is untouched.
	lm.Subs = append(lm.Subs, SubEvent{Minute: 60, ClubID: 2, Off: 21, On: 24, Reason: "TACTICAL"})
	lm.Cards = append(lm.Cards, MatchEvent{Minute: 80, PlayerID: 24, ClubID: 2, Detail: "RED"})
	if got := lm.OnPitch(2); len(got) != 2 {
		t.Fatalf("a sent-off sub-on must leave: on-pitch = %v", got)
	}
	if got := lm.OnPitch(1); len(got) != 2 {
		t.Fatalf("the other side's ejection leaked across: on-pitch = %v", got)
	}
}
