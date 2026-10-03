package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBetaHeaderMergesWithoutDropping(t *testing.T) {
	cases := []struct {
		name string
		req  CallRequest
		want string
	}{
		{"none", CallRequest{}, ""},
		{"updates adds its beta", CallRequest{Thinking: &ThinkingConfig{Type: "adaptive", Display: "updates"}}, thinkingDisplayUpdatesBeta},
		{"summarized adds nothing", CallRequest{Thinking: &ThinkingConfig{Type: "adaptive", Display: "summarized"}}, ""},
		{"caller flag kept", CallRequest{Betas: []string{"context-1m-2025-08-07"}}, "context-1m-2025-08-07"},
		{"caller flag kept and updates appended",
			CallRequest{Betas: []string{"context-1m-2025-08-07"}, Thinking: &ThinkingConfig{Type: "adaptive", Display: "updates"}},
			"context-1m-2025-08-07," + thinkingDisplayUpdatesBeta},
		{"already present is not duplicated",
			CallRequest{Betas: []string{" " + thinkingDisplayUpdatesBeta + " "}, Thinking: &ThinkingConfig{Type: "adaptive", Display: "updates"}},
			thinkingDisplayUpdatesBeta},
		{"comma list from caller is split",
			CallRequest{Betas: []string{"a-1, b-2"}, PriorSignature: "sig"},
			"a-1,b-2,interleaved-thinking-2025-05-14"},
	}
	for _, tc := range cases {
		if got := betaHeader(tc.req); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestBackoffIsLongerFor5xx(t *testing.T) {
	c := &Client{}
	for attempt := 0; attempt < 3; attempt++ {
		short := c.backoff(attempt, 429)
		long := c.backoff(attempt, 500)
		minLong := Upstream5xxBackoff * time.Duration(1<<attempt)
		if long < minLong || long > minLong+minLong/4 {
			t.Fatalf("attempt %d: 5xx backoff %v outside [%v, %v]", attempt, long, minLong, minLong+minLong/4)
		}
		if short >= minLong {
			t.Fatalf("attempt %d: non-5xx backoff %v should stay short", attempt, short)
		}
	}
	fast := &Client{RetryDelay: time.Millisecond}
	if d := fast.backoff(2, 500); d > 6*time.Millisecond {
		t.Fatalf("RetryDelay override ignored for 5xx: %v", d)
	}
}

func TestUpdatesDisplaySendsBetaHeaderOnTheWire(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("anthropic-beta")
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Key: "k", HTTP: srv.Client(), NoRetry: true}
	_, err := c.callMessagesOnce(context.Background(), CallRequest{
		Model: "claude-opus-5-5", MaxTokens: 64, Prompt: "hi",
		Thinking: &ThinkingConfig{Type: "adaptive", Display: "updates"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != thinkingDisplayUpdatesBeta {
		t.Fatalf("anthropic-beta = %q, want %q", got, thinkingDisplayUpdatesBeta)
	}
}
