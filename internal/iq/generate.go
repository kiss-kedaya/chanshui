// Package iq: 参数化高区分度题库 + 精确求解器 + 三模式无工具解码编码器。
// 判分口径见 judge.go；编码包装见 encode.go；卷面与半分规则见 score.go。
package iq

import (
	"fmt"
	"math/rand"
	"sort"
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
	Answer   string // canonical exact answer verified by solver
	Tags     []string
	// ImagePNG carries the rendered prompt image for vision items; it is
	// never persisted (the seed tag rebuilds it).
	ImagePNG []byte `json:"-"`
}

const answerTail = "请在最后单独一行以“最终答案: ”开头给出短结论。"

func newRand(seed int64) *rand.Rand { return rand.New(rand.NewSource(seed)) }

func pickEncoding(r *rand.Rand, modes []Encoding) Encoding {
	if len(modes) > 0 && modes[0] != "" {
		return modes[0]
	}
	all := []Encoding{EncodingURL, EncodingHex, EncodingBase64}
	if r == nil {
		return EncodingURL
	}
	return all[r.Intn(len(all))]
}

// GenerateBattery builds the full 9-question 100-point IQ paper with a
// balanced, randomly shuffled 3/3/3 mix of URL, Hex, and Base64 encodings.
func GenerateBattery(r *rand.Rand) []Question {
	if r == nil {
		r = newRand(1)
	}
	modes := BalancedEncodings(r, 9)
	return []Question{
		GenTruthLiar(r, modes[0]),
		GenSchedule(r, modes[1]),
		GenCRT(r, modes[2]),
		GenGenetics(r, modes[3]),
		GenForbidSteps(r, modes[4]),
		GenCandyClassic(r, modes[5]),
		GenSocks(r, modes[6]),
		GenCandyVariant(r, modes[7]),
		GenBridge(r, modes[8]),
	}
}

// ---------------------------------------------------------------------------
// Q1 谎言排查（5人分层真假话 DAG，精确穷举保证唯一解且破除 K=1 惯性）

type truthStmt struct {
	text string
	eval func(thief int, truth [6]bool) bool
}

