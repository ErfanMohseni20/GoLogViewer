package logger

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// testConfig returns a config writing structured entries to a temp directory
// via a single, unbuffered channel so assertions can read the file directly.
func testConfig(t *testing.T) Config {
	t.Helper()

	return Config{
		Default: "test",
		LogDir:  t.TempDir(),
		Channels: map[string]ChannelConfig{
			"test": {Driver: DriverSingle, Path: "test.log", Level: LevelDebug},
		},
	}
}

func readEntries(t *testing.T, path string) []Entry {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}

	var entries []Entry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("decode %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func TestManagerWritesStructuredEntries(t *testing.T) {
	cfg := testConfig(t)
	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer manager.Close()

	log := manager.MustChannel("")
	if err := log.Info("hello", "user_id", 7); err != nil {
		t.Fatalf("Info: %v", err)
	}
	if err := log.Error("boom", errors.New("disk on fire"), "attempt", 2); err != nil {
		t.Fatalf("Error: %v", err)
	}

	if err := manager.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	entries := readEntries(t, filepath.Join(cfg.LogDir, "test.log"))
	if len(entries) != 2 {
		t.Fatalf("wrote %d entries, want 2", len(entries))
	}

	if entries[0].Level != LevelInfo || entries[0].Message != "hello" {
		t.Errorf("first entry = %+v", entries[0])
	}
	if got := entries[0].Context["user_id"]; got != float64(7) {
		t.Errorf("user_id = %v (%T), want 7", got, got)
	}
	if entries[1].Exception != "disk on fire" {
		t.Errorf("exception = %q", entries[1].Exception)
	}
	if entries[1].Channel != "test" {
		t.Errorf("channel = %q, want test", entries[1].Channel)
	}
}

// This is the v1 regression: every log call read and wrote the channel cache
// without a lock, so concurrent logging raced and could crash the process.
func TestConcurrentLoggingIsRaceFree(t *testing.T) {
	cfg := testConfig(t)
	cfg.Channels["stack"] = ChannelConfig{Driver: DriverStack, Channels: []string{"test", "null"}}
	cfg.Channels["null"] = ChannelConfig{Driver: DriverNull}
	cfg.Default = "stack"

	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer manager.Close()

	const goroutines, perGoroutine = 32, 50

	// "" resolves to the stack, which writes to the file once; "test" writes
	// directly; "null" discards. Count the file-bound selections exactly
	// rather than assuming perGoroutine divides evenly by len(names).
	names := []string{"", "test", "null", "stack"}
	expectedPerGoroutine := 0
	for j := 0; j < perGoroutine; j++ {
		if names[j%len(names)] != "null" {
			expectedPerGoroutine++
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Alternate channels so the factory's build path is exercised
			// concurrently, not just the cached read.
			for j := 0; j < perGoroutine; j++ {
				log, err := manager.Channel(names[j%len(names)])
				if err != nil {
					t.Errorf("Channel: %v", err)
					return
				}
				_ = log.Info("concurrent", "goroutine", id, "iteration", j)
			}
		}(i)
	}
	wg.Wait()

	if err := manager.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	entries := readEntries(t, filepath.Join(cfg.LogDir, "test.log"))
	if want := goroutines * expectedPerGoroutine; len(entries) != want {
		t.Errorf("wrote %d entries, want %d", len(entries), want)
	}
}

func TestWithDoesNotMutateParentContext(t *testing.T) {
	cfg := testConfig(t)
	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer manager.Close()

	base := manager.MustChannel("").With("service", "api")
	child := base.With("request_id", "abc")

	_ = child.Info("child")
	_ = base.Info("base")
	_ = manager.Sync()

	entries := readEntries(t, filepath.Join(cfg.LogDir, "test.log"))
	if len(entries) != 2 {
		t.Fatalf("wrote %d entries, want 2", len(entries))
	}
	if _, leaked := entries[1].Context["request_id"]; leaked {
		t.Error("the parent logger inherited a field bound only to its child")
	}
	if entries[1].Context["service"] != "api" {
		t.Error("the parent lost its own bound field")
	}
}

