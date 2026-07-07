package worldgen

import (
	"strconv"

	"github.com/gaemi/agentic-fc/internal/sim"
)

const (
	AlertResourceTemplate = "agenticfc://manager/{id}/alerts"

	AlertPendingCap        = 512
	AlertFocusPendingCap   = 128
	AlertFocusHysteresisFP = 2
	AlertWatchCap          = 32
)

const (
	AlertKindNews     = "NEWS"
	AlertKindMatch    = "MATCH"
	AlertKindCalendar = "CALENDAR"
	AlertKindFocus    = "FOCUS"
	AlertKindSystem   = "SYSTEM"
)

const (
	AlertEdgeRising  = "rising"
	AlertEdgeFalling = "falling"
)

const (
	AlertReasonOverflow = "OVERFLOW"
)

// AlertWatch is one Manager-scoped wake condition configured through MCP.
type AlertWatch struct {
	ID          int64    `json:"id"`
	Kind        string   `json:"kind"`
	Categories  []string `json:"categories,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	When        string   `json:"when,omitempty"`
	LeadMinutes int      `json:"lead_minutes,omitempty"`
	Threshold   int      `json:"threshold,omitempty"`
	Edge        string   `json:"edge,omitempty"`

	Armed        bool         `json:"armed,omitempty"`
	FiredAlertID int64        `json:"fired_alert_id,omitempty"`
	ScheduledAt  sim.GameTime `json:"scheduled_at,omitempty"`
}

type AlertRef struct {
	News    int64 `json:"news,omitempty"`
	Fixture int64 `json:"fixture,omitempty"`
	Match   int64 `json:"match,omitempty"`
}

type AlertRecord struct {
	ID       int64        `json:"id"`
	GameTime sim.GameTime `json:"game_time"`
	Kind     string       `json:"kind"`
	Reason   string       `json:"reason,omitempty"`
	Refs     []AlertRef   `json:"refs,omitempty"`
	Message  string       `json:"message,omitempty"`
}

type AlertState struct {
	Enabled      bool          `json:"enabled,omitempty"`
	Watches      []AlertWatch  `json:"watches,omitempty"`
	Pending      []AlertRecord `json:"pending,omitempty"`
	NextID       int64         `json:"next_id,omitempty"`
	AckedThrough int64         `json:"acked_through,omitempty"`
	Version      int64         `json:"version,omitempty"`
	OverflowID   int64         `json:"overflow_id,omitempty"`
	NextWatchID  int64         `json:"next_watch_id,omitempty"`
	Initialized  bool          `json:"initialized,omitempty"`
}

// AlertsState returns the manager's alert state, lazily initialising it for
// snapshots created before Agent Alerts existed. Callers must hold the world
// write lock or be running on the single-writer engine goroutine.
func (m *Manager) AlertsState() *AlertState {
	if m.Alerts == nil {
		m.Alerts = &AlertState{Enabled: true, Initialized: true}
	}
	if !m.Alerts.Initialized {
		m.Alerts.Enabled = true
		m.Alerts.Initialized = true
	}
	return m.Alerts
}

func AlertResourceURI(managerID int64) string {
	return "agenticfc://manager/" + strconv.FormatInt(managerID, 10) + "/alerts"
}

func (s *AlertState) IssueAlert(at sim.GameTime, kind, reason, message string, refs []AlertRef) AlertRecord {
	if !s.Enabled {
		return AlertRecord{}
	}
	if kind == AlertKindFocus && s.focusPending() >= AlertFocusPendingCap {
		s.evictOldestKind(AlertKindFocus)
	}
	if len(s.Pending) >= AlertPendingCap {
		s.evictForOverflow(at)
	}
	s.NextID++
	rec := AlertRecord{ID: s.NextID, GameTime: at, Kind: kind, Reason: reason, Message: message, Refs: refs}
	s.Pending = append(s.Pending, rec)
	return rec
}

func (s *AlertState) Ack(through int64) int64 {
	if through <= s.AckedThrough {
		return s.AckedThrough
	}
	s.AckedThrough = through
	out := s.Pending[:0]
	for _, a := range s.Pending {
		if a.ID > through {
			out = append(out, a)
		}
	}
	s.Pending = out
	if s.OverflowID != 0 && through >= s.OverflowID {
		s.OverflowID = 0
	}
	for i := range s.Watches {
		if s.Watches[i].FiredAlertID != 0 && through >= s.Watches[i].FiredAlertID {
			s.Watches[i].FiredAlertID = 0
		}
	}
	return s.AckedThrough
}

func (s *AlertState) focusPending() int {
	n := 0
	for _, a := range s.Pending {
		if a.Kind == AlertKindFocus {
			n++
		}
	}
	return n
}

func (s *AlertState) evictOldestKind(kind string) {
	for i, a := range s.Pending {
		if a.Kind == kind {
			s.Pending = append(s.Pending[:i], s.Pending[i+1:]...)
			return
		}
	}
}

func (s *AlertState) evictForOverflow(at sim.GameTime) {
	if s.OverflowID == 0 {
		for len(s.Pending) >= AlertPendingCap-1 {
			if !s.evictOldestNonOverflow() {
				break
			}
		}
		s.NextID++
		rec := AlertRecord{
			ID:       s.NextID,
			GameTime: at,
			Kind:     AlertKindSystem,
			Reason:   AlertReasonOverflow,
			Message:  "Alert queue overflowed. Use broad observation tools to catch up.",
		}
		s.OverflowID = rec.ID
		s.Pending = append(s.Pending, rec)
		return
	}
	for len(s.Pending) >= AlertPendingCap {
		if !s.evictOldestNonOverflow() {
			break
		}
	}
}

func (s *AlertState) evictOldestNonOverflow() bool {
	for i, a := range s.Pending {
		if a.ID != s.OverflowID {
			s.Pending = append(s.Pending[:i], s.Pending[i+1:]...)
			return true
		}
	}
	return false
}
