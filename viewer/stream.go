package viewer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ErfanMohseni20/GoLogViewer/logger"
)

// handleStream serves a Server-Sent Events feed of new entries appended to a
// file. Polling the file size is enough to know something was written, and it
// avoids an inotify dependency and its per-platform behaviour.
func (v *Viewer) handleStream(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("file")
	if name == "" {
		badRequest(w, "file parameter is required")
		return
	}

	path, err := logger.ResolveLogPath(v.opts.LogDir, name)
	if err != nil {
		writeJSONError(w, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	// Without this, a reverse proxy may buffer the stream into uselessness.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	interval := time.Duration(v.opts.MaxTailIntervalMS) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Start from the current end of file so a client only sees what arrives
	// after it connected; the initial page load already rendered the backlog.
	lastSize := fileSize(path)

	// A keepalive comment stops idle proxies from dropping the connection.
	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return

		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()

		case <-ticker.C:
			size := fileSize(path)
			if size == lastSize {
				continue
			}

			// A shrinking file means the log was rotated or cleared; resync
			// to the new end rather than replaying from a stale offset.
			if size < lastSize {
				lastSize = size
				if err := writeEvent(w, "reset", map[string]any{"file": name}); err != nil {
					return
				}
				flusher.Flush()
				continue
			}

			entries, err := logger.Tail(v.opts.LogDir, name, 100)
			if err != nil {
				return
			}
			lastSize = size

			if len(entries) > 0 {
				if err := writeEvent(w, "entries", entries); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

func writeEvent(w http.ResponseWriter, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	return err
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
