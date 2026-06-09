package logger

import "time"

type Entry struct {
	Level     Level          `json:"level"`
	Channel   string         `json:"channel"`
	Message   string         `json:"message"`
	Context   map[string]any `json:"context,omitempty"`
	Exception string         `json:"exception,omitempty"`
	Time      time.Time      `json:"time"`
}
