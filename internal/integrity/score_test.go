package integrity

import "testing"

func TestCalculateNormalizesKnownSignalsAndCoverage(t *testing.T) {
	good := true
	bad := false
	injection := 10
	r := Calculate(Inputs{Protocol: "messages", InjectionApply: true, InjectionScore: &injection, CachePass: &good, CacheApplicable: true, ToolsPass: &bad, ToolsApplicable: true, WebPass: nil, WebApplicable: true, ThinkingScore: &injection, ThinkingApply: true, StreamPass: &good})
	if r.Score == nil || *r.Score != 76 {
		t.Fatalf("score = %v, signals = %+v", r.Score, r.Signals)
	}
	if r.Status != "partial" || r.Coverage != 85 {
		t.Fatalf("status/coverage = %s/%.2f", r.Status, r.Coverage)
	}
}

func TestNonClaudeThinkingIsNotApplicable(t *testing.T) {
	good := true
	injection := 10
	r := Calculate(Inputs{Protocol: "chat", InjectionApply: true, InjectionScore: &injection, CachePass: &good, CacheApplicable: true, ToolsPass: &good, ToolsApplicable: true, WebPass: nil, WebApplicable: false, ThinkingApply: false})
	if r.ApplicableWeight != 60 || r.Score == nil || *r.Score != 100 {
		t.Fatalf("non-Claude score = %+v", r)
	}
}

func TestNoEvidenceIsUnknown(t *testing.T) {
	r := Calculate(Inputs{Protocol: "chat", WebApplicable: false, ThinkingApply: false})
	if r.Score != nil || r.Status != "unknown" || r.Coverage != 0 {
		t.Fatalf("unknown result = %+v", r)
	}
}