func buildTruthLiarDAG(r *rand.Rand) (lines []string, k int, culprit int, ok bool) {
	// Permute speaker IDs 1..5 so topological order never matches numeric order.
	order := []int{1, 2, 3, 4, 5}
	r.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })

	stmts := make(map[int]truthStmt, 5)
	// Layer 0 (3 speakers): fact statements depending only on thief (1..5).
	for _, sp := range order[:3] {
		switch r.Intn(3) {
		case 0:
			x := 1 + r.Intn(5)
			stmts[sp] = truthStmt{
				text: fmt.Sprintf("%d: %d is the thief.", sp, x),
				eval: func(thief int, _ [6]bool) bool { return thief == x },
			}
		case 1:
			x := 1 + r.Intn(5)
			stmts[sp] = truthStmt{
				text: fmt.Sprintf("%d: %d is not the thief.", sp, x),
				eval: func(thief int, _ [6]bool) bool { return thief != x },
			}
		default:
			x := 1 + r.Intn(5)
			y := 1 + r.Intn(5)
			for y == x {
				y = 1 + r.Intn(5)
			}
			if x > y {
				x, y = y, x
			}
			stmts[sp] = truthStmt{
				text: fmt.Sprintf("%d: %d or %d is the thief.", sp, x, y),
				eval: func(thief int, _ [6]bool) bool { return thief == x || thief == y },
			}
		}
	}

	// Layer 1 (order[3]): meta statement referencing Layer 0 speakers.
	l0 := order[:3]
	sp3 := order[3]
	a := l0[r.Intn(len(l0))]
	b := l0[r.Intn(len(l0))]
	for b == a {
		b = l0[r.Intn(len(l0))]
	}
	switch r.Intn(3) {
	case 0:
		stmts[sp3] = truthStmt{
			text: fmt.Sprintf("%d: %d lies.", sp3, a),
			eval: func(_ int, tr [6]bool) bool { return !tr[a] },
		}
	case 1:
		stmts[sp3] = truthStmt{
			text: fmt.Sprintf("%d: %d tells truth.", sp3, a),
			eval: func(_ int, tr [6]bool) bool { return tr[a] },
		}
	default:
		if a > b {
			a, b = b, a
		}
		stmts[sp3] = truthStmt{
			text: fmt.Sprintf("%d: Both %d and %d lie.", sp3, a, b),
			eval: func(_ int, tr [6]bool) bool { return !tr[a] && !tr[b] },
		}
	}

	// Layer 2 (order[4]): chained meta statement referencing sp3 (2-hop DAG).
	sp4 := order[4]
	c := l0[r.Intn(len(l0))]
	switch r.Intn(3) {
	case 0:
		stmts[sp4] = truthStmt{
			text: fmt.Sprintf("%d: %d lies.", sp4, sp3),
			eval: func(_ int, tr [6]bool) bool { return !tr[sp3] },
		}
	case 1:
		stmts[sp4] = truthStmt{
			text: fmt.Sprintf("%d: %d tells truth.", sp4, sp3),
			eval: func(_ int, tr [6]bool) bool { return tr[sp3] },
		}
	default:
		x, y := sp3, c
		if x > y {
			x, y = y, x
		}
		stmts[sp4] = truthStmt{
			text: fmt.Sprintf("%d: Both %d and %d lie.", sp4, x, y),
			eval: func(_ int, tr [6]bool) bool { return !tr[x] && !tr[y] },
		}
	}

	// Evaluate truth counts for every candidate thief in topological order.
	counts := [6]int{}
	for thief := 1; thief <= 5; thief++ {
		var tr [6]bool
		tc := 0
		for _, sp := range order {
			v := stmts[sp].eval(thief, tr)
			tr[sp] = v
			if v {
				tc++
			}
		}
		counts[thief] = tc
	}

	// Pick K in {2, 3} with a UNIQUE matching culprit (and reject if K=1 yields
	// the exact same unique culprit so memorized K=1 heuristics cannot win).
	kChoices := []int{2, 3}
	if r.Intn(2) == 1 {
		kChoices[0], kChoices[1] = 3, 2
	}
	for _, candK := range kChoices {
		matches := []int{}
		k1Matches := []int{}
		for thief := 1; thief <= 5; thief++ {
			if counts[thief] == candK {
				matches = append(matches, thief)
			}
			if counts[thief] == 1 {
				k1Matches = append(k1Matches, thief)
			}
		}
		if len(matches) != 1 {
			continue
		}
		if len(k1Matches) == 1 && k1Matches[0] == matches[0] {
			continue
		}
		out := make([]string, 5)
		for sp := 1; sp <= 5; sp++ {
			out[sp-1] = stmts[sp].text
		}
		return out, candK, matches[0], true
	}
	return nil, 0, 0, false
}

// GenTruthLiar generates Q1: a 5-suspect layered truth-teller/liar deduction
// with a mathematically verified unique culprit (1..5).
func GenTruthLiar(r *rand.Rand, modes ...Encoding) Question {
	if r == nil {
		r = newRand(1)
	}
	mode := pickEncoding(r, modes)
	var lines []string
	var k, culprit int
	for attempt := 0; attempt < 500; attempt++ {
		if ls, ck, cc, ok := buildTruthLiarDAG(r); ok {
			lines, k, culprit = ls, ck, cc
			break
		}
	}
	if len(lines) == 0 {
		// Deterministic verified fallback:
		// thief=1: 1(F),2(T),3(T),4(F),5(T) -> 3
		// thief=2: 1(F),2(T),3(T),4(F),5(T) -> 3
		// thief=3: 1(T),2(F),3(T),4(T),5(F) -> 3
		// thief=4: 1(T),2(F),3(T),4(T),5(F) -> 3
		// Wait: verify every candidate explicitly for fallback:
		// 1: 3 or 4 is the thief.
		// 2: 1 lies.
		// 3: 5 is not the thief.
		// 4: 2 tells truth.
		// 5: 4 is the thief.
		// thief=1: 1F 2T 3T 4T 5F -> 3
		// thief=2: 1F 2T 3T 4T 5F -> 3
		// thief=3: 1T 2F 3T 4F 5F -> 2  <-- UNIQUE for K=2!
		// thief=4: 1T 2F 3T 4F 5T -> 3
		// thief=5: 1F 2T 3F 4T 5F -> 2  (collides with 3)
		// Better verified fallback:
		// 1: 2 is the thief.        (T iff 2)
		// 2: 4 or 5 is the thief.   (T iff 4,5)
		// 3: 1 lies.                (T iff 1,3,4,5)
		// 4: 3 tells truth.         (T iff 1,3,4,5)
		// 5: Both 1 and 2 lie.      (T iff 1,3)
		// Counts: t=1 -> 0+0+1+1+1=3; t=2 -> 1+0+0+0+0=1; t=3 -> 0+0+1+1+1=3; t=4 -> 0+1+1+1+0=3; t=5 -> 0+1+1+1+0=3 (unique K=1)
		// Swap stmt 5 -> "Both 2 and 3 lie." (T iff t=2):
		// Instead:
		// 1: 2 or 3 is the thief. (T: 2,3)
		// 2: 4 is not the thief.  (T: 1,2,3,5)
		// 3: 5 is the thief.      (T: 5)
		// 4: 1 lies.              (T: 1,4,5)
		// 5: 4 tells truth.       (T: 1,4,5)
		// Counts: t=1: 0+1+0+1+1=3; t=2: 1+1+0+0+0=2; t=3: 1+1+0+0+0=2; t=4: 0+0+0+1+1=2; t=5: 0+1+1+1+1=4 -> UNIQUE K=3 at t=1!
		lines = []string{
			"1: 2 or 3 is the thief.",
			"2: 4 is not the thief.",
			"3: 5 is the thief.",
			"4: 1 lies.",
			"5: 4 tells truth.",
		}
		k = 3
		culprit = 1
	}
	setup := fmt.Sprintf("5 suspects (1-5). Exactly 1 is the thief, and exactly %d tell the truth. Answer a single digit 1-5.", k)
	core := strings.Join(lines, "\n") + "\nWho is the thief?"
	return Question{
		ID:       "Q1",
		Title:    "谎言排查",
		Prompt:   FormatEncodedPrompt(setup, core, mode, answerTail),
		MaxScore: 10,
		LimitSec: 90,
		Answer:   strconv.Itoa(culprit),
		Tags:     []string{"真假推理", "参数化", EncodingTag(mode)},
	}
}

