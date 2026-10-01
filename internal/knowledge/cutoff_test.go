package knowledge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"chanzhushui/internal/gateway"
)

func TestLadderJudgementUsesFinalLineNotSelfReport(t *testing.T) {
	k01 := Ladder[0]
	row := judge(k01, "我的训练截止是 2025-02。\n最终答案: Sam Altman")
	if !row.OK || row.Quarter != "2023Q4" {
		t.Fatalf("altman = %+v", row)
	}
	unknown := judge(k01, "2024-06\n最终答案: 不知道")
	if unknown.OK || !unknown.Unsure {
		t.Fatalf("self-report must not count: %+v", unknown)
	}
	if judge(find("K06"), "最终答案: 8").OK != true {
		t.Fatal("month 8")
	}
	if judge(find("K06"), "最终答案: 18").OK {
		t.Fatal("18 is not August")
	}
	if judge(find("K08"), "最终答案: innovation").OK {
		t.Fatal("innovation must not match November")
	}
	if judge(find("K08"), "最终答案: 11").OK != true {
		t.Fatal("month 11")
	}
	if judge(find("K11"), "最终答案: absolute").OK {
		t.Fatal("absolute must not match sol")
	}
	if !judge(find("K11"), "最终答案: Sol").OK {
		t.Fatal("sol")
	}
	if !judge(find("K05"), "最终答案: DeepSeek-R1").OK {
		t.Fatal("r1")
	}
	if !judge(find("K13"), "最终答案: ChatGPT for Teens").OK {
		t.Fatal("teens")
	}
	long := strings.Repeat("董事会只说解除了CEO职务。", 12) + "这位CEO是 Sam Altman。"
	if !judge(find("K01"), long).OK {
		t.Fatal("name after the first 120 runes must still count")
	}
	if judge(find("K01"), "Sam Altman 曾复职。\n最终答案: 不知道").OK {
		t.Fatal("explicit refusal must beat an earlier mention")
	}
	if !judge(find("K02"), "公布的模型是 Sora。\n最").OK {
		t.Fatal("truncated final-answer fragment must fall back to the body")
	}
	refused := judge(find("K12"), "这场比赛我没有任何可靠信息，也不会去猜一个名字。")
	if refused.OK || !refused.Unsure {
		t.Fatalf("refusal = %+v", refused)
	}
}

func TestSummarizeUsesLatestCorrectQuarter(t *testing.T) {
	rows := make([]Row, len(Ladder))
	for i, p := range Ladder {
		rows[i] = Row{ID: p.ID, Quarter: p.Quarter, HappenedAt: p.HappenedAt, Event: p.Event, Unsure: true}
	}
	for _, id := range []string{"K01", "K02", "K03", "K04", "K05", "K06", "K07", "K08"} {
		i := indexOf(id)
		rows[i].OK = true
		rows[i].Unsure = false
	}
	rows[indexOf("K01")].OK = false
	rows[indexOf("K01")].Unsure = false
	rows[indexOf("K03")].OK = false
	rows[indexOf("K03")].Unsure = false
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	got := Summarize(rows, now)
	if got.Cutoff != "2025Q4" || got.Correct != 6 || got.Label != "知识≈2025Q4（6/13）" {
		t.Fatalf("summary = %+v", got)
	}
	if got.WebSuspect != "" {
		t.Fatalf("old quarter flagged as web: %s", got.WebSuspect)
	}
	if got.GapNote == "" {
		t.Fatal("expected gap note")
	}
	rows[indexOf("K13")].OK = true
	rows[indexOf("K13")].Unsure = false
	recent := Summarize(rows, now)
	if recent.Cutoff != "2026Q3" || recent.WebSuspect == "" {
		t.Fatalf("recent = %+v", recent)
	}
}

func TestCheckRunsLadderAndIgnoresAdvertisedDate(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		text := "最终答案: 不知道"
		q := string(body)
		switch {
		case strings.Contains(q, "OpenAI 董事会"):
			text = "最终答案: Altman"
		case strings.Contains(q, "文生视频"):
			text = "最终答案: Sora"
		case strings.Contains(q, "旗舰多模态"):
			text = "知识截止 2025-02\n最终答案: GPT-4o"
		case strings.Contains(q, "2024 年诺贝尔"):
			text = "最终答案: Hinton"
		case strings.Contains(q, "英伟达"):
			text = "最终答案: DeepSeek"
		case strings.Contains(q, "GPT-5 是在"):
			text = "最终答案: 8"
		case strings.Contains(q, "宏观量子"):
			text = "最终答案: Clarke"
		case strings.Contains(q, "Gemini 3"):
			text = "最终答案: 11"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []any{map[string]any{"type": "text", "text": text}},
			"usage":   map[string]int{"input_tokens": 20, "output_tokens": 4},
		})
	}))
	defer s.Close()
	prev := nowFn
	nowFn = func() time.Time { return time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC) }
	defer func() { nowFn = prev }()
	v, err := Check(context.Background(), gateway.Client{BaseURL: s.URL, NoRetry: true}, "claude-opus-5", 3)
	if err != nil {
		t.Fatal(err)
	}
	if v.Cutoff != "2025Q4" || v.Correct != 8 || v.Total != 13 || v.Label != "知识≈2025Q4（8/13）" {
		t.Fatalf("verdict = %+v", v)
	}
	if v.Cutoff == "2025-02" || strings.Contains(v.Label, "2025-02") {
		t.Fatalf("self-report leaked: %+v", v)
	}
	if v.Status != "estimated" {
		t.Fatalf("status = %s", v.Status)
	}
}

func find(id string) Probe {
	return Ladder[indexOf(id)]
}

func indexOf(id string) int {
	for i, p := range Ladder {
		if p.ID == id {
			return i
		}
	}
	panic(id)
}
