package llm

import (
	"io"
	"strings"
	"testing"
)

type errReader struct{}

func (errReader) Read(p []byte) (int, error) {
	copy(p, "data: ")
	return 6, io.ErrUnexpectedEOF
}

func (errReader) Close() error {
	return nil
}

func TestOpenAIReadStreamReportsMalformedJSON(t *testing.T) {
	provider := &OpenAIProvider{}
	ch := make(chan StreamEvent, 4)

	provider.readStream(io.NopCloser(strings.NewReader("data: {bad-json}\n\n")), ch)

	var sawError bool
	for evt := range ch {
		if evt.Type == "error" {
			sawError = true
		}
		if evt.Type == "done" {
			t.Fatal("malformed stream emitted done")
		}
	}
	if !sawError {
		t.Fatal("missing error event")
	}
}

func TestOpenAIReadStreamReportsCompactMalformedJSON(t *testing.T) {
	provider := &OpenAIProvider{}
	ch := make(chan StreamEvent, 4)

	provider.readStream(io.NopCloser(strings.NewReader("data:{bad-json}\n\ndata: [DONE]\n\n")), ch)

	var sawError bool
	for evt := range ch {
		if evt.Type == "error" {
			sawError = true
		}
		if evt.Type == "done" {
			t.Fatal("malformed stream emitted done")
		}
	}
	if !sawError {
		t.Fatal("missing error event")
	}
}

func TestOpenAIReadStreamReportsScannerError(t *testing.T) {
	provider := &OpenAIProvider{}
	ch := make(chan StreamEvent, 4)

	provider.readStream(errReader{}, ch)

	var sawError bool
	for evt := range ch {
		if evt.Type == "error" {
			sawError = true
		}
	}
	if !sawError {
		t.Fatal("missing scanner error event")
	}
}

func TestOpenAIReadStreamReportsEOFWithoutDone(t *testing.T) {
	provider := &OpenAIProvider{}
	ch := make(chan StreamEvent, 4)

	provider.readStream(io.NopCloser(strings.NewReader(`data: {"choices":[{"delta":{"content":"hello"}}]}`+"\n\n")), ch)

	var sawError bool
	for evt := range ch {
		if evt.Type == "error" {
			sawError = true
		}
		if evt.Type == "done" {
			t.Fatal("truncated stream emitted done")
		}
	}
	if !sawError {
		t.Fatal("missing error event")
	}
}

func TestOpenAIReadStreamSkipsBlankDataFrames(t *testing.T) {
	provider := &OpenAIProvider{}
	ch := make(chan StreamEvent, 4)

	provider.readStream(io.NopCloser(strings.NewReader("data: \n\ndata: [DONE]\n\n")), ch)

	var sawDone bool
	for evt := range ch {
		if evt.Type == "error" {
			t.Fatalf("blank data frame emitted error: %v", evt.Error)
		}
		if evt.Type == "done" {
			sawDone = true
		}
	}
	if !sawDone {
		t.Fatal("missing done event")
	}
}

func TestAnthropicReadStreamReportsMalformedJSON(t *testing.T) {
	provider := &AnthropicProvider{}
	ch := make(chan StreamEvent, 4)

	provider.readStream(io.NopCloser(strings.NewReader("data: {bad-json}\n\n")), ch)

	var sawError bool
	for evt := range ch {
		if evt.Type == "error" {
			sawError = true
		}
		if evt.Type == "done" {
			t.Fatal("malformed stream emitted done")
		}
	}
	if !sawError {
		t.Fatal("missing error event")
	}
}

func TestAnthropicReadStreamReportsCompactMalformedJSON(t *testing.T) {
	provider := &AnthropicProvider{}
	ch := make(chan StreamEvent, 4)

	provider.readStream(io.NopCloser(strings.NewReader(`data:{bad-json}`+"\n\n"+`data: {"type":"message_stop"}`+"\n\n")), ch)

	var sawError bool
	for evt := range ch {
		if evt.Type == "error" {
			sawError = true
		}
		if evt.Type == "done" {
			t.Fatal("malformed stream emitted done")
		}
	}
	if !sawError {
		t.Fatal("missing error event")
	}
}

func TestAnthropicReadStreamReportsScannerError(t *testing.T) {
	provider := &AnthropicProvider{}
	ch := make(chan StreamEvent, 4)

	provider.readStream(errReader{}, ch)

	var sawError bool
	for evt := range ch {
		if evt.Type == "error" {
			sawError = true
		}
	}
	if !sawError {
		t.Fatal("missing scanner error event")
	}
}

func TestAnthropicReadStreamReportsEOFWithoutMessageStop(t *testing.T) {
	provider := &AnthropicProvider{}
	ch := make(chan StreamEvent, 4)

	provider.readStream(io.NopCloser(strings.NewReader(`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`+"\n\n")), ch)

	var sawError bool
	for evt := range ch {
		if evt.Type == "error" {
			sawError = true
		}
		if evt.Type == "done" {
			t.Fatal("truncated stream emitted done")
		}
	}
	if !sawError {
		t.Fatal("missing error event")
	}
}

func TestAnthropicReadStreamSkipsBlankDataFrames(t *testing.T) {
	provider := &AnthropicProvider{}
	ch := make(chan StreamEvent, 4)

	provider.readStream(io.NopCloser(strings.NewReader(`data: `+"\n\n"+`data: {"type":"message_stop"}`+"\n\n")), ch)

	var sawDone bool
	for evt := range ch {
		if evt.Type == "error" {
			t.Fatalf("blank data frame emitted error: %v", evt.Error)
		}
		if evt.Type == "done" {
			sawDone = true
		}
	}
	if !sawDone {
		t.Fatal("missing done event")
	}
}