// ---------------------------------------------------------------------------
// Q2 并行调度（2机不可拆分最小完工时间：破除 ceil(sum/2) 下界与 LPT 贪心）

// SolveSchedule computes the exact minimum makespan for scheduling indivisible
// jobs on 2 identical parallel machines via subset enumeration.
func SolveSchedule(jobs []int) int {
	total := 0
	for _, d := range jobs {
		total += d
	}
	n := len(jobs)
	best := total
	limit := 1 << n
	for mask := 0; mask < limit; mask++ {
		s := 0
		for i := 0; i < n; i++ {
			if (mask>>i)&1 == 1 {
				s += jobs[i]
			}
		}
		other := total - s
		m := s
		if other > m {
			m = other
		}
		if m < best {
			best = m
		}
	}
	return best
}

// lptSchedule simulates the greedy Largest-Processing-Time-first heuristic on
// 2 identical machines.
func lptSchedule(jobs []int) int {
	sorted := append([]int(nil), jobs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] > sorted[j] })
	m1, m2 := 0, 0
	for _, d := range sorted {
		if m1 <= m2 {
			m1 += d
		} else {
			m2 += d
		}
	}
	if m1 > m2 {
		return m1
	}
	return m2
}

// GenSchedule generates Q2: a 5-or-6-job 2-machine indivisible scheduling
// problem whose exact optimum strictly exceeds ceil(sum/2) and strictly beats
// greedy LPT.
func GenSchedule(r *rand.Rand, modes ...Encoding) Question {
	if r == nil {
		r = newRand(2)
	}
	mode := pickEncoding(r, modes)
	var jobs []int
	var opt int
	for attempt := 0; attempt < 1500; attempt++ {
		n := 5
		if r.Intn(3) == 0 {
			n = 6
		}
		cand := make([]int, n)
		total := 0
		for i := range cand {
			cand[i] = 5 + r.Intn(10) // 5..14
			total += cand[i]
		}
		lb := (total + 1) / 2
		o := SolveSchedule(cand)
		lpt := lptSchedule(cand)
		if o > lb && lpt > o {
			jobs, opt = cand, o
			break
		}
	}
	if len(jobs) == 0 {
		// Verified fallback: sum=37, lb=19, opt=20 ({9,8}=17,{7,7,6}=20), LPT=21.
		jobs = []int{9, 8, 7, 7, 6}
		opt = SolveSchedule(jobs)
	}
	r.Shuffle(len(jobs), func(i, j int) { jobs[i], jobs[j] = jobs[j], jobs[i] })
	parts := make([]string, len(jobs))
	for i, d := range jobs {
		parts[i] = strconv.Itoa(d)
	}
	core := fmt.Sprintf(
		"2 identical ovens run in parallel.\nNo job can be split.\nJobs (min): %s.\nMin total minutes?",
		strings.Join(parts, ", "),
	)
	return Question{
		ID:       "Q2",
		Title:    "并行调度",
		Prompt:   FormatEncodedPrompt("", core, mode, answerTail),
		MaxScore: 10,
		LimitSec: 90,
		Answer:   strconv.Itoa(opt),
		Tags:     []string{"最坏情况优化", "参数化", fmt.Sprintf("实例:%s", intJoin(jobs)), EncodingTag(mode)},
	}
}

