// Package logger provides a Laravel-style channel/driver logging system for
// Go, with structured JSON output, daily and size-based rotation, buffered and
// asynchronous writes, context propagation, and an implementation of
// log/slog.Handler for interoperability with the standard library.
//
// The package-level helpers write to the default channel of a process-wide
// manager, which mirrors Laravel's Log facade:
//
//	logger.Init(logger.DefaultConfig())
//	defer logger.Shutdown()
//
//	logger.Info("user registered", "user_id", 42)
//
// Applications that prefer explicit dependencies can hold a *Manager instead
// and skip the package-level state entirely.
package logger

import (
	"context"
	"sync"
)

var (
	defaultMu  sync.RWMutex
	defaultMgr *Manager
)

// Init replaces the process-wide manager. The previous manager, if any, is
// closed so its buffers are flushed and its descriptors released.
func Init(cfg Config) error {
	manager, err := NewManager(cfg)
	if err != nil {
		return err
	}

	defaultMu.Lock()
	previous := defaultMgr
	defaultMgr = manager
	defaultMu.Unlock()

	if previous != nil {
		_ = previous.Close()
	}
	return nil
}

// SetDefault installs an already-built manager as the process-wide default.
// The caller keeps ownership of the previous manager.
func SetDefault(manager *Manager) *Manager {
	defaultMu.Lock()
	defer defaultMu.Unlock()

	previous := defaultMgr
	defaultMgr = manager
	return previous
}

// Default returns the process-wide manager, creating one from DefaultConfig on
// first use.
func Default() *Manager {
	defaultMu.RLock()
	manager := defaultMgr
	defaultMu.RUnlock()

	if manager != nil {
		return manager
	}

	defaultMu.Lock()
	defer defaultMu.Unlock()

	// Re-check: another goroutine may have initialised it while we waited.
	if defaultMgr != nil {
		return defaultMgr
	}

	manager, err := NewManager(DefaultConfig())
	if err != nil {
		panic("logger: failed to initialize default logger: " + err.Error())
	}
	defaultMgr = manager
	return manager
}

// Shutdown flushes and closes the process-wide manager. Call it from main via
// defer; without it, buffered and asynchronous channels may lose their tail.
func Shutdown() error {
	defaultMu.Lock()
	manager := defaultMgr
	defaultMgr = nil
	defaultMu.Unlock()

	if manager == nil {
		return nil
	}
	return manager.Close()
}

// Sync flushes the process-wide manager without closing it.
func Sync() error { return Default().Sync() }

// CurrentConfig returns the active configuration.
func CurrentConfig() Config { return Default().Config() }

// Channel returns a logger for the named channel, panicking if it is not
// defined. Use Default().Channel for a version that returns an error.
func Channel(name string) *Logger { return Default().MustChannel(name) }

// With returns a default-channel logger carrying the given fields.
func With(keysAndValues ...any) *Logger {
	return Default().MustChannel("").With(keysAndValues...)
}

// Ctx returns a default-channel logger bound to ctx.
func Ctx(ctx context.Context) *CtxLogger {
	return Default().MustChannel("").Ctx(ctx)
}

func Emergency(message string, keysAndValues ...any) error {
	return Default().MustChannel("").Emergency(message, keysAndValues...)
}

func Alert(message string, keysAndValues ...any) error {
	return Default().MustChannel("").Alert(message, keysAndValues...)
}

func Critical(message string, keysAndValues ...any) error {
	return Default().MustChannel("").Critical(message, keysAndValues...)
}

// Error records an error-level entry; err may be nil.
func Error(message string, err error, keysAndValues ...any) error {
	return Default().MustChannel("").Error(message, err, keysAndValues...)
}

func Warning(message string, keysAndValues ...any) error {
	return Default().MustChannel("").Warning(message, keysAndValues...)
}

func Notice(message string, keysAndValues ...any) error {
	return Default().MustChannel("").Notice(message, keysAndValues...)
}

func Info(message string, keysAndValues ...any) error {
	return Default().MustChannel("").Info(message, keysAndValues...)
}

func Debug(message string, keysAndValues ...any) error {
	return Default().MustChannel("").Debug(message, keysAndValues...)
}
