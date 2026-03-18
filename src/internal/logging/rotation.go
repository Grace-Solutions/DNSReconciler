package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultMaxFileSize is the maximum size of a single log file (10 MB).
	DefaultMaxFileSize int64 = 10 * 1024 * 1024
	// DefaultMaxFiles is the maximum number of retained log files.
	DefaultMaxFiles = 3
)

// RotatingFileWriter writes to date-stamped log files with size-based rotation.
// Files are named <basename>.yyyy.mm.dd.log. When a file exceeds MaxSize, the
// current file is closed and a new one is opened. When the total number of log
// files exceeds MaxFiles, the oldest files are removed.
type RotatingFileWriter struct {
	mu       sync.Mutex
	dir      string // directory for log files
	basename string // binary name prefix
	maxSize  int64
	maxFiles int
	file     *os.File
	size     int64
	today    string // current date tag (yyyy.mm.dd)
}

// NewRotatingFileWriter creates a rotating file writer. The dir parameter is the
// directory where log files are stored. The basename is used as the file prefix
// (typically the binary name). Returns an error if the directory cannot be created.
func NewRotatingFileWriter(dir, basename string) (*RotatingFileWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory %q: %w", dir, err)
	}
	w := &RotatingFileWriter{
		dir:      dir,
		basename: basename,
		maxSize:  DefaultMaxFileSize,
		maxFiles: DefaultMaxFiles,
	}
	if err := w.openOrRotate(); err != nil {
		return nil, err
	}
	return w, nil
}

// Write implements io.Writer. Thread-safe.
func (w *RotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	dateTag := time.Now().UTC().Format("2006.01.02")
	if dateTag != w.today || w.size >= w.maxSize {
		if err := w.openOrRotate(); err != nil {
			return 0, err
		}
	}

	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// Close closes the current log file.
func (w *RotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// openOrRotate closes the current file (if any), opens a new date-stamped
// file, and prunes old files beyond maxFiles.
func (w *RotatingFileWriter) openOrRotate() error {
	if w.file != nil {
		_ = w.file.Close()
	}

	w.today = time.Now().UTC().Format("2006.01.02")
	filename := fmt.Sprintf("%s.%s.log", w.basename, w.today)
	path := filepath.Join(w.dir, filename)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file %q: %w", path, err)
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("stat log file %q: %w", path, err)
	}

	w.file = f
	w.size = info.Size()

	// Prune old files
	w.pruneOldFiles()
	return nil
}

// pruneOldFiles removes the oldest log files when total count exceeds maxFiles.
func (w *RotatingFileWriter) pruneOldFiles() {
	pattern := filepath.Join(w.dir, w.basename+".*.log")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) <= w.maxFiles {
		return
	}

	// Sort alphabetically — date format ensures chronological order.
	sort.Strings(matches)

	// Remove from oldest until we're within the limit.
	toRemove := matches[:len(matches)-w.maxFiles]
	for _, path := range toRemove {
		// Safety: only remove files that match our naming pattern.
		base := filepath.Base(path)
		if strings.HasPrefix(base, w.basename+".") && strings.HasSuffix(base, ".log") {
			_ = os.Remove(path)
		}
	}
}

