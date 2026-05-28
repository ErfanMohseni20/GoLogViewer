package main

import (
    "github.com/ErfanMohseni20/GoLogViewer/server"
    "github.com/gin-gonic/gin"
)

func main() {
    router := gin.Default()
    server.Start(router)
    router.Run(":8080")

}