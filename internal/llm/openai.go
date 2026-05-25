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

type OpenAIProvider struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
}

func NewOpenAIProvider(baseURL, apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OpenAIProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	body := p.buildRequest(req)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, string(errBody))
	}

	ch := make(chan StreamEvent, 32)
	go p.readStream(resp.Body, ch)
	return ch, nil
}

func (p *OpenAIProvider) buildRequest(req *ChatRequest) map[string]interface{} {
	msgs := make([]map[string]interface{}, 0, len(req.Messages)+1)

	if req.SystemPrompt != "" {
		msgs = append(msgs, map[string]interface{}{
			"role":    "system",
			"content": req.SystemPrompt,
		})
	}

	for _, m := range req.Messages {
		msg := map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		}
		// MiMo API requires reasoning_content on assistant messages with tool_calls
		// (even if empty), otherwise it returns 400.
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			msg["reasoning_content"] = m.ReasoningContent
		} else if m.ReasoningContent != "" {
			msg["reasoning_content"] = m.ReasoningContent
		}
		if len(m.ToolCalls) > 0 {
			calls := make([]map[string]interface{}, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				calls[i] = map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				}
			}
			msg["tool_calls"] = calls
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		msgs = append(msgs, msg)
	}

	body := map[string]interface{}{
		"model":    req.Model,
		"messages": msgs,
		"stream":   true,
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}

	if len(req.Tools) > 0 {
		tools := make([]map[string]interface{}, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.InputSchema,
				},
			}
		}
		body["tools"] = tools
	}

	return body
}

func (p *OpenAIProvider) readStream(body io.ReadCloser, ch chan<- StreamEvent) {
	defer close(ch)
	defer func() {
		if v := recover(); v != nil {
			ch <- StreamEvent{Type: "error", Error: fmt.Errorf("openai stream panic: %v", v)}
		}
	}()
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	// Accumulate tool calls across chunks
	toolCalls := map[int]*ToolCall{}
	seenDone := false

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			seenDone = true
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			ch <- StreamEvent{Type: "error", Error: fmt.Errorf("openai stream malformed JSON: %w", err)}
			return
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		if delta.ReasoningContent != "" {
			ch <- StreamEvent{Type: "reasoning", Reasoning: delta.ReasoningContent}
		}

		if delta.Content != "" {
			ch <- StreamEvent{Type: "text", Text: delta.Content}
		}

		for _, tc := range delta.ToolCalls {
			existing, ok := toolCalls[tc.Index]
			if !ok {
				existing = &ToolCall{ID: tc.ID, Name: tc.Function.Name}
				toolCalls[tc.Index] = existing
			}
			if tc.ID != "" {
				existing.ID = tc.ID
			}
			if tc.Function.Name != "" {
				existing.Name = tc.Function.Name
			}
			existing.Arguments += tc.Function.Arguments
		}

		if chunk.Choices[0].FinishReason != nil && *chunk.Choices[0].FinishReason == "tool_calls" {
			for _, tc := range toolCalls {
				ch <- StreamEvent{Type: "tool_call", ToolCall: &ToolCall{
					ID:        tc.ID,
					Name:      tc.Name,
					Arguments: tc.Arguments,
				}}
			}
			toolCalls = map[int]*ToolCall{}
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- StreamEvent{Type: "error", Error: fmt.Errorf("openai stream read error: %w", err)}
		return
	}

	if seenDone {
		ch <- StreamEvent{Type: "done"}
	}
}
