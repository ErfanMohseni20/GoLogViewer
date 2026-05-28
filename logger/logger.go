package logger
import (
    "fmt"
    "os"
    "sync"
    "time"

    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

var once sync.Once
var zapLogger *zap.Logger

func initLogger() {
    once.Do(func() {
        _ = os.MkdirAll("storage/logs", 0755)

        writer := zapcore.AddSync(os.Stdout)

        encoderConfig := zap.NewProductionEncoderConfig()
        encoderConfig.TimeKey = "time"
        encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

        core := zapcore.NewCore(
            zapcore.NewJSONEncoder(encoderConfig),
            writer,
            zap.InfoLevel,
        )

        zapLogger = zap.New(core)
    })
}

func Log(status string, message string, err error) {
    initLogger()

    errorMessage := ""

    if err != nil {
        errorMessage = err.Error()
    }

    entry := LogEntry{
        Status:  status,
        Message: message,
        Error:   errorMessage,
        Time:    time.Now().Format(time.RFC3339),
    }

    _ = SaveLog(entry)

    zapLogger.Info(
        message,
        zap.String("status", status),
        zap.String("error", errorMessage),
    )

    fmt.Println("[GolaveLog]", status, message)
}