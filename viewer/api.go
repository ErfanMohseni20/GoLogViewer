package viewer
import (
    "bufio"
    "encoding/json"
    "net/http"
    "os"
    "github.com/ErfanMohseni20/GoLogViewer/logger"
    "github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine) {
    router.LoadHTMLGlob("web/templates/*")
    router.Static("/static", "web/static")

    router.GET("/logs", LogsPage)
    router.GET("/logs/api", LogsAPI)
}

func LogsAPI(c *gin.Context) {
    file, err := os.Open("storage/logs/log.txt")

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": err.Error(),
        })

        return
    }

    defer file.Close()

    var logs []logger.LogEntry

    scanner := bufio.NewScanner(file)

    for scanner.Scan() {
        var entry logger.LogEntry

        err := json.Unmarshal(scanner.Bytes(), &entry)

        if err == nil {
            logs = append(logs, entry)
        }
    }

    c.JSON(http.StatusOK, logs)
}