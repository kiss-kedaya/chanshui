package claudecheck

import (
	"context"
	"testing"
)

type fakeCaller struct {
	result Result
	bodies []map[string]any
}

func (f *fakeCaller) PostMessagesRaw(ctx context.Context, body map[string]any, beta string, timeoutSeconds int) (Result, error) {
	f.bodies = append(f.bodies, body)
	return f.result, nil
}

func TestRunPassesWhenUpstreamHonoursBeta(t *testing.T) {
	c := &fakeCaller{result: Result{Status: 200, StopReason: "compaction", Transforms: []string{"thinking_dropped"}, HasInputTransformations: true, InputTokens: 52000}}
	v, err := Run(context.Background(), c, "claude-opus-5", "sig-abc", "prior question", 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Checks) != 2 {
		t.Fatalf("checks = %d", len(v.Checks))
	}
	for _, check := range v.Checks {
		if !check.Pass || check.Skipped {
			t.Fatalf("%s did not pass: %+v", check.Name, check)
		}
	}
	if len(c.bodies) != 2 {
		t.Fatalf("bodies = %d", len(c.bodies))
	}
}

func TestRunFlagsLaxUpstream(t *testing.T) {
	c := &fakeCaller{result: Result{Status: 200, StopReason: "end_turn"}}
	v, _ := Run(context.Background(), c, "claude-opus-5", "sig-abc", "prior", 30)
	for _, check := range v.Checks {
		if check.Pass {
			t.Fatalf("%s passed without evidence: %+v", check.Name, check)
		}
	}
}

func TestRunSkipsBindingWithoutSignature(t *testing.T) {
	c := &fakeCaller{result: Result{Status: 200, StopReason: "compaction"}}
	v, _ := Run(context.Background(), c, "claude-opus-5", "", "", 30)
	if !v.Checks[0].Skipped || v.Checks[0].Pass {
		t.Fatalf("binding = %+v", v.Checks[0])
	}
	if !v.Checks[1].Pass {
		t.Fatalf("compact = %+v", v.Checks[1])
	}
}

func TestCompactCorpusCrossesTrigger(t *testing.T) {
	corpus := compactCorpus(50000)
	if len(corpus) < 50000*4 {
		t.Fatalf("corpus too small: %d", len(corpus))
	}
}
