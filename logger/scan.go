package logger

import (
	"bytes"
	"errors"
	"io"
	"os"
)

// reverseChunkSize is how much of the file the reverse scanner reads per step.
// 64 KiB comfortably holds hundreds of log lines, so most page requests touch
// the disk only a handful of times.
const reverseChunkSize = 64 * 1024

// errStopScan is returned by a scan callback to end the walk early.
var errStopScan = errors.New("logger: stop scan")

// scanReverse walks the lines of a file from the last one to the first,
// reading fixed-size chunks backwards. This is what lets the viewer serve page
// 1 of a multi-gigabyte log by touching only its tail, instead of decoding
// every line into memory the way the v1 reader did.
//
// fn is called with a slice valid only for the duration of the call; copy
// anything that must outlive it. Returning errStopScan ends the walk cleanly.
func scanReverse(file *os.File, size int64, fn func(line []byte) error) error {
	if size == 0 {
		return nil
	}

	buf := make([]byte, reverseChunkSize)
	// remainder holds the bytes of a line that straddles a chunk boundary,
	// carried down into the next (earlier) chunk.
	var remainder []byte
	offset := size

	for offset > 0 {
		readSize := int64(reverseChunkSize)
		if offset < readSize {
			readSize = offset
		}
		offset -= readSize

		chunk := buf[:readSize]
		if _, err := file.ReadAt(chunk, offset); err != nil && err != io.EOF {
			return err
		}

		// Everything after the last newline in this chunk belongs to the line
		// that continues into the chunk we already processed.
		for i := len(chunk) - 1; i >= 0; i-- {
			if chunk[i] != '\n' {
				continue
			}

			line := chunk[i+1:]
			if len(remainder) > 0 {
				line = append(append([]byte(nil), line...), remainder...)
				remainder = nil
			}

			if err := emitLine(line, fn); err != nil {
				return err
			}

			chunk = chunk[:i]
		}

		// Carry the head of the chunk into the next iteration.
		head := chunk
		if len(head) > 0 || len(remainder) > 0 {
			remainder = append(append([]byte(nil), head...), remainder...)
		}
	}

	if len(remainder) > 0 {
		if err := emitLine(remainder, fn); err != nil {
			return err
		}
	}
	return nil
}

func emitLine(line []byte, fn func([]byte) error) error {
	line = bytes.TrimRight(line, "\r\n")
	if len(bytes.TrimSpace(line)) == 0 {
		return nil
	}
	return fn(line)
}
