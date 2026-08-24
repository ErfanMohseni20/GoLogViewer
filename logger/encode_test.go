package logger

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

// The hand-rolled encoder replaced encoding/json on the write path, so its
// output must stay byte-identical to what reflection produced — otherwise
// files written by different versions would not agree.
func TestAppendEntryMatchesEncodingJSON(t *testing.T) {
	moment := time.Date(2026, 6, 9, 10, 6, 31, 195364689, time.UTC)

	entries := []Entry{
		{Level: LevelInfo, Channel: "daily", Message: "plain", Time: moment},
		{
			Level:     LevelError,
			Channel:   "stack",
			Message:   `quotes " backslash \ newline`,
			Context:   map[string]any{"zebra": 1, "alpha": "a", "middle": true},
			Exception: "connection timeout",
			Time:      moment,
		},
		{Level: LevelDebug, Channel: "c", Message: "m", Caller: "app/main.go:12", Time: moment},
		{Level: LevelWarning, Channel: "c", Message: "m", Stack: "goroutine 1", Time: moment},
		{Level: LevelInfo, Channel: "c", Message: "m", Raw: "original text", Time: moment},
		{
			Level:   LevelInfo,
			Channel: "c",
			Message: "unicode: héllo 日本語 <script>&",
			Context: map[string]any{
				"float":  3.5,
				"int":    -42,
				"uint":   uint64(7),
				"nil":    nil,
				"nested": map[string]any{"deep": []int{1, 2, 3}},
			},
			Time: moment,
		},
	}

	for i, entry := range entries {
		want, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("entry %d: json.Marshal: %v", i, err)
		}

		got := appendEntry(nil, entry)
		if string(got) != string(want) {
			t.Errorf("entry %d mismatch\n got: %s\nwant: %s", i, got, want)
		}
	}
}

func TestAppendEntryRoundTrips(t *testing.T) {
	original := Entry{
		Level:     LevelError,
		Channel:   "daily",
		Message:   "payment failed",
		Context:   map[string]any{"order_id": 99, "currency": "IRR"},
		Exception: "gateway timeout",
		Time:      time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
	}

	var decoded Entry
	if err := json.Unmarshal(appendEntry(nil, original), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Level != original.Level || decoded.Message != original.Message {
		t.Errorf("decoded = %+v", decoded)
	}
	if decoded.Context["order_id"] != float64(99) {
		t.Errorf("order_id = %v", decoded.Context["order_id"])
	}
	if !decoded.Time.Equal(original.Time) {
		t.Errorf("time = %v, want %v", decoded.Time, original.Time)
	}
}

// Context keys must be emitted in sorted order so repeated runs are diffable.
func TestAppendContextSortsKeys(t *testing.T) {
	entry := Entry{
		Level:   LevelInfo,
		Message: "m",
		Context: map[string]any{"c": 3, "a": 1, "b": 2},
	}

	line := string(appendEntry(nil, entry))
	if !strings.Contains(line, `"context":{"a":1,"b":2,"c":3}`) {
		t.Errorf("context keys are not sorted: %s", line)
	}
}

// A NaN or Inf must not produce a token that makes the whole line undecodable.
func TestAppendValueHandlesNonFiniteFloats(t *testing.T) {
	entry := Entry{
		Level:   LevelInfo,
		Message: "m",
		Context: map[string]any{"nan": math.NaN(), "inf": math.Inf(1)},
	}

	line := appendEntry(nil, entry)

	var decoded Entry
	if err := json.Unmarshal(line, &decoded); err != nil {
		t.Fatalf("a non-finite float produced invalid JSON: %s (%v)", line, err)
	}
	if decoded.Context["nan"] != nil || decoded.Context["inf"] != nil {
		t.Errorf("context = %v, want nulls", decoded.Context)
	}
}

// U+2028 and U+2029 are legal JSON but terminate a JavaScript string literal.
func TestAppendStringEscapesLineSeparators(t *testing.T) {
	line := string(appendString(nil, "before\u2028after\u2029end"))

	if strings.ContainsRune(line, '\u2028') || strings.ContainsRune(line, '\u2029') {
		t.Errorf("line separators were not escaped: %q", line)
	}
	if !strings.Contains(line, `\u2028`) || !strings.Contains(line, `\u2029`) {
		t.Errorf("expected escaped separators, got %q", line)
	}
}

func TestAppendStringEscapesHTML(t *testing.T) {
	line := string(appendString(nil, `<script>alert("x")&</script>`))

	for _, unsafe := range []string{"<", ">", "&"} {
		if strings.Contains(line, unsafe) {
			t.Errorf("%q survived escaping: %s", unsafe, line)
		}
	}
}

func TestAppendStringHandlesInvalidUTF8(t *testing.T) {
	line := appendEntry(nil, Entry{Level: LevelInfo, Message: "bad \xff\xfe bytes"})

	var decoded Entry
	if err := json.Unmarshal(line, &decoded); err != nil {
		t.Fatalf("invalid UTF-8 produced undecodable JSON: %s (%v)", line, err)
	}
	if !strings.Contains(decoded.Message, "�") {
		t.Errorf("message = %q, want replacement characters", decoded.Message)
	}
}
