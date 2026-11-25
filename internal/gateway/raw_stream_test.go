package gateway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostMessagesRawStreamUsage(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		fmt.Fprint(w, "event: message_start\n")
		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":2,\"cache_creation\":{\"ephemeral_5m_input_tokens\":5900},\"cache_read_input_tokens\":0}}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"OK\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":3}}\n\n")
	}))
	defer up.Close()
	client := Client{BaseURL: up.URL, Key: "k"}
	res, err := client.PostMessagesRaw(context.Background(), map[string]any{
		"model": "claude-opus-5", "stream": true, "max_tokens": 16,
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	}, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 200 || res.InputTokens != 2 || res.CacheCreation5m != 5900 || res.StopReason != "end_turn" || res.Answer != "OK" {
		t.Fatalf("stream result = %+v", res)
	}
}
