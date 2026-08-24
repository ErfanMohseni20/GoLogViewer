package logger

import (
	"sync"
	"sync/atomic"
)

const defaultAsyncBuffer = 4096

// asyncDriver decouples the caller from disk I/O by handing entries to a
// background goroutine. When the queue is full it drops the entry rather than
// blocking the application — a logger must never become the bottleneck — and
// counts the drop so it can be reported.
type asyncDriver struct {
	inner   Driver
	queue   chan Entry
	onError func(error)

	dropped atomic.Uint64

	// mu guards the queue against a send racing with the close in Close.
	// Log takes it for reading, so concurrent producers never contend.
	mu     sync.RWMutex
	closed bool

	closeOnce sync.Once
	done      chan struct{}
}

func newAsyncDriver(inner Driver, bufferSize int, onError func(error)) *asyncDriver {
	if bufferSize <= 0 {
		bufferSize = defaultAsyncBuffer
	}

	a := &asyncDriver{
		inner:   inner,
		queue:   make(chan Entry, bufferSize),
		onError: onError,
		done:    make(chan struct{}),
	}

	go a.run()
	return a
}

func (a *asyncDriver) run() {
	defer close(a.done)

	for entry := range a.queue {
		if err := a.inner.Log(entry); err != nil && a.onError != nil {
			a.onError(err)
		}
	}
}

func (a *asyncDriver) Name() string { return a.inner.Name() }

func (a *asyncDriver) Log(entry Entry) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.closed {
		return nil
	}

	// The entry is handed to another goroutine, so copy the context map the
	// caller may still hold a reference to.
	select {
	case a.queue <- entry.clone():
		return nil
	default:
		a.dropped.Add(1)
		return nil
	}
}

// Dropped reports how many entries were discarded because the queue was full.
func (a *asyncDriver) Dropped() uint64 { return a.dropped.Load() }

// Sync waits for the queue to drain, then flushes the wrapped driver.
func (a *asyncDriver) Sync() error {
	for {
		a.mu.RLock()
		pending := len(a.queue) > 0 && !a.closed
		a.mu.RUnlock()
		if !pending {
			break
		}
		runtimeGosched()
	}

	if syncer, ok := a.inner.(Syncer); ok {
		return syncer.Sync()
	}
	return nil
}

// Close drains the queue, then closes the wrapped driver. Entries submitted
// after Close are dropped instead of panicking on a closed channel.
func (a *asyncDriver) Close() error {
	a.closeOnce.Do(func() {
		a.mu.Lock()
		a.closed = true
		close(a.queue)
		a.mu.Unlock()

		<-a.done
	})
	return a.inner.Close()
}
