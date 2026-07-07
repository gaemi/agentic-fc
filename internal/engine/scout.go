package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/rng"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// The scout commission (docs/11 §5): the documented exception to "no tool
// performs an in-world act" — an asynchronous process that mutates nothing
// but the commissioning Agent's own knowledge. Duration ~1–2 game-weeks;
// quality scales with scout staff Judgement once staff generation exists —
// until then a registered baseline stands in.
const (
	scoutDurationMinDays  = 7
	scoutDurationSpanDays = 7
	// scoutJudgementBaseline stands in for scout staff Judgement (1–20)
	// until staff are generated (docs/08 §6 simplification).
	scoutJudgementBaseline = 12

	scoutPayloadPrefix = "scout_report:"
)

// ScheduleScout enqueues a scouting completion for a manager. targetSpec is
// "p<playerID>" or "profile:<position>". The duration rolls from a stream
// labeled with the commissioning call's game time — stateless and
// replay-exact like every other roll (NFR-2).
func (e *Engine) ScheduleScout(managerID int64, targetSpec string, now sim.GameTime) sim.GameTime {
	r := rng.Stream(e.world.Config.Seed,
		fmt.Sprintf("scout/%d/%s@%d", managerID, targetSpec, int64(now)))
	due := now +
		sim.GameTime(int64(scoutDurationMinDays)*sim.MinutesPerDay) +
		sim.GameTime(r.Int64N(int64(scoutDurationSpanDays)*sim.MinutesPerDay))
	e.queue.Schedule(&sim.Event{
		Due:      due,
		Priority: sim.PriorityDecision,
		Kind:     sim.KindManager,
		EntityID: managerID,
		Payload:  scoutPayloadPrefix + targetSpec,
	})
	return due
}

func (e *Engine) handleScoutReport(ev *sim.Event, payload string) error {
	m, ok := e.managers[ev.EntityID]
	if !ok {
		return e.log(ev, "scout", nil, "unknown_manager", 0, 0)
	}
	spec := strings.TrimPrefix(payload, scoutPayloadPrefix)
	p := e.resolveScoutTarget(m, spec)
	if p == nil || p.Retired {
		// A direct target who retired while the scout was travelling reads as
		// gone — the report is void, not filed on an ended career (the gateway
		// rejects retired targets up front; this covers the in-flight race).
		return e.log(ev, "scout", map[string]any{"spec": spec}, "target_gone", 0, 0)
	}

	k := e.world.KnowledgeFor(m.ID, p.ID)
	if k.Level < 3 {
		k.Level++
	}
	mergeEvidence(k, scoutEvidence(p, k.Level, ev.Due))

	// The report lands as a PRIVATE news item — knowledge belongs to the
	// commissioning Agent alone (FR-22a).
	e.addNews(worldgen.NewsItem{
		GameTime:  ev.Due,
		Category:  "media",
		Key:       "news.scout.report",
		Params:    map[string]any{"player": p.Name, "level": k.Level},
		ManagerID: m.ID,
	})
	return e.log(ev, "scout", map[string]any{
		"player": p.ID, "level": k.Level,
	}, "report_filed", 0, m.Mindset.Version)
}

// resolveScoutTarget finds the subject: a direct player ref, or for profile
// scouting the strongest not-yet-scouted player in that position outside
// the manager's own club (deterministic: pool desc, then id asc).
func (e *Engine) resolveScoutTarget(m *worldgen.Manager, spec string) *worldgen.Player {
	if id, ok := strings.CutPrefix(spec, "p"); ok {
		if pid, err := strconv.ParseInt(id, 10, 64); err == nil {
			return e.players[pid]
		}
		return nil
	}
	pos, ok := strings.CutPrefix(spec, "profile:")
	if !ok {
		return nil
	}
	var best *worldgen.Player
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.Position != pos || p.ClubID == m.ClubID || p.Retired {
			continue
		}
		if e.world.KnowledgeLevel(m.ID, p.ID) > 0 {
			continue
		}
		if best == nil || p.AbilityPool > best.AbilityPool ||
			(p.AbilityPool == best.AbilityPool && p.ID < best.ID) {
			best = p
		}
	}
	return best
}

// scoutEvidence derives impression lines from hidden attributes as PROSE
// phrases keyed per (attribute, direction) — never the raw attribute name
// or value (FR-22, docs/08 §5). It surfaces the single most extreme high
// and low hidden attributes; the prose itself carries the valence, so no
// strength/weakness framing is needed. Confidence rises with knowledge.
func scoutEvidence(p *worldgen.Player, level int, at sim.GameTime) []worldgen.Evidence {
	scanned := []attr.Hidden{
		attr.Professionalism, attr.Ambition, attr.Loyalty, attr.Temperament,
		attr.Pressure, attr.Adaptability, attr.Sociability, attr.Influence,
		attr.Consistency, attr.BigMatchNerve, attr.InjuryProne, attr.Recovery,
		attr.Discipline, attr.Versatility,
	}
	hiA, loA := scanned[0], scanned[0]
	for _, a := range scanned {
		if p.Hidden[a] > p.Hidden[hiA] {
			hiA = a
		}
		if p.Hidden[a] < p.Hidden[loA] {
			loA = a
		}
	}
	confidence := "LOW"
	switch {
	case level >= 3 && scoutJudgementBaseline >= 10:
		confidence = "HIGH"
	case level >= 2:
		confidence = "MEDIUM"
	}
	return []worldgen.Evidence{
		{Key: evidenceKey(hiA, true), Confidence: confidence, GameTime: at},
		{Key: evidenceKey(loA, false), Confidence: confidence, GameTime: at},
	}
}

// mergeEvidence folds fresh impressions into a knowledge record so that
// re-scouting *deepens* rather than duplicates (docs/11 §5): an impression
// already on file has its confidence and date refreshed in place; a genuinely
// new one is appended. Without this, every re-scout would stack another
// identical prose line at a higher confidence.
func mergeEvidence(k *worldgen.Knowledge, fresh []worldgen.Evidence) {
	for _, f := range fresh {
		replaced := false
		for i := range k.Evidence {
			if k.Evidence[i].Key == f.Key {
				k.Evidence[i] = f
				replaced = true
				break
			}
		}
		if !replaced {
			k.Evidence = append(k.Evidence, f)
		}
	}
}

// evidenceKey maps a hidden attribute + direction to its prose catalog key,
// e.g. (CONSISTENCY, false) → "evidence.consistency.low". The lowercase key
// is a stable identifier; the rendered text is behavioral prose, and no
// attribute value ever accompanies it.
func evidenceKey(a attr.Hidden, high bool) string {
	dir := "low"
	if high {
		dir = "high"
	}
	return "evidence." + strings.ToLower(string(a)) + "." + dir
}
