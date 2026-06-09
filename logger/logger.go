package logger

import "sync"

var (
	initMu     sync.Mutex
	defaultMgr *Manager
)

func Init(cfg Config) error {
	initMu.Lock()
	defer initMu.Unlock()

	manager, err := NewManager(cfg)
	if err != nil {
		return err
	}

	defaultMgr = manager
	return nil
}

func ensureDefault() {
	initMu.Lock()
	defer initMu.Unlock()

	if defaultMgr != nil {
		return
	}

	manager, err := NewManager(DefaultConfig())
	if err != nil {
		panic("golavelog: failed to initialize default logger")
	}
	defaultMgr = manager
}

func Default() *Manager {
	ensureDefault()
	return defaultMgr
}

func CurrentConfig() Config {
	return Default().Config()
}

func Channel(name string) *Logger {
	return Default().MustChannel(name)
}

func With(keysAndValues ...any) *Logger {
	return Default().MustChannel("").With(keysAndValues...)
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
