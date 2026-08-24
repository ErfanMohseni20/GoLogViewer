// Command nethttp-example demonstrates GoLogViewer with the standard library
// only — no web framework involved.
//
//	go run ./examples/nethttp
//	open http://127.0.0.1:8080/logs
package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ErfanMohseni20/GoLogViewer/logger"
	"github.com/ErfanMohseni20/GoLogViewer/middleware"
	"github.com/ErfanMohseni20/GoLogViewer/viewer"
)

const viewerPath = "/logs"

func main() {
	cfg := logger.DefaultConfig()
	cfg.IncludeCaller = true

	if err := logger.Init(cfg); err != nil {
		log.Fatalf("init logger: %v", err)
	}
	defer logger.Shutdown()

	// Anything logging through log/slog — including third-party libraries —
	// now lands in the same files the viewer reads.
	slog.SetDefault(slog.New(logger.NewSlogHandler("")))

	mux := http.NewServeMux()

	viewer.Register(mux, viewer.Options{
		PathPrefix: viewerPath,
		LogDir:     cfg.ResolveLogDir(),
		Title:      "net/http Demo",
		// Reachable without credentials only because this binds to localhost.
		// Set BasicAuthUser/BasicAuthPassword for anything else.
		ReadOnly: false,
	})

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// Fields on the request context are attached automatically.
		_ = logger.Ctx(r.Context()).Info("index served")
		_, _ = w.Write([]byte("GoLogViewer demo — see " + viewerPath + "\n"))
	})

	mux.HandleFunc("GET /boom", func(w http.ResponseWriter, r *http.Request) {
		_ = logger.Ctx(r.Context()).Error("checkout failed",
			errors.New("inventory service timeout"), "sku", "A-1024")
		http.Error(w, "internal error", http.StatusInternalServerError)
	})

	handler := middleware.HTTP(middleware.Options{
		SkipPaths:            []string{viewerPath},
		SlowRequestThreshold: 500 * time.Millisecond,
	})(mux)

	slog.Info("seeding sample entries")
	_ = logger.Info("application started", "env", "local")
	_ = logger.Warning("cache driver fallback enabled")

	server := &http.Server{
		Addr:              ":" + envOr("PORT", "8080"),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("listening on %s — viewer at %s", server.Addr, viewerPath)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	// Shut down cleanly so buffered entries reach disk.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	log.Println("stopped")
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
