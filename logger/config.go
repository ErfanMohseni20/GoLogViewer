package logger

import "path/filepath"

const (
	DriverSingle = "single"
	DriverDaily  = "daily"
	DriverStack  = "stack"
	DriverStdout = "stdout"
)

type ChannelConfig struct {
	Driver   string   `json:"driver"`
	Path     string   `json:"path"`
	Level    Level    `json:"level"`
	Channels []string `json:"channels"`
	Days     int      `json:"days"`
}

type Config struct {
	Default  string                   `json:"default"`
	LogDir   string                   `json:"log_dir"`
	Channels map[string]ChannelConfig `json:"channels"`
}

func DefaultConfig() Config {
	return Config{
		Default: "stack",
		LogDir:  "storage/logs",
		Channels: map[string]ChannelConfig{
			"stack": {
				Driver:   DriverStack,
				Channels: []string{"daily", "stdout"},
			},
			"daily": {
				Driver: DriverDaily,
				Path:   "laravel.log",
				Level:  LevelDebug,
				Days:   14,
			},
			"single": {
				Driver: DriverSingle,
				Path:   "laravel.log",
				Level:  LevelDebug,
			},
			"stdout": {
				Driver: DriverStdout,
				Level:  LevelDebug,
			},
		},
	}
}

func (c Config) ResolvePath(channel ChannelConfig) string {
	if channel.Path == "" {
		return filepath.Join(c.LogDir, "laravel.log")
	}
	return filepath.Join(c.LogDir, channel.Path)
}

func (c Config) ResolveLogDir() string {
	if c.LogDir == "" {
		return "storage/logs"
	}
	return c.LogDir
}
