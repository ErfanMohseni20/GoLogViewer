package logger

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"
)

// appendEntry writes entry as a JSON object into dst and returns the extended
// slice.
//
// Encoding by hand rather than through encoding/json removes the reflection,
// the per-field interface boxing and the intermediate allocation that a
// profile showed accounted for roughly half of every log call. The field names
// and order match the struct tags on Entry, so files written here decode
// straight back through json.Unmarshal.
func appendEntry(dst []byte, entry Entry) []byte {
	dst = append(dst, `{"level":"`...)
	dst = append(dst, entry.Level.String()...)

	dst = append(dst, `","channel":`...)
	dst = appendString(dst, entry.Channel)

	dst = append(dst, `,"message":`...)
	dst = appendString(dst, entry.Message)

	if len(entry.Context) > 0 {
		dst = append(dst, `,"context":`...)
		dst = appendContext(dst, entry.Context)
	}
	if entry.Exception != "" {
		dst = append(dst, `,"exception":`...)
		dst = appendString(dst, entry.Exception)
	}
	if entry.Stack != "" {
		dst = append(dst, `,"stack":`...)
		dst = appendString(dst, entry.Stack)
	}
	if entry.Caller != "" {
		dst = append(dst, `,"caller":`...)
		dst = appendString(dst, entry.Caller)
	}

	dst = append(dst, `,"time":"`...)
	dst = entry.Time.AppendFormat(dst, time.RFC3339Nano)
	dst = append(dst, '"')

	if entry.Raw != "" {
		dst = append(dst, `,"raw":`...)
		dst = appendString(dst, entry.Raw)
	}

	return append(dst, '}')
}

// appendContext encodes the context map with its keys sorted, so two runs of
// the same code produce byte-identical lines and diffs stay meaningful.
// encoding/json sorts map keys for the same reason.
func appendContext(dst []byte, context map[string]any) []byte {
	keys := make([]string, 0, len(context))
	for key := range context {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	dst = append(dst, '{')
	for i, key := range keys {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendString(dst, key)
		dst = append(dst, ':')
		dst = appendValue(dst, context[key])
	}
	return append(dst, '}')
}

// appendValue encodes the types that dominate log context directly and defers
// anything else to encoding/json.
func appendValue(dst []byte, value any) []byte {
	switch v := value.(type) {
	case nil:
		return append(dst, "null"...)
	case string:
		return appendString(dst, v)
	case bool:
		return strconv.AppendBool(dst, v)
	case int:
		return strconv.AppendInt(dst, int64(v), 10)
	case int8:
		return strconv.AppendInt(dst, int64(v), 10)
	case int16:
		return strconv.AppendInt(dst, int64(v), 10)
	case int32:
		return strconv.AppendInt(dst, int64(v), 10)
	case int64:
		return strconv.AppendInt(dst, v, 10)
	case uint:
		return strconv.AppendUint(dst, uint64(v), 10)
	case uint8:
		return strconv.AppendUint(dst, uint64(v), 10)
	case uint16:
		return strconv.AppendUint(dst, uint64(v), 10)
	case uint32:
		return strconv.AppendUint(dst, uint64(v), 10)
	case uint64:
		return strconv.AppendUint(dst, v, 10)
	case float32:
		return appendFloat(dst, float64(v), 32)
	case float64:
		return appendFloat(dst, v, 64)
	case time.Time:
		dst = append(dst, '"')
		dst = v.AppendFormat(dst, time.RFC3339Nano)
		return append(dst, '"')
	case time.Duration:
		return appendString(dst, v.String())
	case error:
		return appendString(dst, v.Error())
	case fmt.Stringer:
		return appendString(dst, v.String())
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return appendString(dst, fmt.Sprintf("%v", value))
		}
		return append(dst, data...)
	}
}

// appendFloat rejects the values JSON cannot represent rather than emitting
// invalid tokens that would make the whole line undecodable.
func appendFloat(dst []byte, value float64, bits int) []byte {
	if value != value || value > 1e308 || value < -1e308 {
		return append(dst, "null"...)
	}
	return strconv.AppendFloat(dst, value, 'g', -1, bits)
}

const hexDigits = "0123456789abcdef"

// appendString writes a quoted, escaped JSON string. It escapes <, > and & the
// way encoding/json does by default, so entries stay safe to embed in HTML.
func appendString(dst []byte, s string) []byte {
	dst = append(dst, '"')

	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if safeByte(b) {
				i++
				continue
			}

			dst = append(dst, s[start:i]...)
			switch b {
			case '\\', '"':
				dst = append(dst, '\\', b)
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				// Control characters and the HTML-significant bytes.
				dst = append(dst, '\\', 'u', '0', '0', hexDigits[b>>4], hexDigits[b&0xF])
			}
			i++
			start = i
			continue
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			dst = append(dst, s[start:i]...)
			dst = append(dst, "\ufffd"...)
			i += size
			start = i
			continue
		}

		// U+2028 and U+2029 are valid JSON but break JavaScript string
		// literals, and these lines are read back into a browser.
		if r == '\u2028' || r == '\u2029' {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hexDigits[r&0xF])
			i += size
			start = i
			continue
		}

		i += size
	}

	dst = append(dst, s[start:]...)
	return append(dst, '"')
}

// safeByte reports whether a byte can be copied into a JSON string unescaped.
func safeByte(b byte) bool {
	return b >= 0x20 && b != '"' && b != '\\' && b != '<' && b != '>' && b != '&'
}
