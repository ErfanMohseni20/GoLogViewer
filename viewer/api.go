package viewer

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ErfanMohseni20/GoLogViewer/logger"
)

func (v *Viewer) handleFiles(w http.ResponseWriter, r *http.Request) {
	files, err := logger.ListLogFiles(v.opts.LogDir)
	if err != nil {
		writeJSONError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"files":   files,
		"log_dir": v.opts.LogDir,
	})
}

func (v *Viewer) handleEntries(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r)
	if err != nil {
		writeJSONError(w, err)
		return
	}
	if query.File == "" {
		badRequest(w, "file parameter is required")
		return
	}

	result, err := logger.ReadEntries(v.opts.LogDir, query)
	if err != nil {
		writeJSONError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (v *Viewer) handleDownload(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("file")

	file, info, err := logger.OpenLogFile(v.opts.LogDir, name)
	if err != nil {
		writeJSONError(w, err)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	// The filename is validated by OpenLogFile and cannot contain a quote or
	// a path separator, so it is safe to interpolate here.
	w.Header().Set("Content-Disposition", `attachment; filename="`+info.Name()+`"`)

	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}

func (v *Viewer) handleDelete(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("file")
	if name == "" {
		badRequest(w, "file parameter is required")
		return
	}

	if err := logger.DeleteLogFile(v.opts.LogDir, name); err != nil {
		writeJSONError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"deleted": name})
}

func (v *Viewer) handleClear(w http.ResponseWriter, r *http.Request) {
	// Clearing one file truncates it so the driver's open descriptor keeps
	// writing to the same inode; clearing everything removes the files.
	if name := r.URL.Query().Get("file"); name != "" {
		if err := logger.TruncateLogFile(v.opts.LogDir, name); err != nil {
			writeJSONError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"cleared": name})
		return
	}

	removed, err := logger.DeleteAllLogFiles(v.opts.LogDir)
	if err != nil {
		writeJSONError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"removed": removed})
}

// parseQuery converts request parameters into a logger.Query, rejecting
// malformed values instead of silently substituting defaults for them.
func parseQuery(r *http.Request) (logger.Query, error) {
	params := r.URL.Query()

	query := logger.Query{
		File:   params.Get("file"),
		Search: params.Get("search"),
	}

	for _, raw := range params["level"] {
		for _, name := range strings.Split(raw, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			level, err := logger.ParseLevel(name)
			if err != nil {
				return query, err
			}
			query.Levels = append(query.Levels, level)
		}
	}

	query.Page = atoiDefault(params.Get("page"), 1)
	query.PerPage = atoiDefault(params.Get("per_page"), 50)

	if from := params.Get("from"); from != "" {
		parsed, err := parseTimeParam(from)
		if err != nil {
			return query, err
		}
		query.From = parsed
	}
	if to := params.Get("to"); to != "" {
		parsed, err := parseTimeParam(to)
		if err != nil {
			return query, err
		}
		query.To = parsed
	}

	return query, nil
}

// parseTimeParam accepts the value produced by an <input type="datetime-local">
// as well as a full RFC 3339 timestamp.
func parseTimeParam(value string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02",
	}

	var lastErr error
	for _, layout := range layouts {
		parsed, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return parsed, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

func atoiDefault(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// constantTimeMatch compares credentials without leaking their length
// relationship through timing.
func constantTimeMatch(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
