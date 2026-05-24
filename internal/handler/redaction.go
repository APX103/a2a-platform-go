package handler

import "a2a-platform/internal/redact"

func redactSensitiveText(input string) string {
	return redact.Text(input)
}

func safeTraceData(input string, max int) string {
	return redact.SafeTraceData(input, max)
}
