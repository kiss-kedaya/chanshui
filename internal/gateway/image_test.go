package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The vision item only works if every protocol carries the PNG as an inline image.
func TestInlineImagesPerProtocol(t *testing.T) {
	img := Image{MIME: "image/png", Data: []byte{1, 2, 3}}
	cases := []struct {
		model string
		kind  string
	}{
		{"claude-opus-5", `"type":"image"`},
		{"gpt-5.4", `"type":"input_image"`},
		{"deepseek-v4.1-flash", `"type":"image_url"`},
	}
	for _, c := range cases {
		var raw []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ = io.ReadAll(r.Body)
			w.Header().Set("content-type", "application/json")
			io.WriteString(w, `{"content":[{"type":"text","text":"ok"}],"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}],"choices":[{"message":{"content":"ok"}}],"usage":{"input_tokens":1,"output_tokens":1}}`)
		}))
		cl := &Client{BaseURL: srv.URL, Key: "k", NoRetry: true}
		if _, err := cl.CallMessages(context.Background(), CallRequest{Model: c.model, Prompt: "p", MaxTokens: 16, Images: []Image{img}}); err != nil {
			t.Fatalf("%s: %v", c.model, err)
		}
		srv.Close()
		body := string(raw)
		if !strings.Contains(body, c.kind) {
			t.Fatalf("%s body has no %s: %s", c.model, c.kind, body)
		}
		if !strings.Contains(body, "AQID") {
			t.Fatalf("%s image data not base64-encoded: %s", c.model, body)
		}
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("%s body is not JSON: %v", c.model, err)
		}
	}
}

// Without images the request shape must stay exactly as it was.
func TestNoImagesKeepsStringContent(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		w.Header().Set("content-type", "application/json")
		io.WriteString(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer srv.Close()
	cl := &Client{BaseURL: srv.URL, Key: "k", NoRetry: true}
	if _, err := cl.CallMessages(context.Background(), CallRequest{Model: "claude-opus-5", Prompt: "p", MaxTokens: 16}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"content":"p"`) {
		t.Fatalf("text-only content changed shape: %s", raw)
	}
}

// The chart question lives in the image; an empty prompt must not emit an empty text block.
func TestImageOnlyOmitsEmptyTextBlock(t *testing.T) {
	img := Image{MIME: "image/png", Data: []byte{1, 2, 3}}
	for _, model := range []string{"claude-opus-5", "gpt-5.4", "deepseek-v4.1-flash"} {
		var raw []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ = io.ReadAll(r.Body)
			w.Header().Set("content-type", "application/json")
			io.WriteString(w, `{"content":[{"type":"text","text":"ok"}],"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}],"choices":[{"message":{"content":"ok"}}],"usage":{"input_tokens":1,"output_tokens":1}}`)
		}))
		cl := &Client{BaseURL: srv.URL, Key: "k", NoRetry: true}
		if _, err := cl.CallMessages(context.Background(), CallRequest{Model: model, Prompt: "", MaxTokens: 16, Images: []Image{img}}); err != nil {
			srv.Close()
			t.Fatalf("%s: %v", model, err)
		}
		srv.Close()
		body := string(raw)
		if strings.Contains(body, `"text":""`) || strings.Contains(body, `"text": ""`) {
			t.Fatalf("%s sent an empty text block: %s", model, body)
		}
		if !strings.Contains(body, "AQID") {
			t.Fatalf("%s lost the image: %s", model, body)
		}
	}
}
