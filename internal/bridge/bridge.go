package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"a2a-platform/internal/config"
)

type BridgeAgent struct {
	Config config.BridgeAgent
}

func New(cfg config.BridgeAgent) *BridgeAgent {
	return &BridgeAgent{Config: cfg}
}

func (b *BridgeAgent) HandleRequest(ctx context.Context, w http.ResponseWriter, inputText, taskId, contextId string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, "streaming not supported", 500)
		return
	}

	skill := b.selectSkill(inputText)

	tctx := &TemplateContext{
		InputText: inputText,
		TaskId:    taskId,
		ContextId: contextId,
		SkillId:   skill.ID,
	}

	// Send working status
	b.writeSSE(w, flusher, map[string]interface{}{
		"jsonrpc": "2.0",
		"result": map[string]interface{}{
			"id":        taskId,
			"contextId": contextId,
			"status":    map[string]string{"state": "working"},
		},
	})

	// Invoke skill
	var resultText string
	var invokeErr error

	switch skill.Invoke.Type {
	case "http":
		resultText, invokeErr = invokeHTTP(ctx, &skill.Invoke, b.Config.Target.HTTP, tctx)
	case "cli":
		resultText, invokeErr = invokeCLI(ctx, &skill.Invoke, b.Config.Target.CLI, tctx)
	default:
		invokeErr = fmt.Errorf("unknown invoke type: %s", skill.Invoke.Type)
	}

	if invokeErr != nil {
		slog.Error("Bridge skill invoke failed", "agent", b.Config.Name, "skill", skill.ID, "error", invokeErr)
		b.writeSSE(w, flusher, map[string]interface{}{
			"jsonrpc": "2.0",
			"result": map[string]interface{}{
				"id":        taskId,
				"contextId": contextId,
				"status": map[string]interface{}{
					"state":   "failed",
					"message": map[string]interface{}{"role": "agent", "parts": []map[string]string{{"text": invokeErr.Error()}}},
				},
			},
		})
		return
	}

	// Send artifact
	b.writeSSE(w, flusher, map[string]interface{}{
		"jsonrpc": "2.0",
		"result": map[string]interface{}{
			"id":        taskId,
			"contextId": contextId,
			"artifact": map[string]interface{}{
				"parts": []map[string]string{{"text": resultText}},
			},
		},
	})

	// Send completed
	b.writeSSE(w, flusher, map[string]interface{}{
		"jsonrpc": "2.0",
		"result": map[string]interface{}{
			"id":        taskId,
			"contextId": contextId,
			"status": map[string]interface{}{
				"state":   "completed",
				"message": map[string]interface{}{"role": "agent", "parts": []map[string]string{{"text": resultText}}},
			},
		},
	})
}

func (b *BridgeAgent) selectSkill(inputText string) *config.BridgeSkill {
	if len(b.Config.Skills) == 0 {
		return &config.BridgeSkill{ID: "default", Invoke: config.SkillInvoke{Type: "http"}}
	}
	if len(b.Config.Skills) == 1 {
		return &b.Config.Skills[0]
	}
	firstWord := strings.Fields(inputText)
	if len(firstWord) > 0 {
		for i := range b.Config.Skills {
			if strings.EqualFold(b.Config.Skills[i].ID, firstWord[0]) {
				return &b.Config.Skills[i]
			}
		}
	}
	return &b.Config.Skills[0]
}

func (b *BridgeAgent) writeSSE(w http.ResponseWriter, flusher http.Flusher, data interface{}) {
	jsonBytes, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: %s\n\n", string(jsonBytes))
	flusher.Flush()
}

func writeJSONError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// BridgeRegistry manages in-process bridge agents.
type BridgeRegistry struct {
	agents map[string]*BridgeAgent
	mu     sync.RWMutex
}

func NewRegistry() *BridgeRegistry {
	return &BridgeRegistry{agents: make(map[string]*BridgeAgent)}
}

func (r *BridgeRegistry) Register(name string, agent *BridgeAgent) {
	r.mu.Lock()
	r.agents[name] = agent
	r.mu.Unlock()
}

func (r *BridgeRegistry) Get(name string) *BridgeAgent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.agents[name]
}

func (r *BridgeRegistry) Remove(name string) {
	r.mu.Lock()
	delete(r.agents, name)
	r.mu.Unlock()
}

func (r *BridgeRegistry) List() []config.BridgeAgent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []config.BridgeAgent
	for _, a := range r.agents {
		result = append(result, a.Config)
	}
	return result
}
