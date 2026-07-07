package mcpserver

import "testing"

// Drift test: docs/11 §1.2 defines exactly 10 error codes.
func TestErrorCodesGolden(t *testing.T) {
	if len(AllErrorCodes) != 10 {
		t.Fatalf("error codes = %d, docs/11 §1.2 says 10", len(AllErrorCodes))
	}
	seen := map[ErrorCode]bool{}
	for _, c := range AllErrorCodes {
		if seen[c] {
			t.Fatalf("duplicate error code %s", c)
		}
		seen[c] = true
	}
}
