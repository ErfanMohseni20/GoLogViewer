package logger

import (
	"fmt"
	"io"
	"os"
)

// OpenLogFile opens a log file for reading after validating its name. The
// caller is responsible for closing the returned file.
func OpenLogFile(dir, name string) (*os.File, os.FileInfo, error) {
	path, err := ResolveLogPath(dir, name)
	if err != nil {
		return nil, nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("logger: open log file: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, fmt.Errorf("logger: stat log file: %w", err)
	}

	if info.IsDir() {
		_ = file.Close()
		return nil, nil, fmt.Errorf("%w: %q is a directory", ErrInvalidFile, name)
	}

	return file, info, nil
}

// DeleteLogFile removes one log file from the log directory.
func DeleteLogFile(dir, name string) error {
	path, err := ResolveLogPath(dir, name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("logger: delete log file: %w", err)
	}
	return nil
}

// TruncateLogFile empties a log file without removing it, so a driver holding
// the descriptor open keeps writing to the same inode.
func TruncateLogFile(dir, name string) error {
	path, err := ResolveLogPath(dir, name)
	if err != nil {
		return err
	}
	if err := os.Truncate(path, 0); err != nil {
		return fmt.Errorf("logger: truncate log file: %w", err)
	}
	return nil
}

// DeleteAllLogFiles removes every log file in the directory and reports how
// many were removed.
func DeleteAllLogFiles(dir string) (int, error) {
	files, err := ListLogFiles(dir)
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, file := range files {
		if err := DeleteLogFile(dir, file.Name); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// CopyLogFile streams a log file to w, which is how the viewer serves
// downloads without loading the file into memory.
func CopyLogFile(dir, name string, w io.Writer) (int64, error) {
	file, _, err := OpenLogFile(dir, name)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	written, err := io.Copy(w, file)
	if err != nil {
		return written, fmt.Errorf("logger: stream log file: %w", err)
	}
	return written, nil
}
