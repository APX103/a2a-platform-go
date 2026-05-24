package handler

import (
	"regexp"
	"strings"
)

var sensitiveJSONFieldRe = regexp.MustCompile(`(?i)("?(?:admin_token|api_key|secret|token|authorization|x-admin-token)"?\s*:\s*"?)([^",}\r\n]+)`)
var bearerTokenRe = regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9._~+/\-=]+`)
var namedHeaderTokenRe = regexp.MustCompile(`(?i)((?:X-Admin-Token|X-Group-Member-Token|Authorization)\s*:\s*)(?:Bearer\s+)?([^\s,;]+)`)
var providerAPIKeyRe = regexp.MustCompile(`\b(?:sk|sk-ant)-[A-Za-z0-9._~+/\-=]+`)

func redactSensitiveText(input string) string {
	if input == "" {
		return ""
	}
	out := sensitiveJSONFieldRe.ReplaceAllString(input, `${1}[REDACTED]`)
	out = bearerTokenRe.ReplaceAllString(out, `${1}[REDACTED]`)
	out = namedHeaderTokenRe.ReplaceAllString(out, `${1}[REDACTED]`)
	out = providerAPIKeyRe.ReplaceAllString(out, `[REDACTED]`)
	return out
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
