package api

import (
	"encoding/json"
	"testing"
)

func TestDetectionClientDefaultsToHighEffort(t *testing.T) {
	s, _, _, _ := testSetup(t)
	g, err := s.client(Request{BaseURL: "https://example.com", Key: "sk-test", Model: "claude-opus-5", Timeout: 10})
	if err != nil {
		t.Fatal(err)
	}
	if g.DefaultEffort != "high" {
		t.Fatalf("DefaultEffort = %q, want high", g.DefaultEffort)
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
	seen := false
	for _, q := range task.questions {
		sum += q.MaxScore
		if q.ID == "Q10" && (q.MaxScore != 20 || len(q.ImagePNG) == 0) {
			t.Fatalf("Q10 = %+v", q)
		}
		if q.ID == "Q9" {
			seen = true
			if q.Title != "夜间过桥" || q.Answer != "10" || q.MaxScore != 8 {
				t.Fatalf("Q9 = %+v", q)
			}
		}
	}
	if !seen {
		t.Fatal("Q9 missing from the battery")
	}
	if sum != 100 {
		t.Fatalf("battery max = %d, want 100", sum)
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
	if len(lines) != 10 {
		t.Fatalf("per_question = %d, want 9", len(lines))
	}
	for _, line := range lines {
		if line.Error != "" {
			t.Fatalf("%s did not run: %s", line.ID, line.Error)
		}
	}
	if lines[9].ID != "Q10" {
		t.Fatalf("last question = %s, want Q9", lines[8].ID)
	}
}
