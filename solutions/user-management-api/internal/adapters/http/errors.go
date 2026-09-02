package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// writeError maps err to an HTTP status and JSON body. Unexpected errors are
// logged with their real detail server-side and returned to the client as a
// generic 500 so internals never leak over the wire.
func writeError(c echo.Context, logger *slog.Logger, err error) error {
	var (
		verr   *validationError
		domErr *domain.ValidationError
	)

	switch {
	case errors.As(err, &verr):
		return c.JSON(http.StatusBadRequest, errorResponse{Error: verr.Error()})
	case errors.As(err, &domErr):
		return c.JSON(http.StatusBadRequest, errorResponse{Error: domErr.Message})
	case errors.Is(err, domain.ErrUserNotFound):
		return c.JSON(http.StatusNotFound, errorResponse{Error: "user not found"})
	case errors.Is(err, domain.ErrEmailAlreadyExists):
		return c.JSON(http.StatusConflict, errorResponse{Error: "email already exists"})
	case errors.Is(err, domain.ErrPasswordTooLong):
		return c.JSON(http.StatusBadRequest, errorResponse{Error: "password must not exceed 72 bytes"})
	case errors.Is(err, domain.ErrInvalidCredentials):
		return c.JSON(http.StatusUnauthorized, errorResponse{Error: "invalid credentials"})
	case errors.Is(err, domain.ErrInvalidToken):
		return c.JSON(http.StatusUnauthorized, errorResponse{Error: "invalid or expired token"})
	default:
		logger.ErrorContext(c.Request().Context(), "unhandled error", "error", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal server error"})
	}
}
