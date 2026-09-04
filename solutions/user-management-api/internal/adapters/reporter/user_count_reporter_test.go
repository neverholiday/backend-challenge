package reporter_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/adapters/reporter"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// countingRepo is a minimal domain.UserRepository stub that only needs to
// answer CountUsers; every other method panics if called.
type countingRepo struct {
	calls chan struct{}
	count uint
}

func (r *countingRepo) CountUsers(context.Context) (uint, error) {
	select {
	case r.calls <- struct{}{}:
	default:
	}
	return r.count, nil
}

func (r *countingRepo) CreateUser(context.Context, domain.User) error { panic("not used") }
func (r *countingRepo) GetUserByID(context.Context, string) (*domain.User, error) {
	panic("not used")
}
func (r *countingRepo) GetUserByEmail(context.Context, string) (*domain.User, error) {
	panic("not used")
}
func (r *countingRepo) ListUsers(context.Context) ([]domain.User, error) { panic("not used") }
func (r *countingRepo) UpdateUser(context.Context, string, domain.UserUpdateParam) (*domain.User, error) {
	panic("not used")
}
func (r *countingRepo) DeleteUser(context.Context, string) error { panic("not used") }

func TestUserCountReporter_Start(t *testing.T) {
	t.Run("reports on each tick and stops when ctx is cancelled", func(t *testing.T) {
		repo := &countingRepo{calls: make(chan struct{}, 3), count: 7}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		r := reporter.NewUserCountReporter(repo, 10*time.Millisecond, logger)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			r.Start(ctx)
			close(done)
		}()

		select {
		case <-repo.calls:
		case <-time.After(time.Second):
			t.Fatal("Start() did not call CountUsers before timeout")
		}

		cancel()

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Start() did not return after context cancellation")
		}
	})
}
