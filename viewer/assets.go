package viewer

import (
	"embed"
	"io/fs"
	"net/http"
	"sync"
)

//go:embed templates/index.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var (
	staticOnce    sync.Once
	staticHandled http.Handler
)

// staticHandler serves the embedded CSS and JS. The sub-filesystem is derived
// once and reused, since fs.Sub allocates a new value on every call.
func staticHandler() http.Handler {
	staticOnce.Do(func() {
		sub, err := fs.Sub(staticFS, "static")
		if err != nil {
			panic("viewer: load embedded static assets: " + err.Error())
		}
		staticHandled = cacheControl(http.FileServer(http.FS(sub)))
	})
	return staticHandled
}

// cacheControl lets browsers cache the assets briefly. They are versioned with
// the binary, so a short max-age keeps a redeploy from serving stale CSS.
func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=300")
		next.ServeHTTP(w, r)
	})
}
