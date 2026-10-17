package api

import (
	"testing"

	"github.com/kiss-kedaya/chanshui/internal/fingerprint"
)

func TestBehaviorFallbackUsesTopModelAndConfidence(t *testing.T) {
	bank := &fingerprint.Bank{
		Behavioral: fingerprint.BehavioralBank{
			Models: make([]fingerprint.BehavioralModel, 34),
		},
	}
	res := &fingerprint.BehaviorResult{Top3: []fingerprint.BehaviorMatch{
		{ModelID: "openai/gpt-4o", DisplayName: "GPT-4o", Family: "openai", Similarity: 0.91},
		{ModelID: "openai/gpt-4.1-mini", DisplayName: "GPT-4.1 Mini", Family: "openai", Similarity: 0.84},
	}}
	out := behaviorFallback("deepseek-flash", bank, res)
	if out["basis"] != "behavioral" || out["top_model"] != "openai/gpt-4o" {
		t.Fatalf("fallback output %+v", out)
	}
	if out["low_confidence"] != false || out["used_outputs"] != 3 {
		t.Fatalf("fallback confidence %+v", out)
	}
	if out["family_name"] != "GPT" || out["behavioral_total"] != 34 {
		t.Fatalf("fallback family %+v", out)
	}

	weak := behaviorFallback("deepseek-flash", bank, &fingerprint.BehaviorResult{Top3: []fingerprint.BehaviorMatch{
		{ModelID: "deepseek/deepseek-v3.2", DisplayName: "DeepSeek V3.2", Family: "deepseek", Similarity: 0.81},
		{ModelID: "deepseek/deepseek-v4-flash", DisplayName: "DeepSeek V4 Flash", Family: "deepseek", Similarity: 0.80},
	}})
	if weak["low_confidence"] != true {
		t.Fatalf("weak match marked confident: %+v", weak)
	}
}

func TestBehaviorFallbackWithoutCandidates(t *testing.T) {
	out := behaviorFallback("x-model", &fingerprint.Bank{}, nil)
	if out["basis"] != "behavioral" || out["family_name"] != "行为库无可比候选" {
		t.Fatalf("empty fallback %+v", out)
	}
}
