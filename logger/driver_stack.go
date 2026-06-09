package logger

type StackDriver struct {
	name     string
	channels []Driver
}

func NewStackDriver(name string, channels []Driver) *StackDriver {
	return &StackDriver{name: name, channels: channels}
}

func (s *StackDriver) Name() string {
	return s.name
}

func (s *StackDriver) Log(entry Entry) error {
	for _, channel := range s.channels {
		if err := channel.Log(entry); err != nil {
			return err
		}
	}
	return nil
}
