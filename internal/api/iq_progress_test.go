package api

import (
	"testing"

	"github.com/kiss-kedaya/chanshui/internal/iq"
)

func TestBuildIQVerdictIncrementalAndFinal(t *testing.T) {
	questions := []iq.Question{
		{ID: "Q1", Title: "谎言排查", MaxScore: 10},
		{ID: "Q2", Title: "并行调度", MaxScore: 10},
		{ID: "Q3", Title: "余数锁定", MaxScore: 12},
		{ID: "Q4", Title: "伴X遗传", MaxScore: 12},
		{ID: "Q5", Title: "禁区步数", MaxScore: 12},
		{ID: "Q6", Title: "经典糖果", MaxScore: 10},
		{ID: "Q7", Title: "袜子配对", MaxScore: 10},
		{ID: "Q8", Title: "糖果变体", MaxScore: 12},
		{ID: "Q9", Title: "夜间过桥", MaxScore: 12},
		{ID: "vision", Title: "K线读数", MaxScore: 10},
	}
	task := &Task{
		questions: questions,
		answers:   map[string]QuestionResult{},
	}

	// Before any IQ question finishes, incremental build leaves Verdict.IQ nil.
	if lines := task.buildIQVerdict(false); len(lines) != 0 || task.Verdict.IQ != nil {
		t.Fatalf("expected nil IQ before first answer, got lines=%d verdict=%#v", len(lines), task.Verdict.IQ)
	}

	// Q5 finishes first (out of order) with full 12/12.
	task.answers["Q5"] = QuestionResult{ID: "Q5", Title: "禁区步数", Max: 12, Earned: 12, Answer: "最终答案: 7"}
	lines := task.buildIQVerdict(false)
	if len(lines) != 1 || lines[0].ID != "Q5" {
		t.Fatalf("expected [Q5] immediately, got %+v", lines)
	}
	m, ok := task.Verdict.IQ.(map[string]any)
	if !ok {
		t.Fatalf("missing IQ map: %#v", task.Verdict.IQ)
	}
	if m["complete"] != false {
		t.Fatalf("expected complete=false while 8 questions remain, got %v", m["complete"])
	}
	if m["done_count"] != 1 || m["total_questions"] != 9 {
		t.Fatalf("unexpected progress counts: done=%v total=%v", m["done_count"], m["total_questions"])
	}
	if m["score"] != 12 {
		t.Fatalf("expected scaled live score 12/100, got %v", m["score"])
	}

	// Q1 finishes second with an error; both Q1 and Q5 appear in paper order.
	task.answers["Q1"] = QuestionResult{ID: "Q1", Title: "谎言排查", Max: 10, Error: "timeout"}
	lines = task.buildIQVerdict(false)
	if len(lines) != 2 || lines[0].ID != "Q1" || lines[1].ID != "Q5" {
		t.Fatalf("expected [Q1, Q5], got %+v", lines)
	}
	m = task.Verdict.IQ.(map[string]any)
	if m["done_count"] != 2 || m["score"] != 12 {
		t.Fatalf("expected done=2 score=12, got done=%v score=%v", m["done_count"], m["score"])
	}

	// Final pass fills remaining unanswered items as unfinished errors.
	finalLines := task.buildIQVerdict(true)
	if len(finalLines) != 9 {
		t.Fatalf("expected 9 final IQ rows (excluding vision), got %d", len(finalLines))
	}
	m = task.Verdict.IQ.(map[string]any)
	if m["complete"] != false || m["done_count"] != 9 || m["paper_max"] != 100 || m["score"] != 12 {
		t.Fatalf("unexpected final verdict: %#v", m)
	}
}
