package logger

// nullDriver discards everything. Useful in tests and for switching a channel
// off without removing it from the configuration.
type nullDriver struct {
	name string
}

func newNullDriver(name string) *nullDriver { return &nullDriver{name: name} }

func (n *nullDriver) Name() string    { return n.name }
func (n *nullDriver) Log(Entry) error { return nil }
func (n *nullDriver) Close() error    { return nil }
