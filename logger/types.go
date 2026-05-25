package logger
type LogEntry struct {
    Type    string `json:"type"`
    Status  string `json:"status"`
    Message string `json:"message"`
    Error   string `json:"error"`
    Time    string `json:"time"`
}