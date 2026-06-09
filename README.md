# GolaveLog

Laravel-style logging system for Go with an embedded web log viewer.

---

## Installation

```bash
go get github.com/ErfanMohseni20/GoLogViewer
```

---

## Quick Start

```go
package main

import (
    "errors"

    "github.com/ErfanMohseni20/GoLogViewer/logger"
    "github.com/ErfanMohseni20/GoLogViewer/middleware"
    "github.com/ErfanMohseni20/GoLogViewer/server"
    "github.com/gin-gonic/gin"
)

func main() {
    _ = logger.Init(logger.DefaultConfig())

    router := gin.Default()
    router.Use(middleware.Gin())
    server.Register(router)

    _ = logger.Info("Application started")
    _ = logger.Error("Database connection failed", errors.New("connection timeout"), "host", "127.0.0.1")

    _ = router.Run(":8080")
}
```

Open the viewer at:

```txt
http://127.0.0.1:8080/logs
```

The viewer runs on the same port as your Gin application.

---

## Logging API

PSR-3 compatible levels:

```go
logger.Emergency("System is unusable")
logger.Alert("Action must be taken immediately")
logger.Critical("Critical conditions")
logger.Error("Something failed", err)
logger.Warning("Deprecated API usage")
logger.Notice("Normal but significant")
logger.Info("User logged in", "user_id", 42)
logger.Debug("Query executed", "sql", query)
```

Context fields:

```go
logger.With("request_id", "abc-123").Info("Order created", "order_id", 99)
```

Channels:

```go
logger.Channel("daily").Info("Daily channel log")
logger.Channel("single").Warning("Single file log")
```

Custom config:

```go
cfg := logger.DefaultConfig()
cfg.LogDir = "storage/logs"
cfg.Default = "stack"
cfg.Channels["daily"] = logger.ChannelConfig{
    Driver: logger.DriverDaily,
    Path:   "app.log",
    Level:  logger.LevelInfo,
    Days:   30,
}
_ = logger.Init(cfg)
```

---

## Channels

| Driver | Description |
|--------|-------------|
| `single` | One log file |
| `daily` | Rotated daily files like `laravel-2026-06-09.log` |
| `stack` | Write to multiple channels |
| `stdout` | Human-readable console output |

Default stack channel writes to both `daily` and `stdout`.

---

## HTTP Middleware

```go
router.Use(middleware.Gin())
```

Logs every HTTP request with method, path, status, duration, IP, and user agent.

---

## Log Viewer

Similar to Laravel Log Viewer:

- Embedded HTML/CSS UI via `go:embed`
- Works as a library from any import path
- File list sidebar
- Level filter
- Search
- Pagination
- Auto refresh

Register manually:

```go
viewer.Register(router, viewer.Options{
    PathPrefix: "/logs",
    LogDir:     "storage/logs",
})
```

---

## Log Storage

Daily logs are stored as JSON lines:

```txt
storage/logs/laravel-2026-06-09.log
```

Each line:

```json
{
  "level": "info",
  "channel": "daily",
  "message": "Application started",
  "context": {"env": "local"},
  "time": "2026-06-09T12:00:00+03:30"
}
```

---

## License

MIT
