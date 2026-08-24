package logger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// ANSI colours used when the console driver is attached to a terminal.
const (
	ansiReset   = "\033[0m"
	ansiDim     = "\033[2m"
	ansiBold    = "\033[1m"
	ansiRed     = "\033[31m"
	ansiBrightR = "\033[91m"
	ansiYellow  = "\033[33m"
	ansiGreen   = "\033[32m"
	ansiBlue    = "\033[34m"
	ansiMagenta = "\033[35m"
	ansiCyan    = "\033[36m"
)

func levelColor(level Level) string {
	switch level {
	case LevelEmergency, LevelAlert, LevelCritical:
		return ansiBrightR
	case LevelError:
		return ansiRed
	case LevelWarning:
		return ansiYellow
	case LevelNotice:
		return ansiGreen
	case LevelInfo:
		return ansiBlue
	default:
		return ansiMagenta
	}
}

// consoleDriver renders entries in a human-readable single line, the way
// Laravel's console formatter does.
type consoleDriver struct {
	name     string
	minLevel Level
	color    bool

	mu  sync.Mutex
	out *bufio.Writer
	sb  strings.Builder
}

func newConsoleDriver(name string, minLevel Level, file *os.File, color *bool) *consoleDriver {
	useColor := isTerminal(file)
	if color != nil {
		useColor = *color
	}

	return &consoleDriver{
		name:     name,
		minLevel: minLevel,
		color:    useColor,
		out:      bufio.NewWriterSize(file, 8*1024),
	}
}

// isTerminal reports whether the file is a character device, which is a good
// enough proxy for "colour is safe here" without pulling in a dependency.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func (c *consoleDriver) Name() string { return c.name }

func (c *consoleDriver) Log(entry Entry) error {
	if !entry.Level.Allows(c.minLevel) {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.sb.Reset()
	c.format(&c.sb, entry)

	if _, err := c.out.WriteString(c.sb.String()); err != nil {
		return fmt.Errorf("logger: write console entry: %w", err)
	}
	// Console output is for humans watching in real time, so flush every line
	// rather than waiting for the buffer to fill.
	return c.out.Flush()
}

func (c *consoleDriver) format(sb *strings.Builder, entry Entry) {
	color := ""
	reset := ""
	dim := ""
	if c.color {
		color = levelColor(entry.Level)
		reset = ansiReset
		dim = ansiDim
	}

	sb.WriteString(dim)
	sb.WriteByte('[')
	sb.WriteString(entry.Time.Format("2006-01-02 15:04:05.000"))
	sb.WriteByte(']')
	sb.WriteString(reset)
	sb.WriteByte(' ')

	channel := entry.Channel
	if channel == "" {
		channel = c.name
	}
	sb.WriteString(channel)
	sb.WriteByte('.')

	sb.WriteString(color)
	if c.color {
		sb.WriteString(ansiBold)
	}
	sb.WriteString(strings.ToUpper(entry.Level.String()))
	sb.WriteString(reset)

	sb.WriteString(": ")
	sb.WriteString(entry.Message)

	if entry.Caller != "" {
		sb.WriteByte(' ')
		sb.WriteString(dim)
		sb.WriteString(entry.Caller)
		sb.WriteString(reset)
	}

	if entry.Exception != "" {
		sb.WriteString(" | ")
		sb.WriteString(color)
		sb.WriteString(entry.Exception)
		sb.WriteString(reset)
	}

	// Sort keys so repeated runs produce diffable output.
	if len(entry.Context) > 0 {
		keys := make([]string, 0, len(entry.Context))
		for key := range entry.Context {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		sb.WriteString(dim)
		for _, key := range keys {
			sb.WriteByte(' ')
			sb.WriteString(key)
			sb.WriteByte('=')
			sb.WriteString(consoleValue(entry.Context[key]))
		}
		sb.WriteString(reset)
	}

	sb.WriteByte('\n')
}

func consoleValue(value any) string {
	switch v := value.(type) {
	case string:
		if strings.ContainsAny(v, " \t\"") {
			return strconv.Quote(v)
		}
		return v
	case error:
		return v.Error()
	case fmt.Stringer:
		return v.String()
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprintf("%v", value)
		}
		return string(data)
	}
}

func (c *consoleDriver) Sync() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.out.Flush()
}

func (c *consoleDriver) Close() error { return c.Sync() }
