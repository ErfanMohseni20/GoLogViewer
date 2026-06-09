package viewer

import (
	"html/template"
	"net/http"
	"strconv"

	"github.com/ErfanMohseni20/GoLogViewer/logger"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	pathPrefix string
	logDir     string
}

func (h *Handler) LogsPage(c *gin.Context) {
	tmpl, err := template.ParseFS(templateFS, "templates/index.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load log viewer template")
		return
	}

	data := gin.H{
		"PathPrefix": h.pathPrefix,
		"Levels":     logger.AllLevels(),
	}

	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(c.Writer, data); err != nil {
		c.String(http.StatusInternalServerError, "failed to render log viewer")
	}
}

func (h *Handler) LogFilesAPI(c *gin.Context) {
	files, err := logger.ListLogFiles(h.logDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if files == nil {
		files = []logger.LogFile{}
	}

	c.JSON(http.StatusOK, files)
}

func (h *Handler) LogEntriesAPI(c *gin.Context) {
	fileName := c.Query("file")
	if fileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file parameter is required"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))

	result, err := logger.ReadLogEntries(
		h.logDir,
		fileName,
		c.Query("level"),
		c.Query("search"),
		page,
		perPage,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}
