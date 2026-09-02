// Package reporter periodically logs aggregate state about the system.
package reporter

import (
	"context"
	"log/slog"
	"time"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// UserCountReporter periodically logs the total number of registered users.
type UserCountReporter struct {
	repo     domain.UserRepository
	interval time.Duration
	logger   *slog.Logger
}

// NewUserCountReporter builds a reporter that logs the user count every
// interval, using logger for output.
func NewUserCountReporter(repo domain.UserRepository, interval time.Duration, logger *slog.Logger) *UserCountReporter {
	return &UserCountReporter{repo: repo, interval: interval, logger: logger}
}

// Start runs the report loop until ctx is cancelled. It blocks the calling
// goroutine, so callers typically invoke it via `go reporter.Start(ctx)`.
func (r *UserCountReporter) Start(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.report(ctx)
		}
	}
}

func (r *UserCountReporter) report(ctx context.Context) {
	count, err := r.repo.CountUsers(ctx)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to count users", "error", err)
		return
	}
	r.logger.InfoContext(ctx, "user count", "total", count)
}
