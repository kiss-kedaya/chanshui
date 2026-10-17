package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kiss-kedaya/chanzhushui/internal/gateway"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStructuredToolProbeAcrossProtocols(t *testing.T) {
	for _, mode := range []string{"anthropic", "openai", "chat"} {
		t.Run(mode, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
					t.Fatal(e)
				}
				if respondToolRoundtrip(t, w, body, mode) {
					return
				}
				var prompt string
				if mode == "openai" {
					prompt = body["input"].([]any)[0].(map[string]any)["content"].(string)
				} else {
					prompt = body["messages"].([]any)[0].(map[string]any)["content"].(string)
				}
				start := strings.Index(prompt, "nonce ") + 6
				nonce := strings.Fields(prompt[start:])[0]
				args := map[string]any{"nonce": nonce, "sum": 42}
				raw, _ := json.Marshal(args)
				if body["tool_choice"] == nil || len(body["tools"].([]any)) != 1 {
					t.Error("missing tool definition")
				}
				var reply any
				switch mode {
				case "anthropic":
					reply = map[string]any{"content": []any{map[string]any{"type": "tool_use", "id": "call_test", "name": "audit_echo", "input": args}}, "usage": map[string]int{"input_tokens": 10}}
				case "openai":
					reply = map[string]any{"status": "completed", "output": []any{map[string]any{"type": "function_call", "call_id": "call_test", "name": "audit_echo", "arguments": string(raw)}}}
				default:
					reply = map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "", "tool_calls": []any{map[string]any{"id": "call_test", "type": "function", "function": map[string]string{"name": "audit_echo", "arguments": string(raw)}}}}, "finish_reason": "tool_calls"}}}
				}
				json.NewEncoder(w).Encode(reply)
			}))
			defer s.Close()
			r, e := Tools(context.Background(), gateway.Client{BaseURL: s.URL, Mode: mode}, "claude-opus-5", 10)
			if e != nil || r.Pass == nil || !*r.Pass {
				t.Fatal(r, e)
			}
		})
	}
}
func TestNetworkNeedsSearchEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		pass       bool
	}{{"claim", `{"content":[{"type":"text","text":"I searched online, https://example.com"}]}`, false}, {"evidence", `{"content":[{"type":"server_tool_use","id":"search_1","name":"web_search","input":{"query":"go releases"}},{"type":"web_search_tool_result","tool_use_id":"search_1","content":[{"type":"web_search_result","url":"https://github.com/golang/go/releases","title":"Go releases"}]}]}`, true}} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }))
			defer s.Close()
			r, e := Web(context.Background(), gateway.Client{BaseURL: s.URL, Mode: "anthropic"}, "claude-opus-5", 10)
			got := r.Pass != nil && *r.Pass
			if e != nil || got != tc.pass {
				t.Fatal(r, e)
			}
		})
	}
	r, e := Web(context.Background(), gateway.Client{Mode: "chat"}, "gpt-6-sol", 10)
	if e != nil || r.Status != "unknown" {
		t.Fatal(r, e)
	}
}
func respondToolRoundtrip(t *testing.T, w http.ResponseWriter, body map[string]any, mode string) bool {
	var items []any
	if mode == "openai" {
		items, _ = body["input"].([]any)
	} else {
		items, _ = body["messages"].([]any)
	}
	if len(items) < 3 {
		return false
	}
	last := items[len(items)-1].(map[string]any)
	var token string
	switch mode {
	case "openai":
		if last["call_id"] != "call_test" {
			t.Error("wrong call_id")
		}
		token, _ = last["output"].(string)
		json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": token}}}}})
	case "anthropic":
		blocks := last["content"].([]any)
		b := blocks[0].(map[string]any)
		if b["tool_use_id"] != "call_test" {
			t.Error("wrong tool_use_id")
		}
		token, _ = b["content"].(string)
		json.NewEncoder(w).Encode(map[string]any{"content": []any{map[string]string{"type": "text", "text": token}}})
	default:
		if last["tool_call_id"] != "call_test" {
			t.Error("wrong tool_call_id")
		}
		token, _ = last["content"].(string)
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": token}, "finish_reason": "stop"}}})
	}
	return true
}
