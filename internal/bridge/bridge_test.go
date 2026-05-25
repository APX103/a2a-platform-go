package bridge

import (
	"context"
	"strings"
	"testing"

	"a2a-platform/internal/config"
)

func TestInvokeHTTPRejectsUnsupportedScheme(t *testing.T) {
	_, err := invokeHTTP(context.Background(), &config.SkillInvoke{
		Method: "GET",
		URL:    "file:///etc/passwd",
	}, nil, &TemplateContext{})
	if err == nil || !strings.Contains(err.Error(), "unsupported URL scheme") {
		t.Fatalf("err = %v, want unsupported scheme", err)
	}
}

func TestInvokeCLIBoundsOutput(t *testing.T) {
	out, err := invokeCLI(context.Background(), &config.SkillInvoke{
		Command: "printf",
		Args:    []string{"%02000000d", "1"},
		Timeout: 1000,
	}, &config.BridgeCLITarget{}, &TemplateContext{})
	if err != nil {
		t.Fatalf("invokeCLI: %v", err)
	}
	if len(out) > maxCLIOutputBytes {
		t.Fatalf("output len = %d, want <= %d", len(out), maxCLIOutputBytes)
	}
}
