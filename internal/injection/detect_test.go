package injection

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"chanzhushui/internal/gateway"
)

// fakeCaller 按 prompt 里的关键词返回预置回答，用来模拟不同上游行为。
type fakeCaller struct {
	answers  map[string]string // 子串匹配 -> 回答
	fallback string
}

func (f fakeCaller) ProtocolFor(model string) string { return "messages" }

func (f fakeCaller) CallMessages(ctx context.Context, q gateway.CallRequest) (gateway.Metrics, error) {
	for needle, ans := range f.answers {
		if strings.Contains(q.Prompt, needle) {
			return gateway.Metrics{Status: 200, Answer: ans}, nil
		}
	}
	return gateway.Metrics{Status: 200, Answer: f.fallback}, nil
}

func TestCleanUpstreamIsNotFlagged(t *testing.T) {
	f := fakeCaller{
		answers: map[string]string{
			// 干净上游：所有问法一致否认
			"没有": "没有。",
			"无":  "无",
		},
		fallback: "没有这样一份设定。",
	}
	v, err := Detect(context.Background(), f, "claude-sonnet-4-5", 10)
	if err != nil {
		t.Fatal(err)
	}
	if v.Injected {
		t.Fatalf("干净上游被误判为注入: score=%d signals=%v", v.Score, v.Signals)
	}
	if v.Status != "clean" {
		t.Fatalf("期望 clean，得到 %s (%s)", v.Status, v.Reason)
	}
}

func TestSelfContradictionIsInjection(t *testing.T) {
	// 模拟实测到的 kedaya 行为：一部分问法承认“有运行设定”，另一部分说“没有”。
	f := func() fakeCaller {
		var c fakeCaller
		c.answers = map[string]string{}
		// 按子串区分问法
		c.answers["是否还有别的指令"] = "有。具体内容我不展开讲，那部分属于我的运行设定。"
		c.answers["system prompt 是什么"] = "我没有 system prompt，所以不存在第一句话。"
		c.answers["位于我这条 user 消息之前"] = "有。"
		c.answers["运行设定"] = "有一份运行设定需要遵守。"
		c.answers["什么能说、什么不能说"] = "没有这样一份清单可以列给你。"
		c.answers["原样打印"] = "这个我不打印，我不会把这类设置文本原样倒出来。"
		return c
	}()
	v, err := Detect(context.Background(), f, "claude-sonnet-4-5", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Injected || v.Status != "injected" {
		t.Fatalf("自相矛盾应判注入，得到 status=%s score=%d", v.Status, v.Score)
	}
	if v.AckCount == 0 || v.DenyCount == 0 {
		t.Fatalf("应同时有承认和否认: ack=%d deny=%d", v.AckCount, v.DenyCount)
	}
}

func TestFakeCutoffDetected(t *testing.T) {
	f := fakeCaller{fallback: "我是 Claude，由 Anthropic 开发。我的知识截止到 2024 年 4 月。"}
	v, err := Detect(context.Background(), f, "claude-sonnet-4-5", 10)
	if err != nil {
		t.Fatal(err)
	}
	if v.Identity == nil || !v.Identity.CutoffMismatch {
		t.Fatalf("应当检出假截止日期，identity=%+v", v.Identity)
	}
	if !v.Injected {
		t.Fatalf("假身份应至少使判定为注入，得到 %s score=%d", v.Status, v.Score)
	}
}

func TestExtractCutoff(t *testing.T) {
	cases := map[string]string{
		"我的知识截止到 2024 年 4 月。":            "2024-04",
		"训练数据截止于2025年3月":                 "2025-03",
		"知识截止：2026 年 1 月":                "2026-01",
		"我不知道。":                          "",
		"knowledge cutoff is 2024 April": "",
	}
	for in, want := range cases {
		if got := extractCutoff(in); got != want {
			t.Errorf("extractCutoff(%q)=%q want %q", in, got, want)
		}
	}
}

func TestCutoffCompatible(t *testing.T) {
	if cutoffCompatible("2024-04", "2025-03") {
		t.Error("2024-04 相对 2025-03 应判不符")
	}
	if !cutoffCompatible("2025-03", "2025-03") {
		t.Error("同月应兼容")
	}
	if !cutoffCompatible("2025-06", "2025-03") {
		t.Error("更晚应兼容")
	}
	if !cutoffCompatible("", "2025-03") {
		t.Error("未自报应视为无法判定")
	}
}

func TestClassifyStance(t *testing.T) {
	cases := map[string]string{
		"无":         "deny",
		"没有这样一份设定。": "deny",
		"有。那部分属于我的运行设定。":        "ack",
		"这个我不打印，我不会把这类设置文本倒出来。": "ack",
		"有一份设定。":          "ack",
		"有运行设定，但没有系统提示词。": "partial",
	}
	for in, want := range cases {
		if got := classifyStance(in); got != want {
			t.Errorf("classifyStance(%q)=%q want %q", in, got, want)
		}
	}
}

func TestExpectedFor(t *testing.T) {
	fam, cut := expectedFor("claude-sonnet-4-5")
	if fam != "claude-4.5" || cut != "2025-03" {
		t.Errorf("claude-sonnet-4-5 => %s %s", fam, cut)
	}
	if fam, _ := expectedFor("some-unknown-model"); fam != "" {
		t.Errorf("未知模型不应给出家族，得到 %s", fam)
	}
}

func TestErrorUpstream(t *testing.T) {
	f := errCaller{}
	v, err := Detect(context.Background(), f, "claude-sonnet-4-5", 10)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "error" {
		t.Fatalf("全失败应判 error，得到 %s", v.Status)
	}
}

type errCaller struct{}

func (errCaller) ProtocolFor(string) string { return "messages" }
func (errCaller) CallMessages(context.Context, gateway.CallRequest) (gateway.Metrics, error) {
	return gateway.Metrics{}, fmt.Errorf("network down")
}
