package mcpserver

// ErrorCode is the wire-level error taxonomy (docs/11 §1.2).
type ErrorCode string

const (
	ErrAuthRequired      ErrorCode = "AUTH_REQUIRED"
	ErrInvalidToken      ErrorCode = "INVALID_TOKEN"
	ErrManagerRetired    ErrorCode = "MANAGER_RETIRED"
	ErrInsufficientFocus ErrorCode = "INSUFFICIENT_FOCUS"
	ErrNotFound          ErrorCode = "NOT_FOUND"
	ErrInvalidTarget     ErrorCode = "INVALID_TARGET"
	ErrConflict          ErrorCode = "CONFLICT"
	ErrCapExceeded       ErrorCode = "CAP_EXCEEDED"
	ErrUnemployedScope   ErrorCode = "UNEMPLOYED_SCOPE"
	ErrValidation        ErrorCode = "VALIDATION"
)

// AllErrorCodes exists for drift tests against docs/11 §1.2.
var AllErrorCodes = []ErrorCode{
	ErrAuthRequired, ErrInvalidToken, ErrManagerRetired, ErrInsufficientFocus,
	ErrNotFound, ErrInvalidTarget, ErrConflict, ErrCapExceeded,
	ErrUnemployedScope, ErrValidation,
}
