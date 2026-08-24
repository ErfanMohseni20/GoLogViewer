package logger

import (
	"context"
	"log/slog"
	"strings"
)

// SlogHandler adapts a channel to log/slog, so any library that accepts a
// *slog.Logger writes into the same files the viewer reads.
//
//	slog.SetDefault(slog.New(logger.NewSlogHandler("")))
type SlogHandler struct {
	logger *Logger
	attrs  map[string]any
	groups []string
}

var _ slog.Handler = (*SlogHandler)(nil)

// NewSlogHandler returns a slog handler writing to the named channel of the
// default manager. An empty name selects the default channel.
func NewSlogHandler(channel string) *SlogHandler {
	return &SlogHandler{logger: Default().MustChannel(channel)}
}

// Handler returns a slog handler writing to this logger's channel.
func (l *Logger) Handler() *SlogHandler {
	return &SlogHandler{logger: l}
}

// Enabled reports whether the level would be recorded. The channel's own
// minimum level is applied by its driver, so this only rejects levels below
// debug.
func (h *SlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelDebug
}

// Handle converts a slog record into an Entry.
func (h *SlogHandler) Handle(ctx context.Context, record slog.Record) error {
	fields := make(map[string]any, len(h.attrs)+record.NumAttrs())
	for key, value := range h.attrs {
		fields[key] = value
	}

	record.Attrs(func(attr slog.Attr) bool {
		collectAttr(fields, h.groups, attr)
		return true
	})

	entry := Entry{
		Level:   levelFromSlog(record.Level),
		Channel: h.logger.name,
		Message: record.Message,
		Context: mergeContext(h.logger.context, fields),
		Time:    record.Time,
	}

	if entry.Time.IsZero() {
		entry.Time = h.logger.manager.clock()
	}

	// slog callers conventionally pass the error as an "error" attribute;
	// surface it as the entry's exception so the viewer highlights it.
	if err, ok := fields["error"]; ok {
		if text, ok := err.(string); ok {
			entry.Exception = text
		}
	}

	if ctx != nil {
		if carried := fieldsFromContext(ctx); len(carried) > 0 {
			entry.Context = mergeContext(carried, entry.Context)
		}
	}

	h.logger.manager.redactor.apply(entry.Context)

	return h.logger.driver.Log(entry)
}

// WithAttrs returns a handler with the attributes permanently attached.
func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}

	merged := make(map[string]any, len(h.attrs)+len(attrs))
	for key, value := range h.attrs {
		merged[key] = value
	}
	for _, attr := range attrs {
		collectAttr(merged, h.groups, attr)
	}

	return &SlogHandler{logger: h.logger, attrs: merged, groups: h.groups}
}

// WithGroup returns a handler that prefixes subsequent attribute keys with
// name, joined by dots.
func (h *SlogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	groups := make([]string, len(h.groups), len(h.groups)+1)
	copy(groups, h.groups)
	groups = append(groups, name)

	return &SlogHandler{logger: h.logger, attrs: h.attrs, groups: groups}
}

// collectAttr flattens a possibly-grouped attribute into dotted keys, which
// keeps the on-disk shape flat and searchable by the viewer.
func collectAttr(into map[string]any, groups []string, attr slog.Attr) {
	value := attr.Value.Resolve()

	if value.Kind() == slog.KindGroup {
		// Copy rather than append in place: groups is shared with the handler
		// this call descends from, and two sibling groups would otherwise
		// overwrite each other's key prefix.
		nested := make([]string, len(groups), len(groups)+1)
		copy(nested, groups)
		if attr.Key != "" {
			nested = append(nested, attr.Key)
		}
		for _, member := range value.Group() {
			collectAttr(into, nested, member)
		}
		return
	}

	if attr.Key == "" {
		return
	}

	key := attr.Key
	if len(groups) > 0 {
		key = strings.Join(groups, ".") + "." + key
	}
	into[key] = normalizeValue(value.Any())
}

// levelFromSlog maps slog's numeric levels onto PSR-3 severities. slog defines
// Debug/Info/Warn/Error at -4/0/4/8 and allows arbitrary values in between.
func levelFromSlog(level slog.Level) Level {
	switch {
	case level >= slog.LevelError+8:
		return LevelEmergency
	case level >= slog.LevelError+6:
		return LevelAlert
	case level >= slog.LevelError+4:
		return LevelCritical
	case level >= slog.LevelError:
		return LevelError
	case level >= slog.LevelWarn:
		return LevelWarning
	case level >= slog.LevelInfo+2:
		return LevelNotice
	case level >= slog.LevelInfo:
		return LevelInfo
	default:
		return LevelDebug
	}
}

// SlogLevel converts a Level back into the nearest slog.Level.
func (l Level) SlogLevel() slog.Level {
	switch l {
	case LevelEmergency:
		return slog.LevelError + 8
	case LevelAlert:
		return slog.LevelError + 6
	case LevelCritical:
		return slog.LevelError + 4
	case LevelError:
		return slog.LevelError
	case LevelWarning:
		return slog.LevelWarn
	case LevelNotice:
		return slog.LevelInfo + 2
	case LevelInfo:
		return slog.LevelInfo
	default:
		return slog.LevelDebug
	}
}
