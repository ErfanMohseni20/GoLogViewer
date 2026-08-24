package logger

import (
	"encoding/json"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		input   string
		want    Level
		wantErr bool
	}{
		{"debug", LevelDebug, false},
		{"INFO", LevelInfo, false},
		{" Warning ", LevelWarning, false},
		{"warn", LevelWarning, false},
		{"err", LevelError, false},
		{"emergency", LevelEmergency, false},
		{"", levelUnset, true},
		{"nonsense", levelUnset, true},
	}

	for _, tc := range cases {
		got, err := ParseLevel(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseLevel(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// v1 silently mapped every unrecognised level onto debug, which turned a typo
// in a filter into a wrong result rather than an error.
func TestParseLevelRejectsUnknownInsteadOfDefaulting(t *testing.T) {
	if _, err := ParseLevel("informational-ish"); err == nil {
		t.Fatal("expected an error for an unknown level")
	}
}

func TestLevelAllows(t *testing.T) {
	if !LevelError.Allows(LevelWarning) {
		t.Error("error should pass a warning threshold")
	}
	if LevelDebug.Allows(LevelInfo) {
		t.Error("debug should not pass an info threshold")
	}
	if !LevelInfo.Allows(LevelInfo) {
		t.Error("a level should pass its own threshold")
	}
}

func TestLevelJSONRoundTrip(t *testing.T) {
	for _, level := range AllLevels() {
		data, err := json.Marshal(level)
		if err != nil {
			t.Fatalf("marshal %v: %v", level, err)
		}

		var decoded Level
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal %s: %v", data, err)
		}
		if decoded != level {
			t.Errorf("round trip of %v produced %v", level, decoded)
		}
	}
}

// The on-disk format must stay compatible with files written by v1.
func TestLevelDecodesV1Format(t *testing.T) {
	var entry Entry
	if err := json.Unmarshal([]byte(`{"level":"warning","message":"hi"}`), &entry); err != nil {
		t.Fatalf("unmarshal v1 entry: %v", err)
	}
	if entry.Level != LevelWarning {
		t.Errorf("level = %v, want warning", entry.Level)
	}
}
