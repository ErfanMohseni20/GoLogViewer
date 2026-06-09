package logger

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type DailyDriver struct {
	name          string
	basePath      string
	minLevel      Level
	retentionDays int

	mu         sync.Mutex
	writer     *FileWriter
	activeDate string
}

func NewDailyDriver(name, basePath string, minLevel Level, retentionDays int) (*DailyDriver, error) {
	if retentionDays <= 0 {
		retentionDays = 14
	}

	writer, err := NewFileWriter(dailyPath(basePath, time.Now()))
	if err != nil {
		return nil, err
	}

	return &DailyDriver{
		name:          name,
		basePath:      basePath,
		minLevel:      minLevel,
		retentionDays: retentionDays,
		writer:        writer,
		activeDate:    time.Now().Format("2006-01-02"),
	}, nil
}

func dailyPath(basePath string, date time.Time) string {
	dir := filepath.Dir(basePath)
	base := strings.TrimSuffix(filepath.Base(basePath), ".log")
	return filepath.Join(dir, base+"-"+date.Format("2006-01-02")+".log")
}

func (d *DailyDriver) Name() string {
	return d.name
}

func (d *DailyDriver) Log(entry Entry) error {
	if !entry.Level.Allows(d.minLevel) {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if today != d.activeDate {
		d.activeDate = today
		if err := d.writer.SetPath(dailyPath(d.basePath, time.Now())); err != nil {
			return err
		}
		go d.cleanupOldFiles()
	}

	entry.Channel = d.name
	return d.writer.Write(entry)
}

func (d *DailyDriver) cleanupOldFiles() {
	dir := filepath.Dir(d.basePath)
	base := strings.TrimSuffix(filepath.Base(d.basePath), ".log")
	cutoff := time.Now().AddDate(0, 0, -d.retentionDays)

	entries, err := filepath.Glob(filepath.Join(dir, base+"-*.log"))
	if err != nil {
		return
	}

	for _, path := range entries {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(path)
		}
	}
}
