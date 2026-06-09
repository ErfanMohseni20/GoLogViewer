package logger

import (
	"fmt"
	"sync"
	"time"
)

type Manager struct {
	cfg     Config
	factory *channelFactory
	mu      sync.RWMutex
}

func NewManager(cfg Config) (*Manager, error) {
	if cfg.Default == "" {
		cfg.Default = "stack"
	}
	if cfg.Channels == nil {
		cfg = DefaultConfig()
	}

	return &Manager{
		cfg:     cfg,
		factory: newChannelFactory(cfg),
	}, nil
}

func (m *Manager) Config() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

func (m *Manager) Channel(name string) (*Logger, error) {
	if name == "" {
		name = m.cfg.Default
	}

	channel, err := m.factory.build(name)
	if err != nil {
		return nil, err
	}

	return &Logger{
		manager: m,
		channel: channel,
		name:    name,
	}, nil
}

func (m *Manager) Log(channelName string, level Level, message string, err error, fields map[string]any) error {
	logger, logErr := m.Channel(channelName)
	if logErr != nil {
		return logErr
	}
	return logger.log(level, message, err, fields)
}

type Logger struct {
	manager *Manager
	channel Driver
	name    string
	context map[string]any
}

func (l *Logger) With(keysAndValues ...any) *Logger {
	clone := &Logger{
		manager: l.manager,
		channel: l.channel,
		name:    l.name,
		context: copyContext(l.context),
	}

	for key, value := range parseFields(keysAndValues...) {
		clone.context[key] = value
	}

	return clone
}

func (l *Logger) Emergency(message string, keysAndValues ...any) error {
	return l.log(LevelEmergency, message, nil, parseFields(keysAndValues...))
}

func (l *Logger) Alert(message string, keysAndValues ...any) error {
	return l.log(LevelAlert, message, nil, parseFields(keysAndValues...))
}

func (l *Logger) Critical(message string, keysAndValues ...any) error {
	return l.log(LevelCritical, message, nil, parseFields(keysAndValues...))
}

func (l *Logger) Error(message string, err error, keysAndValues ...any) error {
	return l.log(LevelError, message, err, parseFields(keysAndValues...))
}

func (l *Logger) Warning(message string, keysAndValues ...any) error {
	return l.log(LevelWarning, message, nil, parseFields(keysAndValues...))
}

func (l *Logger) Notice(message string, keysAndValues ...any) error {
	return l.log(LevelNotice, message, nil, parseFields(keysAndValues...))
}

func (l *Logger) Info(message string, keysAndValues ...any) error {
	return l.log(LevelInfo, message, nil, parseFields(keysAndValues...))
}

func (l *Logger) Debug(message string, keysAndValues ...any) error {
	return l.log(LevelDebug, message, nil, parseFields(keysAndValues...))
}

func (l *Logger) log(level Level, message string, err error, fields map[string]any) error {
	entry := Entry{
		Level:   level,
		Channel: l.name,
		Message: message,
		Context: mergeContext(l.context, fields),
		Time:    time.Now(),
	}

	if err != nil {
		entry.Exception = err.Error()
	}

	return l.channel.Log(entry)
}

func parseFields(keysAndValues ...any) map[string]any {
	fields := make(map[string]any)
	for i := 0; i+1 < len(keysAndValues); i += 2 {
		key, ok := keysAndValues[i].(string)
		if !ok {
			continue
		}
		fields[key] = keysAndValues[i+1]
	}
	return fields
}

func copyContext(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	return mergeContext(source, nil)
}

func mergeContext(base map[string]any, extra map[string]any) map[string]any {
	if len(base) == 0 && len(extra) == 0 {
		return nil
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

func (m *Manager) MustChannel(name string) *Logger {
	logger, err := m.Channel(name)
	if err != nil {
		panic(fmt.Sprintf("golavelog: %v", err))
	}
	return logger
}
