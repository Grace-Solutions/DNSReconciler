// Package scheduler implements the cron-based reconciliation loop (§25).
package scheduler

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
	"github.com/robfig/cron/v3"
)

// ReconcileFunc is the function signature the scheduler calls on each tick.
type ReconcileFunc func(ctx context.Context) error

// Config holds scheduler configuration derived from runtime config.
type Config struct {
	// Schedule is a 6-field cron expression (seconds enabled).
	// Example: "0 0 */4 * * *" = every 4 hours at :00.
	Schedule string

	// Jitter controls randomised delay added after each cron fire time.
	// "auto"     — 10% of the interval to the next fire (default).
	// "disabled" — no jitter.
	// "<value>"  — a Go duration string (e.g. "30s", "5m").
	Jitter string

	// Location is the timezone for cron evaluation. Nil defaults to UTC.
	Location *time.Location
}

// Scheduler runs the reconciliation loop on a cron schedule with optional jitter.
type Scheduler struct {
	logger    *logging.Logger
	config    Config
	schedule  cron.Schedule
	reconcile ReconcileFunc
}

// New creates a Scheduler. Returns an error if the cron expression is invalid.
func New(logger *logging.Logger, cfg Config, fn ReconcileFunc) (*Scheduler, error) {
	if cfg.Schedule == "" {
		cfg.Schedule = "0 0 */4 * * *"
	}
	if cfg.Jitter == "" {
		cfg.Jitter = "auto"
	}
	if cfg.Location == nil {
		cfg.Location = time.UTC
	}

	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(cfg.Schedule)
	if err != nil {
		return nil, fmt.Errorf("invalid schedule %q: %w", cfg.Schedule, err)
	}

	return &Scheduler{
		logger:    logger,
		config:    cfg,
		schedule:  sched,
		reconcile: fn,
	}, nil
}

// Run executes the reconciliation loop. It runs once immediately at startup,
// then repeatedly on the cron schedule until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	// §25: run at startup
	s.logger.Information("Scheduler: running initial reconciliation")
	if err := s.reconcile(ctx); err != nil {
		s.logger.Error(fmt.Sprintf("Scheduler: initial reconciliation error: %s", err))
	}

	for {
		delay := s.nextDelay()

		select {
		case <-ctx.Done():
			s.logger.Information("Scheduler: shutdown signal received")
			return ctx.Err()
		case <-time.After(delay):
			s.logger.Information("Scheduler: running scheduled reconciliation")
			if err := s.reconcile(ctx); err != nil {
				s.logger.Error(fmt.Sprintf("Scheduler: reconciliation error: %s", err))
			}
		}
	}
}

// nextDelay calculates the duration until the next cron fire time plus jitter,
// and logs the next run time and countdown in human-readable form.
func (s *Scheduler) nextDelay() time.Duration {
	now := time.Now().In(s.config.Location)
	nextFire := s.schedule.Next(now)
	baseDelay := nextFire.Sub(now)

	jitter := s.computeJitter(baseDelay)
	totalDelay := baseDelay + jitter

	nextRun := now.Add(totalDelay)
	s.logger.Information(fmt.Sprintf("Scheduler: next reconciliation at %s (%s)",
		nextRun.Format("Monday, 15:04:05"),
		formatDuration(totalDelay)))

	return totalDelay
}

// computeJitter returns a randomised duration based on the jitter config.
func (s *Scheduler) computeJitter(baseDelay time.Duration) time.Duration {
	j := strings.TrimSpace(strings.ToLower(s.config.Jitter))
	switch j {
	case "disabled":
		return 0
	case "auto":
		maxJitter := baseDelay / 10
		if maxJitter <= 0 {
			return 0
		}
		return time.Duration(rand.Int63n(int64(maxJitter)))
	default:
		d, err := time.ParseDuration(j)
		if err != nil || d <= 0 {
			return 0
		}
		return time.Duration(rand.Int63n(int64(d)))
	}
}

// ResolveTimezone loads a *time.Location from a timezone string.
// "UTC" returns time.UTC, "auto" reads from TZ env var or /etc/timezone,
// anything else is treated as a Unix timezone name (e.g. "America/New_York").
func ResolveTimezone(tz string) (*time.Location, error) {
	switch strings.TrimSpace(strings.ToLower(tz)) {
	case "", "utc":
		return time.UTC, nil
	case "auto":
		return detectSystemTimezone()
	default:
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return nil, fmt.Errorf("unknown timezone %q: %w", tz, err)
		}
		return loc, nil
	}
}

// detectSystemTimezone reads the system timezone from the TZ environment
// variable or /etc/timezone (common in Docker containers).
func detectSystemTimezone() (*time.Location, error) {
	if tz := os.Getenv("TZ"); tz != "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return nil, fmt.Errorf("TZ env var %q is not a valid timezone: %w", tz, err)
		}
		return loc, nil
	}

	data, err := os.ReadFile("/etc/timezone")
	if err == nil {
		name := strings.TrimSpace(string(data))
		if name != "" {
			loc, err := time.LoadLocation(name)
			if err == nil {
				return loc, nil
			}
		}
	}

	// Fall back to UTC if system timezone cannot be determined
	return time.UTC, nil
}

// formatDuration formats a duration as "X days, Y hours, Z minutes, W seconds".
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Seconds())
	days := total / 86400
	hours := (total % 86400) / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	return fmt.Sprintf("%d days, %d hours, %d minutes, %d seconds", days, hours, minutes, seconds)
}

