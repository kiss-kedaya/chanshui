// Package iq: 参数化生成器 + 最坏情况求解器（色盲 / 糖果 / 糖果变体 / 袜子配对 / 安眠药）。
// 判分口径见 judge.go；卷面与半分规则见 score.go。
package iq

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
)

// Question is one IQ probe item.
type Question struct {
	ID       string
	Title    string
	Prompt   string
	MaxScore int
	LimitSec int
	Answer   string // canonical answer for solver-based items; keyword-based for Q1
	Tags     []string
}

const answerTail = "请在最后单独一行以“最终答案: ”开头给出短结论。"

func newRand(seed int64) *rand.Rand { return rand.New(rand.NewSource(seed)) }

// ---------------------------------------------------------------------------
// Q1 安眠药（静态）

func GenPill() Question {
	prompt := "病人已经安稳睡着，医嘱为睡前口服安眠药，但药尚未发放。护士此刻应如何处理？\n" + answerTail
	return Question{
		ID:       "Q1",
		Title:    "安眠药",
		Prompt:   prompt,
		MaxScore: 6,
		LimitSec: 55,
		Answer:   "不叫醒，暂缓给药，持药观察",
		Tags:     []string{"安全判断"},
	}
}

// GenCarWash is iqbench Q2, a simple instruction-following control.
func GenCarWash() Question {
	return Question{ID: "Q2", Title: "洗车路径", Prompt: "车在楼下，洗车店 4 公里。请问应该如何前往？\n" + answerTail, MaxScore: 5, LimitSec: 45, Answer: "开车去", Tags: []string{"认知反射"}}
}

// GenBatBall is iqbench Q3, the classic bat-and-ball numerical trap.
func GenBatBall() Question {
	return Question{ID: "Q3", Title: "球拍与球", Prompt: "球拍和球一共 1.10 元，球拍比球贵 1 元。球多少钱？\n" + answerTail, MaxScore: 6, LimitSec: 45, Answer: "0.05", Tags: []string{"认知反射", "数值等价"}}
}

// ---------------------------------------------------------------------------
// Q4 红绿色盲（三组数字对随机，答案恒「非亲生」）

type cbPair struct {
	father   string // 色觉正常者读出的数字
	daughter string // 女儿读出的数字
	remark   string // 父亲对圆点红绿分布的描述
}

var cbPairs = []cbPair{
	{father: "88", daughter: "38", remark: "第二个数字明明是上下两个圆圈拼成的 8，左半圈的圆点是红色、右半圈是绿色，他看得清清楚楚"},
	{father: "72", daughter: "27", remark: "那张图的红绿圆点排布他看得分明，两个数字一前一后根本不会看反"},
	{father: "54", daughter: "45", remark: "两个数字左右位置分明，红绿圆点的边界他看得很清楚"},
}

func GenColorBlind(r *rand.Rand) Question {
	p := cbPairs[r.Intn(len(cbPairs))]
	prompt := fmt.Sprintf(
		"体检时用的红绿色盲检测图上印着一个两位数。色觉完全正常的父亲盯着图看，说图上是「%s」，%s；可女儿却坚持说图上是「%s」。父亲反复核对后确认女儿看错了，随后崩溃了。医生最终确诊：女儿是红绿色盲，而父亲色觉完全正常。本题为经典孟德尔遗传模型：红绿色盲是完全外显的伴X染色体隐性遗传；排除新发突变、染色体异常及诊断误差。\n"+
			"请结合遗传学规律给出最终结论：女儿是亲生还是非亲生？\n"+answerTail,
		p.father, p.remark, p.daughter)
	return Question{
		ID:       "Q4",
		Title:    "红绿色盲",
		Prompt:   prompt,
		MaxScore: 14,
		LimitSec: 90,
		Answer:   "非亲生",
		Tags:     []string{"伴X隐性遗传", "参数化"},
	}
}

