package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeStrictness(t *testing.T) {
	strict := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			MaxTokens *int `json:"max_tokens"`
			Messages  []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.MaxTokens == nil || *body.MaxTokens == 0 {
			w.WriteHeader(400)
			return
		}
		for _, m := range body.Messages {
			if m.Role == "system" {
				w.WriteHeader(400)
				return
			}
		}
		w.WriteHeader(200)
	}))
	defer strict.Close()
	client := Client{BaseURL: strict.URL, Key: "k"}
	checks := client.ProbeStrictness(context.Background(), "claude-opus-5", 5)
	if len(checks) != 3 {
		t.Fatalf("checks = %d", len(checks))
	}
	for _, c := range checks {
		if !c.Pass || c.Status != 400 {
			t.Fatalf("%s not rejected: %+v", c.Name, c)
		}
	}

	lax := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("BANANA"))
	}))
	defer lax.Close()
	laxClient := Client{BaseURL: lax.URL, Key: "k"}
	for _, c := range laxClient.ProbeStrictness(context.Background(), "claude-opus-5", 5) {
		if c.Pass {
			t.Fatalf("%s passed on a lax upstream", c.Name)
		}
	}
}
