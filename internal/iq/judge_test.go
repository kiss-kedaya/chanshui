package iq

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestExtractAnswerSlot(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"推理若干……\n最终答案: 1/6", "1/6"},
		{"推理若干……\n最终答案：20", "20"},
		{"some reasoning\nFinal Answer: at least 21", "at least 21"},
		{"The answer is 29", "29"},
		{"前文答案: 被覆盖\n后文\n本题答案: 覆盖后", "覆盖后"},
		{"最后答案: 二十一", "二十一"},
	}
	for _, c := range cases {
		slot, _ := ExtractAnswer(c.in)
		if strings.TrimSpace(slot) != c.want {
			t.Fatalf("ExtractAnswer(%q) slot=%q want %q", c.in, slot, c.want)
		}
	}
}

func TestExtractAnswerStripsMarkdown(t *testing.T) {
	in := "```text\n草稿 38\n```\n## 小结\n**最终答案**: **1/10**"
	slot, _ := ExtractAnswer(in)
	if strings.TrimSpace(slot) != "1/10" {
		t.Fatalf("markdown not stripped: %q", slot)
	}
}

func TestExtractAnswerChainTailFallback(t *testing.T) {
	long := strings.Repeat("推理链条。", 500)
	_, tail := ExtractAnswer(long + "末尾结论：至少 21 颗")
	if !strings.Contains(tail, "至少 21 颗") {
		t.Fatalf("chain tail should contain the ending: %q", tail[len(tail)-40:])
	}
	if len([]rune(tail)) > 2000 {
		t.Fatalf("chain tail must be capped at 2000 chars, got %d", len([]rune(tail)))
	}
}

func TestJudgeTruthLiar(t *testing.T) {
	q := GenTruthLiar(newRand(42), EncodingURL)
	earned, half, _ := Judge(q, "逐个假设验证……\n最终答案: "+q.Answer)
	if earned != 10 || half {
		t.Fatalf("Q1 exact answer %q -> (%d,%v), want (10,false)", q.Answer, earned, half)
	}
	wrong := "1"
	if q.Answer == "1" {
		wrong = "2"
	}
	// Earlier reasoning mentions the right number, but final slot is wrong -> must score 0!
	earned, _, _ = Judge(q, "排除 "+q.Answer+" 号后认为是 "+wrong+"\n最终答案: "+wrong)
	if earned != 0 {
		t.Fatalf("Q1 wrong slot scored %d", earned)
	}
}

func TestJudgeScheduleTraps(t *testing.T) {
	q := GenSchedule(newRand(42), EncodingHex)
	jobs, ok := scheduleParamsOf(q)
	if !ok {
		t.Fatal("missing schedule params")
	}
	total := 0
	for _, d := range jobs {
		total += d
	}
	lb := (total + 1) / 2
	lpt := lptSchedule(jobs)

	earned, half, _ := Judge(q, "最终答案: "+q.Answer)
	if earned != 10 || half {
		t.Fatalf("Q2 exact -> (%d,%v)", earned, half)
	}
	earned, half, tags := Judge(q, fmt.Sprintf("最终答案: %d", lb))
	if earned != 0 || half || len(tags) == 0 || tags[0] != "忽略不可拆分" {
		t.Fatalf("Q2 lb trap -> (%d,%v,%v)", earned, half, tags)
	}
	earned, half, tags = Judge(q, fmt.Sprintf("最终答案: %d", lpt))
	if earned != 5 || !half || len(tags) == 0 || tags[0] != "贪心次优" {
		t.Fatalf("Q2 LPT trap -> (%d,%v,%v)", earned, half, tags)
	}
}

func TestJudgeCRTAndLowerBoundTrap(t *testing.T) {
	q := GenCRT(newRand(42), EncodingBase64)
	base, ok := crtBaseOf(q)
	if !ok {
		t.Fatal("missing CRT base")
	}
	earned, half, _ := Judge(q, "最终答案: "+q.Answer)
	if earned != 10 || half {
		t.Fatalf("Q3 exact -> (%d,%v)", earned, half)
	}
	earned, half, tags := Judge(q, fmt.Sprintf("最终答案: %d", base))
	if earned != 0 || half || len(tags) == 0 || tags[0] != "忽略下界" {
		t.Fatalf("Q3 base trap -> (%d,%v,%v)", earned, half, tags)
	}
}

