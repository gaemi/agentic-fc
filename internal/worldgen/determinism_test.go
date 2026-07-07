package worldgen

import (
	"reflect"
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
	}
	for i, mutate := range bad {
		cfg := DefaultConfig(1)
		mutate(&cfg)
		if _, err := Generate(cfg); err == nil {
			t.Errorf("bad config %d accepted", i)
		}
	}
}
