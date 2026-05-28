package logger
import (
    "encoding/json"
    "os"
)

func SaveLog(entry LogEntry) error {
    file, err := os.OpenFile(
        LogFilePath,
        os.O_APPEND|os.O_CREATE|os.O_WRONLY,
        0666,
    )

    if err != nil {
        return err
    }

    defer file.Close()

    data, err := json.Marshal(entry)

    if err != nil {
        return err
    }

    _, err = file.WriteString(string(data) + "\n")

    return err
}