func scheduleParamsOf(q Question) (jobs []int, ok bool) {
	for _, tg := range q.Tags {
		if !strings.HasPrefix(tg, "实例:") {
			continue
		}
		for _, part := range strings.Split(strings.TrimPrefix(tg, "实例:"), ",") {
			v, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil {
				return nil, false
			}
			jobs = append(jobs, v)
		}
		return jobs, len(jobs) >= 2
	}
	return nil, false
}

// ---------------------------------------------------------------------------
// Q3 余数锁定（三模数中国剩余定理 + 下界约束，破除无下界惯性）

var crtModuliPool = [][3]int{
	{4, 5, 7},  // LCM 140
	{4, 5, 9},  // LCM 180
	{5, 6, 7},  // LCM 210
	{3, 7, 11}, // LCM 231
	{4, 7, 9},  // LCM 252
	{5, 7, 8},  // LCM 280
	{5, 7, 9},  // LCM 315
	{5, 8, 9},  // LCM 360
}

// SolveCRT finds the base solution in [0, m1*m2*m3) and the smallest N > lower
// satisfying N mod m[i] == rem[i] for pairwise-coprime moduli m.
func SolveCRT(m, rem [3]int, lower int) (ans, base int) {
	lcm := m[0] * m[1] * m[2]
	base = -1
	for x := 0; x < lcm; x++ {
		if x%m[0] == rem[0] && x%m[1] == rem[1] && x%m[2] == rem[2] {
			base = x
			break
		}
	}
	if base < 0 {
		return -1, -1
	}
	ans = base
	for ans <= lower {
		ans += lcm
	}
	return ans, base
}

// GenCRT generates Q3: a 3-modulus Chinese Remainder problem with a strict
// lower bound L > base so the smallest positive base solution is a trap.
func GenCRT(r *rand.Rand, modes ...Encoding) Question {
	if r == nil {
		r = newRand(3)
	}
	mode := pickEncoding(r, modes)
	var m, rem [3]int
	var lower, ans, base int
	for attempt := 0; attempt < 200; attempt++ {
		m = crtModuliPool[r.Intn(len(crtModuliPool))]
		for i := 0; i < 3; i++ {
			rem[i] = 1 + r.Intn(m[i]-1)
		}
		lcm := m[0] * m[1] * m[2]
		_, b := SolveCRT(m, rem, 0)
		if b < 15 || b > lcm-40 {
			continue
		}
		// Pick a clean lower bound strictly between base and base+lcm.
		raw := b + 15 + r.Intn(lcm-35)
		lo := (raw / 10) * 10
		if lo <= b {
			lo = b + 10
		}
		if lo >= b+lcm {
			lo = b + lcm - 10
		}
		a, b2 := SolveCRT(m, rem, lo)
		if a > lo && a != b2 {
			lower, ans, base = lo, a, b2
			break
		}
	}
	if ans == 0 {
		m = [3]int{5, 7, 9}
		rem = [3]int{3, 4, 2}
		lower = 300
		ans, base = SolveCRT(m, rem, lower) // base=263, ans=578
	}
	core := fmt.Sprintf(
		"Smallest integer N > %d:\nN mod %d = %d\nN mod %d = %d\nN mod %d = %d\nFind N.",
		lower, m[0], rem[0], m[1], rem[1], m[2], rem[2],
	)
	return Question{
		ID:       "Q3",
		Title:    "余数锁定",
		Prompt:   FormatEncodedPrompt("", core, mode, answerTail),
		MaxScore: 10,
		LimitSec: 90,
		Answer:   strconv.Itoa(ans),
		Tags: []string{
			"数论同余", "参数化",
			fmt.Sprintf("实例:%d,%d,%d:%d,%d,%d:%d:%d", m[0], m[1], m[2], rem[0], rem[1], rem[2], lower, base),
			EncodingTag(mode),
		},
	}
}

