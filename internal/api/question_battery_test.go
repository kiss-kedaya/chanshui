package api

import (
	"encoding/json"
	"testing"
)

func TestDetectionClientDefaultsToMediumEffort(t *testing.T) {
	s, _, _, _ := testSetup(t)
	g, err := s.client(Request{BaseURL: "https://example.com", Key: "sk-test", Model: "claude-opus-5", Timeout: 10})
	if err != nil {
		t.Fatal(err)
	}
	if g.DefaultEffort != "medium" {
		t.Fatalf("DefaultEffort = %q, want medium", g.DefaultEffort)
	}
}

func TestQuestionBatteryIncludesBridgeItem(t *testing.T) {
	s, up, _, _ := testSetup(t)
	w := post(t, s, "/api/detect", Request{BaseURL: up.URL + "/v1", Key: "sk-test", Model: "claude-opus-5", Timeout: 10})
	if w.Code != 202 {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	var d map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &d)
	task := s.find(d["task_id"])
	if task == nil {
		t.Fatal("task not registered")
	}
	sum := 0
	seenBridge, seenVision := false, false
	for _, q := range task.questions {
		if q.ID == "vision" {
			seenVision = true
			if q.MaxScore != 20 || len(q.ImagePNG) == 0 {
				t.Fatalf("vision = %+v", q)
			}
			continue
		}
		sum += q.MaxScore
		if q.ID == "Q9" {
			seenBridge = true
			if q.Title != "夜间过桥" || q.Answer != "10" || q.MaxScore != 11 {
				t.Fatalf("Q9 = %+v", q)
			}
		}
	}
	if !seenBridge {
		t.Fatal("Q9 missing from the battery")
	}
	if !seenVision {
		t.Fatal("vision item missing from the battery")
	}
	if sum != 100 {
		t.Fatalf("IQ battery max = %d, want 100", sum)
	}
	_ = waitTask(t, s, d["task_id"])
}

func TestPublicAuditIQSectionActuallyRunsTheQuestions(t *testing.T) {
	s, up, _, _ := testSetup(t)
	body := `{"base_url":"` + up.URL + `","api_key":"sk-test","model":"claude-opus-5","timeout":10,"sections":["iq"]}`
	w := postPublic(s, body, "203.0.113.77:1234")
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
	iq, ok := task.Verdict.IQ.(map[string]any)
	if !ok {
		t.Fatalf("missing IQ verdict: %#v", task.Verdict.IQ)
	}
	lines, ok := iq["per_question"].([]QuestionResult)
	if !ok {
		t.Fatalf("per_question type %T", iq["per_question"])
	}
	if len(lines) != 9 {
		t.Fatalf("per_question = %d, want 9", len(lines))
	}
	for _, line := range lines {
		if line.Error != "" {
			t.Fatalf("%s did not run: %s", line.ID, line.Error)
		}
	}
	if lines[8].ID != "Q9" {
		t.Fatalf("last question = %s, want Q9", lines[8].ID)
	}
}

func TestPublicAuditVisionSectionScoresBothDimensions(t *testing.T) {
	s, up, _, _ := testSetup(t)
	body := `{"base_url":"` + up.URL + `","api_key":"sk-test","model":"claude-opus-5","timeout":10,"sections":["vision"]}`
	w := postPublic(s, body, "203.0.113.88:1234")
	if w.Code != 202 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID       string   `json:"id"`
		Sections []string `json:"sections"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if len(created.Sections) != 1 || created.Sections[0] != "vision" {
		t.Fatalf("sections %+v", created.Sections)
	}
	task := waitTask(t, s, created.ID)
	if task.Verdict.IQ != nil {
		t.Fatalf("vision-only audit ran the IQ paper: %#v", task.Verdict.IQ)
	}
	vision, ok := task.Verdict.Vision.(map[string]any)
	if !ok {
		t.Fatalf("missing vision verdict: %#v", task.Verdict.Vision)
	}
	recognized, _ := vision["recognized"].(bool)
	seen, _ := asInt(vision["recognized_score"])
	want := 0
	if recognized {
		want = 50
	}
	if seen != want {
		t.Fatalf("recognized_score = %d, want %d: %#v", seen, want, vision)
	}
	answer, ok := asInt(vision["answer_score"])
	if !ok || answer < 0 || answer > 50 {
		t.Fatalf("answer_score = %#v", vision["answer_score"])
	}
	if total, _ := asInt(vision["score"]); total != seen+answer {
		t.Fatalf("vision total = %d, want %d", total, seen+answer)
	}
	if max, _ := asInt(vision["max"]); max != 100 {
		t.Fatalf("vision max = %d", max)
	}
}
