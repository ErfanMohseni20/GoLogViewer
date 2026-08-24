package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func benchManager(b *testing.B, mutate func(*ChannelConfig)) *Manager {
	b.Helper()

	channel := ChannelConfig{Driver: DriverSingle, Path: "bench.log", Level: LevelDebug}
	if mutate != nil {
		mutate(&channel)
	}

	manager, err := NewManager(Config{
		Default:    "bench",
		LogDir:     b.TempDir(),
		Channels:   map[string]ChannelConfig{"bench": channel},
		RedactKeys: []string{},
	})
	if err != nil {
		b.Fatalf("NewManager: %v", err)
	}
	b.Cleanup(func() { _ = manager.Close() })
	return manager
}

// BenchmarkWriteUnbuffered mirrors what v1 did on every single call: open the
// file, write one line, close it.
func BenchmarkWriteUnbuffered(b *testing.B) {
	log := benchManager(b, nil).MustChannel("")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = log.Info("benchmark message", "iteration", i, "key", "value")
	}
}

func BenchmarkWriteBuffered(b *testing.B) {
	log := benchManager(b, func(c *ChannelConfig) { c.Buffered = true }).MustChannel("")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = log.Info("benchmark message", "iteration", i, "key", "value")
	}
}

func BenchmarkWriteAsync(b *testing.B) {
	log := benchManager(b, func(c *ChannelConfig) {
		c.Buffered = true
		c.Async = true
		c.AsyncBufferSize = 65536
	}).MustChannel("")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = log.Info("benchmark message", "iteration", i, "key", "value")
	}
}

func BenchmarkWriteParallel(b *testing.B) {
	log := benchManager(b, func(c *ChannelConfig) { c.Buffered = true }).MustChannel("")

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = log.Info("benchmark message", "key", "value")
		}
	})
}

// BenchmarkReadPage measures serving one page from a large file, which is the
// case where v1 decoded and sorted the whole file into memory.
func BenchmarkReadPage(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "big.log")

	file, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	base := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	padding := strings.Repeat("x", 120)
	for i := 0; i < 200_000; i++ {
		fmt.Fprintf(file, `{"level":"info","channel":"daily","message":"entry %d %s","time":"%s"}`+"\n",
			i, padding, base.Add(time.Duration(i)*time.Second).Format(time.RFC3339))
	}
	_ = file.Close()

	info, _ := os.Stat(path)
	b.Logf("log file is %.1f MB", float64(info.Size())/(1024*1024))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Tail(dir, "big.log", 50); err != nil {
			b.Fatal(err)
		}
	}
}
