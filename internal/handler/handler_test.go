package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"a2a-platform/internal/svc"
)

func TestAgentProxyRejectsInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/agent/test", strings.NewReader("{"))
	req.Header.Set("X-Path-Param-Name", "test")
	rec := httptest.NewRecorder()

	NewAgentProxyHandler(&svc.ServiceContext{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid JSON") {
		t.Fatalf("body = %q, want invalid JSON error", rec.Body.String())
	}
}
