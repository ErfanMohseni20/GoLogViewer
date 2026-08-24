// Package server provides the one-line Gin setup that GoLogViewer v1 exposed.
//
// Deprecated: use ginlog.RegisterViewer, which makes the viewer's options — in
// particular authentication — explicit at the call site. This package is kept
// so that v1 code continues to compile.
package server

import (
	"github.com/ErfanMohseni20/GoLogViewer/ginlog"
	"github.com/ErfanMohseni20/GoLogViewer/viewer"
	"github.com/gin-gonic/gin"
)

// Register mounts the embedded log viewer on the provided Gin router, on the
// same port as the application.
//
// Deprecated: use ginlog.RegisterViewer.
func Register(router gin.IRouter, opts ...viewer.Options) *viewer.Viewer {
	option := viewer.DefaultOptions()
	if len(opts) > 0 {
		option = opts[0]
	}
	return ginlog.RegisterViewer(router, option)
}

// Start is kept for backward compatibility with v1.
//
// Deprecated: use ginlog.RegisterViewer.
func Start(router *gin.Engine) {
	Register(router)
}
