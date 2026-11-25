package thinking

import (
	"context"
	"strings"
	"testing"
	"time"
)

type proberCaller struct {
	*fakeCaller
	checks []StrictnessCheck
}

func (p proberCaller) ProbeStrictness(ctx context.Context, model string) []StrictnessCheck {
	return p.checks
}

func TestAuditRecordsSignatureAnatomyAndStrictness(t *testing.T) {
	sig := buildSignature(time.Now().Unix(), []int{1, 3, 5, 6, 7, 8, 11, 14, 17, 19, 21, 22, 24, 27})
	first := Metrics{Status: 200, ThinkLen: 10, SigLen: len(sig), ThinkingText: "reasoning", Signature: sig}
	base := &fakeCaller{first: first, replay: func(req CallRequest) (Metrics, error) {
		if req.Replay != nil && req.Replay.Signature == sig {
			return Metrics{Status: 200, Answer: "I am Claude, made by Anthropic."}, nil
		}
		return Metrics{Status: 400, Err: "messages.1.content.0: Invalid \u0060signature\u0060 in \u0060thinking\u0060 block"}, nil
	}}
	checks := []StrictnessCheck{
		{Name: "missing_max_tokens", Status: 400, Pass: true},
		{Name: "system_role", Status: 200, Pass: false, Detail: "system role accepted"},
	}
	v, err := Check(context.Background(), proberCaller{fakeCaller: base, checks: checks}, "claude-sonnet-5")
	if err != nil {
		t.Fatal(err)
	}
	if v.SigClass != "真签(现生成)" || v.SigFresh == nil || !*v.SigFresh || v.SigFieldCount != 14 {
		t.Fatalf("anatomy not recorded: %+v", v)
	}
	if len(v.Strictness) != 2 || v.Strictness[1].Pass {
		t.Fatalf("strictness = %+v", v.Strictness)
	}
	if !strings.Contains(v.Reason, "签名结构 真签(现生成)") || !strings.Contains(v.Reason, "协议严格性 1/2（未拒绝：system_role）") {
		t.Fatalf("reason = %q", v.Reason)
	}
}
