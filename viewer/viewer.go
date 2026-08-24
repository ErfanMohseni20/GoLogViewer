// Package viewer serves a web UI for browsing the log files written by the
// logger package.
//
// The viewer is a plain http.Handler, so it mounts on net/http, chi, echo or
// anything else that speaks the standard interface:
//
//	http.Handle("/logs/", viewer.New(viewer.Options{PathPrefix: "/logs"}))
//
// A thin Gin adapter lives in viewer/ginviewer for projects using Gin.
package viewer

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/ErfanMohseni20/GoLogViewer/logger"
)

// DefaultPathPrefix is where the viewer mounts unless configured otherwise.
const DefaultPathPrefix = "/logs"

// Options configures a viewer instance.
type Options struct {
	// PathPrefix is the URL prefix the viewer is mounted under. It must match
	// the pattern the router dispatches from, because links and API calls in
	// the page are built from it.
	PathPrefix string

	// LogDir is the directory to browse. Defaults to the active logger
	// configuration's directory.
	LogDir string

	// Title is shown in the page header and the browser tab.
	Title string

	// Authorize gates every request. Returning false yields 403 unless
	// BasicAuth is also configured, in which case a challenge is sent.
	// Leaving this nil and ReadOnly false means anyone who can reach the
	// route can read and delete logs — see the package README.
	Authorize func(r *http.Request) bool

	// BasicAuthUser and BasicAuthPassword enable HTTP basic authentication.
	// When set, they are checked before Authorize.
	BasicAuthUser     string
	BasicAuthPassword string

	// ReadOnly disables the delete and clear endpoints. Recommended for any
	// deployment where the viewer is reachable outside a trusted network.
	ReadOnly bool

	// DisableDownload removes the download endpoint.
	DisableDownload bool

	// MaxTailInterval bounds how often a live-tail stream polls the file.
	// Defaults to one second.
	MaxTailIntervalMS int
}

// Viewer is the http.Handler serving the log UI and its JSON API.
type Viewer struct {
	opts         Options
	prefix       string
	template     *template.Template
	mux          *http.ServeMux
	clientConfig template.JS
}

// New builds a viewer. It panics only if the embedded assets are corrupt,
// which would mean a broken build rather than a runtime condition.
func New(opts Options) *Viewer {
	opts.PathPrefix = normalizePrefix(opts.PathPrefix)
	if opts.LogDir == "" {
		opts.LogDir = logger.CurrentConfig().ResolveLogDir()
	}
	if opts.Title == "" {
		opts.Title = "GoLogViewer"
	}
	if opts.MaxTailIntervalMS <= 0 {
		opts.MaxTailIntervalMS = 1000
	}

	// Parsing once at construction avoids re-reading and re-compiling the
	// template on every page view, which the v1 handler did.
	tmpl, err := template.ParseFS(templateFS, "templates/index.html")
	if err != nil {
		panic("viewer: parse embedded template: " + err.Error())
	}

	v := &Viewer{
		opts:     opts,
		prefix:   opts.PathPrefix,
		template: tmpl,
	}

	// json.Marshal escapes <, > and & into \u00xx, so the result is safe to
	// place inside a script element verbatim.
	clientConfig, err := json.Marshal(map[string]any{
		"prefix":         opts.PathPrefix,
		"readOnly":       opts.ReadOnly,
		"canDownload":    !opts.DisableDownload,
		"tailIntervalMs": opts.MaxTailIntervalMS,
	})
	if err != nil {
		panic("viewer: encode client config: " + err.Error())
	}
	v.clientConfig = template.JS(clientConfig)

	v.mux = v.routes()
	return v
}

// Prefix returns the normalized mount path.
func (v *Viewer) Prefix() string { return v.prefix }

// LogDir returns the directory being browsed.
func (v *Viewer) LogDir() string { return v.opts.LogDir }

