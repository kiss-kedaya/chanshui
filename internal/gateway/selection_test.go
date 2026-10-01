package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestExplicitProtocolsDoNotFallback(t *testing.T) {
	for _, tc := range []struct{ mode, model, path string }{{"chat", "claude-opus-5", "/v1/chat/completions"}, {"anthropic", "gpt-6-astra", "/v1/messages"}, {"openai", "claude-opus-5", "/v1/responses"}} {
		t.Run(tc.mode, func(t *testing.T) {
			var count atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count.Add(1)
				if r.URL.Path != tc.path {
					t.Error(r.URL.Path)
				}
				w.WriteHeader(404)
			}))
			defer s.Close()
			c := Client{BaseURL: s.URL, Mode: tc.mode, RetryDelay: time.Millisecond}
			m, e := c.CallMessages(context.Background(), CallRequest{Model: tc.model, Prompt: "x"})
			if e != nil || count.Load() != 4 || m.Attempts != 4 {
				t.Fatal(count.Load(), m, e)
			}
		})
	}
}