func crtBaseOf(q Question) (int, bool) {
	for _, tg := range q.Tags {
		if !strings.HasPrefix(tg, "实例:") {
			continue
		}
		segs := strings.Split(strings.TrimPrefix(tg, "实例:"), ":")
		if len(segs) != 4 {
			return 0, false
		}
		v, err := strconv.Atoi(segs[3])
		return v, err == nil
	}
	return 0, false
}

// ---------------------------------------------------------------------------
// Q4 伴X遗传（两代伴X隐性贝叶斯遗传与必为携带者陷阱，精确有理数答案）

type geneticsCase struct {
	core     string
	answer   string // reduced fraction p/q
	trap     string // known naive/inverted trap fraction
	trapTag  string
	instance string
}

var geneticsPool = []geneticsCase{
	// Group A: maternal grandmother is carrier (prior P(mother carrier)=1/2), k normal sons
	{
		core:     "Mother's parents: mom=XBXb, dad=XBY.\nFirst 1 son is normal.\nProb mother is carrier (p/q)?",
		answer:   "1/3",
		trap:     "1/2",
		trapTag:  "忽略贝叶斯",
		instance: "bayes-carrier-k1",
	},
	{
		core:     "Mother's parents: mom=XBXb, dad=XBY.\nFirst 2 sons are normal.\nProb mother is carrier (p/q)?",
		answer:   "1/5",
		trap:     "1/2",
		trapTag:  "忽略贝叶斯",
		instance: "bayes-carrier-k2",
	},
	{
		core:     "Mother's parents: mom=XBXb, dad=XBY.\nFirst 3 sons are normal.\nProb mother is carrier (p/q)?",
		answer:   "1/9",
		trap:     "1/2",
		trapTag:  "忽略贝叶斯",
		instance: "bayes-carrier-k3",
	},
	{
		core:     "Mother's parents: mom=XBXb, dad=XBY.\nFirst 1 son is normal.\nProb next son affected (p/q)?",
		answer:   "1/6",
		trap:     "1/4",
		trapTag:  "忽略贝叶斯",
		instance: "bayes-son-k1",
	},
	{
		core:     "Mother's parents: mom=XBXb, dad=XBY.\nFirst 2 sons are normal.\nProb next son affected (p/q)?",
		answer:   "1/10",
		trap:     "1/4",
		trapTag:  "忽略贝叶斯",
		instance: "bayes-son-k2",
	},
	{
		core:     "Mother's parents: mom=XBXb, dad=XBY.\nFirst 3 sons are normal.\nProb next son affected (p/q)?",
		answer:   "1/18",
		trap:     "1/4",
		trapTag:  "忽略贝叶斯",
		instance: "bayes-son-k3",
	},
	// Group B: obligate carrier trap (maternal grandfather affected XbY -> P(carrier)=1 always)
	{
		core:     "Mother's parents: dad=XbY, mom=XBXB.\nFirst 2 sons are normal.\nProb next son affected (p/q)?",
		answer:   "1/2",
		trap:     "1/10",
		trapTag:  "忽略必为携带者",
		instance: "obligate-son-k2",
	},
	{
		core:     "Mother's parents: dad=XbY, mom=XBXB.\nFirst 3 sons are normal.\nProb next child (unknown sex) affected (p/q)?",
		answer:   "1/4",
		trap:     "1/36",
		trapTag:  "忽略必为携带者",
		instance: "obligate-child-k3",
	},
}

const geneticsSetup = "X-linked recessive trait (Xb=affected allele, XB=normal, no mutation). Father=XBY; mother has normal vision. Give reduced fraction p/q."

// GenGenetics generates Q4: an X-linked Bayesian pedigree or obligate-carrier
// probability problem with an exact reduced-fraction answer.
func GenGenetics(r *rand.Rand, modes ...Encoding) Question {
	if r == nil {
		r = newRand(4)
	}
	mode := pickEncoding(r, modes)
	c := geneticsPool[r.Intn(len(geneticsPool))]
	return Question{
		ID:       "Q4",
		Title:    "伴X遗传",
		Prompt:   FormatEncodedPrompt(geneticsSetup, c.core, mode, answerTail),
		MaxScore: 12,
		LimitSec: 90,
		Answer:   c.answer,
		Tags:     []string{"贝叶斯遗传", "参数化", "实例:" + c.instance, EncodingTag(mode)},
	}
}

