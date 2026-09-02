// Package grpc implements the gRPC adapter: a UserService server backed by
// the same application use cases as the HTTP adapter.
package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	userv1 "github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/adapters/grpc/userv1"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/application"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// Server implements userv1.UserServiceServer over RegisterUser and GetUser.
type Server struct {
	userv1.UnimplementedUserServiceServer

	registerUser *application.RegisterUser
	getUser      *application.GetUser
}

// NewServer builds a Server.
func NewServer(registerUser *application.RegisterUser, getUser *application.GetUser) *Server {
	return &Server{registerUser: registerUser, getUser: getUser}
}

// CreateUser registers a new user.
func (s *Server) CreateUser(ctx context.Context, req *userv1.CreateUserRequest) (*userv1.User, error) {
	user, err := s.registerUser.Execute(ctx, application.RegisterUserInput{
		Name:     req.GetName(),
		Email:    req.GetEmail(),
		Password: req.GetPassword(),
	})
	if err != nil {
		return nil, mapError(err)
	}

	return toProto(*user), nil
}

// GetUser fetches a user by id.
func (s *Server) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.User, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	user, err := s.getUser.Execute(ctx, application.GetUserInput{ID: req.GetId()})
	if err != nil {
		return nil, mapError(err)
	}

	return toProto(*user), nil
}

func mapError(err error) error {
	var validationErr *domain.ValidationError

	switch {
	case errors.As(err, &validationErr):
		return status.Error(codes.InvalidArgument, validationErr.Message)
	case errors.Is(err, domain.ErrUserNotFound):
		return status.Error(codes.NotFound, "user not found")
	case errors.Is(err, domain.ErrEmailAlreadyExists):
		return status.Error(codes.AlreadyExists, "email already exists")
	case errors.Is(err, domain.ErrPasswordTooLong):
		return status.Error(codes.InvalidArgument, "password must not exceed 72 bytes")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

func toProto(user domain.User) *userv1.User {
	return &userv1.User{
		Id:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		CreatedAt: timestamppb.New(user.CreatedAt),
	}
}
