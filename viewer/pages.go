package viewer
import "github.com/gin-gonic/gin"

func LogsPage(c *gin.Context) {
    c.HTML(200, "index.html", gin.H{})
}