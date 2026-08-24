package logger

// singleDriver appends every entry to one file.
type singleDriver struct {
	name     string
	minLevel Level
	writer   *fileWriter
}

func newSingleDriver(name, path string, minLevel Level, opts fileWriterOptions) (*singleDriver, error) {
	writer, err := newFileWriter(path, opts)
	if err != nil {
		return nil, err
	}

	return &singleDriver{name: name, minLevel: minLevel, writer: writer}, nil
}

func (s *singleDriver) Name() string { return s.name }

func (s *singleDriver) Log(entry Entry) error {
	if !entry.Level.Allows(s.minLevel) {
		return nil
	}
	entry.Channel = s.name
	return s.writer.Write(entry)
}

func (s *singleDriver) Sync() error  { return s.writer.Sync() }
func (s *singleDriver) Close() error { return s.writer.Close() }
