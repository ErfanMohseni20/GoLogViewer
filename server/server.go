package server
import (
    "github.com/ErfanMohseni20/GoLogViewer/viewer"

    "github.com/gin-gonic/gin"
)

var started = false

func Start(router *gin.Engine) {
    if started {
        return
    }

    started = true

    viewer.RegisterRoutes(router)
}