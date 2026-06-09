package server

import (
	"github.com/ErfanMohseni20/GoLogViewer/logger"
	"github.com/ErfanMohseni20/GoLogViewer/viewer"
	"github.com/gin-gonic/gin"
)

// Register mounts the embedded log viewer on the provided Gin router.
// The viewer runs on the same port as your application.
func Register(router *gin.Engine, opts ...viewer.Options) {
	option := viewer.DefaultOptions()
	if len(opts) > 0 {
		option = opts[0]
		if option.LogDir == "" {
			option.LogDir = logger.CurrentConfig().ResolveLogDir()
		}
	}

	viewer.Register(router, option)
}

// Start is kept for backward compatibility.
func Start(router *gin.Engine) {
	Register(router)
}
