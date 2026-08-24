# GoLogViewer

Laravel-style logging for Go with an embedded web log viewer.

Structured JSON logs, PSR-3 levels, daily and size-based rotation, and a `/logs`
web UI with search, level filters and live tail — mounted on your app's own port.
The core is standard-library only, so it works with net/http, chi and echo; Gin
users import `ginlog`. Requires Go 1.25+.

```bash
go get github.com/ErfanMohseni20/GoLogViewer
```

```go
logger.Init(logger.DefaultConfig())
defer logger.Shutdown()

mux := http.NewServeMux()
viewer.Register(mux, viewer.Options{
    PathPrefix:        "/logs",
    BasicAuthUser:     "admin",
    BasicAuthPassword: os.Getenv("LOG_VIEWER_PASSWORD"),
})

logger.Info("user logged in", "user_id", 42)
logger.Error("payment failed", err, "gateway", "zarinpal")
```

Runnable examples: [`examples/nethttp`](examples/nethttp) and [`examples/gin`](examples/gin).
Full docs on [pkg.go.dev](https://pkg.go.dev/github.com/ErfanMohseni20/GoLogViewer). MIT licensed.

> The viewer has no authentication unless you configure it. Set `BasicAuth*` or
> `Authorize`, and prefer `ReadOnly: true`, for anything reachable beyond localhost.
