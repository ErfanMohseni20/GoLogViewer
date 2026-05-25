package server
import (
    "github.com/ErfanMohseni20/GoLogViewer/viewer"

    "github.com/gin-gonic/gin"
)

var started = false

func Start() {
    if started {
        return
    }

    started = true

    go func() {
        router := gin.Default()

        viewer.RegisterRoutes(router)

        _ = router.Run("127.0.0.1:3000")
    }()
}