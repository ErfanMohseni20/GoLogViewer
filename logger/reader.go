package logger

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ErrInvalidFile is returned when a requested log file name escapes the log
// directory or is not a log file.
var ErrInvalidFile = errors.New("logger: invalid log file")

// LogFile describes one file in the log directory.
type LogFile struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
}

// Query describes a viewer request for log entries.
type Query struct {
	File    string
	Levels  []Level
	Search  string
	From    time.Time
	To      time.Time
	Page    int
	PerPage int
}

// QueryResult is one page of entries plus the counts the viewer displays.
type QueryResult struct {
	Entries     []Entry        `json:"entries"`
	Total       int            `json:"total"`
	Page        int            `json:"page"`
	PerPage     int            `json:"per_page"`
	TotalPages  int            `json:"total_pages"`
	LevelCounts map[string]int `json:"level_counts"`
	FileSize    int64          `json:"file_size"`
	Truncated   bool           `json:"truncated"`
}

const (
	defaultPerPage = 50
	maxPerPage     = 500

	// maxScanEntries bounds how many matching lines a single query will walk
	// before giving up on an exact total. Without it, a filter that matches
	// nothing in a 10 GB file would read the entire thing to prove it.
	maxScanEntries = 500_000
)

// ListLogFiles returns the log files in dir, newest first. Size-rotated
// backups (app.log.1) are included so they remain reachable from the viewer.
func ListLogFiles(dir string) ([]LogFile, error) {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return nil, fmt.Errorf("logger: create log dir: %w", err)
	}

	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("logger: read log dir: %w", err)
	}

	files := make([]LogFile, 0, len(dirEntries))
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() || !isLogFileName(dirEntry.Name()) {
			continue
		}

		info, err := dirEntry.Info()
		if err != nil {
			continue
		}

		files = append(files, LogFile{
			Name:     dirEntry.Name(),
			Size:     info.Size(),
			Modified: info.ModTime().Unix(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].Modified != files[j].Modified {
			return files[i].Modified > files[j].Modified
		}
		return files[i].Name > files[j].Name
	})

	return files, nil
}

// isLogFileName accepts "app.log" and size-rotated "app.log.1".
func isLogFileName(name string) bool {
	if strings.HasSuffix(name, ".log") {
		return true
	}
	base := name
	if idx := strings.LastIndexByte(name, '.'); idx > 0 {
		base = name[:idx]
	}
	return strings.HasSuffix(base, ".log")
}

// ResolveLogPath joins dir and name after verifying that name cannot escape
// the directory. Callers must never join untrusted names themselves.
func ResolveLogPath(dir, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%w: empty name", ErrInvalidFile)
	}
	// Reject anything with a path separator outright rather than relying on
	// Base to sanitise it, so traversal attempts are visible as errors.
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return "", fmt.Errorf("%w: %q", ErrInvalidFile, name)
	}
	if !isLogFileName(name) {
		return "", fmt.Errorf("%w: %q is not a log file", ErrInvalidFile, name)
	}
	return filepath.Join(dir, name), nil
}

