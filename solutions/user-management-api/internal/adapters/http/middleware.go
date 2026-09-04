package http

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// userIDContextKey is the echo.Context key the auth middleware stores the
// authenticated user's ID under.
const userIDContextKey = "user_id"

// loggingMiddleware logs the HTTP method, path, status, and execution time
// of every request.
//
// Errors returned by downstream handlers are turned into a response here,
// before the status is read. Echo runs its HTTPErrorHandler only after the
// whole middleware chain has unwound, so logging first would record the
// pre-error status (200) for every error response - a 404 would be reported
// as a 200. The path comes from the request URL rather than echo's route
// template, which is empty when no route matched.
func loggingMiddleware(logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			if err := next(c); err != nil {
				c.Error(err)
			}

			logger.InfoContext(c.Request().Context(), "http request",
				"method", c.Request().Method,
				"path", c.Request().URL.Path,
				"status", c.Response().Status,
				"duration", time.Since(start).String(),
			)

			// The error, if any, has already been written to the response by
			// c.Error, so it must not be returned again.
			return nil
		}
	}
}

// authMiddleware requires a valid "Authorization: Bearer <token>" header,
// rejecting the request with 401 before the handler runs otherwise. On
// success it stores the authenticated user's ID on the context.
func authMiddleware(tokenService domain.TokenService) echo.MiddlewareFunc {
	const prefix = "Bearer "

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			if !strings.HasPrefix(header, prefix) {
				return c.JSON(http.StatusUnauthorized, errorResponse{Error: "missing or malformed authorization header"})
			}

			raw := strings.TrimPrefix(header, prefix)
			claims, err := tokenService.ValidateToken(c.Request().Context(), raw)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, errorResponse{Error: "invalid or expired token"})
			}

			c.Set(userIDContextKey, claims.UserID)
			return next(c)
		}
	}
}
