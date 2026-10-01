package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

const successRespJSON = `{
  "id": "msg_01",
  "type": "message",
  "role": "assistant",
  "content": [
    {"type": "thinking", "thinking": "need to compute 2+2", "signature": "sig-abc-123"},
    {"type": "text", "text": "4"}
  ],
  "model": "claude-x",
  "stop_reason": "end_turn",
  "usage": {
    "input_tokens": 44,
    "output_tokens": 33,
    "cache_creation": {"ephemeral_5m_input_tokens": 12, "ephemeral_1h_input_tokens": 5},
    "cache_read_input_tokens": 7,
    "output_tokens_details": {"thinking_tokens": 21}
  }
}`

type fakeWireBody struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	Stream    bool   `json:"stream"`
	Thinking  *struct {
		Type         string `json:"type"`
		Display      string `json:"display"`
		BudgetTokens int    `json:"budget_tokens"`
	} `json:"thinking"`
	System []struct {
		Type         string          `json:"type"`
		Text         string          `json:"text"`
		CacheControl *map[string]any `json:"cache_control"`
	} `json:"system"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func readFakeReq(t *testing.T, r *http.Request) (fakeWireBody, http.Header) {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var fb fakeWireBody
	if err := json.Unmarshal(b, &fb); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	return fb, r.Header.Clone()
}

func TestCallMessagesFullMetrics(t *testing.T) {
	var req fakeWireBody
	var hdr http.Header
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		req, hdr = readFakeReq(t, r)
		w.Header().Set("x-cache", "HIT")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, successRespJSON)
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, Key: "sk-test-1"}
	m, err := c.callMessagesOnce(context.Background(), CallRequest{
		Model: "claude-x", MaxTokens: 16, Prompt: "hi",
	})
	if err != nil {
		t.Fatalf("CallMessages returned error: %v", err)
	}

	if path != "/v1/messages" {
		t.Errorf("request path = %q, want /v1/messages", path)
	}
	if got := hdr.Get("x-api-key"); got != "sk-test-1" {
		t.Errorf("x-api-key = %q", got)
	}
	if got := hdr.Get("anthropic-version"); got != "2023-06-01" {
		t.Errorf("anthropic-version = %q", got)
	}
	if got := hdr.Get("content-type"); got != "application/json" {
		t.Errorf("content-type = %q", got)
	}
	if got := hdr.Get("user-agent"); got != DefaultUA {
		t.Errorf("user-agent = %q, want %q", got, DefaultUA)
	}

	if req.Model != "claude-x" {
		t.Errorf("body model = %q", req.Model)
	}
	if req.MaxTokens != 16 {
		t.Errorf("body max_tokens = %d", req.MaxTokens)
	}
	if len(req.Messages) != 1 || req.Messages[0].Content != "hi" {
		t.Errorf("body messages = %+v", req.Messages)
	}
	if req.Thinking != nil {
		t.Errorf("body thinking sent without ThinkingConfig: %+v", req.Thinking)
	}
	if len(req.System) != 0 {
		t.Errorf("body system sent without System blocks: %+v", req.System)
	}

	if m.Status != 200 {
		t.Errorf("Status = %d", m.Status)
	}
	if m.Ms < 0 {
		t.Errorf("Ms = %d, want >= 0", m.Ms)
	}
	if m.Err != "" {
		t.Errorf("Err = %q, want empty", m.Err)
	}
	if m.InputTokens != 44 {
		t.Errorf("InputTokens = %d, want 44", m.InputTokens)
	}
	if m.OutputTokens != 33 {
		t.Errorf("OutputTokens = %d, want 33", m.OutputTokens)
	}
	if m.ThinkingTokens != 21 {
		t.Errorf("ThinkingTokens = %d, want 21", m.ThinkingTokens)
	}
	if m.CacheCreation5m != 12 {
		t.Errorf("CacheCreation5m = %d, want 12", m.CacheCreation5m)
	}
	if m.CacheCreation1h != 5 {
		t.Errorf("CacheCreation1h = %d, want 5", m.CacheCreation1h)
	}
	if m.CacheRead != 7 {
		t.Errorf("CacheRead = %d, want 7", m.CacheRead)
	}
	if m.ThinkLen != len("need to compute 2+2") {
		t.Errorf("ThinkLen = %d, want %d", m.ThinkLen, len("need to compute 2+2"))
	}
	if m.SigLen != len("sig-abc-123") {
		t.Errorf("SigLen = %d, want %d", m.SigLen, len("sig-abc-123"))
	}
	if m.ThinkingText != "need to compute 2+2" {
		t.Errorf("ThinkingText = %q", m.ThinkingText)
	}
	if m.Signature != "sig-abc-123" {
		t.Errorf("Signature = %q", m.Signature)
	}
	if m.Answer != "4" {
		t.Errorf("Answer = %q, want 4", m.Answer)
	}
	if want := []string{"thinking", "text"}; !reflect.DeepEqual(m.Blocks, want) {
		t.Errorf("Blocks = %v, want %v", m.Blocks, want)
	}
	if m.StopReason != "end_turn" {
		t.Errorf("StopReason = %q", m.StopReason)
	}
	if m.CacheHeader != "HIT" {
		t.Errorf("CacheHeader = %q, want HIT", m.CacheHeader)
	}
	if m.Via != "direct" {
		t.Errorf("Via = %q, want direct", m.Via)
	}
	if m.SSEFrames != nil {
		t.Errorf("SSEFrames = %v, want nil for non-stream call", m.SSEFrames)
	}
}

func TestCallMessagesThinkingAndSystemSerialization(t *testing.T) {
	var req fakeWireBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, _ = readFakeReq(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, Key: "k"}
	_, err := c.callMessagesOnce(context.Background(), CallRequest{
		Model:     "m",
		MaxTokens: 32,
		Prompt:    "p",
		System: []SystemBlock{
			{Text: "plain"},
			{Text: "cached", CacheControl: &CacheControl{}},
			{Text: "cached-1h", CacheControl: &CacheControl{TTL: "1h"}},
		},
		Thinking: &ThinkingConfig{Type: "enabled", Display: "summarized", BudgetTokens: 1024},
	})
	if err != nil {
		t.Fatalf("CallMessages: %v", err)
	}

	if req.Thinking == nil {
		t.Fatal("body thinking missing")
	}
	if req.Thinking.Type != "enabled" || req.Thinking.Display != "summarized" || req.Thinking.BudgetTokens != 1024 {
		t.Errorf("body thinking = %+v", req.Thinking)
	}
	if len(req.System) != 3 {
		t.Fatalf("body system len = %d, want 3", len(req.System))
	}
	if req.System[0].Text != "plain" || req.System[0].CacheControl != nil {
		t.Errorf("system[0] = %+v, want text without cache_control", req.System[0])
	}
	if req.System[1].CacheControl == nil || (*req.System[1].CacheControl)["type"] != "ephemeral" || len(*req.System[1].CacheControl) != 1 {
		t.Errorf("system[1].cache_control = %v, want {type: ephemeral}", req.System[1].CacheControl)
	}
	if req.System[2].CacheControl == nil || (*req.System[2].CacheControl)["ttl"] != "1h" {
		t.Errorf("system[2].cache_control = %v, want ttl 1h", req.System[2].CacheControl)
	}
}

func TestCallMessagesErrorBodyTruncation(t *testing.T) {
	longMsg := strings.Repeat("x", 300)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error","message":"`+longMsg+`"}}`)
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, Key: "k"}
	m, err := c.callMessagesOnce(context.Background(), CallRequest{Model: "m", MaxTokens: 16, Prompt: "p"})
	if err != nil {
		t.Fatalf("CallMessages returned Go error, want nil: %v", err)
	}
	want := "HTTP 429 rate_limit_error " + strings.Repeat("x", 200)
	if m.Err != want {
		t.Errorf("Err = %q (len %d), want %q (len %d)", m.Err, len(m.Err), want, len(want))
	}
	if m.Status != 429 {
		t.Errorf("Status = %d", m.Status)
	}
}

