package main

import (
	"errors"
	"os"

	"github.com/ErfanMohseni20/GoLogViewer/logger"
	"github.com/ErfanMohseni20/GoLogViewer/middleware"
	"github.com/ErfanMohseni20/GoLogViewer/server"
	"github.com/gin-gonic/gin"
)

func main() {
	if err := logger.Init(logger.DefaultConfig()); err != nil {
		panic(err)
	}

	router := gin.Default()
	router.Use(middleware.Gin())
	server.Register(router)

	_ = logger.Info("Application started", "env", "local")
	_ = logger.Warning("Cache driver fallback enabled")
	_ = logger.Error(
		"Database connection failed",
		errors.New("connection timeout"),
		"host", "127.0.0.1",
		"attempt", 3,
	)
	_ = logger.Debug("Bootstrap completed", "duration_ms", 12)

	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "GolaveLog demo app",
			"viewer":  "/logs",
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	_ = router.Run(":" + port)
}
