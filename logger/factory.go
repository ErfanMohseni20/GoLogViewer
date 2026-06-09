package logger

import "fmt"

type channelFactory struct {
	cfg      Config
	channels map[string]Driver
}

func newChannelFactory(cfg Config) *channelFactory {
	return &channelFactory{
		cfg:      cfg,
		channels: make(map[string]Driver),
	}
}

func (f *channelFactory) build(name string) (Driver, error) {
	if channel, ok := f.channels[name]; ok {
		return channel, nil
	}

	channelConfig, ok := f.cfg.Channels[name]
	if !ok {
		return nil, fmt.Errorf("log channel %q is not defined", name)
	}

	channel, err := f.buildChannel(name, channelConfig)
	if err != nil {
		return nil, err
	}

	f.channels[name] = channel
	return channel, nil
}

func (f *channelFactory) buildChannel(name string, cfg ChannelConfig) (Driver, error) {
	switch cfg.Driver {
	case DriverSingle:
		path := f.cfg.ResolvePath(cfg)
		level := cfg.Level
		if level == "" {
			level = LevelDebug
		}
		return NewSingleDriver(name, path, level)

	case DriverDaily:
		path := f.cfg.ResolvePath(cfg)
		level := cfg.Level
		if level == "" {
			level = LevelDebug
		}
		return NewDailyDriver(name, path, level, cfg.Days)

	case DriverStdout:
		level := cfg.Level
		if level == "" {
			level = LevelDebug
		}
		return NewStdoutDriver(name, level), nil

	case DriverStack:
		if len(cfg.Channels) == 0 {
			return nil, fmt.Errorf("stack channel %q requires at least one channel", name)
		}

		var nested []Driver
		for _, nestedName := range cfg.Channels {
			nestedChannel, err := f.build(nestedName)
			if err != nil {
				return nil, err
			}
			nested = append(nested, nestedChannel)
		}

		return NewStackDriver(name, nested), nil

	default:
		return nil, fmt.Errorf("unsupported log driver %q", cfg.Driver)
	}
}
