package thinking

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

// fakeCaller returns one first-turn metric and lets the test decide how replay
// and temperature requests behave.
type fakeCaller struct {
	first    Metrics
	firstErr error
	replay   func(CallRequest) (Metrics, error)
	calls    []CallRequest
}

func (f *fakeCaller) CallMessages(ctx context.Context, req CallRequest) (Metrics, error) {
	f.calls = append(f.calls, req)
	if (req.Replay != nil || req.Temperature != nil) && f.replay != nil {
		return f.replay(req)
	}
	if f.firstErr != nil {
		return Metrics{}, f.firstErr
	}
	return f.first, nil
}

func validReplay(req CallRequest) (Metrics, error) {
	if req.Temperature != nil {
		return Metrics{Status: 400, Err: "`temperature` is deprecated for this model."}, nil
	}
	if req.Replay != nil {
		if req.Replay.Signature == "VALID" {
			return Metrics{Status: 200, Answer: "I'm Claude, an AI assistant made by Anthropic."}, nil
		}
		return Metrics{Status: 400, Err: "messages.1.content.0: Invalid `signature` in `thinking` block"}, nil
	}
	return Metrics{}, nil
}

func TestCheckScoresSignatureAndTamper(t *testing.T) {
	base := Metrics{Status: 200, ThinkLen: 812, SigLen: 5, ThinkingText: "最小正整数 is 221.", Signature: "VALID"}
	cases := []struct {
		name       string
		model      string
		first      Metrics
		replay     func(CallRequest) (Metrics, error)
		wantPass   bool
		wantScore  int
		wantMax    int
		wantReason string
		wantCalls  int
	}{
		{
			name: "full sonnet", model: "claude-sonnet-4-5", first: base, replay: validReplay,
			wantPass: true, wantScore: 10, wantMax: 10, wantReason: "篡改拒绝", wantCalls: 3,
		},
		{
			name: "signature only", model: "claude-sonnet-4-5",
			first:  Metrics{Status: 200, SigLen: 5, Signature: "VALID"},
			replay: validReplay, wantPass: false, wantScore: 5, wantMax: 10, wantReason: "摘要 0/5", wantCalls: 3,
		},
		{
			name: "no signature", model: "claude-sonnet-4-5", first: Metrics{Status: 200},
			wantPass: false, wantScore: 0, wantMax: 10, wantReason: "没有思维摘要，也没有签名", wantCalls: 1,
		},
		{
			name: "tamper accepted", model: "claude-sonnet-4-5", first: base,
			replay: func(req CallRequest) (Metrics, error) {
				if req.Temperature != nil {
					return Metrics{Status: 400, Err: "`temperature` is deprecated for this model."}, nil
				}
				return Metrics{Status: 200, Answer: "I'm Claude."}, nil
			},
			wantPass: false, wantScore: 8, wantMax: 10, wantReason: "篡改签名仍被接受", wantCalls: 3,
		},
		{
			name: "original replay fails", model: "claude-sonnet-4-5", first: base,
			replay: func(req CallRequest) (Metrics, error) {
				return Metrics{Status: 400, Err: "messages.1.content.0: Invalid `signature` in `thinking` block"}, nil
			},
			wantPass: false, wantScore: 7, wantMax: 10, wantReason: "原始签名续轮失败", wantCalls: 3,
		},
		{
			name: "opus full with temperature", model: "claude-opus-5-5", first: base, replay: validReplay,
			wantPass: true, wantScore: 12, wantMax: 12, wantReason: "temperature 2/2", wantCalls: 4,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fc := &fakeCaller{first: tc.first, replay: tc.replay}
			v, err := Check(context.Background(), fc, tc.model)
			if err != nil {
				t.Fatalf("Check error: %v", err)
			}
			if v.Pass != tc.wantPass || v.Score != tc.wantScore || v.MaxScore != tc.wantMax {
				t.Fatalf("pass=%v score=%d max=%d want pass=%v score=%d max=%d (%+v)", v.Pass, v.Score, v.MaxScore, tc.wantPass, tc.wantScore, tc.wantMax, v)
			}
			if !strings.Contains(v.Reason, tc.wantReason) {
				t.Fatalf("Reason = %q, want contains %q", v.Reason, tc.wantReason)
			}
			if len(fc.calls) != tc.wantCalls {
				t.Fatalf("calls = %d, want %d", len(fc.calls), tc.wantCalls)
			}
		})
	}
}

func TestCheckReplayKeepsAdaptiveThinkingAndTamperFlag(t *testing.T) {
	fc := &fakeCaller{
		first:  Metrics{Status: 200, ThinkLen: 10, SigLen: 5, ThinkingText: "reasoning", Signature: "VALID"},
		replay: validReplay,
	}
	if _, err := Check(context.Background(), fc, "claude-sonnet-4-5"); err != nil {
		t.Fatal(err)
	}
	var replays []CallRequest
	for _, req := range fc.calls {
		if req.Replay != nil {
			replays = append(replays, req)
		}
	}
	if len(replays) != 2 {
		t.Fatalf("replays = %d, want 2", len(replays))
	}
	for _, req := range replays {
		if !req.StopOnInvalidSignature || req.MaxTokens != 2048 {
			t.Fatalf("replay shape missing: %+v", req)
		}
	}
}