func TestContextFieldsPropagate(t *testing.T) {
	cfg := testConfig(t)
	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer manager.Close()

	ctx := WithFields(context.Background(), "request_id", "req-42")
	_ = manager.MustChannel("").Ctx(ctx).Info("handled")
	_ = manager.Sync()

	entries := readEntries(t, filepath.Join(cfg.LogDir, "test.log"))
	if len(entries) != 1 {
		t.Fatalf("wrote %d entries, want 1", len(entries))
	}
	if entries[0].Context["request_id"] != "req-42" {
		t.Errorf("request_id = %v, want req-42", entries[0].Context["request_id"])
	}
}

// An explicit field at the call site must win over one carried on the context.
func TestCallSiteFieldOverridesContextField(t *testing.T) {
	cfg := testConfig(t)
	manager, _ := NewManager(cfg)
	defer manager.Close()

	ctx := WithFields(context.Background(), "tenant", "from-context")
	_ = manager.MustChannel("").Ctx(ctx).Info("hi", "tenant", "from-call")
	_ = manager.Sync()

	entries := readEntries(t, filepath.Join(cfg.LogDir, "test.log"))
	if entries[0].Context["tenant"] != "from-call" {
		t.Errorf("tenant = %v, want from-call", entries[0].Context["tenant"])
	}
}

func TestMinimumLevelIsEnforced(t *testing.T) {
	cfg := testConfig(t)
	cfg.Channels["test"] = ChannelConfig{Driver: DriverSingle, Path: "test.log", Level: LevelWarning}

	manager, _ := NewManager(cfg)
	defer manager.Close()

	log := manager.MustChannel("")
	_ = log.Debug("dropped")
	_ = log.Info("dropped")
	_ = log.Warning("kept")
	_ = log.Emergency("kept")
	_ = manager.Sync()

	entries := readEntries(t, filepath.Join(cfg.LogDir, "test.log"))
	if len(entries) != 2 {
		t.Fatalf("wrote %d entries, want 2", len(entries))
	}
}

func TestRedactionMasksSensitiveFields(t *testing.T) {
	cfg := testConfig(t)
	manager, _ := NewManager(cfg)
	defer manager.Close()

	_ = manager.MustChannel("").Info("login",
		"username", "erfan",
		"password", "hunter2",
		"access_token", "sk-live-123",
	)
	_ = manager.Sync()

	entries := readEntries(t, filepath.Join(cfg.LogDir, "test.log"))
	context := entries[0].Context

	if context["username"] != "erfan" {
		t.Error("a non-sensitive field was altered")
	}
	if context["password"] != RedactedPlaceholder {
		t.Errorf("password = %v, want redacted", context["password"])
	}
	// Substring matching must catch prefixed variants of a listed key.
	if context["access_token"] != RedactedPlaceholder {
		t.Errorf("access_token = %v, want redacted", context["access_token"])
	}
}

func TestRedactionCanBeDisabled(t *testing.T) {
	cfg := testConfig(t)
	cfg.RedactKeys = []string{}

	manager, _ := NewManager(cfg)
	defer manager.Close()

	_ = manager.MustChannel("").Info("login", "password", "hunter2")
	_ = manager.Sync()

	entries := readEntries(t, filepath.Join(cfg.LogDir, "test.log"))
	if entries[0].Context["password"] != "hunter2" {
		t.Error("redaction ran despite an explicitly empty key list")
	}
}

// v1's stack driver returned on the first failing channel, so a broken channel
// silently disabled every channel listed after it.
func TestStackKeepsWritingAfterAChannelFails(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Default: "stack",
		LogDir:  dir,
		Channels: map[string]ChannelConfig{
			"stack":  {Driver: DriverStack, Channels: []string{"broken", "good"}},
			"broken": {Driver: DriverSingle, Path: "broken.log"},
			"good":   {Driver: DriverSingle, Path: "good.log"},
		},
	}

	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer manager.Close()

	// Close the first member's writer behind the stack's back to force a
	// delivery failure on it alone.
	broken, err := manager.Channel("broken")
	if err != nil {
		t.Fatalf("Channel: %v", err)
	}
	if err := broken.driver.Close(); err != nil {
		t.Fatalf("close broken driver: %v", err)
	}

	logErr := manager.MustChannel("stack").Info("fan out")
	if logErr == nil {
		t.Error("expected the stack to report the failing channel")
	}

	_ = manager.Sync()

	entries := readEntries(t, filepath.Join(dir, "good.log"))
	if len(entries) != 1 {
		t.Fatalf("the healthy channel wrote %d entries, want 1", len(entries))
	}
}

