package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

const focusAlertPayloadPrefix = worldgen.PayloadFocusAlert + ":"
const calendarAlertPayloadPrefix = worldgen.PayloadCalendarAlert + ":"

type AlertSink interface {
	OnManagerAlert(managerID int64)
}

func (e *Engine) SetAlertSink(s AlertSink) { e.alertSink = s }

func (e *Engine) addNews(n worldgen.NewsItem) worldgen.NewsItem {
	item := e.world.AddNews(n)
	e.issueNewsAlerts(item)
	return item
}

func (e *Engine) issueNewsAlerts(n worldgen.NewsItem) {
	for i := range e.world.Managers {
		m := &e.world.Managers[i]
		if m.Status == worldgen.ManagerRetired {
			continue
		}
		st := m.AlertsState()
		if !st.Enabled {
			continue
		}
		for _, w := range st.Watches {
			if w.Kind != worldgen.AlertKindNews || !e.alertNewsVisible(n, m, w.Scope) ||
				!alertCategoryMatches(w.Categories, n.Category) {
				continue
			}
			rec := st.IssueAlert(n.GameTime, worldgen.AlertKindNews, n.Category,
				"News matched an Agent Alert watch. Use get_news for detail.",
				[]worldgen.AlertRef{{News: n.ID}})
			if rec.ID != 0 {
				e.notifyManagerAlert(m.ID)
			}
			break
		}
	}
}

func alertCategoryMatches(categories []string, got string) bool {
	if len(categories) == 0 {
		return true
	}
	for _, c := range categories {
		if c == got {
			return true
		}
	}
	return false
}

func (e *Engine) alertNewsVisible(n worldgen.NewsItem, m *worldgen.Manager, scope string) bool {
	if scope == "" {
		scope = "own"
	}
	if n.ManagerID != 0 {
		// Match get_news privacy: manager-private items only wake the personal
		// feed, not the public world/league breadth scopes.
		return n.ManagerID == m.ID && scope != "world" && scope != "league"
	}
	if len(n.ClubIDs) == 0 {
		return true
	}
	switch scope {
	case "world":
		return true
	case "league":
		tier := 0
		if c, ok := e.clubs[m.ClubID]; ok {
			tier = c.DivisionTier
		}
		for _, id := range n.ClubIDs {
			if c, ok := e.clubs[id]; ok && c.DivisionTier == tier {
				return true
			}
		}
		return false
	default:
		for _, id := range n.ClubIDs {
			if id == m.ClubID && id != 0 {
				return true
			}
		}
		return false
	}
}

func (e *Engine) RescheduleManagerAlerts(managerID int64) {
	m := e.managers[managerID]
	if m == nil || m.Status == worldgen.ManagerRetired {
		return
	}
	worldgen.SyncManagerFocus(m, e.now)
	st := m.AlertsState()
	if !st.Enabled {
		return
	}
	for i := range st.Watches {
		w := &st.Watches[i]
		if w.Kind == worldgen.AlertKindCalendar && w.When == "DATE_CHANGED" {
			e.scheduleDateChangedAlert(m, st, w)
			continue
		}
		if w.Kind != worldgen.AlertKindFocus {
			continue
		}
		e.reconcileFocusWatch(m, w)
		if !w.Armed || w.FiredAlertID != 0 || w.Edge == worldgen.AlertEdgeFalling {
			continue
		}
		if m.FocusBalance >= w.Threshold {
			e.fireFocusWatch(m, w, e.now)
			continue
		}
		due := m.FocusRegenMark + sim.GameTime((w.Threshold-m.FocusBalance)*worldgen.FocusMinutesPerFP)
		if due <= e.now {
			due = e.now
		}
		if w.ScheduledAt != 0 && due >= w.ScheduledAt {
			continue
		}
		w.ScheduledAt = due
		e.queue.Schedule(&sim.Event{
			Due:      due,
			Priority: sim.PriorityDecision,
			Kind:     sim.KindManager,
			EntityID: m.ID,
			Payload:  fmt.Sprintf("%s%d:%d:%d", focusAlertPayloadPrefix, st.Version, w.ID, due),
		})
	}
}

func (e *Engine) scheduleDateChangedAlert(m *worldgen.Manager, st *worldgen.AlertState, w *worldgen.AlertWatch) {
	nextDay := e.now - e.now%sim.MinutesPerDay + sim.MinutesPerDay
	if nextDay <= e.now {
		nextDay += sim.MinutesPerDay
	}
	if w.ScheduledAt == nextDay {
		return
	}
	w.ScheduledAt = nextDay
	e.queue.Schedule(&sim.Event{
		Due:      nextDay,
		Priority: sim.PriorityWorld,
		Kind:     sim.KindManager,
		EntityID: m.ID,
		Payload:  fmt.Sprintf("%s%d:%d:%d", calendarAlertPayloadPrefix, st.Version, w.ID, nextDay),
	})
}

