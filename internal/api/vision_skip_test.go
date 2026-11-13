package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kiss-kedaya/chanshui/internal/history"
	"github.com/kiss-kedaya/chanshui/internal/iq"
)

// A text-only upstream rejects the vision image. The vision section records
// zero recognition while the IQ paper still scores on its own.
func TestVisionUnsupportedScoresZero(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			fmt.Fprint(w, `{"data":[{"id":"claude-opus-5"}]}`)
			return
		}
		raw := make([]byte, 0, 64<<10)
		buf := make([]byte, 32<<10)
		for {
			n, err := r.Body.Read(buf)
			raw = append(raw, buf[:n]...)
			if err != nil {
				break
			}
		}
		if strings.Contains(string(raw), `"type":"image"`) {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":{"type":"invalid_request_error","message":"unknown variant image, expected text"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":2}}`)
	}))
	defer up.Close()

	db, err := history.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s, err := New(db, Options{AllowPrivate: true, HTTP: up.Client()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	w := post(t, s, "/api/detect", Request{BaseURL: up.URL + "/v1", Key: "sk-test", Model: "claude-opus-5", Timeout: 10})
	if w.Code != 202 {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	var d map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &d)
	task := waitTask(t, s, d["task_id"])

	iq, ok := task.Verdict.IQ.(map[string]any)
	if !ok {
		t.Fatalf("missing IQ verdict: %#v", task.Verdict.IQ)
	}
	if done, _ := iq["complete"].(bool); !done {
		t.Fatalf("the vision item must not mark the IQ paper incomplete: %#v", iq)
	}
	if _, ok := asInt(iq["score"]); !ok {
		t.Fatalf("paper score missing: %#v", iq["score"])
	}
	lines, _ := iq["per_question"].([]QuestionResult)
	if len(lines) != 9 {
		t.Fatalf("per_question = %d, want 9", len(lines))
	}
	vision, ok := task.Verdict.Vision.(map[string]any)
	if !ok {
		t.Fatalf("missing vision verdict: %#v", task.Verdict.Vision)
	}
	if skipped, _ := vision["skipped"].(bool); !skipped {
		t.Fatalf("vision not marked skipped: %#v", vision)
	}
	if recognized, _ := vision["recognized"].(bool); recognized {
		t.Fatalf("a rejected image cannot count as recognized: %#v", vision)
	}
	if n, _ := asInt(vision["score"]); n != 0 {
		t.Fatalf("skipped vision score = %d, want 0", n)
	}
	if reason, _ := vision["error"].(string); !strings.Contains(reason, "图片") {
		t.Fatalf("skip reason lost: %q", reason)
	}
}

func TestVisionResultScoresTwoDimensions(t *testing.T) {
	item := iq.Question{ID: "vision", MaxScore: 20}
	cases := []struct {
		name       string
		result     QuestionResult
		recognized bool
		score      int
	}{
		{"exact", QuestionResult{ID: "vision", Answer: "66", Earned: 20}, true, 100},
		{"near", QuestionResult{ID: "vision", Answer: "64", Earned: 10, Half: true}, true, 75},
		{"wrong but seen", QuestionResult{ID: "vision", Answer: "12", Earned: 0}, true, 50},
		{"no number", QuestionResult{ID: "vision", Answer: "看不懂"}, false, 0},
		{"skipped", QuestionResult{ID: "vision", Skipped: true, Error: "该模型不接受图片输入，识图项记 0 分"}, false, 0},
	}
	for _, tc := range cases {
		out := visionResult(item, tc.result, nil)
		if out["recognized"] != tc.recognized || out["score"] != tc.score {
			t.Fatalf("%s: %+v", tc.name, out)
		}
	}
}

// 题面只画在图片里，报告不能再把文字题面回显出来。
func TestVisionResultDropsPromptText(t *testing.T) {
	out := visionResult(iq.Question{ID: "vision", MaxScore: 20}, QuestionResult{ID: "vision", Prompt: "把下图 40 根 K 线..."}, nil)
	q, _ := out["question"].(QuestionResult)
	if q.Prompt != "" {
		t.Fatalf("vision prompt echoed: %q", q.Prompt)
	}
}

// 截断但已有正文时照常判分；整段空白时去掉 thinking/effort 再要一次短答。
func TestVisionTruncationFallsBackWithoutThinking(t *testing.T) {
	var calls int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			fmt.Fprint(w, `{"data":[{"id":"claude-opus-5"}]}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(raw), `"thinking"`) {
			fmt.Fprint(w, `{"content":[],"stop_reason":"max_tokens","usage":{"input_tokens":10,"output_tokens":8192}}`)
			return
		}
		fmt.Fprint(w, `{"content":[{"type":"text","text":"最终答案: 66"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":4}}`)
	}))
	defer up.Close()

	db, err := history.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s, err := New(db, Options{AllowPrivate: true, HTTP: up.Client()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	w := postPublic(s, `{"base_url":"`+up.URL+`","api_key":"sk-test","model":"claude-opus-5","timeout":10,"sections":["vision"]}`, "203.0.113.91:1234")
	if w.Code != 202 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	task := waitTask(t, s, created.ID)
	vision, ok := task.Verdict.Vision.(map[string]any)
	if !ok {
		t.Fatalf("missing vision verdict: %#v", task.Verdict.Vision)
	}
	if reason, _ := vision["error"].(string); reason != "" {
		t.Fatalf("truncation leaked as error: %q", reason)
	}
	if recognized, _ := vision["recognized"].(bool); !recognized {
		t.Fatalf("fallback answer not recognized: %#v", vision)
	}
	if n := atomic.LoadInt32(&calls); n < 2 {
		t.Fatalf("image requests = %d, want a no-thinking retry", n)
	}
}
