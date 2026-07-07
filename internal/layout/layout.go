// Package layout holds the Console's responsive Layout Tier table in one
// place (docs/07 §2/§6): thresholds are the acceptance spec for every
// screen's tier matrix, and tuning them is a one-line change.
package layout

// Tier is the column-driven layout class (docs/07 §2).
type Tier int

const (
	TierXS Tier = iota // not playable: centered size notice only
	TierS              // 60–99 cols: single pane, hotkey tabs
	TierM              // 100–139: main + one side pane
	TierL              // 140–179: three panes
	TierXL             // ≥180: dashboard
)

func (t Tier) String() string {
	switch t {
	case TierS:
		return "S"
	case TierM:
		return "M"
	case TierL:
		return "L"
	case TierXL:
		return "XL"
	}
	return "XS"
}

// RowClass is the row modifier applied within any tier (docs/07 §2).
type RowClass int

const (
	RowsShort     RowClass = iota // 16–27: one-line header, no ticker
	RowsTall                      // 28–41: two-line header + bottom ticker
	RowsExtraTall                 // ≥42: expanded panels
)

// Column and row thresholds (docs/07 §2).
const (
	MinCols    = 60
	MinRows    = 16
	colsTierM  = 100
	colsTierL  = 140
	colsTierXL = 180
	rowsTall   = 28
	rowsExtra  = 42
)

// Compute classifies a terminal size. Rows below the minimum force XS
// regardless of width.
func Compute(cols, rows int) Tier {
	switch {
	case cols < MinCols || rows < MinRows:
		return TierXS
	case cols < colsTierM:
		return TierS
	case cols < colsTierL:
		return TierM
	case cols < colsTierXL:
		return TierL
	default:
		return TierXL
	}
}

// Rows classifies the row modifier.
func Rows(rows int) RowClass {
	switch {
	case rows >= rowsExtra:
		return RowsExtraTall
	case rows >= rowsTall:
		return RowsTall
	default:
		return RowsShort
	}
}
