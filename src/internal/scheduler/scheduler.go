// Package scheduler implements the interval-based reconciliation loop (§25).
package scheduler

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
)

// ReconcileFunc is the function signature the scheduler calls on each tick.
type ReconcileFunc func(ctx context.Context) error

// Config holds scheduler configuration derived from runtime config.
type Config struct {
	IntervalSeconds int
	JitterPercent   int // 0-100, percentage of interval used as jitter
}

// Scheduler runs the reconciliation loop on a fixed interval with jitter (§25).
type Scheduler struct {
	logger    *logging.Logger
	config    Config
	reconcile ReconcileFunc
}

// New creates a Scheduler.
func New(logger *logging.Logger, cfg Config, fn ReconcileFunc) *Scheduler {
	if cfg.IntervalSeconds <= 0 {
		cfg.IntervalSeconds = 120
	}
	if cfg.JitterPercent <= 0 {
		cfg.JitterPercent = 10
	}
	return &Scheduler{logger: logger, config: cfg, reconcile: fn}
}

// Run executes the reconciliation loop. It runs once immediately at startup,
// then repeatedly on interval+jitter until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	// §25: run at startup
	s.logger.Information("Scheduler: running initial reconciliation")
	if err := s.reconcile(ctx); err != nil {
		s.logger.Error(fmt.Sprintf("Scheduler: initial reconciliation error: %s", err))
	}

	for {
		delay := s.nextDelay()
		s.logger.Debug(fmt.Sprintf("Scheduler: next reconciliation in %s", delay))

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

// nextDelay calculates interval + random jitter to avoid startup stampedes.
func (s *Scheduler) nextDelay() time.Duration {
	base := time.Duration(s.config.IntervalSeconds) * time.Second
	maxJitter := base * time.Duration(s.config.JitterPercent) / 100
	if maxJitter <= 0 {
		return base
	}
	jitter := time.Duration(rand.Int63n(int64(maxJitter)))
	return base + jitter
}

