package thinking

import (
	"context"
	"strings"
	"testing"
)

type scriptCaller struct {
	calls []CallRequest
	reply func(CallRequest) Metrics
}

func (s *scriptCaller) CallMessages(ctx context.Context, req CallRequest) (Metrics, error) {
	s.calls = append(s.calls, req)
	return s.reply(req), nil
}

func TestAuditRelatedThinkingPasses(t *testing.T) {
	s := &scriptCaller{reply: func(req CallRequest) Metrics {
		if req.Replay == nil || req.Replay.PriorUser != probePrompt || req.Replay.Signature != "real" || req.Thinking != nil || req.Stream {
			t.Fatalf("replay shape %+v", req)
		}
		return Metrics{Status: 200, Answer: "17 19 23 n=7417"}
	}}
	got, err := Audit(context.Background(), s, "claude-sonnet-4-5", probePrompt, "real")
	if err != nil || got.Pass == nil || !*got.Pass || !got.Related {
		t.Fatalf("%v %+v", err, got)
	}
	if strings.Contains(got.Recovered, "real") {
		t.Fatal("recovered must not echo the signature")
	}
}

func TestAuditUnrelatedThinkingFails(t *testing.T) {
	s := &scriptCaller{reply: func(req CallRequest) Metrics {
		return Metrics{Status: 200, Answer: "<cot>the capital of france is paris and the weather is mild</cot>"}
	}}
	got, err := Audit(context.Background(), s, "claude-sonnet-4-5", probePrompt, "real")
	if err != nil || got.Pass == nil || *got.Pass || got.Reason != "解出的思考与当前题目不相关" {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestAuditInvalidSignatureFails(t *testing.T) {
	s := &scriptCaller{reply: func(req CallRequest) Metrics {
		return Metrics{Status: 400, Err: "Invalid signature in thinking block"}
	}}
	got, err := Audit(context.Background(), s, "claude-sonnet-4-5", probePrompt, "real")
	if err != nil || got.Pass == nil || *got.Pass || got.Related {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestAuditRefusalIsNotRelated(t *testing.T) {
	s := &scriptCaller{reply: func(req CallRequest) Metrics {
		return Metrics{Status: 200, Answer: "I can't share my private reasoning."}
	}}
	got, err := Audit(context.Background(), s, "claude-sonnet-4-5", probePrompt, "real")
	if err != nil || got.Pass == nil || *got.Pass || got.Reason != "签名没有解出思考，不能确认与题目相关" {
		t.Fatalf("%v %+v", err, got)
	}
}
