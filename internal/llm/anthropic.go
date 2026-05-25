package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type AnthropicProvider struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
}

func NewAnthropicProvider(baseURL, apiKey string) *AnthropicProvider {
	return &AnthropicProvider{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *AnthropicProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	body := p.buildRequest(req)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("Anthropic API error %d: %s", resp.StatusCode, string(errBody))
	}

	ch := make(chan StreamEvent, 32)
	go p.readStream(resp.Body, ch)
	return ch, nil
}

func (p *AnthropicProvider) buildRequest(req *ChatRequest) map[string]interface{} {
	msgs := make([]map[string]interface{}, 0, len(req.Messages))

	for _, m := range req.Messages {
		switch m.Role {
		case "assistant":
			content := make([]interface{}, 0)
			if m.Content != "" {
				content = append(content, map[string]interface{}{"type": "text", "text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				var input interface{}
				json.Unmarshal([]byte(tc.Arguments), &input)
				content = append(content, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": input,
				})
			}
			msgs = append(msgs, map[string]interface{}{"role": "assistant", "content": content})
		case "tool":
			msgs = append(msgs, map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": m.ToolCallID,
						"content":     m.Content,
					},
				},
			})
		default:
			msgs = append(msgs, map[string]interface{}{"role": m.Role, "content": m.Content})
		}
	}

	body := map[string]interface{}{
		"model":      req.Model,
		"messages":   msgs,
		"max_tokens": req.MaxTokens,
		"stream":     true,
	}
	if req.SystemPrompt != "" {
		body["system"] = req.SystemPrompt
	}

	if len(req.Tools) > 0 {
		tools := make([]map[string]interface{}, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = map[string]interface{}{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": t.InputSchema,
			}
		}
		body["tools"] = tools
	}

	return body
}

func (p *AnthropicProvider) readStream(body io.ReadCloser, ch chan<- StreamEvent) {
	defer close(ch)
	defer func() {
		if v := recover(); v != nil {
			ch <- StreamEvent{Type: "error", Error: fmt.Errorf("anthropic stream panic: %v", v)}
		}
	}()
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentToolCall *ToolCall

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var evt map[string]interface{}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			ch <- StreamEvent{Type: "error", Error: fmt.Errorf("anthropic stream malformed JSON: %w", err)}
			return
		}

		evtType, _ := evt["type"].(string)

		switch evtType {
		case "content_block_start":
			if cb, ok := evt["content_block"].(map[string]interface{}); ok {
				if cbType, _ := cb["type"].(string); cbType == "tool_use" {
					currentToolCall = &ToolCall{
						ID:   stringVal(cb, "id"),
						Name: stringVal(cb, "name"),
					}
				}
			}

		case "content_block_delta":
			if delta, ok := evt["delta"].(map[string]interface{}); ok {
				deltaType, _ := delta["type"].(string)
				switch deltaType {
				case "text_delta":
					if text, _ := delta["text"].(string); text != "" {
						ch <- StreamEvent{Type: "text", Text: text}
					}
				case "input_json_delta":
					if currentToolCall != nil {
						if partial, _ := delta["partial_json"].(string); partial != "" {
							currentToolCall.Arguments += partial
						}
					}
				}
			}

		case "content_block_stop":
			if currentToolCall != nil {
				ch <- StreamEvent{Type: "tool_call", ToolCall: currentToolCall}
				currentToolCall = nil
			}

		case "message_stop":
			ch <- StreamEvent{Type: "done"}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- StreamEvent{Type: "error", Error: fmt.Errorf("anthropic stream read error: %w", err)}
	}
}

func stringVal(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}
