package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeLogFile creates a log file whose entries are ordered oldest-first, the
// way a driver appends them.
func writeLogFile(t *testing.T, dir, name string, entries []Entry) string {
	t.Helper()

	path := filepath.Join(dir, name)
	var sb strings.Builder
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("marshal entry: %v", err)
		}
		sb.Write(data)
		sb.WriteByte('\n')
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	return path
}

func sampleEntries(n int) []Entry {
	base := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	levels := []Level{LevelDebug, LevelInfo, LevelWarning, LevelError}

	entries := make([]Entry, n)
	for i := range entries {
		entries[i] = Entry{
			Level:   levels[i%len(levels)],
			Channel: "daily",
			Message: fmt.Sprintf("message number %d", i),
			Context: map[string]any{"index": i},
			Time:    base.Add(time.Duration(i) * time.Second),
		}
	}
	return entries
}

func TestReadEntriesReturnsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "app.log", sampleEntries(10))

	result, err := ReadEntries(dir, Query{File: "app.log", PerPage: 5})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}

	if result.Total != 10 {
		t.Errorf("total = %d, want 10", result.Total)
	}
	if len(result.Entries) != 5 {
		t.Fatalf("returned %d entries, want 5", len(result.Entries))
	}
	if result.TotalPages != 2 {
		t.Errorf("total pages = %d, want 2", result.TotalPages)
	}

	// The newest entry is the last one written.
	if result.Entries[0].Message != "message number 9" {
		t.Errorf("first entry = %q, want the newest", result.Entries[0].Message)
	}
	for i := 1; i < len(result.Entries); i++ {
		if result.Entries[i].Time.After(result.Entries[i-1].Time) {
			t.Fatalf("entries are not in descending time order at index %d", i)
		}
	}
}

func TestReadEntriesPagination(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "app.log", sampleEntries(25))

	seen := map[string]bool{}
	for page := 1; page <= 3; page++ {
		result, err := ReadEntries(dir, Query{File: "app.log", Page: page, PerPage: 10})
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}

		want := 10
		if page == 3 {
			want = 5
		}
		if len(result.Entries) != want {
			t.Errorf("page %d returned %d entries, want %d", page, len(result.Entries), want)
		}

		for _, entry := range result.Entries {
			if seen[entry.Message] {
				t.Errorf("entry %q appeared on more than one page", entry.Message)
			}
			seen[entry.Message] = true
		}
	}

	if len(seen) != 25 {
		t.Errorf("pagination covered %d distinct entries, want 25", len(seen))
	}
}

func TestReadEntriesLevelFilterAndCounts(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "app.log", sampleEntries(20))

	result, err := ReadEntries(dir, Query{File: "app.log", Levels: []Level{LevelError}})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}

	if result.Total != 5 {
		t.Errorf("filtered total = %d, want 5", result.Total)
	}
	for _, entry := range result.Entries {
		if entry.Level != LevelError {
			t.Errorf("filter leaked a %v entry", entry.Level)
		}
	}

	// Counts describe the whole file, not the filtered subset, so the UI can
	// show what is available behind each filter.
	if result.LevelCounts["info"] != 5 || result.LevelCounts["debug"] != 5 {
		t.Errorf("level counts = %v, want 5 of each", result.LevelCounts)
	}
}

func TestReadEntriesMultiLevelFilter(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "app.log", sampleEntries(20))

	result, err := ReadEntries(dir, Query{
		File:   "app.log",
		Levels: []Level{LevelError, LevelWarning},
	})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if result.Total != 10 {
		t.Errorf("total = %d, want 10", result.Total)
	}
}

func TestReadEntriesSearch(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "app.log", []Entry{
		{Level: LevelInfo, Message: "user login", Context: map[string]any{"email": "a@example.com"}, Time: time.Now()},
		{Level: LevelInfo, Message: "cache miss", Time: time.Now()},
		{Level: LevelError, Message: "payment failed", Exception: "gateway timeout", Time: time.Now()},
	})

	cases := map[string]int{
		"login":          1,
		"EXAMPLE.COM":    1, // context values, case-insensitively
		"gateway":        1, // exception text
		"payment failed": 1,
		"nothing here":   0,
	}

	for term, want := range cases {
		result, err := ReadEntries(dir, Query{File: "app.log", Search: term})
		if err != nil {
			t.Fatalf("search %q: %v", term, err)
		}
		if result.Total != want {
			t.Errorf("search %q matched %d entries, want %d", term, result.Total, want)
		}
	}
}

func TestReadEntriesTimeRange(t *testing.T) {
	dir := t.TempDir()
	entries := sampleEntries(10) // one second apart from 10:00:00
	writeLogFile(t, dir, "app.log", entries)

	result, err := ReadEntries(dir, Query{
		File: "app.log",
		From: entries[3].Time,
		To:   entries[6].Time,
	})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if result.Total != 4 {
		t.Errorf("range matched %d entries, want 4", result.Total)
	}
}

