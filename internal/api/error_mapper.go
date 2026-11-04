package api

import (
	"errors"
	"net/http"

	"github.com/uloydev/modulaengine/internal/core"
	"github.com/uloydev/modulaengine/internal/query"
)

func MapDomainError(err error) (statusCode int, title string) {
	if err == nil {
		return http.StatusInternalServerError, "Internal Server Error"
	}

	switch {
	case errors.Is(err, core.ErrNotFound):
		return http.StatusNotFound, "Not Found"
	case errors.Is(err, core.ErrVersionConflict):
		return http.StatusConflict, "Optimistic Concurrency Conflict"
	case errors.Is(err, query.ErrFieldNotPermitted):
		return http.StatusForbidden, "Forbidden"
	case errors.Is(err, core.ErrTenantMismatch):
		return http.StatusUnauthorized, "Unauthorized"
	case errors.Is(err, core.ErrValidation),
		errors.Is(err, core.ErrInvalidIdentifier),
		errors.Is(err, core.ErrReservedName),
		errors.Is(err, query.ErrUnknownField),
		errors.Is(err, query.ErrInvalidFilterNode),
		errors.Is(err, query.ErrDepthExceeded),
		errors.Is(err, query.ErrArraySizeExceeded),
		errors.Is(err, query.ErrUnsupportedOp):
		return http.StatusBadRequest, "Validation Error"
	default:
		return http.StatusInternalServerError, "Internal Server Error"
	}
}

func RespondDomainError(w http.ResponseWriter, r *http.Request, err error) {
	statusCode, title := MapDomainError(err)
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	RespondError(w, r, statusCode, title, detail)
}
