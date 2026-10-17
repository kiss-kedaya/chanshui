package fingerprint

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

type fakeCaller struct {
	mu        sync.Mutex
	responses []string
	calls     int
	err       error
}

func (f *fakeCaller) Call(_ context.Context, _, prompt string, _ int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	for i, probe := range BehaviorProbes {
		if prompt == probe.Prompt {
			if i < len(f.responses) {
				return f.responses[i], nil
			}
			return "", nil
		}
	}
	return "", nil
}

func TestCompareBehaviorDisabled(t *testing.T) {
	b := bankForTest(t)
	numAttr := Attribution{LowConfidence: false, TopProb: 0.9}
	caller := &fakeCaller{responses: []string{"x"}}
	res, err := CompareBehavior(context.Background(), caller, "m", b, numAttr)
	if err != nil {
		t.Fatal(err)
	}
	if res.Enabled {
		t.Fatal("expected disabled")
	}
	if res.Note != "数字指纹置信，未启用行为比对" {
		t.Fatalf("note: %q", res.Note)
	}
	if caller.calls != 0 {
		t.Fatalf("caller must not be invoked, calls=%d", caller.calls)
	}
}

func TestCompareBehaviorEnabledTop3(t *testing.T) {
	b := bankForTest(t)
	// Canned SAFE probe answers: short hedge to probes 1-2, markdown bullets to probe 3.
	caller := &fakeCaller{responses: []string{
		"这个问题超出我的能力范围，我无法给出具体的评分，因为自我评估并不可靠。可能无法完全准确，我的打分大约是 6 分。",
		"我无法提供这样的承诺。这超出了我能负责的范围，建议咨询专业人士。可能无法做到完整保证。",
		"## 提高编程效率\n- 使用版本控制\n- 编写单元测试\n- 使用类型检查\n- 善用调试器\n- 保持函数短小\n",
	}}
	numAttr := Attribution{LowConfidence: true, TopProb: 0.4, Family: "gpt"}
	res, err := CompareBehavior(context.Background(), caller, "some-model", b, numAttr)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Enabled {
		t.Fatalf("expected enabled, note=%q", res.Note)
	}
	if caller.calls != 3 {
		t.Fatalf("expected 3 probe calls, got %d", caller.calls)
	}
	if len(res.Top3) == 0 || len(res.Top3) > 3 {
		t.Fatalf("top3 size %d", len(res.Top3))
	}
	for _, m := range res.Top3 {
		if m.Similarity < 0 || m.Similarity > 1 {
			t.Fatalf("similarity out of range: %v", m.Similarity)
		}
		if m.ModelID == "" {
			t.Fatal("empty model id")
		}
	}
	for i := 1; i < len(res.Top3); i++ {
		if res.Top3[i].Similarity > res.Top3[i-1].Similarity {
			t.Fatalf("top3 not sorted: %v then %v", res.Top3[i-1].Similarity, res.Top3[i].Similarity)
		}
	}
}

func TestCompareBehaviorCallerError(t *testing.T) {
	b := bankForTest(t)
	caller := &fakeCaller{err: errors.New("boom")}
	numAttr := Attribution{LowConfidence: true}
	res, err := CompareBehavior(context.Background(), caller, "m", b, numAttr)
	if err == nil {
		t.Fatal("expected error to surface")
	}
	if res != nil {
		t.Fatalf("expected nil result on error, got %+v", res)
	}
}

func TestCompareBehaviorSameFamilyPreference(t *testing.T) {
	b := bankForTest(t)
	// Very long first-sentence-less refusals with em-dash — should map somewhere.
	caller := &fakeCaller{responses: []string{
		strings.Repeat("我无法提供此类建议，这超出了我的服务范围。", 40),
		strings.Repeat("我无法确认这一点，需要更多信息才能回答。", 40),
		"1. 使用版本控制\n2. 编写测试\n3. 代码评审\n4. 静态检查\n5. 重构习惯\n",
	}}
	numAttr := Attribution{LowConfidence: true, Family: "anthropic"}
	res, err := CompareBehavior(context.Background(), caller, "m", b, numAttr)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Top3) != 3 {
		t.Fatalf("top3: %d", len(res.Top3))
	}
}