func TestCallMessagesNon200NoErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "<html>bad gateway</html>")
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, Key: "k"}
	m, err := c.callMessagesOnce(context.Background(), CallRequest{Model: "m", MaxTokens: 16, Prompt: "p"})
	if err != nil {
		t.Fatalf("CallMessages returned Go error, want nil: %v", err)
	}
	if !strings.HasPrefix(m.Err, "HTTP 502") {
		t.Errorf("Err = %q, want HTTP 502", m.Err)
	}
}

func TestCallMessagesStreamFrames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fb, _ := readFakeReq(t, r)
		if !fb.Stream {
			t.Errorf("stream flag not sent in request body")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"message_start\",\"message\":{\"content\":[],\"usage\":{\"input_tokens\":1}}}\n\n"+
			"data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"hi\"}}\n\n"+
			"data: [DONE]\n\n"+
			"data: {\"type\":\"should_not_be_read\"}\n\n")
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, Key: "k"}
	m, err := c.callMessagesOnce(context.Background(), CallRequest{Model: "m", MaxTokens: 16, Prompt: "p", Stream: true})
	if err != nil {
		t.Fatalf("CallMessages: %v", err)
	}
	want := []string{
		`{"type":"message_start","message":{"content":[],"usage":{"input_tokens":1}}}`,
		`{"type":"content_block_delta","delta":{"text":"hi"}}`,
	}
	if !reflect.DeepEqual(m.SSEFrames, want) {
		t.Errorf("SSEFrames = %v, want %v", m.SSEFrames, want)
	}
}

func TestCallMessagesCacheHeaderFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("cf-cache-status", "MISS")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, Key: "k"}
	m, err := c.callMessagesOnce(context.Background(), CallRequest{Model: "m", MaxTokens: 16, Prompt: "p"})
	if err != nil {
		t.Fatalf("CallMessages: %v", err)
	}
	if m.CacheHeader != "MISS" {
		t.Errorf("CacheHeader = %q, want MISS from cf-cache-status", m.CacheHeader)
	}
}
