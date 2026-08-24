package logger

import "time"

// Entry is a single log record. It is the unit written to disk (one JSON
// object per line) and the unit returned by the reader to the viewer.
type Entry struct {
	Level     Level          `json:"level"`
	Channel   string         `json:"channel"`
	Message   string         `json:"message"`
	Context   map[string]any `json:"context,omitempty"`
	Exception string         `json:"exception,omitempty"`
	Stack     string         `json:"stack,omitempty"`
	Caller    string         `json:"caller,omitempty"`
	Time      time.Time      `json:"time"`

	// Raw holds the original line when it could not be decoded as a structured
	// entry. The reader sets it so the viewer can still display plain-text logs
	// such as those produced by Laravel itself.
	Raw string `json:"raw,omitempty"`
}

// clone returns a copy safe to hand to a driver that may retain or mutate it.
func (e Entry) clone() Entry {
	if e.Context != nil {
		ctx := make(map[string]any, len(e.Context))
		for k, v := range e.Context {
			ctx[k] = v
		}
		e.Context = ctx
	}
	return e
}
