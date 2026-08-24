package logger

import (
	"context"
	"fmt"
	"time"
)

// Manager owns the configuration and the built channels. It is safe for
// concurrent use and must be closed to flush buffered and async channels.
type Manager struct {
	cfg      Config
	factory  *channelFactory
	redactor *redactor
	clock    func() time.Time
}

// NewManager validates the configuration and returns a manager. Channels are
// built lazily on first use, so a misconfigured channel that is never used
// costs nothing — but Validate has already rejected it either way.
func NewManager(cfg Config) (*Manager, error) {
	cfg = cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Manager{
		cfg:      cfg,
		factory:  newChannelFactory(cfg),
		redactor: newRedactor(cfg.RedactKeys),
		clock:    time.Now,
	}, nil
}

// Config returns the normalized configuration.
func (m *Manager) Config() Config { return m.cfg }

// Channel returns a logger bound to the named channel. An empty name selects
// the default channel.
func (m *Manager) Channel(name string) (*Logger, error) {
	if name == "" {
		name = m.cfg.Default
	}

	driver, err := m.factory.build(name)
	if err != nil {
		return nil, err
	}

	return &Logger{manager: m, driver: driver, name: name}, nil
}

// MustChannel is Channel for channel names known to be valid at compile time.
// Prefer Channel on any request-handling path: a panic in a logger should
// never take down the request it was describing.
func (m *Manager) MustChannel(name string) *Logger {
	logger, err := m.Channel(name)
	if err != nil {
		panic(fmt.Sprintf("logger: %v", err))
	}
	return logger
}

// Log writes one entry to the named channel.
func (m *Manager) Log(ctx context.Context, channel string, level Level, message string, err error, fields map[string]any) error {
	logger, channelErr := m.Channel(channel)
	if channelErr != nil {
		return channelErr
	}
	return logger.log(ctx, level, message, err, fields)
}

// Sync flushes every buffered and async channel without closing it.
func (m *Manager) Sync() error { return m.factory.sync() }

// Close flushes and releases every channel. The manager is unusable afterwards.
func (m *Manager) Close() error { return m.factory.close() }

// Logger writes entries to one channel, optionally carrying bound context
// fields. It is immutable: With returns a new Logger and never mutates the
// receiver, so a Logger stored in a struct is safe to share across goroutines.
type Logger struct {
	manager *Manager
	driver  Driver
	name    string
	context map[string]any
}

// Name returns the channel this logger writes to.
func (l *Logger) Name() string { return l.name }

// With returns a logger that adds the given key/value pairs to every entry.
// Keys must be strings; a malformed pair is skipped rather than panicking.
func (l *Logger) With(keysAndValues ...any) *Logger {
	fields := parseFields(keysAndValues...)
	if len(fields) == 0 {
		return l
	}

	clone := &Logger{
		manager: l.manager,
		driver:  l.driver,
		name:    l.name,
		context: mergeContext(l.context, fields),
	}
	return clone
}

// WithError returns a logger that attaches err to every entry it writes.
func (l *Logger) WithError(err error) *Logger {
	if err == nil {
		return l
	}
	return l.With("error", err.Error())
}

func (l *Logger) Emergency(message string, keysAndValues ...any) error {
	return l.log(nil, LevelEmergency, message, nil, parseFields(keysAndValues...))
}

func (l *Logger) Alert(message string, keysAndValues ...any) error {
	return l.log(nil, LevelAlert, message, nil, parseFields(keysAndValues...))
}

func (l *Logger) Critical(message string, keysAndValues ...any) error {
	return l.log(nil, LevelCritical, message, nil, parseFields(keysAndValues...))
}

// Error records an error-level entry. err may be nil, in which case only the
// message is recorded.
func (l *Logger) Error(message string, err error, keysAndValues ...any) error {
	return l.log(nil, LevelError, message, err, parseFields(keysAndValues...))
}

func (l *Logger) Warning(message string, keysAndValues ...any) error {
	return l.log(nil, LevelWarning, message, nil, parseFields(keysAndValues...))
}

func (l *Logger) Notice(message string, keysAndValues ...any) error {
	return l.log(nil, LevelNotice, message, nil, parseFields(keysAndValues...))
}

func (l *Logger) Info(message string, keysAndValues ...any) error {
	return l.log(nil, LevelInfo, message, nil, parseFields(keysAndValues...))
}

func (l *Logger) Debug(message string, keysAndValues ...any) error {
	return l.log(nil, LevelDebug, message, nil, parseFields(keysAndValues...))
}

// Log records an entry at an arbitrary level.
func (l *Logger) Log(level Level, message string, keysAndValues ...any) error {
	return l.log(nil, level, message, nil, parseFields(keysAndValues...))
}

func (l *Logger) log(ctx context.Context, level Level, message string, err error, fields map[string]any) error {
	entry := Entry{
		Level:   level,
		Channel: l.name,
		Message: message,
		Context: mergeContext(l.context, fields),
		Time:    l.manager.clock(),
	}

	// Fields carried on the context (request IDs, trace IDs) are merged last so
	// an explicit field at the call site still wins.
	if ctx != nil {
		if carried := fieldsFromContext(ctx); len(carried) > 0 {
			entry.Context = mergeContext(carried, entry.Context)
		}
	}

	if err != nil {
		entry.Exception = err.Error()
	}

	if l.manager.cfg.IncludeCaller {
		entry.Caller = callerFrame(callerSkip)
	}

	l.manager.redactor.apply(entry.Context)

	return l.driver.Log(entry)
}

// parseFields turns a variadic key/value list into a map, skipping pairs whose
// key is not a string and any trailing value without a key.
func parseFields(keysAndValues ...any) map[string]any {
	if len(keysAndValues) == 0 {
		return nil
	}

	fields := make(map[string]any, len(keysAndValues)/2)
	for i := 0; i+1 < len(keysAndValues); i += 2 {
		key, ok := keysAndValues[i].(string)
		if !ok {
			continue
		}
		fields[key] = normalizeValue(keysAndValues[i+1])
	}

	if len(fields) == 0 {
		return nil
	}
	return fields
}

// normalizeValue converts values that do not survive a JSON round trip into
// something the viewer can display.
func normalizeValue(value any) any {
	switch v := value.(type) {
	case error:
		return v.Error()
	case time.Duration:
		return v.String()
	default:
		return value
	}
}

// mergeContext returns base overlaid with extra.
//
// When base is empty it returns extra directly. Every caller passes a map that
// parseFields has just built for this one entry, so there is no shared map to
// protect and skipping the copy removes one allocation from the common case of
// a logger with no bound fields. When base is non-empty a fresh map is always
// built, so a bound context can never be mutated by a later call.
func mergeContext(base, extra map[string]any) map[string]any {
	if len(base) == 0 {
		return extra
	}
	if len(extra) == 0 {
		merged := make(map[string]any, len(base))
		for key, value := range base {
			merged[key] = value
		}
		return merged
	}

	merged := make(map[string]any, len(base)+len(extra))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	return merged
}
