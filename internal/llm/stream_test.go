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
