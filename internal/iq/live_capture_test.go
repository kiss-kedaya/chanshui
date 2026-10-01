package iq

import (
	"encoding/json"
	"os"
	"testing"
)

func TestCapturedLivePillAnswer(t *testing.T) {
	b, e := os.ReadFile("../../.artifacts/live-first-report.json")
	if e != nil {
		t.Skip("local captured evidence unavailable")
	}
	var r struct {
		Verdict struct {
			IQ struct {
				Questions []struct{ ID, Answer string } `json:"per_question"`
			}
		}
	}
	if e = json.Unmarshal(b, &r); e != nil {
		t.Fatal(e)
	}
	for _, q := range r.Verdict.IQ.Questions {
		if q.ID == "Q1" {
			score, _, _ := Judge(GenPill(), q.Answer)
			if score != 6 {
				t.Fatalf("captured natural answer scored %d", score)
			}
			return
		}
	}
	t.Fatal("missing Q1")
}
