// Package watcher provides a polling-based file change detector.
// It monitors a file's modification time and signals when it changes.
package watcher

import (
	"context"
	"os"
	"time"

	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
)

const (
	// defaultPollInterval is the interval between file stat checks.
	defaultPollInterval = 5 * time.Second
)

// FileWatcher monitors a file for modifications by polling its mod time.
type FileWatcher struct {
	path     string
	logger   *logging.Logger
	interval time.Duration
	lastMod  time.Time
}

// New creates a FileWatcher for the given path.
func New(path string, logger *logging.Logger) *FileWatcher {
	return &FileWatcher{
		path:     path,
		logger:   logger,
		interval: defaultPollInterval,
	}
}

// Init captures the current modification time as the baseline.
// Call this once before starting Watch.
func (w *FileWatcher) Init() error {
	info, err := os.Stat(w.path)
	if err != nil {
		return err
	}
	w.lastMod = info.ModTime()
	return nil
}

// Watch polls the file for changes and sends on the returned channel
// whenever a modification is detected. It blocks until ctx is cancelled.
// The channel is closed when Watch returns.
func (w *FileWatcher) Watch(ctx context.Context) <-chan struct{} {
	ch := make(chan struct{}, 1)

	go func() {
		defer close(ch)
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				info, err := os.Stat(w.path)
				if err != nil {
					w.logger.Debug("Config watcher: unable to stat file: " + err.Error())
					continue
				}
				if info.ModTime().After(w.lastMod) {
					w.lastMod = info.ModTime()
					w.logger.Information("Config file change detected: " + w.path)
					// Non-blocking send — if a change is already queued, skip.
					select {
					case ch <- struct{}{}:
					default:
					}
				}
			}
		}
	}()

	return ch
}