// Traversal must be rejected outright rather than quietly sanitised, so a
// probe shows up as an error instead of returning an unrelated file.
func TestResolveLogPathRejectsTraversal(t *testing.T) {
	for _, name := range []string{
		"../../etc/passwd",
		"..",
		"/etc/passwd",
		`..\..\windows\system32\config\sam`,
		"subdir/app.log",
		"",
	} {
		if _, err := ResolveLogPath("/tmp/logs", name); err == nil {
			t.Errorf("ResolveLogPath accepted %q", name)
		}
	}

	if _, err := ResolveLogPath("/tmp/logs", "app.log"); err != nil {
		t.Errorf("ResolveLogPath rejected a valid name: %v", err)
	}
	if _, err := ResolveLogPath("/tmp/logs", "app.log.1"); err != nil {
		t.Errorf("ResolveLogPath rejected a rotated file: %v", err)
	}
}

func TestReadEntriesMissingFile(t *testing.T) {
	result, err := ReadEntries(t.TempDir(), Query{File: "absent.log"})
	if err != nil {
		t.Fatalf("a missing file should not be an error: %v", err)
	}
	if result.Total != 0 || len(result.Entries) != 0 {
		t.Error("a missing file should yield an empty result")
	}
}

func TestReadEntriesSkipsCorruptLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	content := `{"level":"info","message":"first","time":"2026-06-09T10:00:00Z"}
this line is not json at all
{"level":"error","message":"second","time":"2026-06-09T10:00:01Z"}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ReadEntries(dir, Query{File: "app.log"})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}

	// The malformed line is preserved as a plain-text entry rather than
	// dropped, so nothing in the file becomes invisible.
	if result.Total != 3 {
		t.Errorf("total = %d, want 3", result.Total)
	}
}

// Laravel's own text format should be browsable, not just this package's JSON.
func TestReadEntriesParsesLaravelTextFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "laravel.log")

	content := "[2026-06-09 10:06:31] local.ERROR: Database connection failed\n" +
		"[2026-06-09 10:06:32] production.INFO: Cache warmed\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ReadEntries(dir, Query{File: "laravel.log"})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if result.Total != 2 {
		t.Fatalf("total = %d, want 2", result.Total)
	}

	newest := result.Entries[0]
	if newest.Level != LevelInfo || newest.Channel != "production" {
		t.Errorf("newest entry = %+v", newest)
	}
	if newest.Message != "Cache warmed" {
		t.Errorf("message = %q", newest.Message)
	}
	if result.LevelCounts["error"] != 1 {
		t.Errorf("level counts = %v", result.LevelCounts)
	}
}

func TestListLogFilesOrdersByModifiedTime(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "old.log", sampleEntries(1))
	writeLogFile(t, dir, "new.log", sampleEntries(1))

	old := filepath.Join(dir, "old.log")
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}

	// A non-log file must not appear in the listing.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := ListLogFiles(dir)
	if err != nil {
		t.Fatalf("ListLogFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("listed %d files, want 2", len(files))
	}
	if files[0].Name != "new.log" {
		t.Errorf("first file = %q, want the newest", files[0].Name)
	}
}

func TestTailReturnsLastEntries(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "app.log", sampleEntries(50))

	entries, err := Tail(dir, "app.log", 5)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("returned %d entries, want 5", len(entries))
	}
	if entries[0].Message != "message number 49" {
		t.Errorf("first entry = %q, want the newest", entries[0].Message)
	}
}

// The reverse scanner must reassemble lines that straddle a chunk boundary.
func TestReverseScanAcrossChunkBoundaries(t *testing.T) {
	dir := t.TempDir()

	// Entries large enough that many of them cross the 64 KiB read window.
	padding := strings.Repeat("y", 3000)
	entries := make([]Entry, 200)
	base := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	for i := range entries {
		entries[i] = Entry{
			Level:   LevelInfo,
			Message: fmt.Sprintf("%04d-%s", i, padding),
			Time:    base.Add(time.Duration(i) * time.Second),
		}
	}
	writeLogFile(t, dir, "big.log", entries)

	result, err := ReadEntries(dir, Query{File: "big.log", PerPage: 500})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if result.Total != 200 {
		t.Fatalf("total = %d, want 200 — a line was lost at a chunk boundary", result.Total)
	}

	for i, entry := range result.Entries {
		want := fmt.Sprintf("%04d-", 199-i)
		if !strings.HasPrefix(entry.Message, want) {
			t.Fatalf("entry %d = %.10q, want prefix %q", i, entry.Message, want)
		}
	}
}

// A file whose final line has no trailing newline must not lose that line.
func TestReverseScanHandlesMissingTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	content := `{"level":"info","message":"first","time":"2026-06-09T10:00:00Z"}
{"level":"info","message":"last","time":"2026-06-09T10:00:01Z"}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ReadEntries(dir, Query{File: "app.log"})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("total = %d, want 2", result.Total)
	}
	if result.Entries[0].Message != "last" {
		t.Errorf("newest = %q, want last", result.Entries[0].Message)
	}
}

func TestReadEntriesEmptyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "empty.log"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ReadEntries(dir, Query{File: "empty.log"})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total = %d, want 0", result.Total)
	}
}
