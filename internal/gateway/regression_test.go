package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUsageCacheTotalAndV1(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("wrong path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"type":"message","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":3,"cache_creation_input_tokens":800,"cache_read_input_tokens":2}}`))
	}))
	defer s.Close()
	c := Client{BaseURL: s.URL + "/v1"}
	m, err := c.CallMessages(context.Background(), CallRequest{Model: "test", Prompt: "A", MaxTokens: 16})
	if err != nil || m.Err != "" {
		t.Fatalf("%v %s", err, m.Err)
	}
	if m.CacheCreation5m+m.CacheCreation1h != 800 {
		t.Fatalf("lost aggregate cache: %+v", m)
	}
}

func TestMalformedSuccessNotAccepted(t *testing.T) {
	for _, body := range []string{`<html>oops</html>`, `{}`, `{"error":{"message":"bad"}}`} {
		t.Run(body, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer s.Close()
			c := Client{BaseURL: s.URL}
			m, e := c.CallMessages(context.Background(), CallRequest{Model: "t", Prompt: "x"})
			if e == nil && m.Err == "" {
				t.Fatal("invalid 200 accepted")
			}
		})
	}
}

func TestSSEReconstruction(t *testing.T) {
	body := `event: message_start
data:{"type":"message_start","message":{"type":"message","content":[],"usage":{"input_tokens":7}}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"计算"}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"abc"}}

data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"7417"}}

data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":12}}

data: {"type":"message_stop"}

`
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(body))
	}))
	defer s.Close()
	c := Client{BaseURL: s.URL}
	m, e := c.CallMessages(context.Background(), CallRequest{Model: "t", Prompt: "x", Stream: true})
	if e != nil || m.Err != "" || m.Answer != "7417" || m.ThinkLen != 2 || m.Signature != "abc" || m.OutputTokens != 12 {
		t.Fatalf("bad stream: %+v %v", m, e)
	}
	m = Metrics{}
	if e = c.readStream(strings.NewReader("data: {}\n\n"), &m); e == nil {
		t.Fatal("truncated stream accepted")
	}
}