func TestJudgeGeneticsFractionsDecimalsAndTraps(t *testing.T) {
	for _, gc := range geneticsPool {
		q := Question{
			ID:       "Q4",
			Title:    "伴X遗传",
			MaxScore: 12,
			Answer:   gc.answer,
			Tags:     []string{"贝叶斯遗传", "实例:" + gc.instance},
		}
		earned, half, _ := Judge(q, "推算……\n最终答案: "+gc.answer)
		if earned != 12 || half {
			t.Fatalf("%s exact %s -> (%d,%v)", gc.instance, gc.answer, earned, half)
		}
		earned, half, tags := Judge(q, "推算……\n最终答案: "+gc.trap)
		if earned != 0 || half || len(tags) == 0 || tags[0] != gc.trapTag {
			t.Fatalf("%s trap %s -> (%d,%v,%v), want tag %q", gc.instance, gc.trap, earned, half, tags, gc.trapTag)
		}
	}
	// Equivalent forms: LaTeX \frac, decimal, percentage, unreduced fraction
	q := Question{ID: "Q4", MaxScore: 12, Answer: "1/10", Tags: []string{"实例:bayes-son-k2"}}
	for _, form := range []string{`$\frac{1}{10}$`, "2/20", "0.1", "10%", "10.0%"} {
		earned, _, _ := Judge(q, "最终答案: "+form)
		if earned != 12 {
			t.Fatalf("equivalent probability %q -> %d, want 12", form, earned)
		}
	}
	// Slotless reasoning tail picks LAST probability, not first prior 1/2
	earned, _, _ := Judge(q, "先验概率为 1/2，经过 2 个正常儿子贝叶斯更新后，下一个儿子患病概率为 1/10。")
	if earned != 12 {
		t.Fatalf("slotless tail last probability -> %d, want 12", earned)
	}
}

func TestJudgeForbidStepsAndTrap(t *testing.T) {
	q := GenForbidSteps(newRand(42), EncodingHex)
	base, ok := forbidBaseDistOf(q)
	if !ok {
		t.Fatal("missing forbid base")
	}
	earned, half, _ := Judge(q, "最终答案: "+q.Answer)
	if earned != 10 || half {
		t.Fatalf("Q5 exact -> (%d,%v)", earned, half)
	}
	earned, half, tags := Judge(q, fmt.Sprintf("最终答案: %d", base))
	if earned != 0 || half || len(tags) == 0 || tags[0] != "忽略禁区" {
		t.Fatalf("Q5 forbid trap -> (%d,%v,%v)", earned, half, tags)
	}
}

func TestJudgeCandyClassic(t *testing.T) {
	q := GenCandyClassic(newRand(42), EncodingURL)
	cases := []struct {
		ans  string
		want int
		half bool
	}{
		{"21", 12, false},
		{"21 颗", 12, false},
		{"至少 21.0", 12, false},
		{"二十一", 12, false},
		{"0.10 颗", 0, false},
		{"29", 6, true},
		{"29 颗", 6, true},
		{"二十九", 6, true},
		{"30", 0, false},
		{"8", 0, false},
	}
	for _, c := range cases {
		earned, half, tags := Judge(q, "推理……\n最终答案: "+c.ans)
		if earned != c.want || half != c.half {
			t.Fatalf("Q6 %q → (%d,%v), want (%d,%v) tags=%v", c.ans, earned, half, c.want, c.half, tags)
		}
		if c.half && (len(tags) == 0 || tags[0] != "替代建模") {
			t.Fatalf("Q6 %q must tag 替代建模, got %v", c.ans, tags)
		}
	}
}

func TestJudgeCandyChainTailFallback(t *testing.T) {
	q := GenCandyClassic(newRand(42), EncodingURL)
	in := "思考：先算苹果侧……再算桃子侧……综合两种情况，最少需要取 21 颗糖。"
	earned, half, _ := Judge(q, in)
	if earned != 12 || half {
		t.Fatalf("chain-tail fallback for 21 → (%d,%v), want (12,false)", earned, half)
	}
	earned, _, _ = Judge(q, "```\n算出来是 29\n```\n最终答案: 29")
	if earned != 6 {
		t.Fatalf("fenced 29 → %d, want 6", earned)
	}
}