// GenAnchor is iqbench Q5. Full credit requires 水位下降, or the direction plus its
// displacement/weighting explanation; direction alone receives half credit.
func GenAnchor() Question {
	return Question{ID: "Q5", Title: "船锚水位", Prompt: "船停在池塘中，铁锚原本在船上，后来被扔进池底。池塘水位会上升、下降，还是不变？请说明理由。\n" + answerTail, MaxScore: 10, LimitSec: 60, Answer: "下降", Tags: []string{"多步科学演绎"}}
}

// ---------------------------------------------------------------------------
// Q6 / Q8 糖果

// stock: [apple, peach, melon] counts for one shape.

// SolveCandy returns the minimum k1+k2 such that taking k1 circles and k2
// stars (shape known, flavor unknown) guarantees
// (circle-apple && star-peach) || (circle-peach && star-apple)
// under every flavor assignment feasible for the given stock.
// Analytic form: all four blocking strategies must be made infeasible.
func SolveCandy(circ, star [3]int) int {
	ca, ct, cm := circ[0], circ[1], circ[2]
	sa, st, sm := star[0], star[1], star[2]
	best := -1
	for k1 := cm + 1; k1 <= ca+ct+cm; k1++ {
		for k2 := sm + 1; k2 <= sa+st+sm; k2++ {
			// blocking (b) no apples at all: k1<=ct+cm && k2<=st+sm
			if k1 <= ct+cm && k2 <= st+sm {
				continue
			}
			// blocking (c) no peaches at all: k1<=ca+cm && k2<=sa+sm
			if k1 <= ca+cm && k2 <= sa+sm {
				continue
			}
			if k1+k2 < best || best < 0 {
				best = k1 + k2
			}
		}
	}
	return best
}

// solveCandyBrute is the independent brute-force verification of SolveCandy:
// enumerate every feasible flavor split of k1 circles and k2 stars and require
// the cross condition to hold for all of them.
func solveCandyBrute(circ, star [3]int) int {
	ca, ct, cm := circ[0], circ[1], circ[2]
	sa, st, sm := star[0], star[1], star[2]
	cTotal, sTotal := ca+ct+cm, sa+st+sm
	guaranteed := func(k1, k2 int) bool {
		// enumerate every joint flavor assignment (circle split x star split);
		// guaranteed iff no assignment violates the cross condition
		for x1 := 0; x1 <= k1 && x1 <= ca; x1++ {
			for y1 := 0; y1 <= k1-x1 && y1 <= ct; y1++ {
				if k1-x1-y1 > cm {
					continue
				}
				for y2 := 0; y2 <= k2 && y2 <= st; y2++ {
					for x2 := 0; x2 <= k2-y2 && x2 <= sa; x2++ {
						if k2-y2-x2 > sm {
							continue
						}
						if (x1 == 0 || y2 == 0) && (y1 == 0 || x2 == 0) {
							return false // adversarial split defeats the guarantee
						}
					}
				}
			}
		}
		return true
	}
	for total := 0; total <= cTotal+sTotal; total++ {
		for k1 := 0; k1 <= cTotal && k1 <= total; k1++ {
			k2 := total - k1
			if k2 < 0 || k2 > sTotal {
				continue
			}
			if guaranteed(k1, k2) {
				return total
			}
		}
	}
	return -1
}

func candyPrompt(apple, peach, melon int, appleS, peachS, melonS int, hint bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "一盒糖果中，圆形的有苹果味 %d 颗、桃子味 %d 颗、西瓜味 %d 颗；星形的有苹果味 %d 颗、桃子味 %d 颗、西瓜味 %d 颗。糖果只能看出形状，看不出口味。", apple, peach, melon, appleS, peachS, melonS)
	b.WriteString("从中取糖，至少要取出多少颗，才能保证取出的糖中，要么既有圆形苹果味又有星形桃子味，要么既有圆形桃子味又有星形苹果味？")
	if hint {
		b.WriteString("（取糖时可以看到形状，因此你可以先取够想要的圆形数量，再取够想要的星形数量）")
	}
	b.WriteString("\n" + answerTail)
	return b.String()
}

