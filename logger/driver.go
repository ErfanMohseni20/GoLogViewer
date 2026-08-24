package logger

// Driver receives entries for one channel and delivers them somewhere: a file,
// the console, or a fan-out to other drivers.
//
// Log must be safe for concurrent use. Close must be idempotent and flush any
// buffered records before returning.
type Driver interface {
	Name() string
	Log(entry Entry) error
	Close() error
}

// Syncer is implemented by drivers that buffer writes and can flush on demand
// without being closed.
type Syncer interface {
	Sync() error
}
