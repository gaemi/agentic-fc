package engine

import (
	"encoding/json"
	"testing"

	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// TestSeasonArchival locks the permanent archive: crossing the
// boundary appends the finished season — final tables per tier + the crowned
// cup winner — to World.History, writes every playing player's season line to
// its career ledger BEFORE the reset erases it, clears the season-scoped form
// ring, and resets the cup-champion marker for the next campaign.
func TestSeasonArchival(t *testing.T) {
	e := newEngineCfg(t, worldgen.DefaultConfig(31))
	if _, err := e.RunUntil(day(364)); err != nil {
		t.Fatal(err)
	}
	var pinned *worldgen.Player
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.SeasonApps > 0 {
			pinned = p
			break
		}
	}
	if pinned == nil {
		t.Fatal("nobody played a match all season")
	}
	apps, goals, sum, club := pinned.SeasonApps, pinned.SeasonGoals, pinned.RatingSumX10, pinned.ClubID
	if e.world.CupChampionID == 0 {
		t.Fatal("test vacuous: no cup champion crowned before the boundary")
	}
	champ := e.world.CupChampionID

	// A player leaving at this same boundary must still archive under the club
	// the season was played for — the archival runs before retirement zeroes
	// ClubID. Force a certain retirement on a second player.
	var leaver *worldgen.Player
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.SeasonApps > 0 && p.ID != pinned.ID && p.ClubID != 0 {
			leaver = p
			break
		}
	}
	leaver.Age = playerRetireAgeCeil + 5
	leaverClub := leaver.ClubID

	if _, err := e.RunUntil(day(366)); err != nil {
		t.Fatal(err)
	}
	if len(e.world.History) != 1 {
		t.Fatalf("history has %d entries after one rollover, want 1", len(e.world.History))
	}
	h := e.world.History[0]
	if h.SeasonYear != 1 || h.CupWinnerID != champ {
		t.Fatalf("archived season=%d cup=%d, want season 1 cup %d", h.SeasonYear, h.CupWinnerID, champ)
	}
	if len(h.FinalTables) != e.world.Config.Divisions {
		t.Fatalf("archived %d tables, want %d divisions", len(h.FinalTables), e.world.Config.Divisions)
	}
	for tier, table := range h.FinalTables {
		if len(table) == 0 || table[0].Pos != 1 {
			t.Fatalf("tier %d archived table is empty or unranked: %+v", tier+1, table)
		}
	}
	if e.world.CupChampionID != 0 {
		t.Fatal("cup-champion marker not cleared for the new season")
	}

	if len(pinned.Career) != 1 {
		t.Fatalf("pinned player career has %d entries, want 1", len(pinned.Career))
	}
	rec := pinned.Career[0]
	if rec.SeasonYear != 1 || rec.Apps != apps || rec.Goals != goals || rec.RatingSumX10 != sum || rec.ClubID != club {
		t.Fatalf("career record %+v != played line {s1 club %d apps %d goals %d sum %d}", rec, club, apps, goals, sum)
	}
	if pinned.FormX10 != nil {
		t.Fatal("form ring must clear at the rollover — form is season-scoped")
	}
	if pinned.SeasonApps != 0 {
		t.Fatal("season stats must still reset after archival")
	}

	if !leaver.Retired {
		t.Fatal("setup: the forced leaver did not retire at the boundary")
	}
	if len(leaver.Career) != 1 || leaver.Career[0].ClubID != leaverClub {
		t.Fatalf("boundary retiree's career = %+v, want the played-for club %d preserved", leaver.Career, leaverClub)
	}
}

// TestSeasonResultLedger locks the full-result archive: the
// rollover folds every finished result into History in compact form — count
// preserved, facts intact, commentary stripped — the new season starts with
// an empty current ledger, ArchivedResultFor finds a past fixture, and the
// whole archive survives a JSON round-trip with an identical world hash (the
// snapshot path is JSON of World).
func TestSeasonResultLedger(t *testing.T) {
	e := newEngineCfg(t, worldgen.DefaultConfig(31))
	if _, err := e.RunUntil(day(364)); err != nil {
		t.Fatal(err)
	}
	played := len(e.world.Results)
	if played == 0 {
		t.Fatal("test vacuous: no results before the boundary")
	}
	pin := e.world.Results[0] // value copy — survives the reset for comparison
	if len(pin.Commentary) == 0 {
		t.Fatal("test premise: a live-season result carries commentary")
	}

	if _, err := e.RunUntil(day(366)); err != nil {
		t.Fatal(err)
	}
	if len(e.world.History) != 1 {
		t.Fatalf("history entries = %d, want 1", len(e.world.History))
	}
	archived := e.world.History[0].Results
	if len(archived) != played {
		t.Fatalf("archived %d results, want all %d played", len(archived), played)
	}
	for i := range archived {
		if archived[i].Commentary != nil {
			t.Fatalf("fixture %d archived WITH commentary — the ledger is the compact form", archived[i].FixtureID)
		}
	}
	got := e.world.ArchivedResultFor(pin.FixtureID)
	if got == nil {
		t.Fatalf("ArchivedResultFor(%d) = nil after archival", pin.FixtureID)
	}
	if got.HomeGoals != pin.HomeGoals || got.AwayGoals != pin.AwayGoals ||
		got.HomeID != pin.HomeID || len(got.HomeXI) != len(pin.HomeXI) ||
		len(got.Scorers) != len(pin.Scorers) || len(got.RatingsX10) != len(pin.RatingsX10) {
		t.Fatalf("archived facts drifted: %+v != %+v", got, pin)
	}
	if e.world.ResultFor(pin.FixtureID) != nil {
		t.Fatal("a past-season fixture leaked into the CURRENT ResultFor")
	}

	before, err := e.world.Hash()
	if err != nil {
		t.Fatal(err)
	}
	blob, err := json.Marshal(e.world)
	if err != nil {
		t.Fatal(err)
	}
	var back worldgen.World
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatal(err)
	}
	after, err := back.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatal("world hash changed across a JSON round-trip — the archive doesn't serialize faithfully")
	}
}

// TestFormRingRolls locks the rolling window: ratings append at each full
// time, the ring never exceeds formWindow, and every entry stays inside the
// rating band.
func TestFormRingRolls(t *testing.T) {
	e, _ := newEngine(t, 42)
	if _, err := e.RunUntil(day(120)); err != nil {
		t.Fatal(err)
	}
	regulars := 0
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if len(p.FormX10) > formWindow {
			t.Fatalf("player %d form ring %d entries, cap %d", p.ID, len(p.FormX10), formWindow)
		}
		for _, v := range p.FormX10 {
			if v < ratingMinX10 || v > ratingMaxX10 {
				t.Fatalf("player %d form entry %d outside the rating band", p.ID, v)
			}
		}
		if p.SeasonApps >= formWindow && len(p.FormX10) == formWindow {
			regulars++
		}
	}
	if regulars == 0 {
		t.Fatal("test vacuous: no regular starter filled the form window in 120 days")
	}
}
