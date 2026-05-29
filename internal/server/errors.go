package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/elgnas/dbviz/internal/adapter"
	"github.com/elgnas/dbviz/internal/model"
)

// statusForCode maps a canonical error code to an HTTP status (§8.2).
func statusForCode(code model.ErrorCode) int {
	switch code {
	case model.ErrConnNotFound:
		return http.StatusNotFound
	case model.ErrConnAuthFailed, model.ErrConnSuperuserRejected:
		return http.StatusUnauthorized
	case model.ErrConnInvalidDSN, model.ErrExplainInvalidSQL, model.ErrBadRequest:
		return http.StatusBadRequest
	case model.ErrConnUnreachable, model.ErrConnTimeout, model.ErrExplainTimeout:
		return http.StatusGatewayTimeout
	case model.ErrAdapterNotSupported, model.ErrAdapterOperationNotSupported, model.ErrExplainNotSupported:
		return http.StatusNotImplemented
	case model.ErrAdapterReadOnlyViolation, model.ErrSchemaPermissionDenied:
		return http.StatusForbidden
	case model.ErrSchemaTooLarge:
		return http.StatusRequestEntityTooLarge
	case model.ErrRateLimited:
		return http.StatusTooManyRequests
	case model.ErrSchemaNotFound:
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

// writeError serializes any error to the canonical {error, code, details?, hint?}
// envelope with the appropriate HTTP status.
func writeError(w http.ResponseWriter, err error) {
	apiErr := toAPIError(err)
	writeJSON(w, statusForCode(apiErr.Code), apiErr)
}

func toAPIError(err error) *model.APIError {
	var apiErr *model.APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return model.NewAPIError(model.ErrConnTimeout, "request timed out or was cancelled")
	case errors.Is(err, adapter.ErrOperationNotSupported):
		return model.NewAPIError(model.ErrAdapterOperationNotSupported, "operation not supported for this engine")
	case errors.Is(err, adapter.ErrNotSupported):
		return model.NewAPIError(model.ErrAdapterNotSupported, "engine not supported")
	case errors.Is(err, adapter.ErrReadOnlyViolation):
		return model.NewAPIError(model.ErrAdapterReadOnlyViolation, "write attempted on read-only connection")
	default:
		return model.NewAPIError(model.ErrInternal, err.Error())
	}
}