func geneticsCaseOf(q Question) (geneticsCase, bool) {
	for _, tg := range q.Tags {
		if !strings.HasPrefix(tg, "实例:") {
			continue
		}
		inst := strings.TrimPrefix(tg, "实例:")
		for _, c := range geneticsPool {
			if c.instance == inst {
				return c, true
			}
		}
	}
	return geneticsCase{}, false
}

// ---------------------------------------------------------------------------
// Q5 禁区步数（+1 / -1 / *2 最短路径 + 必经节点禁区，精确 BFS 求解）

// SolveForbidSteps computes the exact minimum number of operations (+1, -1, *2,
// staying >= 0) to move from start to target without ever visiting any value in
// forbid. Returns -1 if unreachable within search bounds.
func SolveForbidSteps(start, target int, forbid []int) int {
	blocked := make(map[int]bool, len(forbid))
	for _, f := range forbid {
		blocked[f] = true
	}
	if blocked[start] || blocked[target] {
		return -1
	}
	if start == target {
		return 0
	}
	limit := target*2 + 8
	if limit < 160 {
		limit = 160
	}
	dist := make([]int, limit+1)
	for i := range dist {
		dist[i] = -1
	}
	queue := make([]int, 0, limit+1)
	dist[start] = 0
	queue = append(queue, start)
	for head := 0; head < len(queue); head++ {
		cur := queue[head]
		d := dist[cur]
		nexts := [3]int{cur + 1, cur - 1, cur * 2}
		for _, nx := range nexts {
			if nx < 0 || nx > limit || blocked[nx] || dist[nx] >= 0 {
				continue
			}
			if nx == target {
				return d + 1
			}
			dist[nx] = d + 1
			queue = append(queue, nx)
		}
	}
	return -1
}

// GenForbidSteps generates Q5: a shortest-path state puzzle (+1, -1, *2) with
// a forbidden intermediate integer chosen to strictly break every unconstrained
// shortest path.
func GenForbidSteps(r *rand.Rand, modes ...Encoding) Question {
	if r == nil {
		r = newRand(5)
	}
	mode := pickEncoding(r, modes)
	var start, target, forbid, opt, baseDist int
	for attempt := 0; attempt < 500; attempt++ {
		s := 2 + r.Intn(4)   // 2..5
		t := 35 + r.Intn(41) // 35..75
		bd := SolveForbidSteps(s, t, nil)
		if bd < 5 || bd > 8 {
			continue
		}
		// Gather all single forbidden nodes that strictly lengthen the shortest path.
		var cands [][2]int
		for f := 3; f <= t+4; f++ {
			if f == s || f == t {
				continue
			}
			d := SolveForbidSteps(s, t, []int{f})
			if d >= bd+1 && d <= 10 {
				cands = append(cands, [2]int{f, d})
			}
		}
		if len(cands) == 0 {
			continue
		}
		// Prefer detours of +2 when available, else +1.
		var bestCands [][2]int
		for _, c := range cands {
			if c[1] >= bd+2 {
				bestCands = append(bestCands, c)
			}
		}
		if len(bestCands) == 0 {
			bestCands = cands
		}
		pick := bestCands[r.Intn(len(bestCands))]
		start, target, forbid, opt, baseDist = s, t, pick[0], pick[1], bd
		break
	}
	if opt == 0 {
		// Verified fallback: 3->47 unconstrained = 5 (3,6,12,24,48,47);
		// forbidding 24 -> 7 (3,6,12,11,22,23,46,47).
		start, target, forbid = 3, 47, 24
		baseDist = SolveForbidSteps(start, target, nil)
		opt = SolveForbidSteps(start, target, []int{forbid})
	}
	core := fmt.Sprintf(
		"Start at %d, reach %d.\nOps: +1, -1, or *2.\nNever land on %d.\nMin steps?",
		start, target, forbid,
	)
	return Question{
		ID:       "Q5",
		Title:    "禁区步数",
		Prompt:   FormatEncodedPrompt("", core, mode, answerTail),
		MaxScore: 10,
		LimitSec: 90,
		Answer:   strconv.Itoa(opt),
		Tags:     []string{"状态搜索", "参数化", fmt.Sprintf("实例:%d:%d:%d:%d", start, target, forbid, baseDist), EncodingTag(mode)},
	}
}

func forbidBaseDistOf(q Question) (int, bool) {
	for _, tg := range q.Tags {
		if !strings.HasPrefix(tg, "实例:") {
			continue
		}
		segs := strings.Split(strings.TrimPrefix(tg, "实例:"), ":")
		if len(segs) != 4 {
			return 0, false
		}
		v, err := strconv.Atoi(segs[3])
		return v, err == nil
	}
	return 0, false
}

