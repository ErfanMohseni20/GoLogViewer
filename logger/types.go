package logger

const LogFilePath = "storage/logs/log.txt"

type LogEntry struct {
    Status  string `json:"status"`
    Message string `json:"message"`
    Error   string `json:"error"`
    Time    string `json:"time"`
}