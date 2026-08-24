package logger

import (
	"errors"
	"fmt"
	"strings"
)

// Level is a PSR-3 / RFC 5424 severity. Higher values are more severe, so a
// simple integer comparison decides whether an entry passes a channel's
// minimum level. The zero value means "unset" and is treated as LevelDebug by
// the channel factory.
type Level int8

const (
	levelUnset Level = iota
	LevelDebug
	LevelInfo
	LevelNotice
	LevelWarning
	LevelError
	LevelCritical
	LevelAlert
	LevelEmergency
)

// ErrInvalidLevel is returned by ParseLevel for unrecognised severity names.
var ErrInvalidLevel = errors.New("logger: invalid level")

var levelNames = [...]string{
	levelUnset:     "",
	LevelDebug:     "debug",
	LevelInfo:      "info",
	LevelNotice:    "notice",
	LevelWarning:   "warning",
	LevelError:     "error",
	LevelCritical:  "critical",
	LevelAlert:     "alert",
	LevelEmergency: "emergency",
}

// String returns the lowercase PSR-3 name of the level.
func (l Level) String() string {
	if l < 0 || int(l) >= len(levelNames) {
		return "unknown"
	}
	return levelNames[l]
}

// Valid reports whether l is one of the eight defined severities.
func (l Level) Valid() bool {
	return l >= LevelDebug && l <= LevelEmergency
}

// Allows reports whether an entry at level l should be emitted by a channel
// whose minimum severity is min.
func (l Level) Allows(min Level) bool {
	return l >= min
}

// MarshalJSON encodes the level as its PSR-3 name so log files stay readable
// and compatible with the v1 on-disk format.
func (l Level) MarshalJSON() ([]byte, error) {
	return []byte(`"` + l.String() + `"`), nil
}

// UnmarshalJSON accepts either a PSR-3 name or a numeric severity.
func (l *Level) UnmarshalJSON(data []byte) error {
	if len(data) >= 2 && data[0] == '"' {
		parsed, err := ParseLevel(string(data[1 : len(data)-1]))
		if err != nil {
			return err
		}
		*l = parsed
		return nil
	}

	var n int8
	if _, err := fmt.Sscanf(string(data), "%d", &n); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidLevel, data)
	}
	*l = Level(n)
	return nil
}

// ParseLevel converts a severity name into a Level. Unlike the v1 helper it
// reports invalid input instead of silently falling back to debug.
func ParseLevel(value string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return LevelDebug, nil
	case "info", "information":
		return LevelInfo, nil
	case "notice":
		return LevelNotice, nil
	case "warning", "warn":
		return LevelWarning, nil
	case "error", "err":
		return LevelError, nil
	case "critical", "crit", "fatal":
		return LevelCritical, nil
	case "alert":
		return LevelAlert, nil
	case "emergency", "emerg", "panic":
		return LevelEmergency, nil
	default:
		return levelUnset, fmt.Errorf("%w: %q", ErrInvalidLevel, value)
	}
}

// MustParseLevel is ParseLevel for constant input; it panics on invalid names.
func MustParseLevel(value string) Level {
	level, err := ParseLevel(value)
	if err != nil {
		panic(err)
	}
	return level
}

// AllLevels returns the severities ordered from most to least severe, which is
// the order the viewer's filter dropdown presents them in.
func AllLevels() []Level {
	return []Level{
		LevelEmergency,
		LevelAlert,
		LevelCritical,
		LevelError,
		LevelWarning,
		LevelNotice,
		LevelInfo,
		LevelDebug,
	}
}