// ReadEntries returns one page of entries, newest first.
//
// It walks the file backwards and keeps only the requested page in memory, so
// peak allocation is bounded by PerPage rather than by the file size. Level
// counts are gathered during the same walk.
func ReadEntries(dir string, query Query) (QueryResult, error) {
	query = query.normalize()

	result := QueryResult{
		Entries:     []Entry{},
		Page:        query.Page,
		PerPage:     query.PerPage,
		LevelCounts: map[string]int{},
	}

	path, err := ResolveLogPath(dir, query.File)
	if err != nil {
		return result, err
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, fmt.Errorf("logger: open log file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return result, fmt.Errorf("logger: stat log file: %w", err)
	}
	result.FileSize = info.Size()

	matcher := query.matcher()

	skip := (query.Page - 1) * query.PerPage
	matched := 0
	scanned := 0

	scanErr := scanReverse(file, info.Size(), func(line []byte) error {
		scanned++
		if scanned > maxScanEntries {
			result.Truncated = true
			return errStopScan
		}

		entry, ok := decodeLine(line)
		if !ok {
			return nil
		}

		result.LevelCounts[entry.Level.String()]++

		if !matcher(entry) {
			return nil
		}

		matched++
		if matched > skip && len(result.Entries) < query.PerPage {
			result.Entries = append(result.Entries, entry)
		}
		return nil
	})

	if scanErr != nil && !errors.Is(scanErr, errStopScan) {
		return result, fmt.Errorf("logger: scan log file: %w", scanErr)
	}

	result.Total = matched
	if matched > 0 {
		result.TotalPages = (matched + query.PerPage - 1) / query.PerPage
	}

	return result, nil
}

// Tail returns the last n entries of a file, newest first. It powers the
// viewer's live tail without re-reading the whole file on every poll.
func Tail(dir, name string, n int) ([]Entry, error) {
	if n <= 0 {
		n = 100
	}
	if n > maxPerPage {
		n = maxPerPage
	}

	path, err := ResolveLogPath(dir, name)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, fmt.Errorf("logger: open log file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("logger: stat log file: %w", err)
	}

	entries := make([]Entry, 0, n)
	scanErr := scanReverse(file, info.Size(), func(line []byte) error {
		entry, ok := decodeLine(line)
		if !ok {
			return nil
		}
		entries = append(entries, entry)
		if len(entries) >= n {
			return errStopScan
		}
		return nil
	})

	if scanErr != nil && !errors.Is(scanErr, errStopScan) {
		return nil, fmt.Errorf("logger: scan log file: %w", scanErr)
	}
	return entries, nil
}

func (q Query) normalize() Query {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PerPage < 1 {
		q.PerPage = defaultPerPage
	}
	if q.PerPage > maxPerPage {
		q.PerPage = maxPerPage
	}
	q.Search = strings.ToLower(strings.TrimSpace(q.Search))
	return q
}

// matcher compiles the query into a single predicate so the scan loop does not
// re-evaluate which filters are active for every line.
func (q Query) matcher() func(Entry) bool {
	var levelSet map[Level]bool
	if len(q.Levels) > 0 {
		levelSet = make(map[Level]bool, len(q.Levels))
		for _, level := range q.Levels {
			levelSet[level] = true
		}
	}

	hasFrom := !q.From.IsZero()
	hasTo := !q.To.IsZero()
	search := q.Search

	return func(entry Entry) bool {
		if levelSet != nil && !levelSet[entry.Level] {
			return false
		}
		if hasFrom && entry.Time.Before(q.From) {
			return false
		}
		if hasTo && entry.Time.After(q.To) {
			return false
		}
		if search != "" && !entryMatchesSearch(entry, search) {
			return false
		}
		return true
	}
}

// decodeLine parses one log line. Structured JSON is the native format; a line
// that is not JSON falls back to the plain-text parser so the viewer can also
// browse logs written by Laravel itself or by another tool.
func decodeLine(line []byte) (Entry, bool) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return Entry{}, false
	}

	if trimmed[0] == '{' {
		var entry Entry
		if err := json.Unmarshal([]byte(trimmed), &entry); err == nil {
			if entry.Level == levelUnset {
				entry.Level = LevelInfo
			}
			return entry, true
		}
	}

	return parsePlainLine(trimmed)
}

// laravelLinePattern matches Laravel's own text format:
//
//	[2026-06-09 10:06:31] local.ERROR: Something failed {"exception":"..."}
var laravelLinePattern = regexp.MustCompile(
	`^\[(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:[+-]\d{2}:?\d{2}|Z)?)\]\s*(?:([\w\-]+)\.)?([A-Za-z]+)\s*:\s*(.*)$`,
)

var plainTimeLayouts = []string{
	"2006-01-02 15:04:05.000000",
	"2006-01-02 15:04:05",
	time.RFC3339Nano,
	time.RFC3339,
}

// parsePlainLine converts a non-JSON line into an Entry, keeping the original
// text in Raw so nothing is lost when the format is unrecognised.
func parsePlainLine(line string) (Entry, bool) {
	match := laravelLinePattern.FindStringSubmatch(line)
	if match == nil {
		return Entry{
			Level:   LevelInfo,
			Message: line,
			Raw:     line,
		}, true
	}

	entry := Entry{
		Level:   LevelInfo,
		Channel: match[2],
		Message: strings.TrimSpace(match[4]),
		Raw:     line,
	}

	if level, err := ParseLevel(match[3]); err == nil {
		entry.Level = level
	}

	timestamp := strings.Replace(match[1], " ", "T", 1)
	for _, layout := range plainTimeLayouts {
		candidate := match[1]
		if strings.Contains(layout, "T") {
			candidate = timestamp
		}
		if parsed, err := time.ParseInLocation(layout, candidate, time.Local); err == nil {
			entry.Time = parsed
			break
		}
	}

	return entry, true
}

func entryMatchesSearch(entry Entry, search string) bool {
	if containsFold(entry.Message, search) ||
		containsFold(entry.Exception, search) ||
		containsFold(entry.Channel, search) ||
		containsFold(entry.Caller, search) ||
		containsFold(entry.Level.String(), search) {
		return true
	}

	for key, value := range entry.Context {
		if containsFold(key, search) || containsFold(formatValue(value), search) {
			return true
		}
	}
	return false
}

// containsFold reports whether needle (already lowercased by the caller)
// appears in haystack, ignoring case.
func containsFold(haystack, needle string) bool {
	if haystack == "" {
		return false
	}
	return strings.Contains(strings.ToLower(haystack), needle)
}

func formatValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprintf("%v", value)
		}
		return string(data)
	}
}
