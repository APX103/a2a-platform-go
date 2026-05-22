package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"a2a-platform/internal/model"
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

func TestApplyContextModeToRPC_StatelessStripsContextId(t *testing.T) {
	rpcReq := map[string]interface{}{
		"params": map[string]interface{}{
			"contextId": "client-context",
			"message": map[string]interface{}{
				"parts": []interface{}{map[string]interface{}{"text": "hello"}},
			},
		},
	}

	contextId := applyContextModeToRPC(rpcReq, model.ContextModeStateless)
	if contextId != nil {
		t.Fatalf("contextId = %v, want nil", *contextId)
	}
	params, ok := rpcReq["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("params missing: %#v", rpcReq)
	}
	if _, ok := params["contextId"]; ok {
		t.Fatalf("contextId was forwarded in stateless mode: %#v", params)
	}
}

func TestApplyContextModeToRPC_ContextInjectsContextId(t *testing.T) {
	rpcReq := map[string]interface{}{
		"params": map[string]interface{}{
			"message": map[string]interface{}{
				"parts": []interface{}{map[string]interface{}{"text": "hello"}},
			},
		},
	}

	contextId := applyContextModeToRPC(rpcReq, model.ContextModeContext)
	if contextId == nil || *contextId == "" {
		t.Fatal("contextId was not generated")
	}
	params, ok := rpcReq["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("params missing: %#v", rpcReq)
	}
	if got, _ := params["contextId"].(string); got != *contextId {
		t.Fatalf("injected contextId = %q, want %q", got, *contextId)
	}
}

func TestResolveRootContextId_HeaderWins(t *testing.T) {
	rpcReq := map[string]interface{}{
		"params": map[string]interface{}{
			"rootContextId": "params-root",
			"contextId":     "ctx-1",
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/agent/test", nil)
	req.Header.Set("X-A2A-Root-Context-Id", "header-root")
	ctx := "ctx-1"

	root := resolveRootContextId(req, rpcReq, &ctx, "")
	if root == nil || *root != "header-root" {
		t.Fatalf("root = %v, want header-root", root)
	}
}

func TestResolveRootContextId_FallsBackToOriginalContextForStateless(t *testing.T) {
	rpcReq := map[string]interface{}{
		"params": map[string]interface{}{
			"contextId": "ui-context",
		},
	}
	originalContextId := getRPCStringParam(rpcReq, "contextId")
	contextId := applyContextModeToRPC(rpcReq, model.ContextModeStateless)
	req := httptest.NewRequest(http.MethodPost, "/agent/test", nil)

	root := resolveRootContextId(req, rpcReq, contextId, originalContextId)
	if root == nil || *root != "ui-context" {
		t.Fatalf("root = %v, want ui-context", root)
	}
}
