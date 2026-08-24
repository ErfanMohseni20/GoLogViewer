package logger

import (
	"runtime"
	"strconv"
	"strings"
)

// callerSkip counts the frames between runtime.Caller and the application:
// caller -> Logger.log -> the level method -> (optionally) the package helper.
const callerSkip = 3

// callerFrame returns "file.go:42" for the first frame outside this package,
// so wrapping helpers do not all report the same line inside logger.go.
func callerFrame(skip int) string {
	// Walk a few frames to step over any package-level helper that forwards
	// into Logger.log.
	pcs := make([]uintptr, 8)
	n := runtime.Callers(skip, pcs)
	if n == 0 {
		return ""
	}

	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if frame.File == "" {
			break
		}
		if !isLoggerFrame(frame.Function, frame.File) {
			return shortPath(frame.File) + ":" + strconv.Itoa(frame.Line)
		}
		if !more {
			break
		}
	}
	return ""
}

// isLoggerFrame reports whether a frame belongs to this package's own
// plumbing. Walking by package rather than by a fixed skip count keeps the
// result correct however many helpers forward into Logger.log.
//
// Test files are excluded so that this package's own tests, which live in
// package logger, report their own call site rather than testing.tRunner.
func isLoggerFrame(function, file string) bool {
	if strings.HasSuffix(file, "_test.go") {
		return false
	}
	return strings.Contains(function, "/GoLogViewer/logger.")
}

// shortPath keeps the last two path segments, enough to identify the file
// without leaking the build machine's directory layout into the logs.
func shortPath(path string) string {
	slash := strings.LastIndexByte(path, '/')
	if slash < 0 {
		return path
	}
	prev := strings.LastIndexByte(path[:slash], '/')
	if prev < 0 {
		return path
	}
	return path[prev+1:]
}
