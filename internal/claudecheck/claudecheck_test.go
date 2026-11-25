package claudecheck

import (
	"context"
	"strings"
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

func findCheck(checks []Check, name string) Check {
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	return Check{}
}

func TestRunPassesWhenUpstreamHonoursBeta(t *testing.T) {
	c := &fakeCaller{result: Result{Status: 200, StopReason: "compaction", Transforms: []string{"thinking_dropped"}, HasInputTransformations: true, InputTokens: 8}}
	v, err := Run(context.Background(), c, "claude-opus-5", "sig-abc", "prior question", 30)
	if err != nil {
		t.Fatal(err)
	}
	if !findCheck(v.Checks, "thinking_binding").Pass || !findCheck(v.Checks, "context_compact").Pass {
		t.Fatalf("beta checks did not pass: %+v", v.Checks)
	}
	if len(c.bodies) < 8 {
		t.Fatalf("bodies = %d", len(c.bodies))
	}
}

func TestRunSkipsBindingWithoutSignature(t *testing.T) {
	c := &fakeCaller{result: Result{Status: 200, StopReason: "compaction"}}
	v, _ := Run(context.Background(), c, "claude-opus-5", "", "", 30)
	binding := findCheck(v.Checks, "thinking_binding")
	if !binding.Skipped || binding.Pass {
		t.Fatalf("binding = %+v", binding)
	}
	if !findCheck(v.Checks, "context_compact").Pass {
		t.Fatalf("compact = %+v", findCheck(v.Checks, "context_compact"))
	}
}

func TestRunFlagsLaxCompact(t *testing.T) {
	c := &fakeCaller{result: Result{Status: 200, StopReason: "end_turn"}}
	v, _ := Run(context.Background(), c, "claude-opus-5", "sig", "prior", 30)
	if findCheck(v.Checks, "context_compact").Pass {
		t.Fatal("compact passed without stop_reason=compaction")
	}
}

func TestCompactCorpusCrossesTrigger(t *testing.T) {
	corpus := compactCorpus(50000)
	if len(corpus) < 50000*4 {
		t.Fatalf("corpus too small: %d", len(corpus))
	}
}

// scriptedCaller answers each fingerprint prompt the way a clean channel does.
type scriptedCaller struct{}

func content(body map[string]any) string {
	msgs, _ := body["messages"].([]any)
	if len(msgs) == 0 {
		return ""
	}
	first, _ := msgs[0].(map[string]any)
	text, _ := first["content"].(string)
	return text
}

func (scriptedCaller) PostMessagesRaw(ctx context.Context, body map[string]any, beta string, timeoutSeconds int) (Result, error) {
	prompt := content(body)
	switch {
	case prompt == "hi":
		return Result{Status: 200, InputTokens: 8, Answer: "Hello"}, nil
	case strings.Contains(prompt, "standing instructions"):
		return Result{Status: 200, Answer: "<note>NONE</note>"}, nil
	case strings.Contains(prompt, "系统提示词"):
		return Result{Status: 200, Answer: "NONE"}, nil
	case strings.Contains(prompt, "full system prompt"):
		return Result{Status: 200, Answer: "NONE"}, nil
	case strings.Contains(prompt, "普通 Claude"):
		return Result{Status: 200, Answer: "我是普通 Claude，没有额外系统指令。"}, nil
	case strings.Contains(prompt, "17*19"):
		return Result{Status: 200, Signature: "sig-one"}, nil
	case strings.Contains(prompt, "41*43"):
		return Result{Status: 200, Signature: "sig-two"}, nil
	case body["system"] != nil:
		return Result{Status: 200, Answer: "MARKER-7f3a OK"}, nil
	}
	return Result{Status: 200, StopReason: "compaction", Transforms: []string{"thinking_dropped"}, HasInputTransformations: true}, nil
}

func TestFingerprintChecksPassOnCleanChannel(t *testing.T) {
	v, err := Run(context.Background(), scriptedCaller{}, "claude-opus-5", "", "", 30)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"hi_usage", "injection", "signature_fresh", "system_passthrough"} {
		check := findCheck(v.Checks, name)
		if !check.Pass {
			t.Fatalf("%s did not pass: %+v", name, check)
		}
	}
}

func TestHiUsageFlagsInjection(t *testing.T) {
	c := &fakeCaller{result: Result{Status: 200, InputTokens: 40}}
	check := hiUsage(context.Background(), c, "claude-opus-5", 10)
	if check.Pass || !strings.Contains(check.Detail, "疑似注入") {
		t.Fatalf("hi check = %+v", check)
	}
}

func TestSignatureFreshDetectsReplay(t *testing.T) {
	c := &fakeCaller{result: Result{Status: 200, Signature: "same"}}
	check := signatureFresh(context.Background(), c, "claude-opus-5", 10)
	if check.Pass || !strings.Contains(check.Detail, "复签") {
		t.Fatalf("fresh check = %+v", check)
	}
}
