package thinking

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func validV4Signature(role string) string {
	if role == "" {
		role = "thinking"
	}
	header := append(pbInt(1, 20), pbInt(3, 2)...)
	header = append(header, pbInt(7, 1)...)
	header = append(header, pbBytes(8, []byte(role))...)
	cipher := make([]byte, 320)
	for i := range cipher {
		cipher[i] = byte(0x80 + (i*31)%113)
	}
	payload := pbBytes(1, header)
	payload = append(payload, pbBytes(2, make([]byte, 12))...)
	payload = append(payload, pbBytes(3, make([]byte, 12))...)
	payload = append(payload, pbBytes(4, make([]byte, 48))...)
	payload = append(payload, pbBytes(5, cipher)...)
	outer := append(pbInt(1, 4), pbBytes(2, payload)...)
	outer = append(outer, pbInt(3, 1)...)
	return base64.StdEncoding.EncodeToString(outer)
}

var testValidSig = validV4Signature("thinking")

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
		if req.Replay.Signature == testValidSig {
			return Metrics{Status: 200, Answer: "I'm Claude, an AI assistant made by Anthropic."}, nil
		}
		return Metrics{Status: 400, Err: "messages.1.content.0: Invalid `signature` in `thinking` block"}, nil
	}
	return Metrics{}, nil
}