func GenCandyClassic(r *rand.Rand) Question {
	circ := [3]int{7, 9, 8}
	star := [3]int{7, 6, 4}
	ans := SolveCandy(circ, star)
	if ans != 21 {
		panic(fmt.Sprintf("classic candy solver drifted: got %d, want 21", ans))
	}
	return Question{
		ID:       "Q6",
		Title:    "经典糖果",
		Prompt:   candyPrompt(7, 9, 8, 7, 6, 4, true),
		MaxScore: 16,
		LimitSec: 120,
		Answer:   "21",
		Tags:     []string{"最坏情况保证"},
	}
}

func GenCandyVariant(r *rand.Rand) Question {
	var circ, star [3]int
	for {
		for i := range circ {
			circ[i] = 4 + r.Intn(7)
			star[i] = 4 + r.Intn(7)
		}
		// avoid degenerate collisions with the classic bookkeeping: solver
		// answer must not equal 21 (the classic answer) so that the
		// 疑似套用 rule stays distinguishable from a genuine solve.
		if SolveCandy(circ, star) != 21 {
			break
		}
	}
	ans := SolveCandy(circ, star)
	return Question{
		ID:       "Q8",
		Title:    "糖果变体",
		Prompt:   candyPrompt(circ[0], circ[1], circ[2], star[0], star[1], star[2], true),
		MaxScore: 16,
		LimitSec: 120,
		Answer:   strconv.Itoa(ans),
		Tags: []string{"最坏情况保证", "参数化",
			fmt.Sprintf("库存:%d,%d,%d:%d,%d,%d", circ[0], circ[1], circ[2], star[0], star[1], star[2])},
	}
}

// blindCandyAnswer is the naive no-shape-strategy answer: 1 + the largest
// stock-constrained draw set that still avoids both cross pairs. For the
// classic stock this is 29.
func blindCandyAnswer(circ, star [3]int) int {
	ca, ct, cm := circ[0], circ[1], circ[2]
	sa, st, sm := star[0], star[1], star[2]
	cTotal, sTotal := ca+ct+cm, sa+st+sm
	best := 0
	for _, need := range []int{
		cm + sTotal + 1,           // circles all melon
		cTotal + sm + 1,           // stars all melon
		(ct + cm) + (st + sm) + 1, // no apples
		(ca + cm) + (sa + sm) + 1, // no peaches
	} {
		if need > best {
			best = need
		}
	}
	return best
}

// ---------------------------------------------------------------------------
// Q7 袜子配对

// SolveSocks: minimum n such that ANY n socks drawn (colors unknown in the
// dark) must contain at least p complete same-color pairs. Adversarial
// multiset enumeration over per-color draw counts.
func SolveSocks(counts []int, p int) int {
	return socksAdversarialBrute(counts, p)
}

// socksAdversarialBrute enumerates every adversarial draw multiset
// (d_1..d_c with 0 <= d_i <= c_i) and returns (max total with < p pairs) + 1.
func socksAdversarialBrute(counts []int, p int) int {
	pairsOf := func(d int) int { return d / 2 }
	best := 0
	var walk func(i, drawn, pairs int)
	walk = func(i, drawn, pairs int) {
		if i == len(counts) {
			if pairs < p && drawn > best {
				best = drawn
			}
			return
		}
		maxD := counts[i]
		// cap: this color alone cannot exceed the pair budget contribution
		lim := maxD
		if lim > 2*(p-1-pairs)+1 {
			lim = 2*(p-1-pairs) + 1
		}
		if lim < 0 {
			lim = 0
		}
		for d := 0; d <= lim; d++ {
			walk(i+1, drawn+d, pairs+pairsOf(d))
		}
	}
	walk(0, 0, 0)
	if best < 0 {
		return 0
	}
	return best + 1
}

var sockColors = []string{"黑", "白", "灰", "蓝", "红", "棕"}