// ---------------------------------------------------------------------------
// Q6 / Q8 糖果（最坏情况双形状博弈，编码防明文查表）

// SolveCandy returns the minimum k1+k2 such that taking k1 circles and k2
// stars (shape known, flavor unknown) guarantees
// (circle-apple && star-peach) || (circle-peach && star-apple)
// under every flavor assignment feasible for the given stock.
func SolveCandy(circ, star [3]int) int {
	ca, ct, cm := circ[0], circ[1], circ[2]
	sa, st, sm := star[0], star[1], star[2]
	best := -1
	for k1 := cm + 1; k1 <= ca+ct+cm; k1++ {
		for k2 := sm + 1; k2 <= sa+st+sm; k2++ {
			if k1 <= ct+cm && k2 <= st+sm {
				continue
			}
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

// solveCandyBrute is the independent brute-force verification of SolveCandy.
func solveCandyBrute(circ, star [3]int) int {
	ca, ct, cm := circ[0], circ[1], circ[2]
	sa, st, sm := star[0], star[1], star[2]
	cTotal, sTotal := ca+ct+cm, sa+st+sm
	guaranteed := func(k1, k2 int) bool {
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
							return false
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

const candySetup = "Candies: shape visible (Circle/Star), flavor hidden. Pick k1 Circles + k2 Stars to guarantee (Circle-apple & Star-peach) or (Circle-peach & Star-apple)."

func candyCoreASCII(circ, star [3]int) string {
	return fmt.Sprintf(
		"Circle: %d apple, %d peach, %d melon.\nStar: %d apple, %d peach, %d melon.\nMin k1+k2?",
		circ[0], circ[1], circ[2], star[0], star[1], star[2],
	)
}

func GenCandyClassic(r *rand.Rand, modes ...Encoding) Question {
	mode := pickEncoding(r, modes)
	circ := [3]int{7, 9, 8}
	star := [3]int{7, 6, 4}
	ans := SolveCandy(circ, star)
	if ans != 21 {
		panic(fmt.Sprintf("classic candy solver drifted: got %d, want 21", ans))
	}
	return Question{
		ID:       "Q6",
		Title:    "经典糖果",
		Prompt:   FormatEncodedPrompt(candySetup, candyCoreASCII(circ, star), mode, answerTail),
		MaxScore: 12,
		LimitSec: 120,
		Answer:   "21",
		Tags:     []string{"最坏情况保证", EncodingTag(mode)},
	}
}

func GenCandyVariant(r *rand.Rand, modes ...Encoding) Question {
	if r == nil {
		r = newRand(8)
	}
	mode := pickEncoding(r, modes)
	var circ, star [3]int
	for {
		for i := range circ {
			circ[i] = 4 + r.Intn(7)
			star[i] = 4 + r.Intn(7)
		}
		if SolveCandy(circ, star) != 21 {
			break
		}
	}
	ans := SolveCandy(circ, star)
	return Question{
		ID:       "Q8",
		Title:    "糖果变体",
		Prompt:   FormatEncodedPrompt(candySetup, candyCoreASCII(circ, star), mode, answerTail),
		MaxScore: 12,
		LimitSec: 120,
		Answer:   strconv.Itoa(ans),
		Tags: []string{
			"最坏情况保证", "参数化",
			fmt.Sprintf("库存:%d,%d,%d:%d,%d,%d", circ[0], circ[1], circ[2], star[0], star[1], star[2]),
			EncodingTag(mode),
		},
	}
}

func blindCandyAnswer(circ, star [3]int) int {
	ca, ct, cm := circ[0], circ[1], circ[2]
	sa, st, sm := star[0], star[1], star[2]
	cTotal, sTotal := ca+ct+cm, sa+st+sm
	best := 0
	for _, need := range []int{
		cm + sTotal + 1,
		cTotal + sm + 1,
		(ct + cm) + (st + sm) + 1,
		(ca + cm) + (sa + sm) + 1,
	} {
		if need > best {
			best = need
		}
	}
	return best
}

// ---------------------------------------------------------------------------
// Q7 袜子配对（库存受限最坏情况穷举，避开 2p+c-1 经典公式）

func SolveSocks(counts []int, p int) int {
	return socksAdversarialBrute(counts, p)
}

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

var sockColorsASCII = []string{"black", "white", "gray", "blue", "red", "brown"}

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
			continue
		}
		worst = SolveSocks(counts, p)
		if worst != 2*p+n-1 {
			return counts, p, worst, false
		}
	}
	counts, p = []int{2, 2, 2}, 2
	worst = SolveSocks(counts, p)
	return counts, p, worst, true
}

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

const socksSetup = "Dark drawer, colors invisible. Draw single socks blindly to guarantee at least P same-color pairs."

func GenSocks(r *rand.Rand, modes ...Encoding) Question {
	if r == nil {
		r = newRand(7)
	}
	mode := pickEncoding(r, modes)
	counts, p, worst, tagged := genSocksInstance(r, 50)
	n := len(counts)
	pool := append([]string(nil), sockColorsASCII...)
	r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	display := pool[:n]
	parts := make([]string, n)
	for i := range counts {
		parts[i] = fmt.Sprintf("%d %s", counts[i], display[i])
	}
	core := fmt.Sprintf("Stocks: %s.\nTarget P = %d pairs.\nMin socks?", strings.Join(parts, ", "), p)
	tags := []string{"最坏情况保证", "穷举出题", fmt.Sprintf("实例:%s:%d", intJoin(counts), p), EncodingTag(mode)}
	if tagged {
		tags = append(tags, "退化实例")
	}
	return Question{
		ID:       "Q7",
		Title:    "袜子配对",
		Prompt:   FormatEncodedPrompt(socksSetup, core, mode, answerTail),
		MaxScore: 12,
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

// ---------------------------------------------------------------------------
// Q9 夜间过桥（定向支架版本 + 参数化速度组，破除背诵 10 与忽略支架 17）

type bridgeSpec struct {
	times [4]int // sorted a < b < c < d with a+b+c <= d so exact makespan == d
}

var bridgePool = []bridgeSpec{
	{times: [4]int{1, 2, 7, 10}}, // opt=10, carry-back=17
	{times: [4]int{1, 3, 7, 11}}, // opt=11, carry-back=21
	{times: [4]int{1, 2, 8, 12}}, // opt=12, carry-back=19
	{times: [4]int{2, 3, 8, 13}}, // opt=13, carry-back=24
	{times: [4]int{1, 3, 8, 12}}, // opt=12, carry-back=22
}

// bridgeCarryBackAnswer returns the classic textbook answer when the torch must
// be carried back on every trip: min(a + 3*b + d, 2*a + b + c + d).
func bridgeCarryBackAnswer(t [4]int) int {
	a, b, c, d := t[0], t[1], t[2], t[3]
	s1 := a + 3*b + d
	s2 := 2*a + b + c + d
	if s1 < s2 {
		return s1
	}
	return s2
}

const bridgeSetup = "4 workers cross a bridge at night (constant speed, max 2 on bridge at once; light required while crossing)."

// GenBridge generates Q9: the night bridge-crossing puzzle with a directional
// torch stand and parameterized crossing speeds.
func GenBridge(r *rand.Rand, modes ...Encoding) Question {
	mode := pickEncoding(r, modes)
	spec := bridgePool[0]
	if r != nil {
		spec = bridgePool[r.Intn(len(bridgePool))]
	}
	t := spec.times
	core := fmt.Sprintf(
		"Times: %d, %d, %d, %d min.\nOne torch on a fixed stand lights the whole bridge.\nMin total minutes?",
		t[0], t[1], t[2], t[3],
	)
	return Question{
		ID:       "Q9",
		Title:    "夜间过桥",
		Prompt:   FormatEncodedPrompt(bridgeSetup, core, mode, answerTail),
		MaxScore: 12,
		LimitSec: 75,
		Answer:   strconv.Itoa(t[3]),
		Tags: []string{
			"最坏情况优化", "定向支架", "参数化",
			fmt.Sprintf("实例:%d,%d,%d,%d", t[0], t[1], t[2], t[3]),
			EncodingTag(mode),
		},
	}
}

func bridgeTimesOf(q Question) ([4]int, bool) {
	for _, tg := range q.Tags {
		if !strings.HasPrefix(tg, "实例:") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(tg, "实例:"), ",")
		if len(parts) != 4 {
			continue
		}
		var out [4]int
		ok := true
		for i, p := range parts {
			v, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil {
				ok = false
				break
			}
			out[i] = v
		}
		if ok {
			return out, true
		}
	}
	return [4]int{1, 2, 7, 10}, false
}
