package handler

import (
	"strings"
	"testing"
)

func TestRedactSensitiveTextMasksKnownSecretShapes(t *testing.T) {
	input := `{
		"admin_token":"a2a-admin-token",
		"api_key":"sk-live-secret",
		"secret":"agent-secret",
		"token":"human-token",
		"Authorization":"Bearer member-token",
		"X-Admin-Token":"root-token",
		"normal":"keep-me"
	}`

	got := redactSensitiveText(input)

	for _, leaked := range []string{
		"a2a-admin-token",
		"sk-live-secret",
		"agent-secret",
		"human-token",
		"member-token",
		"root-token",
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("redacted text leaked %q: %s", leaked, got)
		}
	}
	if !strings.Contains(got, "keep-me") {
		t.Fatalf("redacted text removed non-sensitive value: %s", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("redacted text does not contain marker: %s", got)
	}
}

func TestRedactSensitiveTextMasksBearerInPlainError(t *testing.T) {
	got := redactSensitiveText("upstream said Authorization: Bearer abc.def.ghi and X-Admin-Token: secret")
	if strings.Contains(got, "abc.def.ghi") || strings.Contains(got, "secret") {
		t.Fatalf("plain error leaked token: %s", got)
	}
}

func TestRedactSensitiveTextMasksGroupMemberTokenInJSON(t *testing.T) {
	got := redactSensitiveText(`{"X-Group-Member-Token":"member-secret","normal":"keep-me"}`)
	if strings.Contains(got, "member-secret") {
		t.Fatalf("redacted text leaked group member token: %s", got)
	}
	if !strings.Contains(got, "keep-me") {
		t.Fatalf("redacted text removed non-sensitive value: %s", got)
	}
}

func TestRedactSensitiveTextMasksKnownTokenFieldNames(t *testing.T) {
	input := `{
		"session_token":"session-secret",
		"default_access_token":"default-secret",
		"access_token":"access-secret",
		"invite_token":"invite-secret",
		"x-api-key":"x-api-secret",
		"apiKey":"camel-secret",
		"normal":"keep-me"
	}`

	got := redactSensitiveText(input)

	for _, leaked := range []string{
		"session-secret",
		"default-secret",
		"access-secret",
		"invite-secret",
		"x-api-secret",
		"camel-secret",
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("redacted text leaked %q: %s", leaked, got)
		}
	}
	if !strings.Contains(got, "keep-me") {
		t.Fatalf("redacted text removed non-sensitive value: %s", got)
	}
}

func TestRedactSensitiveTextMasksURLQuerySecrets(t *testing.T) {
	got := redactSensitiveText(`{"url":"https://user:pass@example.test/path?token=query-secret&ok=keep-me"}`)
	if strings.Contains(got, "query-secret") || strings.Contains(got, "user:pass") {
		t.Fatalf("redacted text leaked URL secret: %s", got)
	}
	if !strings.Contains(got, "keep-me") {
		t.Fatalf("redacted text removed non-sensitive query value: %s", got)
	}
}

func TestRedactSensitiveTextMasksQuotedJSONValueWithComma(t *testing.T) {
	got := redactSensitiveText(`{"secret":"abc,def","normal":"keep-me"}`)
	if strings.Contains(got, "abc") || strings.Contains(got, "def") {
		t.Fatalf("redacted text leaked comma-delimited secret: %s", got)
	}
	if !strings.Contains(got, "keep-me") {
		t.Fatalf("redacted text removed non-sensitive value: %s", got)
	}
}

func TestSafeTraceDataRedactsThenTruncates(t *testing.T) {
	input := strings.Repeat("x", 20) + `"api_key":"sk-secret"` + strings.Repeat("y", 20)
	got := safeTraceData(input, 32)
	if len(got) > 32 {
		t.Fatalf("len = %d, want <= 32: %q", len(got), got)
	}
	if strings.Contains(got, "sk-secret") {
		t.Fatalf("safe trace data leaked secret: %s", got)
	}
}