// genSocksInstance draws a random feasible instance whose worst-case value
// differs from the classic formula 2p+c-1 (so the well-known formula answer
// would be wrong on this instance). maxAttempts bounds regeneration; when all
// attempts fail the last instance is accepted with tagged=true.
func genSocksInstance(r *rand.Rand, maxAttempts int) (counts []int, p, worst int, tagged bool) {
	n := 3 + r.Intn(2) // 3..4 colors
	for attempt := 0; attempt < maxAttempts; attempt++ {
		counts = make([]int, n)
		for i := range counts {
			counts[i] = 2 + r.Intn(6) // 2..7
		}
		p = 2 + r.Intn(3) // 2..4
		pairs := 0
		for _, c := range counts {
			pairs += c / 2
		}
		if pairs < p {
			continue // infeasible instance
		}
		worst = SolveSocks(counts, p)
		if worst != 2*p+n-1 {
			return counts, p, worst, false
		}
		// formula coincidence: loop and redraw a fresh stock
	}
	// fallback: use a verified inventory-constrained instance
	counts, p = []int{2, 2, 2}, 2
	// accept the last attempt (or a fixed default when no attempt
	// was made), tagged
	if counts == nil {
		counts, p = []int{2, 2, 2}, 2
	}
	worst = SolveSocks(counts, p)
	return counts, p, worst, true
}

// genSocksInstanceDefault is the degenerate no-attempt variant used by tests
// to reach the forced-fallback branch deterministically.
func genSocksInstanceDefault(maxAttempts int) (counts []int, p, worst int, tagged bool) {
	r := newRand(12345)
	n := 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		counts = make([]int, n)
		for i := range counts {
			counts[i] = 2 + r.Intn(6)
		}
		p = 2 + r.Intn(3)
		pairs := 0
		for _, c := range counts {
			pairs += c / 2
		}
		if pairs < p {
			continue
		}
		worst = SolveSocks(counts, p)
		if worst != 2*p+n-1 {
			return counts, p, worst, false
		}
	}
	if counts == nil {
		counts, p = []int{2, 2, 2}, 2
	}
	worst = SolveSocks(counts, p)
	return counts, p, worst, true
}

func GenSocks(r *rand.Rand) Question {
	counts, p, worst, tagged := genSocksInstance(r, 50)
	n := len(counts)
	pool := append([]string(nil), sockColors...)
	r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	display := pool[:n]
	parts := make([]string, n)
	for i := range counts {
		parts[i] = fmt.Sprintf("%s色袜子 %d 只", display[i], counts[i])
	}
	prompt := fmt.Sprintf(
		"抽屉里混放着 %d 种颜色的袜子：%s。袜子不成对地乱放在一起，房间里一片漆黑，只能靠手感把袜子摸出来，分辨不出颜色。至少要摸出多少只袜子，才能保证其中一定有 %d 双同色的袜子？\n%s",
		n, strings.Join(parts, "、"), p, answerTail)
	tags := []string{"最坏情况保证", "穷举出题", fmt.Sprintf("实例:%s:%d", intJoin(counts), p)}
	if tagged {
		tags = append(tags, "退化实例")
	}
	return Question{
		ID:       "Q7",
		Title:    "袜子配对",
		Prompt:   prompt,
		MaxScore: 16,
		LimitSec: 120,
		Answer:   strconv.Itoa(worst),
		Tags:     tags,
	}
}

func intJoin(xs []int) string {
	ss := make([]string, len(xs))
	for i, v := range xs {
		ss[i] = strconv.Itoa(v)
	}
	return strings.Join(ss, ",")
}

// socksParamsOf recovers the stock/p/worst recorded in a Q7 question's tag
// "实例:<c1,c2,...>:<p>".
func socksParamsOf(q Question) (counts []int, p, worst int, ok bool) {
	for _, tg := range q.Tags {
		if !strings.HasPrefix(tg, "实例:") {
			continue
		}
		body := strings.TrimPrefix(tg, "实例:")
		colon := strings.IndexByte(body, ':')
		if colon < 0 {
			return nil, 0, 0, false
		}
		cs, ps := body[:colon], body[colon+1:]
		for _, part := range strings.Split(cs, ",") {
			v, e := strconv.Atoi(part)
			if e != nil {
				return nil, 0, 0, false
			}
			counts = append(counts, v)
		}
		pv, e := strconv.Atoi(ps)
		if e != nil {
			return nil, 0, 0, false
		}
		wv, e := strconv.Atoi(q.Answer)
		if e != nil {
			return nil, 0, 0, false
		}
		return counts, pv, wv, true
	}
	return nil, 0, 0, false
}