func TestJudgeCandyVariant(t *testing.T) {
	for seed := int64(42); seed < 62; seed++ {
		q := GenCandyVariant(newRand(seed))
		solverF, e := parseNum(q.Answer)
		if e != nil {
			t.Fatalf("Q8 answer not numeric: %q", q.Answer)
		}
		solver := int(solverF)
		blind := blindCandyAnswer(variantStockOf(q))
		cases := []struct {
			ans  string
			earn int
			half bool
			tag  string
		}{
			{q.Answer, 12, false, ""},
			{"21", 0, false, "疑似套用经典答案"},
			{strconvI(blind), 6, true, "替代建模"},
		}
		if solver+1 != 21 && solver+1 != blind {
			cases = append(cases, struct {
				ans  string
				earn int
				half bool
				tag  string
			}{strconvI(solver + 1), 0, false, ""})
		}
		for _, c := range cases {
			earned, half, tags := Judge(q, "推理……\n最终答案: "+c.ans)
			if earned != c.earn || half != c.half {
				t.Fatalf("seed %d Q8 %q → (%d,%v) want (%d,%v)", seed, c.ans, earned, half, c.earn, c.half)
			}
			if c.tag != "" && (len(tags) == 0 || tags[0] != c.tag) {
				t.Fatalf("seed %d Q8 %q tags=%v want %q", seed, c.ans, tags, c.tag)
			}
		}
	}
}

func TestJudgeSocks(t *testing.T) {
	for seed := int64(42); seed < 62; seed++ {
		q := GenSocks(newRand(seed))
		counts, p, worst, ok := socksParamsOf(q)
		if !ok {
			t.Fatalf("no params")
		}
		formula := 2*p + len(counts) - 1
		if formula == worst {
			continue
		}
		wrong := worst + 1
		if wrong == formula {
			wrong = worst - 1
		}
		cases := []struct {
			ans  string
			earn int
			half bool
		}{
			{q.Answer, 12, false},
			{strconvI(formula), 6, true},
			{strconvI(wrong), 0, false},
		}
		for _, c := range cases {
			earned, half, tags := Judge(q, "推理……\n最终答案: "+c.ans)
			if earned != c.earn || half != c.half {
				t.Fatalf("seed %d Q7 %q → (%d,%v) want (%d,%v) tags=%v", seed, c.ans, earned, half, c.earn, c.half, tags)
			}
			if c.half && (len(tags) == 0 || tags[0] != "替代建模") {
				t.Fatalf("seed %d Q7 %q tags=%v want 替代建模", seed, c.ans, tags)
			}
		}
	}
}

func TestJudgeNumericEquivalence(t *testing.T) {
	equivs := [][2]string{
		{"21", "21.0"},
		{"0.10", "0.1"},
		{"2,100", "2100"},
		{"二十一", "21"},
		{" 21 个 ", "21"},
	}
	for _, pair := range equivs {
		a, e1 := parseNum(pair[0])
		b, e2 := parseNum(pair[1])
		if e1 != nil || e2 != nil || !numEq(a, b) {
			t.Fatalf("equivalence %q vs %q failed (%v %v)", pair[0], pair[1], e1, e2)
		}
	}
}

func ExampleJudge_slotPriority() {
	q := GenCandyClassic(newRand(42), EncodingURL)
	e, _, _ := Judge(q, "中间过程提到 29。最终答案: 21")
	fmt.Println(e)
	// Output: 12
}

func strconvI(v int) string { return fmt.Sprintf("%d", v) }
func strconvAtoi(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

func TestSocksAnswerSixWithoutSlot(t *testing.T) {
	q := GenSocks(newRand(42))
	q.Answer = "6"
	q.Tags = []string{"plain"}
	earned, half, _ := Judge(q, "count the socks\nso the number is 6")
	if earned != 12 || half {
		t.Fatalf("essay 6 -> %d half=%v", earned, half)
	}
	earned, _, _ = Judge(q, "mentions 6\n最终答案: 11")
	if earned != 0 {
		t.Fatalf("wrong slot rescued to %d", earned)
	}
}
