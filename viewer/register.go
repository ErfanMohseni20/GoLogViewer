package viewer

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/ErfanMohseni20/GoLogViewer/logger"
	"github.com/gin-gonic/gin"
)

//go:embed templates/*
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

type Options struct {
	PathPrefix string
	LogDir     string
}

func DefaultOptions() Options {
	cfg := logger.DefaultConfig()
	return Options{
		PathPrefix: "/logs",
		LogDir:     cfg.ResolveLogDir(),
	}
}

func Register(router *gin.Engine, opts Options) {
	if opts.PathPrefix == "" {
		opts.PathPrefix = "/logs"
	}
	if opts.LogDir == "" {
		opts.LogDir = logger.DefaultConfig().ResolveLogDir()
	}

	handler := &Handler{
		pathPrefix: normalizePrefix(opts.PathPrefix),
		logDir:     opts.LogDir,
	}

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("golavelog: failed to load embedded static assets")
	}

	router.StaticFS(handler.pathPrefix+"/static", http.FS(staticSub))
	router.GET(handler.pathPrefix, handler.LogsPage)
	router.GET(handler.pathPrefix+"/api/files", handler.LogFilesAPI)
	router.GET(handler.pathPrefix+"/api/entries", handler.LogEntriesAPI)
}

func normalizePrefix(prefix string) string {
	if prefix == "" {
		return "/logs"
	}
	if prefix[0] != '/' {
		prefix = "/" + prefix
	}
	if len(prefix) > 1 && prefix[len(prefix)-1] == '/' {
		prefix = prefix[:len(prefix)-1]
	}
	return prefix
}
