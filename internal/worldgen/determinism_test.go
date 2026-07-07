package worldgen

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// counterReader is a deterministic token entropy source for tests. It hashes
// a running counter so consecutive 16-byte tokens never repeat.
type counterReader struct{ n uint32 }

func (c *counterReader) Read(p []byte) (int, error) {
	for i := range p {
		c.n++
		p[i] = byte((c.n * 2654435761) >> 24) // Knuth multiplicative hash
	}
	return len(p), nil
}

// TestSameSeedSameHash locks the world-generation invariant (NFR-2):
// same seed + same config ⇒ identical world, identical hash, identical
// primed queue.
func TestSameSeedSameHash(t *testing.T) {
	cfg := PresetClassic(42)
	a, err := Generate(cfg, WithTokenReader(&counterReader{}))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate(cfg, WithTokenReader(&counterReader{}))
	if err != nil {
		t.Fatal(err)
	}

	ha, err := a.World.Hash()
	if err != nil {
		t.Fatal(err)
	}
	hb, err := b.World.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Fatalf("same seed + config produced different hashes:\n%s\n%s", ha, hb)
	}
	if !reflect.DeepEqual(a.World, b.World) {
		t.Fatal("same seed + config produced structurally different worlds")
	}

	// The primed queue must drain in the identical total order.
	if a.Queue.Len() != b.Queue.Len() {
		t.Fatalf("queue lengths differ: %d vs %d", a.Queue.Len(), b.Queue.Len())
	}
	for a.Queue.Len() > 0 {
		ea, eb := a.Queue.Pop(), b.Queue.Pop()
		if *ea != *eb {
			t.Fatalf("queue drain diverged: %+v vs %+v", ea, eb)
		}
	}
}

func TestDifferentSeedDifferentWorld(t *testing.T) {
	a, err := Generate(PresetClassic(1), WithTokenReader(&counterReader{}))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate(PresetClassic(2), WithTokenReader(&counterReader{}))
	if err != nil {
		t.Fatal(err)
	}
	ha, _ := a.World.Hash()
	hb, _ := b.World.Hash()
	if ha == hb {
		t.Fatal("different seeds produced the same world hash")
	}
}

// TestTokensOutsideHash: tokens are credentials, not world state — different
// entropy must change the Manifest but never the World or its hash.
func TestTokensOutsideHash(t *testing.T) {
	cfg := PresetCompact(7)
	a, err := Generate(cfg, WithTokenReader(&counterReader{n: 0}))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate(cfg, WithTokenReader(&counterReader{n: 99}))
	if err != nil {
		t.Fatal(err)
	}
	ha, _ := a.World.Hash()
	hb, _ := b.World.Hash()
	if ha != hb {
		t.Fatal("token entropy leaked into the world hash")
	}
	if a.Manifest.Managers[0].Token == b.Manifest.Managers[0].Token {
		t.Fatal("different entropy produced identical tokens")
	}
}

