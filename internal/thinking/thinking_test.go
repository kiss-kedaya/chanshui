package thinking

import (
	"context"
	"strings"
	"testing"
)

// fakeCaller implements Caller with canned Metrics per scenario.
type fakeCaller struct {
	metrics Metrics
	err     error

	// captured request for assertions
	gotReq    CallRequest
	gotCalled bool
}

func (f *fakeCaller) CallMessages(ctx context.Context, req CallRequest) (Metrics, error) {
	f.gotCalled = true
	f.gotReq = req
	if f.err != nil {
		return Metrics{}, f.err
	}
	return f.metrics, nil
}

func TestCheckThreeShapes(t *testing.T) {
	cases := []struct {
		name       string
		metrics    Metrics
		wantPass   bool
		wantReason string
		wantThink  int
		wantSig    int
		wantTokens int
	}{
		{
			name: "text plus signature passes",
			metrics: Metrics{
				Status:         200,
				ThinkLen:       812,
				SigLen:         208,
				ThinkingText:   "最小正整数 is 221.",
				Signature:      strings.Repeat("s", 208),
				ThinkingTokens: 130,
				InputTokens:    88,
				OutputTokens:   260,
			},
			wantPass:   true,
			wantReason: "思考文本正常回传",
			wantThink:  812,
			wantSig:    208,
			wantTokens: 130,
		},
		{
			name: "signature only fails",
			metrics: Metrics{
				Status:         200,
				ThinkLen:       0,
				SigLen:         256,
				Signature:      strings.Repeat("s", 256),
				ThinkingTokens: 128,
			},
			wantPass:   false,
			wantReason: "未回传思考文本，仅收到签名",
			wantThink:  0,
			wantSig:    256,
			wantTokens: 128,
		},
		{
			name: "thinking tokens but no block same as signature only",
			metrics: Metrics{
				Status:         200,
				ThinkLen:       0,
				SigLen:         244,
				Signature:      strings.Repeat("s", 244),
				ThinkingTokens: 110,
			},
			wantPass:   false,
			wantReason: "未回传思考文本，仅收到签名",
			wantThink:  0,
			wantSig:    244,
			wantTokens: 110,
		},
		{
			name: "no thinking block at all",
			metrics: Metrics{
				Status: 200,
			},
			wantPass:   false,
			wantReason: "无思考块",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fc := &fakeCaller{metrics: tc.metrics}
			v, err := Check(context.Background(), fc, "claude-sonnet-4-5")
			if err != nil {
				t.Fatalf("Check error: %v", err)
			}
			if v.Pass != tc.wantPass {
				t.Errorf("Pass = %v, want %v", v.Pass, tc.wantPass)
			}
			if v.Reason != tc.wantReason {
				t.Errorf("Reason = %q, want %q", v.Reason, tc.wantReason)
			}
			if v.ThinkLen != tc.wantThink {
				t.Errorf("ThinkLen = %d, want %d", v.ThinkLen, tc.wantThink)
			}
			if v.SigLen != tc.wantSig {
				t.Errorf("SigLen = %d, want %d", v.SigLen, tc.wantSig)
			}
			if v.ThinkingTokens != tc.wantTokens {
				t.Errorf("ThinkingTokens = %d, want %d", v.ThinkingTokens, tc.wantTokens)
			}
		})
	}
}

func TestCheckCallShape(t *testing.T) {
	fc := &fakeCaller{metrics: Metrics{Status: 200, ThinkLen: 10, SigLen: 10}}
	_, err := Check(context.Background(), fc, "claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if !fc.gotCalled {
		t.Fatal("Check did not call the gateway")
	}
	req := fc.gotReq
	if req.Model != "claude-sonnet-4-5" {
		t.Errorf("Model = %q", req.Model)
	}
	if req.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096", req.MaxTokens)
	}
	if req.Thinking == nil {
		t.Fatal("Thinking config missing")
	}
	if req.Thinking.Type != "adaptive" {
		t.Errorf("Thinking.Type = %q, want adaptive", req.Thinking.Type)
	}
	if req.Thinking.Display != "summarized" {
		t.Errorf("Thinking.Display = %q, want summarized", req.Thinking.Display)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(req.Messages))
	}
	if !strings.Contains(req.Messages[0].Content, "最小正整数") {
		t.Errorf("prompt missing 最小正整数: %q", req.Messages[0].Content)
	}
}

func TestCheckCallerError(t *testing.T) {
	fc := &fakeCaller{err: context.DeadlineExceeded}
	_, err := Check(context.Background(), fc, "claude-sonnet-4-5")
	if err == nil {
		t.Fatal("expected error to propagate from caller")
	}
}