func (v *Viewer) routes() *http.ServeMux {
	mux := http.NewServeMux()
	p := v.prefix

	mux.HandleFunc("GET "+p, v.handlePage)
	mux.HandleFunc("GET "+p+"/", v.handlePage)
	mux.HandleFunc("GET "+p+"/api/files", v.handleFiles)
	mux.HandleFunc("GET "+p+"/api/entries", v.handleEntries)
	mux.HandleFunc("GET "+p+"/api/stream", v.handleStream)

	if !v.opts.DisableDownload {
		mux.HandleFunc("GET "+p+"/api/download", v.handleDownload)
	}
	if !v.opts.ReadOnly {
		mux.HandleFunc("DELETE "+p+"/api/files", v.handleDelete)
		mux.HandleFunc("POST "+p+"/api/clear", v.handleClear)
	}

	mux.Handle("GET "+p+"/static/", http.StripPrefix(p+"/static/", staticHandler()))

	// Anything else under /api must 404 rather than falling through to the
	// page handler, which would answer a disabled or misspelled endpoint with
	// 200 and a mouthful of HTML.
	notFound := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown endpoint"})
	}
	// Registered per method: a method-less pattern here would be broader than
	// "GET <prefix>/" and ServeMux rejects that combination as ambiguous.
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		mux.HandleFunc(method+" "+p+"/api/", notFound)
	}

	return mux
}

// ServeHTTP authorizes the request and dispatches it.
func (v *Viewer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !v.authorize(w, r) {
		return
	}
	v.mux.ServeHTTP(w, r)
}

// authorize applies basic auth then the custom hook, writing the rejection
// response itself and reporting whether the request may proceed.
func (v *Viewer) authorize(w http.ResponseWriter, r *http.Request) bool {
	if v.opts.BasicAuthUser != "" || v.opts.BasicAuthPassword != "" {
		user, password, ok := r.BasicAuth()
		if !ok || !constantTimeMatch(user, v.opts.BasicAuthUser) ||
			!constantTimeMatch(password, v.opts.BasicAuthPassword) {
			w.Header().Set("WWW-Authenticate", `Basic realm="`+v.opts.Title+`", charset="UTF-8"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return false
		}
	}

	if v.opts.Authorize != nil && !v.opts.Authorize(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}

	return true
}

func (v *Viewer) handlePage(w http.ResponseWriter, r *http.Request) {
	data := pageData{
		PathPrefix:  v.prefix,
		Title:       v.opts.Title,
		Levels:      logger.AllLevels(),
		ReadOnly:    v.opts.ReadOnly,
		CanDownload: !v.opts.DisableDownload,
		Config:      v.clientConfig,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The page is generated per request and reflects files that change
	// constantly; caching it would show stale state after a rotation.
	w.Header().Set("Cache-Control", "no-store")

	if err := v.template.Execute(w, data); err != nil {
		// Headers are already sent, so the best we can do is stop writing.
		return
	}
}

type pageData struct {
	PathPrefix  string
	Title       string
	Levels      []logger.Level
	ReadOnly    bool
	CanDownload bool

	// Config is a pre-marshalled JSON object embedded in a
	// <script type="application/json"> tag, which keeps app.js a cacheable
	// static asset instead of a template.
	Config template.JS
}

// normalizePrefix guarantees a leading slash and no trailing slash, so route
// patterns and the URLs the page builds agree.
func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || prefix == "/" {
		return DefaultPathPrefix
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return strings.TrimRight(prefix, "/")
}

// writeJSONError renders an API error, mapping the sentinel errors from the
// logger package onto appropriate status codes.
func writeJSONError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, logger.ErrInvalidFile):
		status = http.StatusBadRequest
	case errors.Is(err, logger.ErrInvalidLevel):
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func badRequest(w http.ResponseWriter, format string, args ...any) {
	writeJSON(w, http.StatusBadRequest, map[string]string{
		"error": fmt.Sprintf(format, args...),
	})
}