func TestNameOverrides(t *testing.T) {
	cfg := PresetCompact(7)
	cfg.NameOverrides = NameOverrides{
		ClubNames:    []string{" Agentic FC ", "Codex United"},
		ManagerNames: []string{"Gaemi Kim", "Codex Park", "Claude Lee"},
	}
	res, err := Generate(cfg, WithTokenReader(&counterReader{}))
	if err != nil {
		t.Fatal(err)
	}
	again, err := Generate(cfg, WithTokenReader(&counterReader{}))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.World, again.World) {
		t.Fatal("same seed + name overrides produced structurally different worlds")
	}
	ha, err := res.World.Hash()
	if err != nil {
		t.Fatal(err)
	}
	hb, err := again.World.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Fatalf("same seed + name overrides produced different hashes:\n%s\n%s", ha, hb)
	}
	if got := res.World.Config.NameOverrides.ClubNames[0]; got != "Agentic FC" {
		t.Fatalf("normalized club override = %q", got)
	}
	if res.World.Clubs[0].Name != "Agentic FC" || res.World.Clubs[1].Name != "Codex United" {
		t.Fatalf("club overrides not applied: %q, %q", res.World.Clubs[0].Name, res.World.Clubs[1].Name)
	}
	if res.World.Clubs[0].ShortName != "AGE" || res.World.Clubs[1].ShortName != "COD" {
		t.Fatalf("override short names = %q, %q", res.World.Clubs[0].ShortName, res.World.Clubs[1].ShortName)
	}
	if res.World.Clubs[2].Name == "" || res.World.Clubs[2].Name == "Agentic FC" {
		t.Fatalf("generated club name invalid after overrides: %q", res.World.Clubs[2].Name)
	}
	if res.World.Managers[0].Name != "Gaemi Kim" || res.World.Managers[1].Name != "Codex Park" ||
		res.World.Managers[2].Name != "Claude Lee" {
		t.Fatalf("manager overrides not applied: %q, %q, %q",
			res.World.Managers[0].Name, res.World.Managers[1].Name, res.World.Managers[2].Name)
	}
	if res.Manifest.Managers[0].ManagerName != "Gaemi Kim" {
		t.Fatalf("manifest manager name = %q", res.Manifest.Managers[0].ManagerName)
	}

	boundary := PresetCompact(8)
	boundary.NameOverrides.ManagerNames = make([]string, boundary.TotalClubs()+1)
	for i := range boundary.NameOverrides.ManagerNames {
		boundary.NameOverrides.ManagerNames[i] = fmt.Sprintf("Override Manager %d", i+1)
	}
	boundaryRes, err := Generate(boundary, WithTokenReader(&counterReader{}))
	if err != nil {
		t.Fatal(err)
	}
	if got := boundaryRes.World.Managers[boundary.TotalClubs()].Name; got != "Override Manager 13" {
		t.Fatalf("first unemployed manager override = %q", got)
	}

	managerBase, err := Generate(PresetCompact(9), WithTokenReader(&counterReader{}))
	if err != nil {
		t.Fatal(err)
	}
	managerCollision := PresetCompact(9)
	managerCollision.NameOverrides.ManagerNames = []string{managerBase.World.Managers[1].Name}
	managerCollisionRes, err := Generate(managerCollision, WithTokenReader(&counterReader{}))
	if err != nil {
		t.Fatal(err)
	}
	if got := managerCollisionRes.World.Managers[0].Name; got != managerBase.World.Managers[1].Name {
		t.Fatalf("operator manager override lost priority: %q, want %q", got, managerBase.World.Managers[1].Name)
	}
	seenManagers := map[string]bool{}
	for _, manager := range managerCollisionRes.World.Managers {
		key := strings.ToLower(manager.Name)
		if seenManagers[key] {
			t.Fatalf("case-folded duplicate manager name %q", manager.Name)
		}
		seenManagers[key] = true
	}

	shortNameCollision := PresetCompact(9)
	shortNameCollision.NameOverrides.ClubNames = []string{"Agentic FC", "Agentic City"}
	shortNameCollisionRes, err := Generate(shortNameCollision, WithTokenReader(&counterReader{}))
	if err != nil {
		t.Fatal(err)
	}
	if shortNameCollisionRes.World.Clubs[0].ShortName == shortNameCollisionRes.World.Clubs[1].ShortName {
		t.Fatalf("override short name collision not resolved: %q",
			shortNameCollisionRes.World.Clubs[0].ShortName)
	}

	base, err := Generate(PresetCompact(9), WithTokenReader(&counterReader{}))
	if err != nil {
		t.Fatal(err)
	}
	collision := PresetCompact(9)
	collision.NameOverrides.ClubNames = []string{strings.ToLower(base.World.Clubs[1].Name)}
	collisionRes, err := Generate(collision, WithTokenReader(&counterReader{}))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, club := range collisionRes.World.Clubs {
		key := strings.ToLower(club.Name)
		if seen[key] {
			t.Fatalf("case-folded duplicate club name %q", club.Name)
		}
		seen[key] = true
	}
}

// TestStageStreamsUnique guards the stream-split property: every stage must
// draw from its own label (docs/03 §5).
func TestStageStreamsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range stageStreams {
		if seen[s] {
			t.Fatalf("duplicate stage stream label %q", s)
		}
		seen[s] = true
	}
	if len(stageStreams) != 8 {
		t.Fatalf("stage stream count = %d, docs/09 §4 pipeline has 8 rolled stages", len(stageStreams))
	}
}

func TestGenerateRejectsInvalidConfig(t *testing.T) {
	bad := []func(*WorldConfig){
		func(c *WorldConfig) { c.Divisions = 6 },
		func(c *WorldConfig) { c.ClubsPerDivision = 9 }, // odd
		func(c *WorldConfig) { c.ClubsPerDivision = 26 },
		func(c *WorldConfig) { c.GameSpeed = 10 },
		func(c *WorldConfig) { c.Quality = "LEGENDARY" },
		func(c *WorldConfig) { c.Economy = "BROKE" },
		func(c *WorldConfig) { c.CultureMix = CultureMix{50, 50, 50, 0} },
		func(c *WorldConfig) { c.IdleAcceleration = 1 },
		func(c *WorldConfig) { c.IdleAcceleration = 65 },
		func(c *WorldConfig) { c.OffseasonAccel = 1 },
		func(c *WorldConfig) { c.OffseasonAccel = 241 },
		func(c *WorldConfig) { c.SquadSizeTarget = 19 },
		func(c *WorldConfig) { c.YouthIntakeBatch = 9 },
		func(c *WorldConfig) { c.NameOverrides.ClubNames = []string{"", "A"} },
		func(c *WorldConfig) { c.NameOverrides.ClubNames = []string{"A", "a"} },
		func(c *WorldConfig) { c.NameOverrides.ClubNames = []string{strings.Repeat("A", maxCustomNameLen+1)} },
		func(c *WorldConfig) { c.NameOverrides.ClubNames = []string{"AFC\tGaemi"} },
		func(c *WorldConfig) { c.NameOverrides.ManagerNames = []string{""} },
		func(c *WorldConfig) { c.NameOverrides.ManagerNames = []string{"Boss", "boss"} },
		func(c *WorldConfig) { c.NameOverrides.ManagerNames = []string{strings.Repeat("A", maxCustomNameLen+1)} },
		func(c *WorldConfig) {
			names := make([]string, c.TotalClubs()+unemployedPoolSize(c.TotalClubs())+1)
			for i := range names {
				names[i] = fmt.Sprintf("Manager %d", i+1)
			}
			c.NameOverrides.ManagerNames = names
		},
	}
	for i, mutate := range bad {
		cfg := DefaultConfig(1)
		mutate(&cfg)
		if _, err := Generate(cfg); err == nil {
			t.Errorf("bad config %d accepted", i)
		}
	}
}
