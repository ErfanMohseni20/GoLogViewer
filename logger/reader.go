package logger

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type LogFile struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
}

type EntriesResult struct {
	Entries    []Entry `json:"entries"`
	Total      int     `json:"total"`
	Page       int     `json:"page"`
	PerPage    int     `json:"per_page"`
	TotalPages int     `json:"total_pages"`
}

func ListLogFiles(logDir string) ([]LogFile, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil, err
	}

	var files []LogFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, LogFile{
			Name:     entry.Name(),
			Size:     info.Size(),
			Modified: info.ModTime().Unix(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Modified > files[j].Modified
	})

	return files, nil
}

func ReadLogEntries(logDir, fileName, level, search string, page, perPage int) (EntriesResult, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 200 {
		perPage = 200
	}

	path := filepath.Join(logDir, filepath.Base(fileName))
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return EntriesResult{
				Page:    page,
				PerPage: perPage,
			}, nil
		}
		return EntriesResult{}, err
	}
	defer file.Close()

	filterLevel := ParseLevel(level)
	hasLevelFilter := level != ""
	search = strings.ToLower(strings.TrimSpace(search))

	var matched []Entry
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		if hasLevelFilter && entry.Level != filterLevel {
			continue
		}

		if search != "" && !entryMatchesSearch(entry, search) {
			continue
		}

		matched = append(matched, entry)
	}

	if err := scanner.Err(); err != nil {
		return EntriesResult{}, err
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Time.After(matched[j].Time)
	})

	total := len(matched)
	totalPages := 0
	if total > 0 {
		totalPages = (total + perPage - 1) / perPage
	}

	start := (page - 1) * perPage
	if start > total {
		start = total
	}

	end := start + perPage
	if end > total {
		end = total
	}

	slice := matched[start:end]
	if slice == nil {
		slice = []Entry{}
	}

	return EntriesResult{
		Entries:    slice,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}, nil
}

func entryMatchesSearch(entry Entry, search string) bool {
	if strings.Contains(strings.ToLower(entry.Message), search) {
		return true
	}
	if strings.Contains(strings.ToLower(entry.Exception), search) {
		return true
	}
	if strings.Contains(strings.ToLower(string(entry.Level)), search) {
		return true
	}
	if strings.Contains(strings.ToLower(entry.Channel), search) {
		return true
	}

	for key, value := range entry.Context {
		if strings.Contains(strings.ToLower(key), search) {
			return true
		}
		if strings.Contains(strings.ToLower(formatValue(value)), search) {
			return true
		}
	}

	return false
}

func formatValue(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}
