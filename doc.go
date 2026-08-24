// Package gologviewer is a Laravel-style logging system for Go with an
// embedded web log viewer.
//
// The module is organised as a small set of packages:
//
//	logger              channels, drivers, structured entries, and the reader
//	viewer              the web UI as a plain http.Handler
//	viewer/ginviewer    a Gin adapter for the viewer
//	middleware          request logging for net/http and for Gin
//	server              the one-line Gin setup kept from v1
//
// A minimal net/http setup:
//
//	logger.Init(logger.DefaultConfig())
//	defer logger.Shutdown()
//
//	mux := http.NewServeMux()
//	viewer.Register(mux, viewer.Options{
//		PathPrefix:        "/logs",
//		BasicAuthUser:     "admin",
//		BasicAuthPassword: os.Getenv("LOG_VIEWER_PASSWORD"),
//	})
//
//	http.ListenAndServe(":8080", middleware.HTTP(middleware.Options{
//		SkipPaths: []string{"/logs"},
//	})(mux))
//
// See the examples directory for complete Gin and net/http programs.
package gologviewer
