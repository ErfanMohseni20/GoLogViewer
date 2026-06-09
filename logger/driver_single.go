package logger

type SingleDriver struct {
	name     string
	minLevel Level
	writer   *FileWriter
}

func NewSingleDriver(name string, path string, minLevel Level) (*SingleDriver, error) {
	writer, err := NewFileWriter(path)
	if err != nil {
		return nil, err
	}

	return &SingleDriver{
		name:     name,
		minLevel: minLevel,
		writer:   writer,
	}, nil
}

func (s *SingleDriver) Name() string {
	return s.name
}

func (s *SingleDriver) Log(entry Entry) error {
	if !entry.Level.Allows(s.minLevel) {
		return nil
	}

	entry.Channel = s.name
	return s.writer.Write(entry)
}
