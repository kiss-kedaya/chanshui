package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kiss-kedaya/chanshui/internal/gateway"
	"github.com/kiss-kedaya/chanshui/internal/iq"
)

// 识图题也走默认 high；上游不认 effort 时由网关回退到不指定。
func TestVisionUsesHighEffort(t *testing.T) {
	var body string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"最终答案: 1\"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2}}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer up.Close()

	g := gateway.Client{BaseURL: up.URL, Key: "sk-test", HTTP: up.Client(), DefaultEffort: "medium"}
	item := iq.Question{ID: "vision", MaxScore: 20, Answer: "1", ImagePNG: []byte{1, 2, 3}}
	v, err := (&Server{}).askQuestion(context.Background(), g, Request{Model: "deepseek-flash", Key: "sk-test", Timeout: 10}, item)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"reasoning_effort":"high"`) {
		t.Fatalf("vision high effort not applied: %s", body)
	}
	if v.Earned != 20 || v.Error != "" {
		t.Fatalf("answer lost: %+v", v)
	}
}
