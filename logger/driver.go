package logger

type Driver interface {
	Name() string
	Log(entry Entry) error
}
