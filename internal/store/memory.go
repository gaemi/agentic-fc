package store

// In-memory log implementations: the engine's default until file-backed
// persistence lands (FR-28), and the workhorse for determinism tests.

// MemAuditLog is an append-only in-memory roll audit trail.
type MemAuditLog struct {
	Entries []RollEntry
}

func (m *MemAuditLog) Append(e RollEntry) error {
	m.Entries = append(m.Entries, e)
	return nil
}

// MemInputLog is an append-only in-memory external-input log.
type MemInputLog struct {
	Entries []InputEntry
}

func (m *MemInputLog) Append(e InputEntry) (uint64, error) {
	e.IngressSeq = uint64(len(m.Entries) + 1)
	m.Entries = append(m.Entries, e)
	return e.IngressSeq, nil
}

func (m *MemInputLog) Seq() uint64 { return uint64(len(m.Entries)) }
