package logger

import (
	"fmt"
	"os"
	"sync"
)

type StdoutDriver struct {
	name     string
	minLevel Level
	mu       sync.Mutex
}

func NewStdoutDriver(name string, minLevel Level) *StdoutDriver {
	return &StdoutDriver{name: name, minLevel: minLevel}
}

func (s *StdoutDriver) Name() string {
	return s.name
}

func (s *StdoutDriver) Log(entry Entry) error {
	if !entry.Level.Allows(s.minLevel) {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	line := fmt.Sprintf(
		"[%s] %s.%s: %s",
		entry.Time.Format("2006-01-02 15:04:05"),
		entry.Channel,
		entry.Level,
		entry.Message,
	)

	if entry.Exception != "" {
		line += " | " + entry.Exception
	}

	if len(entry.Context) > 0 {
		line += fmt.Sprintf(" %v", entry.Context)
	}

	_, err := fmt.Fprintln(os.Stdout, line)
	return err
}