func TestConfigValidation(t *testing.T) {
	cases := map[string]Config{
		"unknown driver": {
			Default:  "a",
			Channels: map[string]ChannelConfig{"a": {Driver: "carrier-pigeon"}},
		},
		"missing default channel": {
			Default:  "nope",
			Channels: map[string]ChannelConfig{"a": {Driver: DriverNull}},
		},
		"stack with no members": {
			Default:  "a",
			Channels: map[string]ChannelConfig{"a": {Driver: DriverStack}},
		},
		"stack referencing an undefined channel": {
			Default:  "a",
			Channels: map[string]ChannelConfig{"a": {Driver: DriverStack, Channels: []string{"ghost"}}},
		},
		"stack cycle": {
			Default: "a",
			Channels: map[string]ChannelConfig{
				"a": {Driver: DriverStack, Channels: []string{"b"}},
				"b": {Driver: DriverStack, Channels: []string{"a"}},
			},
		},
	}

	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewManager(cfg); err == nil {
				t.Error("expected NewManager to reject this configuration")
			}
		})
	}
}

func TestCallerIsRecorded(t *testing.T) {
	cfg := testConfig(t)
	cfg.IncludeCaller = true

	manager, _ := NewManager(cfg)
	defer manager.Close()

	_ = manager.MustChannel("").Info("who called")
	_ = manager.Sync()

	entries := readEntries(t, filepath.Join(cfg.LogDir, "test.log"))
	if !strings.Contains(entries[0].Caller, "logger_test.go:") {
		t.Errorf("caller = %q, want this test file", entries[0].Caller)
	}
}

func TestAsyncChannelDrainsOnClose(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Default: "async",
		LogDir:  dir,
		Channels: map[string]ChannelConfig{
			"async": {Driver: DriverSingle, Path: "async.log", Async: true, Buffered: true},
		},
	}

	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	log := manager.MustChannel("")
	const count = 500
	for i := 0; i < count; i++ {
		_ = log.Info("async entry", "i", i)
	}

	if err := manager.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	entries := readEntries(t, filepath.Join(dir, "async.log"))
	if len(entries) != count {
		t.Errorf("wrote %d entries, want %d — Close did not drain the queue", len(entries), count)
	}
}

// Logging after Close must be a no-op, not a panic on a closed channel.
func TestAsyncLogAfterCloseDoesNotPanic(t *testing.T) {
	inner := &nullDriver{name: "inner"}
	async := newAsyncDriver(inner, 4, nil)

	if err := async.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := async.Log(Entry{Level: LevelInfo, Message: "after close"}); err != nil {
		t.Fatalf("Log after Close: %v", err)
	}
}

func TestSizeBasedRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rotate.log")

	writer, err := newFileWriter(path, fileWriterOptions{maxSizeMB: 1, maxBackups: 3})
	if err != nil {
		t.Fatalf("newFileWriter: %v", err)
	}
	defer writer.Close()

	// Roughly 1.5 MB of entries, enough to cross the 1 MB threshold once.
	padding := strings.Repeat("x", 1024)
	for i := 0; i < 1500; i++ {
		if err := writer.Write(Entry{
			Level:   LevelInfo,
			Message: padding,
			Time:    time.Now(),
		}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf("expected a rotated file at %s.1: %v", path, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat current file: %v", err)
	}
	if info.Size() > 1024*1024 {
		t.Errorf("current file is %d bytes, above the 1 MB limit", info.Size())
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	cfg := testConfig(t)
	manager, _ := NewManager(cfg)

	if err := manager.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
