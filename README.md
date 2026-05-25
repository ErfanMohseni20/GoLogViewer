# GolaveLog

Laravel style logging system for Go.

Simple, fast, and automatic.

---

# Installation

```bash
go get github.com/yourusername/golavelog
```

---

# Usage

```go
package main

import (
    "errors"

    "golavelog/logger"
    "golavelog/server"
)

func main() {

    // Start log viewer server
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
}
```

---

# Open Log Viewer

```txt
http://127.0.0.1:3000/logs
```

---

# Log Format

```go
logger.Log(type, status, message, err)
```

Example:

```go
logger.Log("AUTH", "WARNING", "Invalid login attempt", nil)
```

---

# Features

* Automatic log saving
* Built-in web log viewer
* Real-time log updates
* JSON log storage
* Gin powered server
* Zap logger
* Lightweight and simple

---

# Log Storage

Logs are stored inside:

```txt
/storage/logs/log.txt
```

---

# Todo

* Search
* Filters
* Authentication
* Websocket realtime logs
* Docker support
* Middleware support

---

# License

MIT
