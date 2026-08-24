package logger

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	flushInterval  = 250 * time.Millisecond
	writeBufSize   = 64 * 1024
	filePerm       = 0o644
	dirPerm        = 0o755
	encodeBufLimit = 256 * 1024
)

// fileWriter appends JSON lines to a log file. Unlike the v1 implementation it
// keeps the descriptor open for the lifetime of the driver and optionally
// buffers writes, which is the difference between ~20µs and well under 1µs per
// entry.
type fileWriter struct {
	mu       sync.Mutex
	path     string
	file     *os.File
	buf      *bufio.Writer
	size     int64
	maxBytes int64
	backups  int
	closed   bool

	encodeBuf []byte

	flushStop chan struct{}
	flushDone chan struct{}
}

type fileWriterOptions struct {
	buffered   bool
	maxSizeMB  int
	maxBackups int
}

func newFileWriter(path string, opts fileWriterOptions) (*fileWriter, error) {
	w := &fileWriter{
		path:      path,
		maxBytes:  int64(opts.maxSizeMB) * 1024 * 1024,
		backups:   opts.maxBackups,
		encodeBuf: make([]byte, 0, 1024),
	}

	if err := w.open(path); err != nil {
		return nil, err
	}

	if opts.buffered {
		w.buf = bufio.NewWriterSize(w.file, writeBufSize)
		w.flushStop = make(chan struct{})
		w.flushDone = make(chan struct{})
		go w.flushLoop()
	}

	return w, nil
}

// open assumes the caller holds mu, or that the writer is not yet shared.
func (w *fileWriter) open(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("logger: create log dir: %w", err)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePerm)
	if err != nil {
		return fmt.Errorf("logger: open log file: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("logger: stat log file: %w", err)
	}

	w.file = file
	w.path = path
	w.size = info.Size()
	return nil
}

func (w *fileWriter) flushLoop() {
	defer close(w.flushDone)

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_ = w.Sync()
		case <-w.flushStop:
			return
		}
	}
}

// Write appends one entry as a JSON line, rotating first if the file has grown
// past the configured size limit.
func (w *fileWriter) Write(entry Entry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return os.ErrClosed
	}

	line, err := w.encode(entry)
	if err != nil {
		return err
	}

	if w.maxBytes > 0 && w.size+int64(len(line)) > w.maxBytes && w.size > 0 {
		if err := w.rotateLocked(); err != nil {
			return err
		}
	}

	n, err := w.sink().Write(line)
	w.size += int64(n)
	if err != nil {
		return fmt.Errorf("logger: write log entry: %w", err)
	}
	return nil
}

func (w *fileWriter) sink() io.Writer {
	if w.buf != nil {
		return w.buf
	}
	return w.file
}

// encode renders the entry into a reusable buffer, so a steady stream of
// similar entries stops allocating once the buffer has grown to fit them. An
// oversized entry releases the buffer instead of pinning that capacity for the
// lifetime of the process.
func (w *fileWriter) encode(entry Entry) ([]byte, error) {
	w.encodeBuf = appendEntry(w.encodeBuf[:0], entry)
	w.encodeBuf = append(w.encodeBuf, '\n')
	line := w.encodeBuf

	if cap(w.encodeBuf) > encodeBufLimit {
		w.encodeBuf = make([]byte, 0, 1024)
	}
	return line, nil
}

// Sync flushes buffered bytes to the operating system.
func (w *fileWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.syncLocked()
}

func (w *fileWriter) syncLocked() error {
	if w.buf == nil || w.closed {
		return nil
	}
	if err := w.buf.Flush(); err != nil {
		return fmt.Errorf("logger: flush log buffer: %w", err)
	}
	return nil
}

// SetPath switches the writer to a different file, used by the daily driver
// when the date rolls over.
func (w *fileWriter) SetPath(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.path == path || w.closed {
		return nil
	}

	if err := w.syncLocked(); err != nil {
		return err
	}
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("logger: close log file: %w", err)
	}
	if err := w.open(path); err != nil {
		return err
	}
	if w.buf != nil {
		w.buf.Reset(w.file)
	}
	return nil
}

// Path reports the file currently being written.
func (w *fileWriter) Path() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.path
}

// Close flushes, stops the background flusher and releases the descriptor. It
// is safe to call more than once.
func (w *fileWriter) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	stop := w.flushStop
	done := w.flushDone
	w.mu.Unlock()

	// Stop the flusher outside the lock so a tick in flight cannot deadlock.
	if stop != nil {
		close(stop)
		<-done
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	w.flushStop = nil

	var firstErr error
	if w.buf != nil {
		if err := w.buf.Flush(); err != nil {
			firstErr = fmt.Errorf("logger: flush log buffer: %w", err)
		}
	}
	if err := w.file.Close(); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("logger: close log file: %w", err)
	}
	return firstErr
}

// rotateLocked renames the current file to path.1, shifting existing backups
// up, then reopens a fresh file. The caller must hold mu.
func (w *fileWriter) rotateLocked() error {
	if err := w.syncLocked(); err != nil {
		return err
	}
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("logger: close log file for rotation: %w", err)
	}

	for i := w.highestBackup(); i >= 1; i-- {
		if w.backups > 0 && i >= w.backups {
			_ = os.Remove(backupPath(w.path, i))
			continue
		}
		_ = os.Rename(backupPath(w.path, i), backupPath(w.path, i+1))
	}

	if err := os.Rename(w.path, backupPath(w.path, 1)); err != nil && !os.IsNotExist(err) {
		// Rotation failed; reopen the original so logging keeps working.
		if reopenErr := w.open(w.path); reopenErr != nil {
			return reopenErr
		}
		if w.buf != nil {
			w.buf.Reset(w.file)
		}
		return fmt.Errorf("logger: rotate log file: %w", err)
	}

	if err := w.open(w.path); err != nil {
		return err
	}
	if w.buf != nil {
		w.buf.Reset(w.file)
	}
	return nil
}

func (w *fileWriter) highestBackup() int {
	matches, err := filepath.Glob(w.path + ".*")
	if err != nil {
		return 0
	}

	indexes := make([]int, 0, len(matches))
	for _, match := range matches {
		suffix := strings.TrimPrefix(match, w.path+".")
		n, err := strconv.Atoi(suffix)
		if err == nil && n > 0 {
			indexes = append(indexes, n)
		}
	}
	if len(indexes) == 0 {
		return 0
	}

	sort.Ints(indexes)
	return indexes[len(indexes)-1]
}

func backupPath(path string, index int) string {
	return path + "." + strconv.Itoa(index)
}
