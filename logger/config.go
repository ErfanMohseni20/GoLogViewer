package logger

import (
	"errors"
	"fmt"
	"path/filepath"
)

// Driver names accepted in ChannelConfig.Driver.
const (
	DriverSingle = "single"
	DriverDaily  = "daily"
	DriverStack  = "stack"
	DriverStdout = "stdout"
	DriverStderr = "stderr"
	DriverNull   = "null"
)

// DefaultLogDir is used whenever Config.LogDir is empty.
const DefaultLogDir = "storage/logs"

// ErrInvalidConfig wraps every configuration validation failure.
var ErrInvalidConfig = errors.New("logger: invalid config")

// ChannelConfig describes one named log channel.
type ChannelConfig struct {
	// Driver selects the implementation: single, daily, stack, stdout, stderr
	// or null.
	Driver string `json:"driver"`

	// Path is the log file path relative to Config.LogDir. Used by the single
	// and daily drivers.
	Path string `json:"path,omitempty"`

	// Level is the minimum severity this channel writes. Empty means debug.
	Level Level `json:"level,omitempty"`

	// Channels lists the member channels of a stack driver.
	Channels []string `json:"channels,omitempty"`

	// Days is the retention window for the daily driver. Defaults to 14.
	Days int `json:"days,omitempty"`

	// MaxSizeMB rotates the current file once it grows past this many
	// megabytes, appending .1, .2 and so on. Zero disables size rotation.
	MaxSizeMB int `json:"max_size_mb,omitempty"`

	// MaxBackups caps how many size-rotated files are kept. Zero means keep
	// them all, subject to the daily retention sweep.
	MaxBackups int `json:"max_backups,omitempty"`

	// Buffered writes through a bufio.Writer flushed on an interval and on
	// Close. It trades a small durability window for a large throughput win.
	Buffered bool `json:"buffered,omitempty"`

	// Async hands entries to a background goroutine so callers never block on
	// disk I/O. Requires Close (or Shutdown) to drain the queue at exit.
	Async bool `json:"async,omitempty"`

	// AsyncBufferSize is the queue depth for Async channels. Defaults to 4096.
	AsyncBufferSize int `json:"async_buffer_size,omitempty"`

	// Color forces ANSI colouring for the stdout/stderr drivers. When nil the
	// driver auto-detects whether it is attached to a terminal.
	Color *bool `json:"color,omitempty"`
}

// Config is the root logging configuration, modelled on Laravel's
// config/logging.php.
type Config struct {
	// Default names the channel used by the package-level helpers.
	Default string `json:"default"`

	// LogDir is the directory holding log files. It is also what the viewer
	// browses.
	LogDir string `json:"log_dir"`

	// Channels maps channel names to their configuration.
	Channels map[string]ChannelConfig `json:"channels"`

	// IncludeCaller records the file:line of the call site on every entry.
	IncludeCaller bool `json:"include_caller,omitempty"`

	// RedactKeys lists context keys whose values are replaced with
	// [REDACTED] before an entry reaches any driver. Matching is
	// case-insensitive and substring-based, so "token" also covers
	// "access_token". Nil installs DefaultRedactKeys; an empty non-nil slice
	// disables redaction entirely.
	RedactKeys []string `json:"redact_keys,omitempty"`

	// OnError receives delivery failures from async channels, which have no
	// caller left to return an error to. Optional.
	OnError func(error) `json:"-"`
}

// DefaultConfig returns a Laravel-like setup: a stack that writes rotated
// daily files plus human-readable console output.
func DefaultConfig() Config {
	return Config{
		Default: "stack",
		LogDir:  DefaultLogDir,
		Channels: map[string]ChannelConfig{
			"stack": {
				Driver:   DriverStack,
				Channels: []string{"daily", "stdout"},
			},
			"daily": {
				Driver:   DriverDaily,
				Path:     "laravel.log",
				Level:    LevelDebug,
				Days:     14,
				Buffered: true,
			},
			"single": {
				Driver:   DriverSingle,
				Path:     "laravel.log",
				Level:    LevelDebug,
				Buffered: true,
			},
			"stdout": {
				Driver: DriverStdout,
				Level:  LevelDebug,
			},
			"stderr": {
				Driver: DriverStderr,
				Level:  LevelWarning,
			},
			"null": {
				Driver: DriverNull,
			},
		},
	}
}

// ResolvePath returns the absolute-from-LogDir file path for a channel.
func (c Config) ResolvePath(channel ChannelConfig) string {
	path := channel.Path
	if path == "" {
		path = "laravel.log"
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(c.ResolveLogDir(), path)
}

// ResolveLogDir returns the configured log directory or the default.
func (c Config) ResolveLogDir() string {
	if c.LogDir == "" {
		return DefaultLogDir
	}
	return filepath.Clean(c.LogDir)
}

// normalize fills in defaults so the rest of the package can assume a complete
// configuration.
func (c Config) normalize() Config {
	if c.Channels == nil {
		c.Channels = DefaultConfig().Channels
	}
	if c.Default == "" {
		c.Default = "stack"
		if _, ok := c.Channels["stack"]; !ok {
			for name := range c.Channels {
				c.Default = name
				break
			}
		}
	}
	c.LogDir = c.ResolveLogDir()
	if c.RedactKeys == nil {
		c.RedactKeys = DefaultRedactKeys()
	}
	return c
}

// Validate reports configuration errors up front — unknown drivers, missing
// channels, stacks that reference themselves — instead of letting them surface
// on the first log call.
func (c Config) Validate() error {
	cfg := c.normalize()

	if _, ok := cfg.Channels[cfg.Default]; !ok {
		return fmt.Errorf("%w: default channel %q is not defined", ErrInvalidConfig, cfg.Default)
	}

	for name, channel := range cfg.Channels {
		switch channel.Driver {
		case DriverSingle, DriverDaily, DriverStdout, DriverStderr, DriverNull:
		case DriverStack:
			if len(channel.Channels) == 0 {
				return fmt.Errorf("%w: stack channel %q lists no channels", ErrInvalidConfig, name)
			}
		case "":
			return fmt.Errorf("%w: channel %q has no driver", ErrInvalidConfig, name)
		default:
			return fmt.Errorf("%w: channel %q uses unsupported driver %q", ErrInvalidConfig, name, channel.Driver)
		}

		if channel.Level != levelUnset && !channel.Level.Valid() {
			return fmt.Errorf("%w: channel %q has invalid level %d", ErrInvalidConfig, name, channel.Level)
		}
	}

	// Walk every stack to catch dangling references and cycles.
	for name, channel := range cfg.Channels {
		if channel.Driver != DriverStack {
			continue
		}
		if err := cfg.walkStack(name, map[string]bool{}); err != nil {
			return err
		}
	}

	return nil
}

func (c Config) walkStack(name string, visiting map[string]bool) error {
	if visiting[name] {
		return fmt.Errorf("%w: stack channel %q forms a cycle", ErrInvalidConfig, name)
	}

	channel, ok := c.Channels[name]
	if !ok {
		return fmt.Errorf("%w: channel %q is not defined", ErrInvalidConfig, name)
	}
	if channel.Driver != DriverStack {
		return nil
	}

	visiting[name] = true
	defer delete(visiting, name)

	for _, member := range channel.Channels {
		if err := c.walkStack(member, visiting); err != nil {
			return err
		}
	}
	return nil
}
