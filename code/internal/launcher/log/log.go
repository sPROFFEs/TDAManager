// Package log centralises the launcher's logger.
//
// Output goes to a rotating file under the user's config directory so that
// users can attach a log when reporting bugs. The current logger is also
// exposed via Default() for one-shot use.
package log

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AppID must match the launcher's app ID so the log file lives next to the
// state.json the launcher already writes.
const AppID = "devsecops.flow.launcher"

// MaxLogSize is the byte threshold at which the active log is rotated to
// launcher.log.1. Kept tiny because launcher logs are sparse.
const MaxLogSize = 2 * 1024 * 1024 // 2 MiB

var (
	mu     sync.Mutex
	logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	file   *os.File
)

// Init opens (or creates) the log file under the config dir, rotates if
// needed, and installs a slog logger that writes there.
//
// Safe to call once at startup. Subsequent calls are no-ops apart from
// returning the same logger.
func Init() *slog.Logger {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		return logger
	}

	path := logPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		// Fall back to stderr if the config dir is unwritable.
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
		logger.Warn("log dir not writable, falling back to stderr", "err", err.Error())
		return logger
	}
	rotateIfNeeded(path)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
		logger.Warn("could not open log file, falling back to stderr", "path", path, "err", err.Error())
		return logger
	}
	file = f

	handler := slog.NewTextHandler(f, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: false,
	})
	logger = slog.New(handler).With("ts", time.Now().Format(time.RFC3339))

	logger.Info("logger initialised", "path", path)
	return logger
}

// Default returns the package logger. Returns a no-op logger if Init has not
// been called yet, which keeps callers safe at import-time.
func Default() *slog.Logger {
	mu.Lock()
	defer mu.Unlock()
	return logger
}

// Close flushes and closes the log file. Best called from main on shutdown.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Close()
		file = nil
	}
}

// logPath resolves <config-dir>/<AppID>/launcher.log, with a sane fallback
// chain identical to the one used for state.json so both files end up in
// the same directory.
func logPath() string {
	if cfg, err := os.UserConfigDir(); err == nil {
		return filepath.Join(cfg, AppID, "launcher.log")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "."+AppID, "launcher.log")
	}
	return filepath.Join(".", ".launcher.log")
}

// rotateIfNeeded renames the current log file to <path>.1 when it grows
// past MaxLogSize. Older rotated logs are overwritten.
func rotateIfNeeded(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < MaxLogSize {
		return
	}
	rotated := path + ".1"
	_ = os.Remove(rotated)
	if err := os.Rename(path, rotated); err != nil {
		// Non-fatal: failing to rotate just means the file keeps growing.
		fmt.Fprintf(os.Stderr, "log rotate failed: %v\n", err)
	}
}
