// Package rng provides the stream-split seeded randomness required by NFR-2:
// each subsystem/entity draws from its own named stream so event ordering
// never perturbs unrelated outcomes (docs/03-simulation-engine.md §5).
package rng

import (
	"crypto/sha256"
	"encoding/binary"
	"math/rand/v2"
)

// Stream derives a deterministic, independent RNG from the world seed and a
// stream label (e.g. "gen/clubs", "player/4521/drift", "match/88031").
func Stream(worldSeed uint64, label string) *rand.Rand {
	h := sha256.New()
	var seed [8]byte
	binary.LittleEndian.PutUint64(seed[:], worldSeed)
	h.Write(seed[:])
	h.Write([]byte(label))
	sum := h.Sum(nil)
	hi := binary.LittleEndian.Uint64(sum[0:8])
	lo := binary.LittleEndian.Uint64(sum[8:16])
	return rand.New(rand.NewPCG(hi, lo))
}
