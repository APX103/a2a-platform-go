package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"a2a-platform/internal/config"
)

func invokeHTTP(ctx context.Context, skill *config.SkillInvoke, target *config.BridgeHTTPTarget, tctx *TemplateContext) (string, error) {
	url := ""
	if skill.URL != "" {
		url = renderString(skill.URL, tctx)
	} else if target != nil && target.BaseURL != "" {
		url = strings.TrimRight(target.BaseURL, "/") + "/" + strings.TrimLeft(skill.Path, "/")
	} else {
		return "", fmt.Errorf("no URL configured for skill")
	}

	method := skill.Method
	if method == "" {
		method = "POST"
	}

	var bodyReader io.Reader
	if skill.Body != nil {
		rendered := renderBody(skill.Body, tctx)
		bodyBytes, err := json.Marshal(rendered)
		if err != nil {
			return "", fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	timeout := 60 * time.Second
	if skill.Timeout > 0 {
		timeout = time.Duration(skill.Timeout) * time.Millisecond
	}
	if err := validateBridgeURL(url); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	// Merge headers: target defaults + skill overrides
	if target != nil {
		for k, v := range target.Headers {
			req.Header.Set(k, renderString(v, tctx))
		}
	}
	for k, v := range skill.Headers {
		req.Header.Set(k, renderString(v, tctx))
	}
	if req.Header.Get("Content-Type") == "" && bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateBytes(respBody, 200))
	}

	// Extract response text
	if skill.Response != nil && skill.Response.Raw {
		return string(respBody), nil
	}

	var parsed interface{}
	if json.Unmarshal(respBody, &parsed) != nil {
		return string(respBody), nil
	}

	if skill.Response != nil && skill.Response.Text != "" {
		return extractFromResponse(skill.Response.Text, parsed), nil
	}

	// Default: return full JSON
	return string(respBody), nil
}

func validateBridgeURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}
	switch parsed.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("unsupported URL scheme: %s", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("missing URL host")
	}
	return nil
}

func truncateBytes(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "..."
}
