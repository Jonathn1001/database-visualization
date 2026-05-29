package model

import "fmt"

// ErrorCode is a stable, machine-readable error identifier returned to the
// frontend so it can drive specific UX paths (§8).
type ErrorCode string

const (
	// Connection errors.
	ErrConnInvalidDSN        ErrorCode = "CONN_INVALID_DSN"
	ErrConnAuthFailed        ErrorCode = "CONN_AUTH_FAILED"
	ErrConnUnreachable       ErrorCode = "CONN_UNREACHABLE"
	ErrConnSuperuserRejected ErrorCode = "CONN_SUPERUSER_REJECTED"
	ErrConnNotFound          ErrorCode = "CONN_NOT_FOUND"
	ErrConnTimeout           ErrorCode = "CONN_TIMEOUT"
	ErrConnAlreadyClosed     ErrorCode = "CONN_ALREADY_CLOSED"

	// Schema errors.
	ErrSchemaIntrospectFailed ErrorCode = "SCHEMA_INTROSPECT_FAILED"
	ErrSchemaPermissionDenied ErrorCode = "SCHEMA_PERMISSION_DENIED"
	ErrSchemaNotFound         ErrorCode = "SCHEMA_NOT_FOUND"
	ErrSchemaTooLarge         ErrorCode = "SCHEMA_TOO_LARGE"

	// Query / explain errors.
	ErrExplainInvalidSQL   ErrorCode = "EXPLAIN_INVALID_SQL"
	ErrExplainNotSupported ErrorCode = "EXPLAIN_NOT_SUPPORTED"
	ErrExplainTimeout      ErrorCode = "EXPLAIN_TIMEOUT"

	// Adapter errors.
	ErrAdapterNotSupported          ErrorCode = "ADAPTER_NOT_SUPPORTED"
	ErrAdapterOperationNotSupported ErrorCode = "ADAPTER_OPERATION_NOT_SUPPORTED"
	ErrAdapterReadOnlyViolation     ErrorCode = "ADAPTER_READONLY_VIOLATION"

	// System.
	ErrInternal    ErrorCode = "INTERNAL"
	ErrBadRequest  ErrorCode = "BAD_REQUEST"
	ErrRateLimited ErrorCode = "RATE_LIMITED"
)

// APIError is the canonical error value carried across the adapter and HTTP
// layers. It serializes to {error, code, details?, hint?} (§8.1).
type APIError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"error"`
	Details any       `json:"details,omitempty"`
	Hint    string    `json:"hint,omitempty"` // user-facing suggestion
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Hint)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewAPIError constructs an APIError with a code and message.
func NewAPIError(code ErrorCode, message string) *APIError {
	return &APIError{Code: code, Message: message}
}

// WithHint returns a copy of the error with a user-facing hint set.
func (e *APIError) WithHint(hint string) *APIError {
	e.Hint = hint
	return e
}

// WithDetails returns a copy of the error with structured details set.
func (e *APIError) WithDetails(details any) *APIError {
	e.Details = details
	return e
}
