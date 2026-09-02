// Package http implements the HTTP adapter: echo handlers, routes, and
// middleware over the application layer's use cases.
package http

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/application"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// Server wires the application's use cases to HTTP routes.
type Server struct {
	echo   *echo.Echo
	logger *slog.Logger

	registerUser     *application.RegisterUser
	authenticateUser *application.AuthenticateUser
	getUser          *application.GetUser
	listUsers        *application.ListUsers
	updateUser       *application.UpdateUser
	deleteUser       *application.DeleteUser
}

// NewServer builds a Server with routes and middleware registered.
func NewServer(
	registerUser *application.RegisterUser,
	authenticateUser *application.AuthenticateUser,
	getUser *application.GetUser,
	listUsers *application.ListUsers,
	updateUser *application.UpdateUser,
	deleteUser *application.DeleteUser,
	tokenService domain.TokenService,
	logger *slog.Logger,
) *Server {
	s := &Server{
		logger:           logger,
		registerUser:     registerUser,
		authenticateUser: authenticateUser,
		getUser:          getUser,
		listUsers:        listUsers,
		updateUser:       updateUser,
		deleteUser:       deleteUser,
	}

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(loggingMiddleware(logger))

	e.GET("/healthz", s.handleHealthz)

	api := e.Group("/api/v1")
	api.POST("/auth/register", s.handleRegister)
	api.POST("/auth/login", s.handleLogin)

	users := api.Group("/users", authMiddleware(tokenService))
	users.GET("", s.handleListUsers)
	users.GET("/:id", s.handleGetUser)
	users.PATCH("/:id", s.handleUpdateUser)
	users.DELETE("/:id", s.handleDeleteUser)

	s.echo = e
	return s
}

// Start begins serving HTTP on addr. It blocks until the server stops.
func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}

// ServeHTTP lets Server be exercised directly against an httptest.Recorder
// without binding a real listener.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.echo.ServeHTTP(w, r)
}

// Shutdown gracefully stops the server, waiting for in-flight requests to
// finish or ctx to expire.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.echo.Shutdown(ctx)
}
