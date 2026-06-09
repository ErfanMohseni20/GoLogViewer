package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type FileWriter struct {
	mu   sync.Mutex
	path string
}

func NewFileWriter(path string) (*FileWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	return &FileWriter{path: path}, nil
}

func (w *FileWriter) Write(entry Entry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	file, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}

	return nil
}

func (w *FileWriter) Path() string {
	return w.path
}

func (w *FileWriter) SetPath(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	w.path = path
	return nil
}
