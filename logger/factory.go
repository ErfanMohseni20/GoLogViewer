package logger

import (
	"errors"
	"fmt"
	"os"
	"sync"
)

// channelFactory builds channels on first use and caches them. Every access is
// guarded: in v1 this map was read and written without a lock on the hot path
// of every log call, which raced under any concurrent load.
type channelFactory struct {
	cfg Config

	mu       sync.RWMutex
	channels map[string]Driver
	order    []string // creation order, so Close tears down deterministically
	closed   bool
}

func newChannelFactory(cfg Config) *channelFactory {
	return &channelFactory{
		cfg:      cfg,
		channels: make(map[string]Driver),
	}
}

// build returns the named channel, constructing it if necessary.
func (f *channelFactory) build(name string) (Driver, error) {
	// Fast path: an already-built channel needs only a read lock.
	f.mu.RLock()
	channel, ok := f.channels[name]
	closed := f.closed
	f.mu.RUnlock()

	if ok {
		return channel, nil
	}
	if closed {
		return nil, errors.New("logger: manager is closed")
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buildLocked(name, map[string]bool{})
}

// buildLocked constructs a channel with f.mu held. building tracks the stack
// members currently under construction so a cyclic configuration surfaces as
// an error rather than as unbounded recursion.
func (f *channelFactory) buildLocked(name string, building map[string]bool) (Driver, error) {
	if channel, ok := f.channels[name]; ok {
		return channel, nil
	}
	if f.closed {
		return nil, errors.New("logger: manager is closed")
	}
	if building[name] {
		return nil, fmt.Errorf("%w: stack channel %q forms a cycle", ErrInvalidConfig, name)
	}

	channelConfig, ok := f.cfg.Channels[name]
	if !ok {
		return nil, fmt.Errorf("%w: log channel %q is not defined", ErrInvalidConfig, name)
	}

	building[name] = true
	defer delete(building, name)

	channel, err := f.construct(name, channelConfig, building)
	if err != nil {
		return nil, err
	}

	f.channels[name] = channel
	f.order = append(f.order, name)
	return channel, nil
}

func (f *channelFactory) construct(name string, cfg ChannelConfig, building map[string]bool) (Driver, error) {
	level := cfg.Level
	if level == levelUnset {
		level = LevelDebug
	}

	writerOpts := fileWriterOptions{
		buffered:   cfg.Buffered,
		maxSizeMB:  cfg.MaxSizeMB,
		maxBackups: cfg.MaxBackups,
	}

	var (
		driver Driver
		err    error
	)

	switch cfg.Driver {
	case DriverSingle:
		driver, err = newSingleDriver(name, f.cfg.ResolvePath(cfg), level, writerOpts)

	case DriverDaily:
		driver, err = newDailyDriver(name, f.cfg.ResolvePath(cfg), level, cfg.Days, writerOpts)

	case DriverStdout:
		driver = newConsoleDriver(name, level, os.Stdout, cfg.Color)

	case DriverStderr:
		driver = newConsoleDriver(name, level, os.Stderr, cfg.Color)

	case DriverNull:
		driver = newNullDriver(name)

	case DriverStack:
		if len(cfg.Channels) == 0 {
			return nil, fmt.Errorf("%w: stack channel %q lists no channels", ErrInvalidConfig, name)
		}

		members := make([]Driver, 0, len(cfg.Channels))
		for _, memberName := range cfg.Channels {
			member, memberErr := f.buildLocked(memberName, building)
			if memberErr != nil {
				return nil, memberErr
			}
			members = append(members, member)
		}
		driver = newStackDriver(name, members)

	default:
		return nil, fmt.Errorf("%w: unsupported log driver %q", ErrInvalidConfig, cfg.Driver)
	}

	if err != nil {
		return nil, err
	}

	// A stack must not be wrapped: its members carry their own async setting,
	// and wrapping the stack would double-buffer them.
	if cfg.Async && cfg.Driver != DriverStack {
		driver = newAsyncDriver(driver, cfg.AsyncBufferSize, f.cfg.OnError)
	}

	return driver, nil
}

// sync flushes every built channel.
func (f *channelFactory) sync() error {
	f.mu.RLock()
	channels := make([]Driver, 0, len(f.channels))
	for _, channel := range f.channels {
		channels = append(channels, channel)
	}
	f.mu.RUnlock()

	var errs []error
	for _, channel := range channels {
		if syncer, ok := channel.(Syncer); ok {
			if err := syncer.Sync(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// close tears every built channel down in reverse creation order, so a stack
// is closed before the members it fans out to.
func (f *channelFactory) close() error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true

	channels := make([]Driver, 0, len(f.order))
	for i := len(f.order) - 1; i >= 0; i-- {
		channels = append(channels, f.channels[f.order[i]])
	}
	f.channels = make(map[string]Driver)
	f.order = nil
	f.mu.Unlock()

	var errs []error
	for _, channel := range channels {
		if err := channel.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