func TestCheckScoresSignatureAndTamper(t *testing.T) {
	base := Metrics{Status: 200, ThinkLen: 812, SigLen: len(testValidSig), ThinkingText: "最小正整数 is 221.", Signature: testValidSig}
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
			first:  Metrics{Status: 200, SigLen: len(testValidSig), Signature: testValidSig},
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
		first:  Metrics{Status: 200, ThinkLen: 10, SigLen: len(testValidSig), ThinkingText: "reasoning", Signature: testValidSig},
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

func TestCheckWithSeedReusesQuestionSignature(t *testing.T) {
	fc := &fakeCaller{replay: validReplay}
	v, err := CheckWithSeed(context.Background(), fc, "claude-opus-5-5", Seed{
		ID: "Q6", Prompt: "candy question",
		Metrics: Metrics{Status: 200, ThinkingText: "summary", Signature: testValidSig, Answer: "21"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if v.Source != "Q6" || v.Score != 12 || v.MaxScore != 12 || !v.Pass {
		t.Fatalf("verdict = %+v", v)
	}
	if len(fc.calls) != 3 {
		t.Fatalf("calls = %d, want 3 (no fresh thinking request)", len(fc.calls))
	}
	for _, req := range fc.calls {
		if req.Replay != nil && req.Replay.PriorUser != "candy question" {
			t.Fatalf("prior user = %q", req.Replay.PriorUser)
		}
	}
	if !strings.Contains(v.Reason, "复用 Q6 的签名") {
		t.Fatalf("reason = %q", v.Reason)
	}
}

func forgedRouterNerverSignature(ts int64) string {
	ch := []byte{}
	for _, f := range []int{1, 3, 5, 7, 8, 11, 14, 17, 19, 22, 24, 27} {
		ch = append(ch, pbInt(f, uint64(f))...)
	}
	ch = append(ch, pbBytes(6, []byte("claude-opus-5-5"))...)
	ch = append(ch, pbInt(21, uint64(ts))...)
	container := pbBytes(1, ch)
	container = append(container, pbBytes(2, make([]byte, 12))...)
	container = append(container, pbBytes(3, make([]byte, 12))...)
	container = append(container, pbBytes(4, make([]byte, 48))...)
	container = append(container, pbBytes(5, make([]byte, 200))...)
	// Omits outer field 1 (version=4), includes outer field 3 = 1.
	outer := append(pbBytes(2, container), pbInt(3, 1)...)
	return base64.StdEncoding.EncodeToString(outer)
}

func TestForgedProtobufSignatureScoresZero(t *testing.T) {
	sig := forgedRouterNerverSignature(time.Now().Unix() - 14)
	fc := &fakeCaller{
		first: Metrics{Status: 200, ThinkLen: 400, SigLen: len(sig), ThinkingText: "fake summary", Signature: sig},
		replay: func(req CallRequest) (Metrics, error) {
			if req.Temperature != nil {
				return Metrics{Status: 400, Err: "`temperature` is deprecated for this model."}, nil
			}
			if req.Replay != nil && req.Replay.Signature == sig {
				return Metrics{Status: 200, Answer: "I'm Claude, made by Anthropic."}, nil
			}
			return Metrics{Status: 400, Err: "messages.1.content.0: Invalid `signature` in `thinking` block (request id: fake)"}, nil
		},
	}
	v, err := Check(context.Background(), fc, "claude-opus-5-5")
	if err != nil {
		t.Fatal(err)
	}
	if !v.SigForged || v.Score != 0 || v.Pass || v.AwSB != nil {
		t.Fatalf("forged protobuf must score 0 and never credit AWS-B: %+v", v)
	}
	if !strings.HasPrefix(v.SigClass, "伪造") {
		t.Fatalf("SigClass = %q", v.SigClass)
	}
}

func TestLocalReplayPrefixAndConfessionScoresZero(t *testing.T) {
	sig := "thinking_local_v1_abcdef123456"
	fc := &fakeCaller{
		first: Metrics{Status: 200, ThinkLen: 300, SigLen: len(sig), ThinkingText: "replayed summary", Signature: sig},
		replay: func(req CallRequest) (Metrics, error) {
			if req.Temperature != nil {
				return Metrics{Status: 400, Err: "`temperature` is deprecated for this model."}, nil
			}
			if req.Replay != nil && req.Replay.Signature == sig {
				return Metrics{Status: 200, Answer: "I am a local reference-answer replay service."}, nil
			}
			return Metrics{Status: 400, Err: "Invalid local thinking signature"}, nil
		},
	}
	v, err := Check(context.Background(), fc, "claude-opus-5-5")
	if err != nil {
		t.Fatal(err)
	}
	if !v.SigForged || !v.ReplayConfessed || v.Score != 0 || v.Pass || v.TamperRejected || v.AwSB != nil {
		t.Fatalf("local replay must be caught and zeroed: %+v", v)
	}
}

func TestUnrecognizedShortSignatureGetsNoSignatureCredit(t *testing.T) {
	fc := &fakeCaller{
		first: Metrics{Status: 200, ThinkLen: 100, SigLen: 5, ThinkingText: "ok", Signature: "VALID"},
		replay: func(req CallRequest) (Metrics, error) {
			if req.Replay != nil && req.Replay.Signature == "VALID" {
				return Metrics{Status: 200, Answer: "I am Claude."}, nil
			}
			return Metrics{Status: 400, Err: "Invalid signature"}, nil
		},
	}
	v, err := Check(context.Background(), fc, "claude-sonnet-4-5")
	if err != nil {
		t.Fatal(err)
	}
	if v.SignatureScore != 0 || v.ReplayScore != 0 || v.TamperScore != 0 || v.Pass {
		t.Fatalf("non-envelope signature must not earn signature/replay/tamper credit: %+v", v)
	}
}

func TestAuthenticNarrationV4SignatureAccepted(t *testing.T) {
	sig := validV4Signature("narration")
	fc := &fakeCaller{
		first: Metrics{Status: 200, ThinkLen: 400, SigLen: len(sig), ThinkingText: "real reasoning", Signature: sig},
		replay: func(req CallRequest) (Metrics, error) {
			if req.Temperature != nil {
				return Metrics{Status: 400, Err: "`temperature` is deprecated for this model."}, nil
			}
			if req.Replay != nil && req.Replay.Signature == sig {
				return Metrics{Status: 200, Answer: "I am Claude."}, nil
			}
			return Metrics{Status: 400, Err: "Invalid signature"}, nil
		},
	}
	v, err := Check(context.Background(), fc, "claude-opus-5-5")
	if err != nil {
		t.Fatal(err)
	}
	if v.SigForged || !v.Pass || v.Score != 12 {
		t.Fatalf("genuine narration v4 signature must pass 12/12: %+v", v)
	}
}
