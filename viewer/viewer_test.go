package viewer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ErfanMohseni20/GoLogViewer/logger"
)

func newTestViewer(t *testing.T, mutate func(*Options)) (*Viewer, string) {
	t.Helper()

	dir := t.TempDir()
	writeSampleLog(t, dir, "app.log")

	opts := Options{PathPrefix: "/logs", LogDir: dir}
	if mutate != nil {
		mutate(&opts)
	}
	return New(opts), dir
}

func writeSampleLog(t *testing.T, dir, name string) {
	t.Helper()

	base := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	var sb strings.Builder
	levels := []logger.Level{logger.LevelInfo, logger.LevelError, logger.LevelWarning}

	for i := 0; i < 9; i++ {
		entry := logger.Entry{
			Level:   levels[i%len(levels)],
			Channel: "daily",
			Message: "entry " + string(rune('a'+i)),
			Time:    base.Add(time.Duration(i) * time.Second),
		}
		data, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(data)
		sb.WriteByte('\n')
	}

	if err := os.WriteFile(filepath.Join(dir, name), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func do(v *Viewer, method, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	recorder := httptest.NewRecorder()
	v.ServeHTTP(recorder, request)
	return recorder
}

func TestViewerServesPage(t *testing.T) {
	v, _ := newTestViewer(t, nil)

	response := do(v, http.MethodGet, "/logs")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}

	body := response.Body.String()
	for _, want := range []string{"<title>", "/logs/static/app.js", `id="config"`, "emergency"} {
		if !strings.Contains(body, want) {
			t.Errorf("page is missing %q", want)
		}
	}
}

func TestViewerServesStaticAssets(t *testing.T) {
	v, _ := newTestViewer(t, nil)

	for _, path := range []string{"/logs/static/style.css", "/logs/static/app.js"} {
		response := do(v, http.MethodGet, path)
		if response.Code != http.StatusOK {
			t.Errorf("%s status = %d, want 200", path, response.Code)
		}
		if response.Body.Len() == 0 {
			t.Errorf("%s served an empty body", path)
		}
	}
}

func TestFilesAPI(t *testing.T) {
	v, _ := newTestViewer(t, nil)

	response := do(v, http.MethodGet, "/logs/api/files")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}

	var payload struct {
		Files []logger.LogFile `json:"files"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Files) != 1 || payload.Files[0].Name != "app.log" {
		t.Errorf("files = %+v", payload.Files)
	}
}

func TestEntriesAPI(t *testing.T) {
	v, _ := newTestViewer(t, nil)

	response := do(v, http.MethodGet, "/logs/api/entries?file=app.log&per_page=4")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}

	var result logger.QueryResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Total != 9 {
		t.Errorf("total = %d, want 9", result.Total)
	}
	if len(result.Entries) != 4 {
		t.Errorf("returned %d entries, want 4", len(result.Entries))
	}
	if result.LevelCounts["error"] != 3 {
		t.Errorf("level counts = %v", result.LevelCounts)
	}
}

func TestEntriesAPIRequiresFile(t *testing.T) {
	v, _ := newTestViewer(t, nil)

	if response := do(v, http.MethodGet, "/logs/api/entries"); response.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", response.Code)
	}
}

// An unrecognised level is a client error, not a silent fall back to debug.
func TestEntriesAPIRejectsInvalidLevel(t *testing.T) {
	v, _ := newTestViewer(t, nil)

	response := do(v, http.MethodGet, "/logs/api/entries?file=app.log&level=bogus")
	if response.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", response.Code)
	}
}

func TestEntriesAPIRejectsTraversal(t *testing.T) {
	v, _ := newTestViewer(t, nil)

	for _, name := range []string{"../../etc/passwd", "%2e%2e%2fpasswd", "/etc/passwd"} {
		response := do(v, http.MethodGet, "/logs/api/entries?file="+name)
		if response.Code != http.StatusBadRequest {
			t.Errorf("file=%q returned %d, want 400", name, response.Code)
		}
	}
}

func TestBasicAuthChallengesAndAccepts(t *testing.T) {
	v, _ := newTestViewer(t, func(o *Options) {
		o.BasicAuthUser = "admin"
		o.BasicAuthPassword = "s3cret"
	})

	response := do(v, http.MethodGet, "/logs")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", response.Code)
	}
	if !strings.HasPrefix(response.Header().Get("WWW-Authenticate"), "Basic ") {
		t.Error("missing a Basic auth challenge header")
	}

	request := httptest.NewRequest(http.MethodGet, "/logs", nil)
	request.SetBasicAuth("admin", "wrong")
	recorder := httptest.NewRecorder()
	v.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("wrong password status = %d, want 401", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/logs", nil)
	request.SetBasicAuth("admin", "s3cret")
	recorder = httptest.NewRecorder()
	v.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Errorf("correct credentials status = %d, want 200", recorder.Code)
	}
}

func TestAuthorizeHookBlocksEveryRoute(t *testing.T) {
	v, _ := newTestViewer(t, func(o *Options) {
		o.Authorize = func(r *http.Request) bool {
			return r.Header.Get("X-Admin") == "yes"
		}
	})

	// The API and the static assets must be gated too, not just the page.
	for _, path := range []string{"/logs", "/logs/api/files", "/logs/api/entries?file=app.log", "/logs/static/app.js"} {
		if response := do(v, http.MethodGet, path); response.Code != http.StatusForbidden {
			t.Errorf("%s returned %d, want 403", path, response.Code)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/logs/api/files", nil)
	request.Header.Set("X-Admin", "yes")
	recorder := httptest.NewRecorder()
	v.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Errorf("authorized request returned %d, want 200", recorder.Code)
	}
}

func TestReadOnlyDisablesMutations(t *testing.T) {
	v, dir := newTestViewer(t, func(o *Options) { o.ReadOnly = true })

	if response := do(v, http.MethodDelete, "/logs/api/files?file=app.log"); response.Code == http.StatusOK {
		t.Error("delete succeeded in read-only mode")
	}
	if response := do(v, http.MethodPost, "/logs/api/clear"); response.Code == http.StatusOK {
		t.Error("clear succeeded in read-only mode")
	}

	if _, err := os.Stat(filepath.Join(dir, "app.log")); err != nil {
		t.Error("the log file was removed in read-only mode")
	}

	// The read-only page must not render the destructive controls.
	if body := do(v, http.MethodGet, "/logs").Body.String(); strings.Contains(body, "Delete all") {
		t.Error("read-only page still renders the delete controls")
	}
}

func TestDeleteAndClear(t *testing.T) {
	v, dir := newTestViewer(t, nil)
	writeSampleLog(t, dir, "second.log")

	if response := do(v, http.MethodDelete, "/logs/api/files?file=second.log"); response.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", response.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, "second.log")); !os.IsNotExist(err) {
		t.Error("the file was not deleted")
	}

	// Clearing one file truncates it so a driver's open descriptor stays valid.
	if response := do(v, http.MethodPost, "/logs/api/clear?file=app.log"); response.Code != http.StatusOK {
		t.Fatalf("clear status = %d, want 200", response.Code)
	}
	info, err := os.Stat(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatalf("the cleared file should still exist: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("cleared file is %d bytes, want 0", info.Size())
	}
}

func TestDownload(t *testing.T) {
	v, _ := newTestViewer(t, nil)

	response := do(v, http.MethodGet, "/logs/api/download?file=app.log")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if disposition := response.Header().Get("Content-Disposition"); !strings.Contains(disposition, "app.log") {
		t.Errorf("Content-Disposition = %q", disposition)
	}
	if response.Body.Len() == 0 {
		t.Error("download served an empty body")
	}
}

func TestDisableDownloadRemovesRoute(t *testing.T) {
	v, _ := newTestViewer(t, func(o *Options) { o.DisableDownload = true })

	if response := do(v, http.MethodGet, "/logs/api/download?file=app.log"); response.Code == http.StatusOK {
		t.Error("download succeeded although it was disabled")
	}
}

func TestNormalizePrefix(t *testing.T) {
	cases := map[string]string{
		"":             "/logs",
		"/":            "/logs",
		"logs":         "/logs",
		"/logs/":       "/logs",
		"/admin/logs/": "/admin/logs",
		"  /debug  ":   "/debug",
	}

	for input, want := range cases {
		if got := normalizePrefix(input); got != want {
			t.Errorf("normalizePrefix(%q) = %q, want %q", input, got, want)
		}
	}
}

// A custom prefix must be reflected everywhere the page and the routes refer
// to it; v1's middleware hard-coded /logs and broke under a custom mount.
func TestCustomPrefixIsUsedThroughout(t *testing.T) {
	dir := t.TempDir()
	writeSampleLog(t, dir, "app.log")

	v := New(Options{PathPrefix: "/admin/logs", LogDir: dir})

	if response := do(v, http.MethodGet, "/admin/logs/api/files"); response.Code != http.StatusOK {
		t.Fatalf("api status = %d, want 200", response.Code)
	}

	body := do(v, http.MethodGet, "/admin/logs").Body.String()
	if !strings.Contains(body, "/admin/logs/static/app.js") {
		t.Error("the page does not build asset URLs from the custom prefix")
	}
	if strings.Contains(body, `"prefix":"/logs"`) {
		t.Error("the client config still carries the default prefix")
	}
}

func TestStreamSendsEventStreamHeaders(t *testing.T) {
	v, dir := newTestViewer(t, func(o *Options) { o.MaxTailIntervalMS = 10 })

	request := httptest.NewRequest(http.MethodGet, "/logs/api/stream?file=app.log", nil)
	ctx, cancel := contextWithTimeout(request, 150*time.Millisecond)
	defer cancel()
	request = request.WithContext(ctx)

	// Append while the stream is open so it has something to emit.
	go func() {
		time.Sleep(30 * time.Millisecond)
		file, err := os.OpenFile(filepath.Join(dir, "app.log"), os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer file.Close()
		_, _ = file.WriteString(`{"level":"info","message":"live","time":"2026-06-09T10:00:20Z"}` + "\n")
	}()

	recorder := httptest.NewRecorder()
	v.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "event: entries") {
		t.Errorf("stream body did not contain an entries event: %q", body)
	}
}

// Registering the viewer must not collide with the application's own routes.
// Method-less patterns for the prefix conflict with a caller's "GET /", which
// ServeMux reports by panicking at registration time.
func TestRegisterCoexistsWithApplicationRoutes(t *testing.T) {
	dir := t.TempDir()
	writeSampleLog(t, dir, "app.log")

	mux := http.NewServeMux()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("registering the viewer alongside an application route panicked: %v", r)
		}
	}()

	Register(mux, Options{PathPrefix: "/logs", LogDir: dir})

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("app index"))
	})
	mux.HandleFunc("POST /orders", func(w http.ResponseWriter, r *http.Request) {})

	cases := map[string]string{
		"/":               "app index",
		"/logs/api/files": `"files"`,
	}
	for target, want := range cases {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusOK {
			t.Errorf("%s returned %d, want 200", target, recorder.Code)
		}
		if !strings.Contains(recorder.Body.String(), want) {
			t.Errorf("%s body = %q, want it to contain %q", target, recorder.Body.String(), want)
		}
	}

	// The viewer's own page must still be reachable at the bare prefix.
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/logs", nil))
	if recorder.Code != http.StatusOK {
		t.Errorf("/logs returned %d, want 200", recorder.Code)
	}
}
