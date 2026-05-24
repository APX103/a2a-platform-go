package redact

import (
	"encoding/json"
	"io"
	"net/url"
	"regexp"
	"strings"
)

var sensitiveJSONFieldRe = regexp.MustCompile(`(?i)("?(?:[a-z0-9_-]*(?:token|secret|password)[a-z0-9_-]*|api[_-]?key|apikey|authorization|x-api-key|key)"?\s*:\s*)(?:"(?:\\.|[^"\\])*"|[^,}\r\n\s]+)`)
var bearerTokenRe = regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9._~+/\-=]+`)
var namedHeaderTokenRe = regexp.MustCompile(`(?i)((?:X-Admin-Token|X-Group-Member-Token|X-Api-Key|Authorization)\s*:\s*)(?:Bearer\s+)?([^\s,;]+)`)
var providerAPIKeyRe = regexp.MustCompile(`\b(?:sk|sk-ant)-[A-Za-z0-9._~+/\-=]+`)
var querySecretRe = regexp.MustCompile(`(?i)\b((?:admin_token|session_token|default_access_token|access_token|invite_token|api_key|apikey|x-api-key|token|secret|password|key)=)[^&\s,;]+`)
var urlRe = regexp.MustCompile(`https?://[^\s"'<>]+`)

// Text removes known credential shapes from JSON and plain text before it is
// stored in traces, logs, or error payloads.
func Text(input string) string {
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
	out = querySecretRe.ReplaceAllString(out, `${1}[REDACTED]`)
	out = redactURLSecretsInText(out)
	return out
}

func redactURLSecretsInText(input string) string {
	return urlRe.ReplaceAllStringFunc(input, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return raw
		}

		parsed.User = nil
		query := parsed.Query()
		for key, values := range query {
			if !isSensitiveKey(key) {
				continue
			}
			for i := range values {
				values[i] = "[REDACTED]"
			}
			query[key] = values
		}
		parsed.RawQuery = query.Encode()
		return parsed.String()
	})
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
		if out, ok := redactJSONText(typed); ok {
			return out
		}
		return redactPlainSecretTokens(typed)
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	normalized := strings.NewReplacer("-", "_", " ", "_").Replace(lower)
	compact := strings.NewReplacer("_", "", "-", "", " ", "").Replace(lower)

	switch normalized {
	case "admin_token", "session_token", "default_access_token", "access_token", "invite_token",
		"api_key", "x_api_key", "secret", "token", "authorization", "x_admin_token",
		"x_group_member_token", "password", "key":
		return true
	}
	switch compact {
	case "admintoken", "sessiontoken", "defaultaccesstoken", "accesstoken", "invitetoken",
		"apikey", "xapikey", "secret", "token", "authorization", "xadmintoken",
		"xgroupmembertoken", "password", "key":
		return true
	}
	return strings.HasSuffix(compact, "token") ||
		strings.HasSuffix(compact, "secret") ||
		strings.HasSuffix(compact, "password") ||
		strings.Contains(compact, "apikey") ||
		strings.Contains(compact, "authorization")
}

// SafeTraceData redacts credentials and caps the stored payload length.
func SafeTraceData(input string, max int) string {
	redacted := Text(input)
	if max <= 0 || len(redacted) <= max {
		return redacted
	}
	if max <= len("...(truncated)") {
		return redacted[:max]
	}
	return strings.TrimSpace(redacted[:max-len("...(truncated)")]) + "...(truncated)"
}
