package mcpserver

import (
	"fmt"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Attribute masking (FR-22a, docs/11 §1.4): own-squad players read exact;
// everyone else reads bucketed ranges that narrow with scouting knowledge.
// Buckets are QUANTIZED — the true value sits anywhere inside, never at the
// center, so a range leaks less than value±w would.
//
//	level 0 (unscouted): bucket of 5   → e.g. 11–15
//	level 1:             bucket of 3   → e.g. 13–15
//	level 2:             bucket of 2   → e.g. 13–14
//	level 3:             exact
var knowledgeBuckets = [4]int{5, 3, 2, 1}

// effectiveLevel is a viewer's knowledge fidelity for a player: full (3) for
// their own squad, else the accumulated scouting level (0–3). This is the
// single source of truth for every masked read (attributes, value band).
func effectiveLevel(viewer *worldgen.Manager, w *worldgen.World, p *worldgen.Player) int {
	if viewer.ClubID != 0 && p.ClubID == viewer.ClubID {
		return 3
	}
	return w.KnowledgeLevel(viewer.ID, p.ID)
}

// maskedVisible renders a player's visible attributes for a viewer: exact
// ints for the viewer's own squad or full knowledge, [lo,hi] ranges else.
func maskedVisible(viewer *worldgen.Manager, w *worldgen.World, p *worldgen.Player) map[string]any {
	level := effectiveLevel(viewer, w, p)
	out := make(map[string]any, len(p.Visible))
	for a, v := range p.Visible {
		if level >= 3 {
			out[string(a)] = v
			continue
		}
		lo, hi := bucketRange(v, knowledgeBuckets[level])
		out[string(a)] = []int{lo, hi}
	}
	return out
}

func bucketRange(v, size int) (lo, hi int) {
	lo = ((v-attr.ScaleMin)/size)*size + attr.ScaleMin
	hi = lo + size - 1
	if hi > attr.ScaleMax {
		hi = attr.ScaleMax
	}
	return lo, hi
}

// knowsPersonality: personality descriptors read at own-squad fidelity or
// scouting level ≥ 2 (docs/11 §4 get_person "per knowledge").
func knowsPersonality(viewer *worldgen.Manager, w *worldgen.World, p *worldgen.Player) bool {
	if viewer.ClubID != 0 && p.ClubID == viewer.ClubID {
		return true
	}
	return w.KnowledgeLevel(viewer.ID, p.ID) >= 2
}

// ---- Money rendering (docs/11 §1.4) ----

// crDisplay renders Crowns minor units: "cr2.4M", "cr180k", "cr350".
func crDisplay(minor int64) string {
	crowns := minor / 100
	switch {
	case crowns >= 1_000_000:
		return trimZero(fmt.Sprintf("cr%.1fM", float64(crowns)/1_000_000))
	case crowns >= 1_000:
		return fmt.Sprintf("cr%dk", crowns/1_000)
	default:
		return fmt.Sprintf("cr%d", crowns)
	}
}

func trimZero(s string) string {
	if len(s) > 3 && s[len(s)-3:] == ".0M" {
		return s[:len(s)-3] + "M"
	}
	return s
}

func money(minor int64) map[string]any {
	return map[string]any{"amount": minor, "display": crDisplay(minor)}
}

// Market value estimation (public valuation, coarse by design): quadratic
// in pool, quoted as a ±25% band. Initial values, registered in docs/98.
const valuePerPoolSquaredMinor = 5000

// bucketedPool floors an Ability Pool to the viewer's knowledge bucket (the
// same 5/3/2/1 ladder as attribute masking). The topmost bucket absorbs the
// overflow against the domain cap so it can never collapse to a single
// identifying value below full knowledge: with PoolMax=200 and size 5, a naive
// floor would put pool 200 alone in [200,204]; instead it joins [195,200].
// At full knowledge (own squad / level 3, size 1) there is no clamp — the pool
// resolves exactly, as earned.
func bucketedPool(pool, level int) int {
	size := knowledgeBuckets[level]
	lo := (pool / size) * size
	if size > 1 && lo > attr.PoolMax-size {
		lo = attr.PoolMax - size
	}
	return lo
}

// valueBand estimates a player's market value from Ability Pool. The band's
// edges are pure functions of pool, so an exact pool would be recoverable as
// sqrt(low / k) — a hidden quantity across the wire (FR-22). To prevent that,
// the pool is first quantized via bucketedPool: the band then pins pool only
// to a bucket for scouted-little players, and resolves to exact at full
// knowledge (own squad / level 3), where the exact pool is already earned.
func valueBand(pool, level int) map[string]any {
	lo := bucketedPool(pool, level)
	hi := lo + knowledgeBuckets[level] - 1
	low := int64(lo) * int64(lo) * valuePerPoolSquaredMinor * 3 / 4
	high := int64(hi) * int64(hi) * valuePerPoolSquaredMinor * 5 / 4
	return map[string]any{
		"low":  money(low),
		"high": money(high),
	}
}

// ---- Descriptor bands (hidden → observable, FR-22a) ----

// confidenceBand / securityBand map a club's live board confidence (0–100) to a
// Descriptor — the only form the number crosses the wire in (FR-22a, board confidence).
func confidenceBand(confidence int) string {
	switch {
	case confidence >= 70:
		return "HIGH"
	case confidence >= 45:
		return "MODERATE"
	default:
		return "LOW"
	}
}

func securityBand(confidence int) string {
	switch {
	case confidence >= 70:
		return "SECURE"
	case confidence >= 45:
		return "STABLE"
	default:
		return "UNDER_PRESSURE"
	}
}

func facilityBand(v int) string {
	switch {
	case v >= 16:
		return "EXCELLENT"
	case v >= 11:
		return "GOOD"
	case v >= 6:
		return "ADEQUATE"
	default:
		return "POOR"
	}
}
