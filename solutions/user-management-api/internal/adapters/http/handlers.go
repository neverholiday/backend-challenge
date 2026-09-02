package http

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/application"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

func (s *Server) handleHealthz(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRegister(c echo.Context) error {
	var req registerRequest
	if err := c.Bind(&req); err != nil {
		return writeError(c, s.logger, newValidationError("invalid request body"))
	}

	user, err := s.registerUser.Execute(c.Request().Context(), application.RegisterUserInput{
		Name:     req.Name,
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		return writeError(c, s.logger, err)
	}

	return c.JSON(http.StatusCreated, toUserResponse(*user))
}

func (s *Server) handleLogin(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return writeError(c, s.logger, newValidationError("invalid request body"))
	}
	if err := validateLoginRequest(req); err != nil {
		return writeError(c, s.logger, err)
	}

	token, err := s.authenticateUser.Execute(c.Request().Context(), application.AuthenticateUserInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		return writeError(c, s.logger, err)
	}

	return c.JSON(http.StatusOK, loginResponse{Token: token})
}

func (s *Server) handleListUsers(c echo.Context) error {
	users, err := s.listUsers.Execute(c.Request().Context())
	if err != nil {
		return writeError(c, s.logger, err)
	}

	resp := make([]userResponse, len(users))
	for i, user := range users {
		resp[i] = toUserResponse(user)
	}
	return c.JSON(http.StatusOK, resp)
}

func (s *Server) handleGetUser(c echo.Context) error {
	user, err := s.getUser.Execute(c.Request().Context(), application.GetUserInput{ID: c.Param("id")})
	if err != nil {
		return writeError(c, s.logger, err)
	}
	return c.JSON(http.StatusOK, toUserResponse(*user))
}

func (s *Server) handleUpdateUser(c echo.Context) error {
	id := c.Param("id")

	var req updateUserRequest
	if err := c.Bind(&req); err != nil {
		return writeError(c, s.logger, newValidationError("invalid request body"))
	}

	err := s.updateUser.Execute(c.Request().Context(), application.UpdateUserInput{
		ID: id,
		Param: domain.UserUpdateParam{
			Name:  req.Name,
			Email: req.Email,
		},
	})
	if err != nil {
		return writeError(c, s.logger, err)
	}

	user, err := s.getUser.Execute(c.Request().Context(), application.GetUserInput{ID: id})
	if err != nil {
		return writeError(c, s.logger, err)
	}
	return c.JSON(http.StatusOK, toUserResponse(*user))
}

func (s *Server) handleDeleteUser(c echo.Context) error {
	if err := s.deleteUser.Execute(c.Request().Context(), application.DeleteUserInput{ID: c.Param("id")}); err != nil {
		return writeError(c, s.logger, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func toUserResponse(user domain.User) userResponse {
	return userResponse{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
	}
}
