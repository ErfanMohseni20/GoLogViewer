package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ErfanMohseni20/GoLogViewer/logger"
)

func setupLogger(t *testing.T) (*logger.Manager, string) {
	t.Helper()

	dir := t.TempDir()
	cfg := logger.Config{
		Default: "test",
		LogDir:  dir,
		Channels: map[string]logger.ChannelConfig{
			"test": {Driver: logger.DriverSingle, Path: "test.log", Level: logger.LevelDebug},
		},
	}

	manager, err := logger.NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	previous := logger.SetDefault(manager)
	t.Cleanup(func() {
		_ = manager.Close()
		logger.SetDefault(previous)
	})

	return manager, filepath.Join(dir, "test.log")
}

func readEntries(t *testing.T, path string) []logger.Entry {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read log: %v", err)
	}

	var entries []logger.Entry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var entry logger.Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("decode %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func TestHTTPMiddlewareLogsRequests(t *testing.T) {
	manager, path := setupLogger(t)

	handler := HTTP(Options{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("done"))
	}))

	request := httptest.NewRequest(http.MethodPost, "/orders?ref=abc", nil)
	request.Header.Set("User-Agent", "test-agent")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	_ = manager.Sync()

	entries := readEntries(t, path)
	if len(entries) != 1 {
		t.Fatalf("wrote %d entries, want 1", len(entries))
	}

	entry := entries[0]
	if entry.Level != logger.LevelInfo {
		t.Errorf("level = %v, want info", entry.Level)
	}
	if entry.Context["method"] != "POST" || entry.Context["path"] != "/orders" {
		t.Errorf("context = %v", entry.Context)
	}
	if entry.Context["status"] != float64(http.StatusCreated) {
		t.Errorf("status = %v, want 201", entry.Context["status"])
	}
	if entry.Context["bytes"] != float64(4) {
		t.Errorf("bytes = %v, want 4", entry.Context["bytes"])
	}
	if entry.Context["user_agent"] != "test-agent" {
		t.Errorf("user_agent = %v", entry.Context["user_agent"])
	}
}

func TestHTTPMiddlewareLevelsFollowStatus(t *testing.T) {
	manager, path := setupLogger(t)

	statuses := []int{200, 404, 500}
	want := []logger.Level{logger.LevelInfo, logger.LevelWarning, logger.LevelError}

	for _, status := range statuses {
		handler := HTTP(Options{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	}

	_ = manager.Sync()

	entries := readEntries(t, path)
	if len(entries) != len(statuses) {
		t.Fatalf("wrote %d entries, want %d", len(entries), len(statuses))
	}
	for i, entry := range entries {
		if entry.Level != want[i] {
			t.Errorf("status %d logged at %v, want %v", statuses[i], entry.Level, want[i])
		}
	}
}

// v1 hard-coded "/logs", so a viewer mounted anywhere else logged every one of
// its own poll requests into the file it was displaying.
func TestSkipPathsHonoursACustomPrefix(t *testing.T) {
	manager, path := setupLogger(t)

	handler := HTTP(Options{SkipPaths: []string{"/admin/logs"}})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	for _, target := range []string{"/admin/logs", "/admin/logs/api/files", "/admin/logs/static/app.js"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))
	}
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/orders", nil))

	_ = manager.Sync()

	entries := readEntries(t, path)
	if len(entries) != 1 {
		t.Fatalf("wrote %d entries, want 1 — the viewer's own requests were logged", len(entries))
	}
	if entries[0].Context["path"] != "/orders" {
		t.Errorf("logged path = %v, want /orders", entries[0].Context["path"])
	}
}

// A prefix must not match a sibling path that merely shares its first bytes.
func TestSkipPathsDoesNotMatchSiblingPrefixes(t *testing.T) {
	manager, path := setupLogger(t)

	handler := HTTP(Options{SkipPaths: []string{"/logs"}})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/logsomething", nil))
	_ = manager.Sync()

	if entries := readEntries(t, path); len(entries) != 1 {
		t.Errorf("wrote %d entries, want 1 — /logsomething was wrongly skipped", len(entries))
	}
}

func TestRequestIDPropagatesToHandlerLogs(t *testing.T) {
	manager, path := setupLogger(t)

	handler := HTTP(Options{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A handler logging through the request context inherits the ID.
		_ = logger.Ctx(r.Context()).Info("inside handler")
	}))

	request := httptest.NewRequest(http.MethodGet, "/x", nil)
	request.Header.Set("X-Request-ID", "req-77")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	_ = manager.Sync()

	entries := readEntries(t, path)
	if len(entries) != 2 {
		t.Fatalf("wrote %d entries, want 2", len(entries))
	}
	for _, entry := range entries {
		if entry.Context["request_id"] != "req-77" {
			t.Errorf("%q is missing the request id: %v", entry.Message, entry.Context)
		}
	}
}

func TestSlowRequestIsPromotedToWarning(t *testing.T) {
	manager, path := setupLogger(t)

	handler := HTTP(Options{SlowRequestThreshold: 10 * time.Millisecond})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(25 * time.Millisecond)
		}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/slow", nil))
	_ = manager.Sync()

	entries := readEntries(t, path)
	if len(entries) != 1 {
		t.Fatalf("wrote %d entries, want 1", len(entries))
	}
	if entries[0].Level != logger.LevelWarning {
		t.Errorf("level = %v, want warning for a slow request", entries[0].Level)
	}
}

// The recorder must not hide http.Flusher, or SSE handlers behind the
// middleware would buffer until the response ended.
func TestRecorderPreservesFlusher(t *testing.T) {
	setupLogger(t)

	flushed := false
	handler := HTTP(Options{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("the wrapped writer does not implement http.Flusher")
			return
		}
		_, _ = w.Write([]byte("chunk"))
		flusher.Flush()
		flushed = true
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/stream", nil))
	if !flushed {
		t.Error("the handler could not flush")
	}
}

func TestClientIPPrefersForwardedHeader(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/x", nil)
	request.RemoteAddr = "10.0.0.1:5555"

	if got := ClientIP(request); got != "10.0.0.1" {
		t.Errorf("clientIP = %q, want 10.0.0.1", got)
	}

	request.Header.Set("X-Forwarded-For", "203.0.113.9, 70.41.3.18")
	if got := ClientIP(request); got != "203.0.113.9" {
		t.Errorf("clientIP = %q, want the left-most forwarded address", got)
	}
}
