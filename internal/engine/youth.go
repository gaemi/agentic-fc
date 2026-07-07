package engine

import (
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Youth intake. Each spring, on the club's own YouthIntakeDay,
// a batch of academy prospects joins the club (docs/09 §3, §4.2). The intake is a
// per-club KindClub event — primed at generation for season 1 and re-primed by the
// season rollover for later seasons (Club.YouthIntakeDay is re-rolled each rollover,
// so it can't self-reschedule; it rides the same re-prime path as kickoffs).
//
// The prospects grow through the ordinary player drift and surface to agents via
// scouting as their masked descriptors improve. Until graduation they are excluded
// from match selection and the transfer market; each season boundary ages them
// (player lifecycle), and at youthGraduationAge the boundary pass flips the Youth
// flag — see lifecycle.go. So the academy feeds the senior squad on a yearly
// rhythm, and the intake cap self-heals as graduates leave the academy count.

// NewsYouthIntake announces a club's academy intake (news + Console feed). Count
// only — no per-player data — so it never crosses the FR-22 attribute-masking line.
const NewsYouthIntake = "news.youth.intake"

// handleYouthIntake generates one club's spring academy intake. Deterministic and
// resume-safe: the intake stream is keyed on club + due (rollStream), the new ids
// come from the snapshotted World.NextPlayerID in queue order, and the academy cap
// count is a pure function of replay state — a retempo'd or resumed run reproduces
// the identical intake (NFR-2).
func (e *Engine) handleYouthIntake(ev *sim.Event) error {
	club, ok := e.clubs[ev.EntityID]
	if !ok {
		return e.log(ev, "club", nil, "unknown_club", 0, 0)
	}
	r := e.rollStream(ev) // club/<id>/youth_intake@<due>
	ids := worldgen.GenYouthIntake(e.world, r, club, worldgen.DateOf(ev.Due).Season)
	if len(ids) == 0 {
		// Academy already at capacity — no prospects this spring, no news.
		return e.log(ev, "club", map[string]any{"count": 0}, "academy_full", 0, 0)
	}

	// The append may have reallocated World.Players, invalidating every held
	// *Player — rebuild the index before touching a player pointer.
	e.rebuildPlayerIndex()

	// Each prospect needs a drift roll to develop, staggered across its first week
	// off the same intake stream so an intake day isn't a thundering herd (mirrors
	// primeQueue's player-drift staggering and the newgen decision-roll stagger).
	for _, id := range ids {
		e.scheduleDrift(id, ev.Due+sim.GameTime(r.Int64N(int64(driftIntervalYouthDays)*sim.MinutesPerDay)))
	}

	e.youthNews(ev.Due, club, len(ids), NewsYouthIntake)
	return e.log(ev, "club", map[string]any{"count": len(ids)}, "intake", 0, 0)
}

// scheduleDrift queues a player's next attribute-drift roll. A generated player
// carries one from primeQueue; a runtime-spawned youth (youth intake) gets its first
// one here, from which it self-reschedules like any other player.
func (e *Engine) scheduleDrift(playerID int64, due sim.GameTime) {
	e.queue.Schedule(&sim.Event{
		Due:      due,
		Priority: sim.PriorityDrift,
		Kind:     sim.KindPlayer,
		EntityID: playerID,
		Payload:  worldgen.PayloadPlayerDrift,
	})
}

// youthNews files a count-only academy item (intake or graduation) on the agent
// news ring and the Console feed. A count is public-calendar information and
// carries no maskable attribute data, so — unlike drift, which is feed-only to
// avoid leaking exact values — it is safe as agent news; params are {club, count}
// only (FR-22).
func (e *Engine) youthNews(t sim.GameTime, club *worldgen.Club, count int, key string) {
	params := map[string]any{"club": club.Name, "count": count}
	e.addNews(worldgen.NewsItem{
		GameTime: t, Category: "youth", Key: key, Params: params, ClubIDs: []int64{club.ID},
	})
	e.emit(t, key, cloneParams(params))
}
