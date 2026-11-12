package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kiss-kedaya/chanshui/internal/history"
)

// A text-only upstream rejects Q10's image. That must mark only Q10 as not
// applicable; the rest of the IQ paper still scores.
func TestVisionUnsupportedSkipsOnlyQ10(t *testing.T) {
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
		t.Fatalf("skipping Q10 must not mark the paper incomplete: %#v", iq)
	}
	if _, ok := asInt(iq["score"]); !ok {
		t.Fatalf("paper score missing: %#v", iq["score"])
	}
	lines, _ := iq["per_question"].([]QuestionResult)
	if len(lines) != 10 {
		t.Fatalf("per_question = %d, want 10", len(lines))
	}
	last := lines[9]
	if last.ID != "Q10" || !last.Skipped {
		t.Fatalf("Q10 not marked skipped: %+v", last)
	}
	if !strings.Contains(last.Error, "图片") {
		t.Fatalf("skip reason lost: %q", last.Error)
	}
}
