package rng

import "testing"

func TestStreamDeterminism(t *testing.T) {
	a := Stream(42, "player/1/drift")
	b := Stream(42, "player/1/drift")
	for i := 0; i < 100; i++ {
		if a.Uint64() != b.Uint64() {
			t.Fatal("same seed+label must produce identical streams")
		}
	}
}

func TestStreamIndependence(t *testing.T) {
	a := Stream(42, "player/1/drift")
	b := Stream(42, "player/2/drift")
	same := 0
	for i := 0; i < 64; i++ {
		if a.Uint64() == b.Uint64() {
			same++
		}
	}
	if same > 2 {
		t.Fatalf("streams with different labels look correlated (%d/64 equal)", same)
	}
}
