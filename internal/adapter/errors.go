package adapter

import "errors"

// Sentinel errors returned by adapters. Handlers map these (and *model.APIError
// values) to HTTP statuses (§8.2).
var (
	// ErrNotSupported indicates the engine itself is not implemented.
	ErrNotSupported = errors.New("adapter: engine not supported")
	// ErrOperationNotSupported indicates the engine is implemented but this
	// particular operation is not (e.g. EXPLAIN on a NoSQL store).
	ErrOperationNotSupported = errors.New("adapter: operation not supported")
	// ErrReadOnlyViolation indicates a write was attempted through a read-only
	// adapter.
	ErrReadOnlyViolation = errors.New("adapter: read-only violation")
	// ErrNotOpen indicates a method was called before Open or after Close.
	ErrNotOpen = errors.New("adapter: connection not open")
)
