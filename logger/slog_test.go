package logger

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
)

func TestSlogHandlerWritesEntries(t *testing.T) {
	cfg := testConfig(t)
	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer manager.Close()

	handler := manager.MustChannel("").Handler()
	log := slog.New(handler)

	log.Info("slog says hello", "user_id", 7)
	log.Error("slog reports failure", "error", "boom")
	log.Warn("careful")
	log.Debug("chatty")

	_ = manager.Sync()

	entries := readEntries(t, filepath.Join(cfg.LogDir, "test.log"))
	if len(entries) != 4 {
		t.Fatalf("wrote %d entries, want 4", len(entries))
	}

	if entries[0].Level != LevelInfo || entries[0].Message != "slog says hello" {
		t.Errorf("first entry = %+v", entries[0])
	}
	if entries[0].Context["user_id"] != float64(7) {
		t.Errorf("user_id = %v", entries[0].Context["user_id"])
	}
	if entries[1].Level != LevelError {
		t.Errorf("second entry level = %v, want error", entries[1].Level)
	}
	// An "error" attribute is promoted to the entry's exception so the viewer
	// highlights it the same way logger.Error does.
	if entries[1].Exception != "boom" {
		t.Errorf("exception = %q, want boom", entries[1].Exception)
	}
	if entries[3].Level != LevelDebug {
		t.Errorf("fourth entry level = %v, want debug", entries[3].Level)
	}
}

func TestSlogHandlerGroupsAndAttrs(t *testing.T) {
	cfg := testConfig(t)
	manager, _ := NewManager(cfg)
	defer manager.Close()

	log := slog.New(manager.MustChannel("").Handler()).
		With("service", "api").
		WithGroup("http")

	log.Info("request", "method", "GET", slog.Group("client", "ip", "127.0.0.1"))
	_ = manager.Sync()

	entries := readEntries(t, filepath.Join(cfg.LogDir, "test.log"))
	context := entries[0].Context

	if context["service"] != "api" {
		t.Errorf("service = %v, want api", context["service"])
	}
	if context["http.method"] != "GET" {
		t.Errorf("http.method = %v, want GET", context["http.method"])
	}
	if context["http.client.ip"] != "127.0.0.1" {
		t.Errorf("http.client.ip = %v, want 127.0.0.1", context["http.client.ip"])
	}
}

// Two sibling groups derived from the same handler must not overwrite each
// other's key prefix.
func TestSlogHandlerGroupsDoNotAliasEachOther(t *testing.T) {
	cfg := testConfig(t)
	manager, _ := NewManager(cfg)
	defer manager.Close()

	base := slog.New(manager.MustChannel("").Handler())
	base.Info("two groups",
		slog.Group("a", "key", "value-a"),
		slog.Group("b", "key", "value-b"),
	)
	_ = manager.Sync()

	context := readEntries(t, filepath.Join(cfg.LogDir, "test.log"))[0].Context
	if context["a.key"] != "value-a" || context["b.key"] != "value-b" {
		t.Errorf("context = %v, want distinct a.key and b.key", context)
	}
}

func TestSlogHandlerCarriesContextFields(t *testing.T) {
	cfg := testConfig(t)
	manager, _ := NewManager(cfg)
	defer manager.Close()

	log := slog.New(manager.MustChannel("").Handler())
	ctx := WithFields(context.Background(), "trace_id", "trace-9")

	log.InfoContext(ctx, "traced")
	_ = manager.Sync()

	context := readEntries(t, filepath.Join(cfg.LogDir, "test.log"))[0].Context
	if context["trace_id"] != "trace-9" {
		t.Errorf("trace_id = %v, want trace-9", context["trace_id"])
	}
}

func TestSlogHandlerRedacts(t *testing.T) {
	cfg := testConfig(t)
	manager, _ := NewManager(cfg)
	defer manager.Close()

	slog.New(manager.MustChannel("").Handler()).Info("login", "password", "hunter2")
	_ = manager.Sync()

	context := readEntries(t, filepath.Join(cfg.LogDir, "test.log"))[0].Context
	if context["password"] != RedactedPlaceholder {
		t.Errorf("password = %v, want redacted", context["password"])
	}
}

func TestLevelSlogRoundTrip(t *testing.T) {
	for _, level := range []Level{LevelDebug, LevelInfo, LevelWarning, LevelError, LevelCritical, LevelAlert, LevelEmergency} {
		if got := levelFromSlog(level.SlogLevel()); got != level {
			t.Errorf("round trip of %v produced %v", level, got)
		}
	}
}
