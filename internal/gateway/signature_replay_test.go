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

func TestReplayThinkingBlockOmitsThinkingConfig(t *testing.T) {
	var body wireRequest
	var beta string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		beta = r.Header.Get("anthropic-beta")
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"Invalid signature in thinking block"}}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Key: "k", HTTP: srv.Client(), NoRetry: true}
	m, err := c.callMessagesOnce(context.Background(), CallRequest{
		Model: "claude-sonnet-4-5", MaxTokens: 64, Stream: false,
		PriorUser: "同一句采集题", PriorSignature: "sig-real", Prompt: "Reply with the single word OK.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != 400 || !strings.Contains(strings.ToLower(m.Err), "signature") {
		t.Fatalf("metrics %+v", m)
	}
	if body.Thinking != nil {
		t.Fatalf("replay must not send thinking config: %+v", body.Thinking)
	}
	if beta != "interleaved-thinking-2025-05-14" {
		t.Fatalf("replay beta=%q", beta)
	}
	if len(body.Messages) != 3 {
		t.Fatalf("messages=%d", len(body.Messages))
	}
	raw, _ := json.Marshal(body.Messages[1].Content)
	text := string(raw)
	if !strings.Contains(text, `"thinking":""`) || !strings.Contains(text, `"signature":"sig-real"`) || !strings.Contains(text, `"text":"Done."`) {
		t.Fatalf("assistant block %s", text)
	}
}

func TestInvalidSignatureStopsRetry(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"Invalid signature in thinking block"}}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Key: "k", HTTP: srv.Client()}
	m, err := c.CallMessages(context.Background(), CallRequest{
		Model: "claude-sonnet-4-5", MaxTokens: 64, Prompt: "ok",
		PriorSignature: "forged-signature-not-valid", StopOnInvalidSignature: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hits != 1 || m.Attempts != 1 || m.Status != 400 {
		t.Fatalf("hits=%d attempts=%d status=%d err=%s", hits, m.Attempts, m.Status, m.Err)
	}
}
