package ginlog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ErfanMohseni20/GoLogViewer/logger"
	"github.com/ErfanMohseni20/GoLogViewer/middleware"
	"github.com/ErfanMohseni20/GoLogViewer/viewer"
	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func setupLogger(t *testing.T) (*logger.Manager, string) {
	t.Helper()

	dir := t.TempDir()
	manager, err := logger.NewManager(logger.Config{
		Default: "test",
		LogDir:  dir,
		Channels: map[string]logger.ChannelConfig{
			"test": {Driver: logger.DriverSingle, Path: "test.log", Level: logger.LevelDebug},
		},
	})
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
			t.Fatalf("decode: %v", err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func TestMiddlewareLogsRequests(t *testing.T) {
	manager, path := setupLogger(t)

	router := gin.New()
	router.Use(Middleware())
	router.GET("/orders", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/orders?ref=1", nil))
	_ = manager.Sync()

	entries := readEntries(t, path)
	if len(entries) != 1 {
		t.Fatalf("wrote %d entries, want 1", len(entries))
	}
	if entries[0].Context["path"] != "/orders" || entries[0].Context["query"] != "ref=1" {
		t.Errorf("context = %v", entries[0].Context)
	}
}

// A handler error registered on the Gin context becomes the entry's exception.
func TestMiddlewareRecordsHandlerErrors(t *testing.T) {
	manager, path := setupLogger(t)

	router := gin.New()
	router.Use(Middleware())
	router.GET("/fail", func(c *gin.Context) {
		_ = c.Error(http.ErrBodyNotAllowed)
		c.Status(http.StatusOK)
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/fail", nil))
	_ = manager.Sync()

	entries := readEntries(t, path)
	if len(entries) != 1 {
		t.Fatalf("wrote %d entries, want 1", len(entries))
	}
	if entries[0].Exception == "" {
		t.Error("the handler error was not recorded as an exception")
	}
	if entries[0].Level != logger.LevelError {
		t.Errorf("level = %v, want error", entries[0].Level)
	}
}

// Mounting the viewer must serve every one of its routes through Gin's
// wildcard, and the middleware must not log the viewer's own traffic.
func TestRegisterViewerServesAllRoutesAndIsNotLogged(t *testing.T) {
	manager, path := setupLogger(t)

	logDir := manager.Config().ResolveLogDir()
	if err := os.WriteFile(filepath.Join(logDir, "app.log"),
		[]byte(`{"level":"info","message":"hi","time":"2026-06-09T10:00:00Z"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.Use(Middleware(middleware.Options{SkipPaths: []string{"/logs"}}))
	RegisterViewer(router, viewer.Options{PathPrefix: "/logs", LogDir: logDir})

	for _, target := range []string{
		"/logs",
		"/logs/api/files",
		"/logs/api/entries?file=app.log",
		"/logs/static/style.css",
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusOK {
			t.Errorf("%s returned %d, want 200", target, recorder.Code)
		}
	}

	_ = manager.Sync()

	// Only the seeded entry should be present; browsing must add nothing.
	if entries := readEntries(t, path); len(entries) != 0 {
		t.Errorf("the viewer's own requests were logged: %d entries", len(entries))
	}
}

func TestRegisterViewerRespectsBasicAuth(t *testing.T) {
	manager, _ := setupLogger(t)

	router := gin.New()
	RegisterViewer(router, viewer.Options{
		PathPrefix:        "/logs",
		LogDir:            manager.Config().ResolveLogDir(),
		BasicAuthUser:     "admin",
		BasicAuthPassword: "secret",
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/logs", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", recorder.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/logs", nil)
	request.SetBasicAuth("admin", "secret")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", recorder.Code)
	}
}
