// Command api runs the user management HTTP API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/grpc"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/adapters/bcrypt"
	grpcadapter "github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/adapters/grpc"
	userv1 "github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/adapters/grpc/userv1"
	httpadapter "github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/adapters/http"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/adapters/jwt"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/adapters/mongodb"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/adapters/reporter"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/application"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/config"
)

const (
	mongoConnectTimeout  = 10 * time.Second
	httpShutdownTimeout  = 10 * time.Second
	mongoShutdownTimeout = 5 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, db, err := connectMongo(ctx, cfg)
	if err != nil {
		return err
	}
	defer disconnectMongo(client, logger)

	repo := mongodb.NewUserRepository(db)
	if err := repo.EnsureIndexes(ctx); err != nil {
		return err
	}

	hasher := bcrypt.NewHasher()
	tokenService := jwt.NewTokenService(cfg.JWTSecret, cfg.JWTTTL)

	registerUser := application.NewRegisterUser(repo, hasher)
	authenticateUser := application.NewAuthenticateUser(repo, hasher, tokenService)
	getUser := application.NewGetUser(repo)
	listUsers := application.NewListUsers(repo)
	updateUser := application.NewUpdateUser(repo)
	deleteUser := application.NewDeleteUser(repo)

	server := httpadapter.NewServer(
		registerUser, authenticateUser, getUser, listUsers, updateUser, deleteUser,
		tokenService, logger,
	)

	grpcServer, err := startGRPCServer(ctx, cfg, registerUser, getUser, logger)
	if err != nil {
		return err
	}
	defer grpcServer.GracefulStop()

	userCountReporter := reporter.NewUserCountReporter(repo, cfg.UserCountLogInterval, logger)
	go userCountReporter.Start(ctx)

	serverErrs := make(chan error, 1)
	go func() {
		logger.Info("starting http server", "port", cfg.HTTPPort)
		if err := server.Start(
			":" + cfg.HTTPPort,
		); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			serverErrs <- err
			return
		}
		serverErrs <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErrs:
		// The server stopped on its own (bind failure, etc.) before any
		// shutdown signal, so there is nothing left to gracefully drain.
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}

	return <-serverErrs
}

func startGRPCServer(
	ctx context.Context,
	cfg config.Config,
	registerUser *application.RegisterUser,
	getUser *application.GetUser,
	logger *slog.Logger,
) (*grpc.Server, error) {
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", ":"+cfg.GRPCPort)
	if err != nil {
		return nil, err
	}

	grpcServer := grpc.NewServer()
	userv1.RegisterUserServiceServer(grpcServer, grpcadapter.NewServer(registerUser, getUser))

	go func() {
		logger.Info("starting grpc server", "port", cfg.GRPCPort)
		if err := grpcServer.Serve(listener); err != nil {
			logger.Error("grpc server stopped", "error", err)
		}
	}()

	return grpcServer, nil
}

func connectMongo(ctx context.Context, cfg config.Config) (*mongo.Client, *mongo.Database, error) {
	connectCtx, cancel := context.WithTimeout(ctx, mongoConnectTimeout)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		return nil, nil, err
	}
	if err := client.Ping(connectCtx, nil); err != nil {
		return nil, nil, err
	}

	return client, client.Database(cfg.MongoDatabase), nil
}

func disconnectMongo(client *mongo.Client, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), mongoShutdownTimeout)
	defer cancel()
	if err := client.Disconnect(ctx); err != nil {
		logger.Error("failed to disconnect from mongo", "error", err)
	}
}
