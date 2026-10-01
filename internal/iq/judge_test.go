package iq

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func mdWrap(s string) string {
	return "```text\n思考过程……\n```\n**分析**：先看遗传……\n" + s
}

func TestExtractAnswerSlot(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"推理若干……\n最终答案: 非亲生", "非亲生"},
		{"推理若干……\n最终答案：非亲生", "非亲生"},
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
	in := "```text\n草稿 38\n```\n## 小结\n**最终答案**: **非亲生**"
	slot, _ := ExtractAnswer(in)
	if strings.TrimSpace(slot) != "非亲生" {
		t.Fatalf("markdown not stripped: %q", slot)
	}
}

func TestExtractAnswerChainTailFallback(t *testing.T) {
	long := strings.Repeat("推理链条。", 500) // ~2000+ chars
	_, tail := ExtractAnswer(long + "末尾结论：至少 21 颗")
	if !strings.Contains(tail, "至少 21 颗") {
		t.Fatalf("chain tail should contain the ending: %q", tail[len(tail)-40:])
	}
	if len([]rune(tail)) > 2000 {
		t.Fatalf("chain tail must be capped at 2000 chars, got %d", len([]rune(tail)))
	}
}

func TestJudgePill(t *testing.T) {
	q := GenPill()
	cases := []struct {
		ans  string
		want int
		half bool
	}{
		{"不叫醒，暂缓给药，持药观察", 6, false},
		{"不喂药，继续观察，他已经睡着", 6, false},
		{"无需服药，等他自然醒", 6, false},
		{"暂不给药，先观察呼吸", 6, false},
		{"应该叫醒他再喂一粒", 0, false},
		{"把老人唤醒补服一片", 0, false},
		{"可以叫醒，但先喂水", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		earned, half, _ := Judge(q, "推理……\n最终答案: "+c.ans)
		if earned != c.want || half != c.half {
			t.Fatalf("pill %q → (%d,%v), want (%d,%v)", c.ans, earned, half, c.want, c.half)
		}
	}
	// “不叫醒” must NOT be treated as a wake action
	earned, _, _ := Judge(q, "推理……\n最终答案: 不叫醒")
	if earned != 6 {
		t.Fatalf("不叫醒 must pass, got %d", earned)
	}
}

func TestJudgeColorBlind(t *testing.T) {
	q := GenColorBlind(newRand(42))
	cases := []struct {
		ans  string
		want int
		half bool
		tag  string
	}{
		{"非亲生", 14, false, ""},
		{"女儿不是亲生的", 14, false, ""},
		{"不可能是亲生，伴X隐性遗传女儿发病父亲必病", 14, false, ""},
		{"排除亲生关系", 14, false, ""},
		{"亲生的可能性极低", 0, false, ""},
		{"并非常见的亲生判断，应属非亲生", 14, false, ""},
		{"红绿色盲是伴X隐性遗传，父亲正常则女儿不该发病", 7, true, "仅遗传链"},
		{"这是隔代遗传，外公传给女儿", 7, true, "仅遗传链"},
		{"色盲基因来自外祖父", 7, true, "仅遗传链"},
		{"亲生，可能是基因突变", 0, false, ""},
		{"父亲色觉正常，女儿也正常", 0, false, ""},
		{"", 0, false, ""},
	}
	for _, c := range cases {
		earned, half, tags := Judge(q, "推理……\n最终答案: "+c.ans)
		if earned != c.want || half != c.half {
			t.Fatalf("Q4 %q → (%d,%v), want (%d,%v)", c.ans, earned, half, c.want, c.half)
		}
		if c.tag != "" && len(tags) == 0 {
			t.Fatalf("Q4 %q missing tag %q", c.ans, c.tag)
		}
		if c.tag != "" && tags[0] != c.tag {
			t.Fatalf("Q4 %q tag=%q want %q", c.ans, tags[0], c.tag)
		}
	}
}

func TestJudgeCandyClassic(t *testing.T) {
	q := GenCandyClassic(newRand(42))
	cases := []struct {
		ans  string
		want int
		half bool
	}{
		{"21", 16, false},
		{"21 颗", 16, false},
		{"至少 21.0", 16, false},
		{"二十一", 16, false},
		{"0.10 颗", 0, false},
		{"29", 8, true},
		{"29 颗", 8, true},
		{"二十九", 8, true},
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
	q := GenCandyClassic(newRand(42))
	// no slot at all; the chain tail ends with the number
	in := "思考：先算苹果侧……再算桃子侧……综合两种情况，最少需要取 21 颗糖。"
	earned, half, _ := Judge(q, in)
	if earned != 16 || half {
		t.Fatalf("chain-tail fallback for 21 → (%d,%v), want (16,false)", earned, half)
	}
	// markdown-fenced slot still found after stripping
	earned, _, _ = Judge(q, "```\n算出来是 29\n```\n最终答案: 29")
	if earned != 8 {
		t.Fatalf("fenced 29 → %d, want 8", earned)
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
		formula := strconvI(blind)
		cases := []struct {
			ans  string
			earn int
			half bool
			tag  string
		}{
			{q.Answer, 16, false, ""},
			{strconvI(21), 0, false, "疑似套用经典答案"},
			{formula, 8, true, "替代建模"},
		}
		// solver+1 must be rejected; skip when it collides with 21/25/blind
		if solver+1 != 21 && solver+1 != blind && solver+1 != 25 {
			cases = append(cases, struct {
				ans  string
				earn int
				half bool
				tag  string
			}{strconvI(solver + 1), 0, false, ""})
		}
		// 25 is judged as the classic blind answer only when it collides with
		// none of the stock's own meaningful values (solver, solver+1, blind)
		if blind != 25 && solver != 25 && solver+1 != 25 {
			cases = append(cases, struct {
				ans  string
				earn int
				half bool
				tag  string
			}{strconvI(25), 0, false, ""})
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
			continue // generator fell back on this seed; formula answer == correct
		}
		wrong := worst + 1
		if wrong == formula {
			wrong = worst - 1
		}
		if wrong < 0 {
			continue
		}
		cases := []struct {
			ans  string
			earn int
			half bool
		}{
			{q.Answer, 16, false},
			{strconvI(formula), 8, true},
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
		if e1 != nil || e2 != nil || numEq(a, b) != true {
			t.Fatalf("equivalence %q vs %q failed (%v %v)", pair[0], pair[1], e1, e2)
		}
	}
	if _, e := parseNum("二十一"); e != nil || !numEq(mustNum("二十一"), 21) {
		t.Fatalf("CN numeral parse failed")
	}
	if _, e := parseNum("一百"); e != nil {
		// 一百 = 100 is beyond the ≤99 map; must not parse as 21-like garbage
		t.Logf("一百 intentionally unsupported: %v", e)
	}
}

func TestJudgeAnchorLevelDrop(t *testing.T) {
	q := GenAnchor()
	full := []string{
		"最终答案: 水位下降",
		"最终答案: 水位会下降",
		"推理里写了水位下降。\n最终答案: 下降",
		"最终答案: 下降，因为铁锚在船上时排水体积更大",
	}
	for _, text := range full {
		score, half, _ := Judge(q, text)
		if score != 10 || half {
			t.Fatalf("full %q => %d half=%v", text, score, half)
		}
	}
	score, half, tags := Judge(q, "最终答案: 下降")
	if score != 5 || !half || len(tags) == 0 {
		t.Fatalf("direction only => %d half=%v tags=%v", score, half, tags)
	}
	score, _, _ = Judge(q, "最终答案: 上升")
	if score != 0 {
		t.Fatalf("rise => %d", score)
	}
	score, _, _ = Judge(q, "正文误写水位下降。\n最终答案: 上升")
	if score != 0 {
		t.Fatalf("contradiction => %d", score)
	}
}

func TestJudgeTagsPropagate(t *testing.T) {
	q := GenCandyClassic(newRand(42))
	_, _, tags := Judge(q, "推理……\n最终答案: 29")
	if len(tags) == 0 || tags[0] != "替代建模" {
		t.Fatalf("tags must propagate: %v", tags)
	}
}

func ExampleJudge_slotPriority() {
	q := GenCandyClassic(newRand(42))
	e, _, _ := Judge(q, "中间过程提到 29。最终答案: 21")
	fmt.Println(e)
	// Output: 16
}

func mustNum(s string) float64 {
	v, _ := parseNum(s)
	return v
}

func strconvI(v int) string { return fmt.Sprintf("%d", v) }
func strconvAtoi(s string) (int, error) {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	return v, err
}

func TestSocksAnswerSixWithoutSlot(t *testing.T) {
	q := GenSocks(newRand(42))
	q.Answer = "6"
	q.Tags = []string{"plain"}
	earned, half, _ := Judge(q, "count the socks\nso the number is 6")
	if earned != 16 || half {
		t.Fatalf("essay 6 -> %d half=%v", earned, half)
	}
	earned, _, _ = Judge(q, "mentions 6\n"+"\u6700\u7ec8\u7b54\u6848: 11")
	if earned != 0 {
		t.Fatalf("wrong slot rescued to %d", earned)
	}
}
