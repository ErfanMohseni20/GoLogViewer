package middleware

import (
	"strings"
	"time"

	"github.com/ErfanMohseni20/GoLogViewer/logger"
	"github.com/gin-gonic/gin"
)

func Gin() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		if path == "/logs" || strings.HasPrefix(path, "/logs/") {
			return
		}

		status := c.Writer.Status()
		level := logger.LevelInfo
		if status >= 500 {
			level = logger.LevelError
		} else if status >= 400 {
			level = logger.LevelWarning
		}

		entryLogger := logger.With(
			"method", c.Request.Method,
			"path", path,
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent(),
		)

		switch level {
		case logger.LevelError:
			_ = entryLogger.Error("HTTP request completed", nil)
		case logger.LevelWarning:
			_ = entryLogger.Warning("HTTP request completed")
		default:
			_ = entryLogger.Info("HTTP request completed")
		}
	}
}
