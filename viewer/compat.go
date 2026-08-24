package viewer

import "net/http"

// viewerMethods are the HTTP methods the viewer answers. Registration is
// per-method rather than method-less because a method-less pattern such as
// "/logs/" conflicts with a caller's own "GET /" route: ServeMux rejects a
// pattern that matches more methods but has a more specific path.
var viewerMethods = []string{http.MethodGet, http.MethodPost, http.MethodDelete}

// Register mounts a viewer on a net/http ServeMux and returns it.
//
// Two patterns are registered per method: the bare prefix for the page itself,
// and the subtree for the API, the stream and the static assets.
//
// Gin users should call ginlog.RegisterViewer instead.
func Register(mux *http.ServeMux, opts Options) *Viewer {
	v := New(opts)
	Mount(mux, v)
	return v
}

// Mount attaches an already-built viewer to a ServeMux.
func Mount(mux *http.ServeMux, v *Viewer) {
	prefix := v.Prefix()
	for _, method := range viewerMethods {
		mux.Handle(method+" "+prefix, v)
		mux.Handle(method+" "+prefix+"/", v)
	}
}

// DefaultOptions returns the viewer's defaults.
func DefaultOptions() Options {
	return Options{PathPrefix: DefaultPathPrefix}
}
