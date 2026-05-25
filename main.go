package main

import (
    "errors"
    "time"

    "github.com/ErfanMohseni20/GoLogViewer/logger"
    "github.com/ErfanMohseni20/GoLogViewer/server"
)

func main() {
    server.Start()

    logger.Log(
        "SYSTEM",
        "INFO",
        "Application started",
        nil,
    )

    logger.Log(
        "DATABASE",
        "ERROR",
        "Database connection failed",
        errors.New("connection timeout"),
    )

    logger.Log(
        "AUTH",
        "WARNING",
        "Invalid login attempt",
        nil,
    )

    for {
        time.Sleep(time.Second)
    }
}