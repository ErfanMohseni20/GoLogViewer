// Package middleware logs HTTP requests through the logger package.
//
// It depends only on the standard library. The Gin adapter lives in the
// ginlog package, so importing this one never links a web framework into your
// binary.
package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/ErfanMohseni20/GoLogViewer/logger"
)

// Options configures request logging.
type Options struct {
	// Channel names the log channel to write to. Empty uses the default.
	Channel string

	// SkipPaths lists path prefixes that are not logged. The viewer's own
	// prefix belongs here, otherwise every poll of the log UI appends a line
	// to the file it is displaying.
	SkipPaths []string

	// Skip is an arbitrary predicate evaluated after SkipPaths.
	Skip func(*http.Request) bool

	// SlowRequestThreshold promotes a request to warning level once it takes
	// at least this long. Zero disables the check.
	SlowRequestThreshold time.Duration

	// RequestIDHeader names a header whose value is attached to each entry and
	// injected into the request context, so handlers logging via
	// logger.Ctx(r.Context()) inherit it. Defaults to X-Request-ID.
	RequestIDHeader string

	// Message is the log message used for completed requests.
	Message string
}

// Normalize fills in the defaults. It is exported so that framework adapters
// in other packages share exactly these defaults.
func (o Options) Normalize() Options {
	if o.RequestIDHeader == "" {
		o.RequestIDHeader = "X-Request-ID"
	}
	if o.Message == "" {
		o.Message = "http request"
	}
	return o
}

// ShouldSkip reports whether a request must not be logged. Exported for the
// same reason as Normalize.
func (o Options) ShouldSkip(path string, r *http.Request) bool {
	for _, prefix := range o.SkipPaths {
		if prefix == "" {
			continue
		}
		if path == prefix || strings.HasPrefix(path, strings.TrimRight(prefix, "/")+"/") {
			return true
		}
	}
	return o.Skip != nil && o.Skip(r)
}

// LevelForStatus maps an HTTP status onto a log severity.
func LevelForStatus(status int) logger.Level {
	switch {
	case status >= 500:
		return logger.LevelError
	case status >= 400:
		return logger.LevelWarning
	default:
		return logger.LevelInfo
	}
}

// statusRecorder captures the status code and response size, which the
// ResponseWriter interface does not expose.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written int64
}

func (s *statusRecorder) WriteHeader(status int) {
	if s.status == 0 {
		s.status = status
	}
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.written += int64(n)
	return n, err
}

// Flush forwards to the wrapped writer so SSE and streaming handlers still
// work behind the middleware.
func (s *statusRecorder) Flush() {
	if flusher, ok := s.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer for
// hijacking and deadline control.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// HTTP returns a net/http middleware that logs every request.
func HTTP(opts Options) func(http.Handler) http.Handler {
	opts = opts.Normalize()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if opts.ShouldSkip(path, r) {
				next.ServeHTTP(w, r)
				return
			}

			requestID := r.Header.Get(opts.RequestIDHeader)
			ctx := r.Context()
			if requestID != "" {
				ctx = logger.WithFields(ctx, "request_id", requestID)
				r = r.WithContext(ctx)
			}

			start := time.Now()
			recorder := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(recorder, r)
			duration := time.Since(start)

			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}

			level := LevelForStatus(status)
			if opts.SlowRequestThreshold > 0 && duration >= opts.SlowRequestThreshold && level == logger.LevelInfo {
				level = logger.LevelWarning
			}

			log, err := logger.Default().Channel(opts.Channel)
			if err != nil {
				// A logging failure must not take down the request that was
				// already served successfully.
				return
			}

			_ = log.Ctx(ctx).Log(level, opts.Message,
				"method", r.Method,
				"path", path,
				"status", status,
				"duration_ms", duration.Milliseconds(),
				"bytes", recorder.written,
				"ip", ClientIP(r),
				"user_agent", r.UserAgent(),
			)
		})
	}
}

// ClientIP prefers the left-most X-Forwarded-For entry, falling back to the
// connection address. Trust it only behind a proxy that overwrites the header.
func ClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if comma := strings.IndexByte(forwarded, ','); comma > 0 {
			return strings.TrimSpace(forwarded[:comma])
		}
		return strings.TrimSpace(forwarded)
	}
	if real := r.Header.Get("X-Real-IP"); real != "" {
		return strings.TrimSpace(real)
	}

	addr := r.RemoteAddr
	if colon := strings.LastIndexByte(addr, ':'); colon > 0 {
		return addr[:colon]
	}
	return addr
}