func (e *Engine) handleFocusAlert(ev *sim.Event, payload string) error {
	parts := strings.Split(strings.TrimPrefix(payload, focusAlertPayloadPrefix), ":")
	if len(parts) != 3 {
		return e.log(ev, "alert", nil, "stale", 0, 0)
	}
	version, _ := strconv.ParseInt(parts[0], 10, 64)
	watchID, _ := strconv.ParseInt(parts[1], 10, 64)
	due, _ := strconv.ParseInt(parts[2], 10, 64)
	m := e.managers[ev.EntityID]
	if m == nil || m.Status == worldgen.ManagerRetired {
		return e.log(ev, "alert", nil, "missing_manager", 0, 0)
	}
	worldgen.SyncManagerFocus(m, ev.Due)
	st := m.AlertsState()
	if !st.Enabled || st.Version != version {
		return e.log(ev, "alert", nil, "stale", 0, 0)
	}
	for i := range st.Watches {
		w := &st.Watches[i]
		if w.ID != watchID || w.Kind != worldgen.AlertKindFocus || w.ScheduledAt != sim.GameTime(due) {
			continue
		}
		if w.Armed && w.FiredAlertID == 0 && w.Edge == worldgen.AlertEdgeRising && m.FocusBalance >= w.Threshold {
			e.fireFocusWatch(m, w, ev.Due)
			return e.log(ev, "alert", map[string]any{"manager_id": m.ID}, "focus_alert", 0, 0)
		}
		w.ScheduledAt = 0
		e.RescheduleManagerAlerts(m.ID)
		return e.log(ev, "alert", nil, "not_ready", 0, 0)
	}
	return e.log(ev, "alert", nil, "stale", 0, 0)
}

func (e *Engine) handleCalendarAlert(ev *sim.Event, payload string) error {
	parts := strings.Split(strings.TrimPrefix(payload, calendarAlertPayloadPrefix), ":")
	if len(parts) != 3 {
		return e.log(ev, "alert", nil, "stale", 0, 0)
	}
	version, _ := strconv.ParseInt(parts[0], 10, 64)
	watchID, _ := strconv.ParseInt(parts[1], 10, 64)
	due, _ := strconv.ParseInt(parts[2], 10, 64)
	m := e.managers[ev.EntityID]
	if m == nil || m.Status == worldgen.ManagerRetired {
		return e.log(ev, "alert", nil, "missing_manager", 0, 0)
	}
	st := m.AlertsState()
	if !st.Enabled || st.Version != version {
		return e.log(ev, "alert", nil, "stale", 0, 0)
	}
	for i := range st.Watches {
		w := &st.Watches[i]
		if w.ID != watchID || w.Kind != worldgen.AlertKindCalendar ||
			w.When != "DATE_CHANGED" || w.ScheduledAt != sim.GameTime(due) {
			continue
		}
		rec := st.IssueAlert(ev.Due, worldgen.AlertKindCalendar, "DATE_CHANGED",
			"Game date changed. Use get_time or get_situation for detail.", nil)
		if rec.ID != 0 {
			e.notifyManagerAlert(m.ID)
		}
		w.ScheduledAt = 0
		e.scheduleDateChangedAlert(m, st, w)
		return e.log(ev, "alert", map[string]any{"manager_id": m.ID}, "date_changed", 0, 0)
	}
	return e.log(ev, "alert", nil, "stale", 0, 0)
}

func (e *Engine) issueMatchAlerts(at sim.GameTime, f *worldgen.Fixture, when string) {
	for i := range e.world.Managers {
		m := &e.world.Managers[i]
		if m.Status == worldgen.ManagerRetired || m.ClubID == 0 ||
			(m.ClubID != f.HomeID && m.ClubID != f.AwayID) {
			continue
		}
		st := m.AlertsState()
		if !st.Enabled {
			continue
		}
		for _, w := range st.Watches {
			if w.Kind != worldgen.AlertKindMatch || w.When != when {
				continue
			}
			rec := st.IssueAlert(at, worldgen.AlertKindMatch, when,
				"Match alert matched. Use get_match for detail.",
				[]worldgen.AlertRef{{Fixture: f.ID}})
			if rec.ID != 0 {
				e.notifyManagerAlert(m.ID)
			}
			break
		}
	}
}

func (e *Engine) issueCalendarAlerts(at sim.GameTime, when string) {
	for i := range e.world.Managers {
		m := &e.world.Managers[i]
		if m.Status == worldgen.ManagerRetired {
			continue
		}
		st := m.AlertsState()
		if !st.Enabled {
			continue
		}
		for _, w := range st.Watches {
			if w.Kind != worldgen.AlertKindCalendar || w.When != when {
				continue
			}
			rec := st.IssueAlert(at, worldgen.AlertKindCalendar, when,
				"Calendar alert matched. Use get_time or get_situation for detail.", nil)
			if rec.ID != 0 {
				e.notifyManagerAlert(m.ID)
			}
			break
		}
	}
}

func (e *Engine) reconcileFocusWatch(m *worldgen.Manager, w *worldgen.AlertWatch) {
	if w.Edge == "" {
		w.Edge = worldgen.AlertEdgeRising
	}
	if w.FiredAlertID != 0 || w.Armed {
		if w.Armed && w.Edge == worldgen.AlertEdgeFalling && m.FocusBalance <= w.Threshold {
			e.fireFocusWatch(m, w, e.now)
		}
		return
	}
	switch w.Edge {
	case worldgen.AlertEdgeFalling:
		if m.FocusBalance >= w.Threshold+worldgen.AlertFocusHysteresisFP {
			w.Armed = true
		}
	default:
		if m.FocusBalance <= w.Threshold-worldgen.AlertFocusHysteresisFP {
			w.Armed = true
		}
	}
}

func (e *Engine) fireFocusWatch(m *worldgen.Manager, w *worldgen.AlertWatch, at sim.GameTime) {
	st := m.AlertsState()
	rec := st.IssueAlert(at, worldgen.AlertKindFocus, w.Edge,
		"Focus threshold matched an Agent Alert watch.", nil)
	if rec.ID == 0 {
		return
	}
	w.Armed = false
	w.FiredAlertID = rec.ID
	w.ScheduledAt = 0
	e.notifyManagerAlert(m.ID)
}

func (e *Engine) notifyManagerAlert(managerID int64) {
	if e.alertSink != nil {
		e.alertSink.OnManagerAlert(managerID)
	}
}
