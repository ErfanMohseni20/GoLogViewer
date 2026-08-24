// Package ginlog is the Gin adapter for GoLogViewer: request logging plus a
// one-call mount for the log viewer.
//
// It is the only package in this module that imports Gin. Projects on
// net/http, chi or echo use logger, viewer and middleware directly and never
// link a web framework.
//
//	router := gin.New()
//	router.Use(ginlog.Middleware(middleware.Options{SkipPaths: []string{"/logs"}}))
//	ginlog.RegisterViewer(router, viewer.Options{PathPrefix: "/logs"})
package ginlog

import (
	"time"

	"github.com/ErfanMohseni20/GoLogViewer/logger"
	"github.com/ErfanMohseni20/GoLogViewer/middleware"
	"github.com/ErfanMohseni20/GoLogViewer/viewer"
	"github.com/gin-gonic/gin"
)

// RegisterViewer mounts the log viewer on the router and returns it, so the
// caller can read back its prefix or log directory.
//
// If Options.LogDir is empty it defaults to the active logger configuration's
// directory.
func RegisterViewer(router gin.IRouter, opts viewer.Options) *viewer.Viewer {
	if opts.LogDir == "" {
		opts.LogDir = logger.CurrentConfig().ResolveLogDir()
	}

	v := viewer.New(opts)
	MountViewer(router, v)
	return v
}

// MountViewer attaches an existing viewer to a Gin router. The viewer routes
// internally, so one wildcard route is enough.
func MountViewer(router gin.IRouter, v *viewer.Viewer) {
	handler := gin.WrapH(v)
	prefix := v.Prefix()

	router.GET(prefix, handler)
	router.Any(prefix+"/*any", handler)
}

// Middleware returns a Gin middleware that logs every request.
//
// Pass the viewer's mount path in Options.SkipPaths so browsing the logs does
// not itself append to the file being browsed. v1 hard-coded "/logs" here and
// silently ignored a customised prefix.
func Middleware(opts ...middleware.Options) gin.HandlerFunc {
	option := middleware.Options{}
	if len(opts) > 0 {
		option = opts[0]
	}
	option = option.Normalize()

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if option.ShouldSkip(path, c.Request) {
			c.Next()
			return
		}

		if requestID := c.GetHeader(option.RequestIDHeader); requestID != "" {
			c.Request = c.Request.WithContext(
				logger.WithFields(c.Request.Context(), "request_id", requestID),
			)
		}

		start := time.Now()
		c.Next()
		duration := time.Since(start)

		status := c.Writer.Status()
		level := middleware.LevelForStatus(status)
		if option.SlowRequestThreshold > 0 && duration >= option.SlowRequestThreshold && level == logger.LevelInfo {
			level = logger.LevelWarning
		}

		// Gin accumulates handler errors on the context; the first is the most
		// useful to surface as the entry's exception.
		var handlerErr error
		if len(c.Errors) > 0 {
			handlerErr = c.Errors[0].Err
			if level == logger.LevelInfo {
				level = logger.LevelWarning
			}
		}

		log, err := logger.Default().Channel(option.Channel)
		if err != nil {
			// A logging failure must not affect a request that already
			// completed successfully.
			return
		}

		fields := []any{
			"method", c.Request.Method,
			"path", path,
			"status", status,
			"duration_ms", duration.Milliseconds(),
			"bytes", c.Writer.Size(),
			"ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent(),
		}
		if raw := c.Request.URL.RawQuery; raw != "" {
			fields = append(fields, "query", raw)
		}

		bound := log.Ctx(c.Request.Context())
		if handlerErr != nil {
			_ = bound.Error(option.Message, handlerErr, fields...)
			return
		}
		_ = bound.Log(level, option.Message, fields...)
	}
}
