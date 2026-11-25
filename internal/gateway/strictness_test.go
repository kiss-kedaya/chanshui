package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeStrictnessStrictUpstream(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := new(strings.Builder)
		buf := make([]byte, 1<<16)
		for {
			n, err := r.Body.Read(buf)
			raw.Write(buf[:n])
			if err != nil {
				break
			}
		}
		body := raw.String()
		if r.Header.Get("anthropic-beta") == "invalid-beta-flag-9999" {
			w.WriteHeader(400)
			fmt.Fprint(w, "invalid beta header: consult https://docs.anthropic.com/docs")
			return
		}
		var parsed struct {
			MaxTokens *int `json:"max_tokens"`
			Messages  []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		json.Unmarshal([]byte(body), &parsed)
		malformed := strings.Contains(body, "foo_unknown_field") || parsed.MaxTokens == nil || *parsed.MaxTokens == 0
		for _, m := range parsed.Messages {
			if m.Role == "system" {
				malformed = true
			}
		}
		if malformed {
			w.WriteHeader(400)
			fmt.Fprint(w, "invalid request")
			return
		}
		w.WriteHeader(200)
		fmt.Fprint(w, "{\"content\":[{\"type\":\"text\",\"text\":\"NONE\"}],\"stop_reason\":\"end_turn\"}")
	}))
	defer up.Close()
	client := Client{BaseURL: up.URL, Key: "k"}
	checks := client.ProbeStrictness(context.Background(), "claude-opus-5", 10)
	if len(checks) != 9 {
		t.Fatalf("checks = %d", len(checks))
	}
	for _, check := range checks {
		if !check.Pass || check.Skipped {
			t.Fatalf("%s did not pass: %+v", check.Name, check)
		}
	}
}

func TestProbeStrictnessLaxUpstream(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, "{\"diagnostics\":null,\"input_transformations\":[],\"content\":[{\"type\":\"text\",\"text\":\"BANANA\"}]}")
	}))
	defer up.Close()
	client := Client{BaseURL: up.URL, Key: "k"}
	byName := map[string]StrictnessCheck{}
	for _, check := range client.ProbeStrictness(context.Background(), "claude-opus-5", 10) {
		byName[check.Name] = check
	}
	if byName["system_role"].Pass {
		t.Fatal("system role accepted but marked pass")
	}
	if byName["response_fields"].Pass {
		t.Fatal("non-official fields accepted but marked pass")
	}
	if byName["context_injection"].Pass {
		t.Fatal("injected context accepted but marked pass")
	}
}
