package capability

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kiss-kedaya/chanzhushui/internal/gateway"
)

func TestDeepSeekSearchValidatesResponse(t *testing.T) {
	var gotPath, gotAuth, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var body struct {
			Query       string `json:"query"`
			Count       int    `json:"count"`
			NeedContent bool   `json:"need_content"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotQuery = body.Query
		if body.Count != 3 || body.NeedContent {
			t.Errorf("request body %+v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{
			map[string]any{"rerank_score": 2.1, "title": "DeepSeek V4", "url": "https://example.com/a", "snippet": "summary", "summary": "long summary"},
			map[string]any{"rerank_score": 1.8, "title": "Missing URL", "url": "not-a-url", "snippet": "summary"},
		}})
	}))
	defer srv.Close()
	res, err := DeepSeekSearch(context.Background(), gateway.Client{
		BaseURL: srv.URL,
		Key:     "fixture-key",
		HTTP:    srv.Client(),
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "pass" || res.Pass == nil || !*res.Pass {
		t.Fatalf("result %+v", res)
	}
	if gotPath != "/v1/tools/search" || gotAuth != "Bearer fixture-key" || gotQuery == "" {
		t.Fatalf("path=%q auth=%q query=%q", gotPath, gotAuth, gotQuery)
	}
	evidence, _ := res.Evidence.(map[string]any)
	sources, _ := evidence["sources"].([]string)
	if len(sources) != 1 || sources[0] != "https://example.com/a" {
		t.Fatalf("evidence %+v", res.Evidence)
	}
}

func TestDeepSeekSearchUnsupportedAndEmpty(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"unsupported", http.StatusNotFound, `{}`, "unsupported"},
		{"empty", http.StatusOK, `{"results":[]}`, "fail"},
		{"malformed", http.StatusOK, `not-json`, "fail"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			res, err := DeepSeekSearch(context.Background(), gateway.Client{
				BaseURL: srv.URL + "/v1",
				Key:     "fixture",
				HTTP:    srv.Client(),
			}, 10)
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != tc.want {
				t.Fatalf("status %q want %q (%+v)", res.Status, tc.want, res)
			}
			if tc.want == "unsupported" && res.Pass != nil {
				t.Fatalf("unsupported pass %+v", res.Pass)
			}
		})
	}
}

func TestDeepSeekSearchEndpoint(t *testing.T) {
	for _, tc := range []struct{ base, want string }{
		{"https://api.deepseek.com", "https://api.deepseek.com/v1/tools/search"},
		{"https://api.deepseek.com/v1", "https://api.deepseek.com/v1/tools/search"},
		{"https://api.deepseek.com/v1/chat/completions", "https://api.deepseek.com/v1/tools/search"},
		{"https://gateway.example/openai/v1", "https://gateway.example/openai/v1/tools/search"},
	} {
		got, err := deepSeekSearchEndpoint(tc.base)
		if err != nil || got != tc.want {
			t.Fatalf("%s -> %s, %v", tc.base, got, err)
		}
	}
	if _, err := deepSeekSearchEndpoint("file:///tmp"); err == nil {
		t.Fatal("file URL accepted")
	}
}