func TestCheckCallShape(t *testing.T) {
	fc := &fakeCaller{first: Metrics{Status: 200, ThinkLen: 10, SigLen: 0}}
	_, err := Check(context.Background(), fc, "claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if len(fc.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(fc.calls))
	}
	req := fc.calls[0]
	if req.Model != "claude-sonnet-4-5" || req.MaxTokens != 4096 {
		t.Errorf("Model/MaxTokens = %q/%d", req.Model, req.MaxTokens)
	}
	if req.Thinking == nil || req.Thinking.Type != "adaptive" || req.Thinking.Display != "summarized" {
		t.Errorf("Thinking = %+v", req.Thinking)
	}
	if len(req.Messages) != 1 || !strings.Contains(req.Messages[0].Content, "最小正整数") {
		t.Errorf("Messages = %+v", req.Messages)
	}
}

func TestCheckCallerError(t *testing.T) {
	fc := &fakeCaller{firstErr: context.DeadlineExceeded}
	if _, err := Check(context.Background(), fc, "claude-sonnet-4-5"); err == nil {
		t.Fatal("expected error to propagate from caller")
	}
}

func protoSignature(model string) string {
	header := []byte{0x42, 0x08}
	header = append(header, "thinking"...)
	if model != "" {
		header = append(header, 0x32, byte(len(model)))
		header = append(header, model...)
	}
	nested := append([]byte{0x0a, byte(len(header))}, header...)
	raw := append([]byte{0x08, 0x02, 0x12, byte(len(nested))}, nested...)
	return base64.StdEncoding.EncodeToString(append(raw, 0x18, 0x01))
}

func TestSignatureModelExtraction(t *testing.T) {
	if got := signatureModel(protoSignature("claude-opus-4-8")); got != "claude-opus-4-8" {
		t.Fatalf("signed model = %q", got)
	}
	if got := signatureModel(protoSignature("")); got != "" {
		t.Fatalf("opaque signature model = %q, want empty", got)
	}
	if !signatureModelMatches("claude-fable-5-1", "claude-fable-5") {
		t.Fatal("base family should match a sub-version")
	}
	if !signatureModelMatches("claude-haiku-4-5-20251001", "claude-haiku-4-5") {
		t.Fatal("date stamp should not break the family match")
	}
	if signatureModelMatches("claude-opus-5-5", "claude-opus-4-8") {
		t.Fatal("different families must not match")
	}
}

func TestCheckSignedModelMismatchCostsTwo(t *testing.T) {
	original := protoSignature("claude-opus-4-8")
	replay := func(req CallRequest) (Metrics, error) {
		if req.Temperature != nil {
			return Metrics{Status: 400, Err: "`temperature` is deprecated for this model."}, nil
		}
		if req.Replay != nil && req.Replay.Signature == original {
			return Metrics{Status: 200, Answer: "I'm Claude."}, nil
		}
		return Metrics{Status: 400, Err: "messages.1.content.0: Invalid `signature` in `thinking` block"}, nil
	}
	fc := &fakeCaller{
		first:  Metrics{Status: 200, ThinkLen: 10, SigLen: len(original), ThinkingText: "x", Signature: original},
		replay: replay,
	}
	v, err := Check(context.Background(), fc, "claude-opus-5-5")
	if err != nil {
		t.Fatal(err)
	}
	if v.SignedModel != "claude-opus-4-8" || v.SignedModelMatch == nil || *v.SignedModelMatch {
		t.Fatalf("signed model state = %q / %v", v.SignedModel, v.SignedModelMatch)
	}
	if v.Score != 10 || v.MaxScore != 12 || v.Pass {
		t.Fatalf("score = %d/%d pass=%v, want 10/12 false", v.Score, v.MaxScore, v.Pass)
	}
	if !strings.Contains(v.Reason, "签名型号 claude-opus-4-8 与请求不符") {
		t.Fatalf("reason = %q", v.Reason)
	}
}

func TestCheckSignedModelMatchKeepsFullScore(t *testing.T) {
	original := protoSignature("claude-opus-4-8")
	replay := func(req CallRequest) (Metrics, error) {
		if req.Temperature != nil {
			return Metrics{Status: 400, Err: "`temperature` is deprecated for this model."}, nil
		}
		if req.Replay != nil && req.Replay.Signature == original {
			return Metrics{Status: 200, Answer: "I'm Claude."}, nil
		}
		return Metrics{Status: 400, Err: "messages.1.content.0: Invalid `signature` in `thinking` block"}, nil
	}
	fc := &fakeCaller{
		first:  Metrics{Status: 200, ThinkLen: 10, SigLen: len(original), ThinkingText: "x", Signature: original},
		replay: replay,
	}
	v, err := Check(context.Background(), fc, "claude-opus-4-8")
	if err != nil {
		t.Fatal(err)
	}
	if v.SignedModel != "claude-opus-4-8" || v.SignedModelMatch == nil || !*v.SignedModelMatch {
		t.Fatalf("signed model state = %q / %v", v.SignedModel, v.SignedModelMatch)
	}
	if v.Score != 10 || v.MaxScore != 10 || !v.Pass {
		t.Fatalf("score = %d/%d pass=%v, want 10/10 true", v.Score, v.MaxScore, v.Pass)
	}
}
