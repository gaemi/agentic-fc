// Package narrative is the sole text-producing layer (FR-35): it turns feed
// events into human-readable lines via message-key templates (the i18n seam,
// NFR-5), and emits cadence metadata so clients reproduce classic CM's
// tension pacing (FR-35a, docs/90 L1).
package narrative

// Message is structured data + key + rendered text.
type Message struct {
	Key    string         `json:"key"` // e.g. "news.transfer.completed"
	Params map[string]any `json:"params,omitempty"`
	Text   string         `json:"text"` // rendered in the requester's locale (en/ko at v1), English fallback — FR-35c
}

// Cadence tells a client how to pace one commentary line: dramatic moments
// linger, routine passages compress.
type Cadence struct {
	DisplayMillis int    `json:"display_ms"` // suggested on-screen duration at 1× reading speed
	Density       string `json:"density"`    // ROUTINE | BUILDING | DRAMATIC
}

// Line is one rendered commentary/news line with its pacing hint.
type Line struct {
	Message Message `json:"message"`
	Cadence Cadence `json:"cadence"`
}

// Renderer converts simulation events into Lines. Template catalogs and
// variety budgeting (NFR-8) can be layered on top of this renderer.
type Renderer interface {
	// TODO: Render(event) []Line — defined alongside the event feed types.
}
