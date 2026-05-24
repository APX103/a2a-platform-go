package handler

import (
	"encoding/json"
	"io"
	"regexp"
	"strings"
)

var sensitiveJSONFieldRe = regexp.MustCompile(`(?i)("?(?:admin_token|api_key|secret|token|authorization|x-admin-token|x-group-member-token)"?\s*:\s*)(?:"(?:\\.|[^"\\])*"|[^,}\r\n\s]+)`)
var bearerTokenRe = regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9._~+/\-=]+`)
var namedHeaderTokenRe = regexp.MustCompile(`(?i)((?:X-Admin-Token|X-Group-Member-Token|Authorization)\s*:\s*)(?:Bearer\s+)?([^\s,;]+)`)
var providerAPIKeyRe = regexp.MustCompile(`\b(?:sk|sk-ant)-[A-Za-z0-9._~+/\-=]+`)

func redactSensitiveText(input string) string {
	if input == "" {
		return ""
	}

	if out, ok := redactJSONText(input); ok {
		return out
	}

	out := redactPlainSecretTokens(input)
	out = sensitiveJSONFieldRe.ReplaceAllString(out, `${1}"[REDACTED]"`)
	return out
}

func redactPlainSecretTokens(input string) string {
	out := bearerTokenRe.ReplaceAllString(input, `${1}[REDACTED]`)
	out = namedHeaderTokenRe.ReplaceAllString(out, `${1}[REDACTED]`)
	out = providerAPIKeyRe.ReplaceAllString(out, `[REDACTED]`)
	return out
}

func redactJSONText(input string) (string, bool) {
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()

	var data any
	if err := decoder.Decode(&data); err != nil {
		return "", false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return "", false
	}

	out, err := json.Marshal(redactJSONValue(data))
	if err != nil {
		return "", false
	}
	return string(out), true
}

func redactJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if isSensitiveKey(key) {
				typed[key] = "[REDACTED]"
				continue
			}
			typed[key] = redactJSONValue(nested)
		}
		return typed
	case []any:
		for i, nested := range typed {
			typed[i] = redactJSONValue(nested)
		}
		return typed
	case string:
		return redactPlainSecretTokens(typed)
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	switch strings.ToLower(key) {
	case "admin_token", "api_key", "secret", "token", "authorization", "x-admin-token", "x-group-member-token":
		return true
	default:
		return false
	}
}

func safeTraceData(input string, max int) string {
	redacted := redactSensitiveText(input)
	if max <= 0 || len(redacted) <= max {
		return redacted
	}
	if max <= len("...(truncated)") {
		return redacted[:max]
	}
	return strings.TrimSpace(redacted[:max-len("...(truncated)")]) + "...(truncated)"
}
