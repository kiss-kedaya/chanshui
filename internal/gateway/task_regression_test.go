package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamRequiredRetriesWithStreamOnSameProtocol(t *testing.T) {
	for _, mode := range []string{"openai", "chat", "anthropic", "auto"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			path := ""
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if path != "" && path != r.URL.Path {
					t.Errorf("changed protocol: %s -> %s", path, r.URL.Path)
				}
				path = r.URL.Path
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if body["stream"] != true {
					w.WriteHeader(400)
					fmt.Fprint(w, `{"error":{"message":"streaming is required: this endpoint only accepts \"stream\": true"}}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				switch r.URL.Path {
				case "/v1/responses":
					fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}]}}\n\n")
				case "/v1/messages":
					fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"ok\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n")
				default:
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
				}
			}))
			defer up.Close()
			c := Client{BaseURL: up.URL, Mode: mode, RetryDelay: time.Millisecond}
			// Tools must not cause a protocol fallback when the error is about streaming.
			m, err := c.CallMessages(context.Background(), CallRequest{Model: "gpt-6-astra", Tools: []ToolDefinition{{Name: "echo"}}})
			if err != nil || m.Err != "" || m.Answer != "ok" || !m.StreamComplete || calls != 2 {
				t.Fatalf("calls=%d metrics=%+v err=%v", calls, m, err)
			}
		})
	}
}

func TestPermanentRequestErrorsStopImmediately(t *testing.T) {
	for _, status := range []int{400, 401, 402, 403, 404, 405, 413, 422} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(status)
				fmt.Fprint(w, `{"error":{"message":"request rejected"}}`)
			}))
			defer up.Close()
			c := Client{BaseURL: up.URL, Mode: "openai", RetryDelay: time.Millisecond}
			m, err := c.CallMessages(context.Background(), CallRequest{Model: "gpt-6-astra"})
			if err != nil || m.Err == "" || m.Attempts != 1 || calls != 1 {
				t.Fatalf("calls=%d metrics=%+v err=%v", calls, m, err)
			}
		})
	}
}

type readAfterStop struct{}

func (readAfterStop) Read([]byte) (int, error) { return 0, fmt.Errorf("read after terminal event") }

func TestMessagesStreamReturnsAtTerminalEvent(t *testing.T) {
	for _, terminal := range []string{`{"type":"message_stop"}`, "[DONE]"} {
		body := "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"ok\"}}\n\ndata: " + terminal + "\n\n"
		var m Metrics
		err := parseStream(io.MultiReader(strings.NewReader(body), readAfterStop{}), &m)
		if err != nil || m.Answer != "ok" || !m.StreamComplete {
			t.Fatalf("metrics=%+v err=%v", m, err)
		}
	}
}

func TestStreamFallbackSharesRetryBudget(t *testing.T) {
	for _, noRetry := range []bool{false, true} {
		t.Run(fmt.Sprint(noRetry), func(t *testing.T) {
			calls := 0
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body struct{ Stream bool }
				json.NewDecoder(r.Body).Decode(&body)
				if !body.Stream {
					w.WriteHeader(400)
					fmt.Fprint(w, `{"error":{"message":"streaming is required"}}`)
					return
				}
				w.WriteHeader(503)
			}))
			defer up.Close()
			c := Client{BaseURL: up.URL, NoRetry: noRetry, RetryDelay: time.Millisecond}
			m, err := c.CallMessages(context.Background(), CallRequest{Model: "gpt-6-astra"})
			want := RetryCount + 1
			if noRetry {
				want = 1
			}
			if err != nil || m.Err == "" || calls != want || m.Attempts != want {
				t.Fatalf("calls=%d want=%d metrics=%+v err=%v", calls, want, m, err)
			}
		})
	}
}
