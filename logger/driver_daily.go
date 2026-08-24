package logger

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const dateLayout = "2006-01-02"

// dailyDriver writes to a file named after the current date and prunes files
// older than the retention window.
type dailyDriver struct {
	name          string
	basePath      string
	minLevel      Level
	retentionDays int

	mu         sync.Mutex
	writer     *fileWriter
	activeDate string

	// now is swappable so the rollover and retention logic can be tested
	// without waiting for midnight.
	now func() time.Time

	cleanupWG sync.WaitGroup
}

func newDailyDriver(name, basePath string, minLevel Level, retentionDays int, opts fileWriterOptions) (*dailyDriver, error) {
	if retentionDays <= 0 {
		retentionDays = 14
	}

	d := &dailyDriver{
		name:          name,
		basePath:      basePath,
		minLevel:      minLevel,
		retentionDays: retentionDays,
		now:           time.Now,
	}

	today := d.now()
	writer, err := newFileWriter(dailyPath(basePath, today), opts)
	if err != nil {
		return nil, err
	}

	d.writer = writer
	d.activeDate = today.Format(dateLayout)
	return d, nil
}

// dailyPath turns storage/logs/laravel.log into
// storage/logs/laravel-2026-06-09.log, matching Laravel's naming.
func dailyPath(basePath string, date time.Time) string {
	dir := filepath.Dir(basePath)
	base := strings.TrimSuffix(filepath.Base(basePath), filepath.Ext(basePath))
	return filepath.Join(dir, base+"-"+date.Format(dateLayout)+".log")
}

func (d *dailyDriver) Name() string { return d.name }

func (d *dailyDriver) Log(entry Entry) error {
	if !entry.Level.Allows(d.minLevel) {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.now()
	if today := now.Format(dateLayout); today != d.activeDate {
		if err := d.writer.SetPath(dailyPath(d.basePath, now)); err != nil {
			return err
		}
		d.activeDate = today

		d.cleanupWG.Add(1)
		go func() {
			defer d.cleanupWG.Done()
			d.cleanupOldFiles(now)
		}()
	}

	entry.Channel = d.name
	return d.writer.Write(entry)
}

func (d *dailyDriver) Sync() error { return d.writer.Sync() }

func (d *dailyDriver) Close() error {
	d.cleanupWG.Wait()
	return d.writer.Close()
}

// cleanupOldFiles removes rotated files past the retention window. It decides
// by the date encoded in the filename rather than the modification time, which
// a backup or a file copy would otherwise reset.
func (d *dailyDriver) cleanupOldFiles(now time.Time) {
	dir := filepath.Dir(d.basePath)
	base := strings.TrimSuffix(filepath.Base(d.basePath), filepath.Ext(d.basePath))
	cutoff := now.AddDate(0, 0, -d.retentionDays)

	matches, err := filepath.Glob(filepath.Join(dir, base+"-*.log*"))
	if err != nil {
		return
	}

	for _, path := range matches {
		date, ok := dateFromDailyName(filepath.Base(path), base)
		if !ok {
			continue
		}
		if date.Before(cutoff) {
			_ = os.Remove(path)
		}
	}
}

// dateFromDailyName extracts the date from "laravel-2026-06-09.log" and from
// size-rotated variants such as "laravel-2026-06-09.log.1".
func dateFromDailyName(name, base string) (time.Time, bool) {
	rest, ok := strings.CutPrefix(name, base+"-")
	if !ok {
		return time.Time{}, false
	}
	if len(rest) < len(dateLayout) {
		return time.Time{}, false
	}

	date, err := time.Parse(dateLayout, rest[:len(dateLayout)])
	if err != nil {
		return time.Time{}, false
	}
	return date, true
}
