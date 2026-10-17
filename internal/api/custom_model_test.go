package api

import (
	"context"
	"encoding/json"
	"github.com/kiss-kedaya/chanzhushui/internal/gateway"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCustomModelPreservedOnWire(t *testing.T) {
	s, _, _, _ := testSetup(t)
	for _, name := range []string{"openai/gpt-6.1-sol", "vendor/my-custom-model"} {
		t.Run(name, func(t *testing.T) {
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct{ Model string }
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Model != name {
					t.Errorf("model rewritten: %q", body.Model)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}]}`))
			}))
			defer up.Close()
			q := Request{BaseURL: up.URL, Key: "fixture", Model: name, Protocol: "chat"}
			if err := s.validate(&q, true); err != nil {
				t.Fatal(err)
			}
			g, err := s.client(q)
			if err != nil {
				t.Fatal(err)
			}
			m, err := g.CallMessages(context.Background(), gateway.CallRequest{Model: q.Model, Prompt: "test", MaxTokens: 16, TimeoutSeconds: 10})
			if err != nil || m.Err != "" || m.Answer != "OK" {
				t.Fatalf("%+v %v", m, err)
			}
		})
	}
	for _, name := range []string{"", "a\nb", strings.Repeat("x", 257)} {
		q := Request{BaseURL: "https://fixture.invalid", Key: "fixture", Model: name}
		if s.validate(&q, true) == nil {
			t.Errorf("invalid model accepted: %q", name)
		}
	}
}
