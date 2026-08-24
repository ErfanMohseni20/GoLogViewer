package logger

import "strings"

// RedactedPlaceholder replaces the value of any redacted context key.
const RedactedPlaceholder = "[REDACTED]"

// DefaultRedactKeys are the context keys masked unless Config.RedactKeys says
// otherwise. Logs routinely end up in tickets, screenshots and shared viewers,
// so credentials are masked by default rather than opt-in.
func DefaultRedactKeys() []string {
	return []string{
		"password",
		"passwd",
		"secret",
		"token",
		"authorization",
		"api_key",
		"apikey",
		"private_key",
		"credit_card",
		"card_number",
		"cvv",
		"session_id",
		"refresh_token",
	}
}

// redactor masks sensitive context values. Matching is case-insensitive and
// substring-based so "access_token" is caught by the "token" rule.
type redactor struct {
	keys []string
}

func newRedactor(keys []string) *redactor {
	if len(keys) == 0 {
		return nil
	}

	lowered := make([]string, len(keys))
	for i, key := range keys {
		lowered[i] = strings.ToLower(key)
	}
	return &redactor{keys: lowered}
}

// apply masks matching entries in place. The map is always freshly built by
// mergeContext, so mutating it here cannot affect the caller.
func (r *redactor) apply(context map[string]any) {
	if r == nil || len(context) == 0 {
		return
	}

	for key := range context {
		if r.matches(key) {
			context[key] = RedactedPlaceholder
		}
	}
}

func (r *redactor) matches(key string) bool {
	lowered := strings.ToLower(key)
	for _, needle := range r.keys {
		if strings.Contains(lowered, needle) {
			return true
		}
	}
	return false
}
