package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGuideHandler_List(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/guide", nil)
	rec := httptest.NewRecorder()

	NewGuideHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Guides []map[string]string `json:"guides"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Guides) == 0 {
		t.Fatal("expected at least one guide")
	}

	found := false
	for _, g := range resp.Guides {
		if g["name"] == "bridge" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'bridge' guide in list")
	}
}

func TestGuideHandler_GetBridge(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/guide/bridge", nil)
	rec := httptest.NewRecorder()

	NewGuideHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Bridge") {
		t.Fatalf("expected guide content to contain 'Bridge', got:\n%s", body)
	}
}

func TestGuideHandler_NotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/guide/nonexistent", nil)
	rec := httptest.NewRecorder()

	NewGuideHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
