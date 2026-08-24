// Command gin-example demonstrates GoLogViewer inside a Gin application.
//
//	go run ./examples/gin
//	open http://127.0.0.1:8080/logs   (user: admin, password: secret)
package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/ErfanMohseni20/GoLogViewer/ginlog"
	"github.com/ErfanMohseni20/GoLogViewer/logger"
	"github.com/ErfanMohseni20/GoLogViewer/middleware"
	"github.com/ErfanMohseni20/GoLogViewer/viewer"
	"github.com/gin-gonic/gin"
)

const viewerPath = "/logs"

func main() {
	cfg := logger.DefaultConfig()
	cfg.LogDir = "storage/logs"
	cfg.IncludeCaller = true

	// Buffer and rotate the daily channel; the defaults already buffer, this
	// just shows where the knobs are.
	daily := cfg.Channels["daily"]
	daily.Days = 30
	daily.MaxSizeMB = 64
	daily.MaxBackups = 5
	cfg.Channels["daily"] = daily

	if err := logger.Init(cfg); err != nil {
		log.Fatalf("init logger: %v", err)
	}
	// Without this, buffered and async channels can lose their last entries.
	defer logger.Shutdown()

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(ginlog.Middleware(middleware.Options{
		SkipPaths:            []string{viewerPath},
		SlowRequestThreshold: 500 * time.Millisecond,
	}))

	ginlog.RegisterViewer(router, viewer.Options{
		PathPrefix:        viewerPath,
		LogDir:            cfg.ResolveLogDir(),
		Title:             "Demo Logs",
		BasicAuthUser:     "admin",
		BasicAuthPassword: envOr("LOG_VIEWER_PASSWORD", "secret"),
	})

	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "GoLogViewer demo", "viewer": viewerPath})
	})

	router.GET("/boom", func(c *gin.Context) {
		_ = logger.Error("payment gateway unreachable",
			errors.New("connection timeout"),
			"gateway", "zarinpal", "attempt", 3)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "something broke"})
	})

	seedSampleEntries()

	port := envOr("PORT", "8080")
	log.Printf("listening on :%s — viewer at %s", port, viewerPath)
	if err := router.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}

func seedSampleEntries() {
	_ = logger.Info("application started", "env", "local", "version", "2.0.0")
	_ = logger.Notice("cache warmed", "keys", 1420)
	_ = logger.Warning("deprecated endpoint used", "endpoint", "/v1/orders")
	_ = logger.Debug("bootstrap completed", "duration_ms", 12)

	// Sensitive keys are masked before the entry reaches any driver.
	_ = logger.Info("user authenticated", "user_id", 42, "password", "hunter2")

	_ = logger.Channel("single").Info("written to the single-file channel")
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
