package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDailyDriverRollsOverAtMidnight(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "app.log")

	driver, err := newDailyDriver("daily", base, LevelDebug, 14, fileWriterOptions{})
	if err != nil {
		t.Fatalf("newDailyDriver: %v", err)
	}
	defer driver.Close()

	day1 := time.Date(2026, 6, 9, 23, 59, 0, 0, time.UTC)
	driver.now = func() time.Time { return day1 }
	// The writer opened at construction time; realign it with the fake clock.
	if err := driver.writer.SetPath(dailyPath(base, day1)); err != nil {
		t.Fatal(err)
	}
	driver.activeDate = day1.Format(dateLayout)

	if err := driver.Log(Entry{Level: LevelInfo, Message: "before midnight", Time: day1}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	day2 := day1.Add(2 * time.Minute)
	driver.now = func() time.Time { return day2 }
	if err := driver.Log(Entry{Level: LevelInfo, Message: "after midnight", Time: day2}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	if err := driver.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	for _, name := range []string{"app-2026-06-09.log", "app-2026-06-10.log"} {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", name)
		}
	}
}

// v1 pruned by modification time, so restoring a backup or copying the log
// directory reset every file's age and defeated the retention window.
func TestDailyCleanupUsesFilenameDateNotModTime(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "app.log")

	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)

	stale := filepath.Join(dir, "app-2026-06-01.log") // 19 days old
	fresh := filepath.Join(dir, "app-2026-06-18.log") // 2 days old
	rotated := filepath.Join(dir, "app-2026-06-01.log.1")
	unrelated := filepath.Join(dir, "other.log")

	for _, path := range []string{stale, fresh, rotated, unrelated} {
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		// Give every file a current mtime: if the sweep looked at mtime, none
		// of them would be removed and this test would fail.
		if err := os.Chtimes(path, now, now); err != nil {
			t.Fatal(err)
		}
	}

	driver, err := newDailyDriver("daily", base, LevelDebug, 14, fileWriterOptions{})
	if err != nil {
		t.Fatalf("newDailyDriver: %v", err)
	}
	defer driver.Close()

	driver.cleanupOldFiles(now)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("a file older than the retention window survived")
	}
	if _, err := os.Stat(rotated); !os.IsNotExist(err) {
		t.Error("a size-rotated backup of an expired day survived")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("a file inside the retention window was removed")
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Error("an unrelated log file was removed")
	}
}

func TestDailyPathNaming(t *testing.T) {
	date := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)

	cases := map[string]string{
		"storage/logs/laravel.log": "storage/logs/laravel-2026-06-09.log",
		"storage/logs/app.log":     "storage/logs/app-2026-06-09.log",
		"/var/log/svc.log":         "/var/log/svc-2026-06-09.log",
	}

	for input, want := range cases {
		if got := dailyPath(input, date); got != filepath.FromSlash(want) {
			t.Errorf("dailyPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDateFromDailyName(t *testing.T) {
	cases := []struct {
		name string
		base string
		ok   bool
		day  int
	}{
		{"app-2026-06-09.log", "app", true, 9},
		{"app-2026-06-09.log.3", "app", true, 9},
		{"app.log", "app", false, 0},
		{"other-2026-06-09.log", "app", false, 0},
		{"app-not-a-date.log", "app", false, 0},
	}

	for _, tc := range cases {
		date, ok := dateFromDailyName(tc.name, tc.base)
		if ok != tc.ok {
			t.Errorf("dateFromDailyName(%q) ok = %v, want %v", tc.name, ok, tc.ok)
			continue
		}
		if ok && date.Day() != tc.day {
			t.Errorf("dateFromDailyName(%q) day = %d, want %d", tc.name, date.Day(), tc.day)
		}
	}
}
