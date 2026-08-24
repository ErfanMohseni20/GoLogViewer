package logger

import "errors"

// stackDriver fans an entry out to several channels. Unlike the v1 version it
// keeps going after a failure so one broken channel cannot silently disable
// the ones behind it, and it reports every error that occurred.
type stackDriver struct {
	name     string
	channels []Driver
}

func newStackDriver(name string, channels []Driver) *stackDriver {
	return &stackDriver{name: name, channels: channels}
}

func (s *stackDriver) Name() string { return s.name }

func (s *stackDriver) Log(entry Entry) error {
	var errs []error
	for _, channel := range s.channels {
		if err := channel.Log(entry); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *stackDriver) Sync() error {
	var errs []error
	for _, channel := range s.channels {
		if syncer, ok := channel.(Syncer); ok {
			if err := syncer.Sync(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// Close is a no-op: the factory owns every underlying channel and closes each
// exactly once, so a stack must not close members it merely borrows.
func (s *stackDriver) Close() error { return nil }
