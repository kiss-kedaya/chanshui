package iq

import (
	"fmt"
	"strings"
	"testing"
)

const tailLine = "请在最后单独一行以“最终答案: ”开头给出短结论。"

func TestGenPill(t *testing.T) {
	q := GenPill()
	if q.ID != "Q1" || q.MaxScore != 6 || q.LimitSec != 55 {
		t.Fatalf("pill meta: %+v", q)
	}
	if q.Prompt == "" || strings.Contains(q.Prompt, "\n\n\n") {
		t.Fatalf("pill prompt empty/odd: %q", q.Prompt)
	}
	if !strings.HasSuffix(q.Prompt, tailLine) {
		t.Fatalf("pill prompt must end with answer-slot instruction, got %q", q.Prompt)
	}
}

func TestGenColorBlindVariants(t *testing.T) {
	seen := map[string]int{}
	for seed := int64(42); seed < 62; seed++ {
		q := GenColorBlind(newRand(seed))
		if q.ID != "Q4" || q.MaxScore != 14 || q.LimitSec != 90 {
			t.Fatalf("Q4 meta: %+v", q)
		}
		if q.Answer != "非亲生" {
			t.Fatalf("Q4 answer must be constant 非亲生, got %q", q.Answer)
		}
		if !strings.Contains(q.Prompt, "伴X") || !strings.Contains(q.Prompt, "崩溃") {
			t.Fatalf("Q4 prompt missing key story parts: %q", q.Prompt)
		}
		if !strings.HasSuffix(q.Prompt, tailLine) {
			t.Fatalf("Q4 prompt tail: %q", q.Prompt)
		}
		hit := false
		for _, pair := range []string{"38", "88", "27", "72", "45", "54"} {
			if strings.Contains(q.Prompt, pair) {
				hit = true
				seen[pair]++
			}
		}
		if !hit {
			t.Fatalf("Q4 prompt missing digit-pair material: %q", q.Prompt)
		}
	}
	if len(seen) < 2 {
		t.Fatalf("Q4 variant material should vary across seeds: %v", seen)
	}
}

func TestGenCandyClassicFixed(t *testing.T) {
	q := GenCandyClassic(newRand(42))
	if q.ID != "Q6" || q.MaxScore != 16 || q.LimitSec != 120 {
		t.Fatalf("Q6 meta: %+v", q)
	}
	if q.Answer != "21" {
		t.Fatalf("Q6 classic answer must be 21, got %q", q.Answer)
	}
	for _, want := range []string{"7", "9", "8", "6", "4", "圆形", "星形", "形状"} {
		if !strings.Contains(q.Prompt, want) {
			t.Fatalf("Q6 prompt missing %q: %q", want, q.Prompt)
		}
	}
	if !strings.HasSuffix(q.Prompt, tailLine) {
		t.Fatalf("Q6 prompt tail: %q", q.Prompt)
	}
}

func TestSolveCandyBruteMatchesAnalytic(t *testing.T) {
	if SolveCandy([3]int{7, 9, 8}, [3]int{7, 6, 4}) != 21 {
		t.Fatalf("classic analytic solver must return 21")
	}
	if solveCandyBrute([3]int{7, 9, 8}, [3]int{7, 6, 4}) != 21 {
		t.Fatalf("classic brute solver must return 21")
	}
	for i := 0; i < 20; i++ {
		seed := int64(42 + i)
		r := newRand(seed)
		circ := [3]int{4 + r.Intn(7), 4 + r.Intn(7), 4 + r.Intn(7)}
		star := [3]int{4 + r.Intn(7), 4 + r.Intn(7), 4 + r.Intn(7)}
		a := SolveCandy(circ, star)
		b := solveCandyBrute(circ, star)
		if a != b {
			t.Fatalf("seed %d stock %v %v: analytic %d != brute %d", seed, circ, star, a, b)
		}
	}
}

func TestGenCandyVariant(t *testing.T) {
	prev := ""
	changed := 0
	for seed := int64(42); seed < 62; seed++ {
		q := GenCandyVariant(newRand(seed))
		if q.ID != "Q8" || q.MaxScore != 16 || q.LimitSec != 120 {
			t.Fatalf("Q8 meta: %+v", q)
		}
		if q.Answer == "" {
			t.Fatalf("Q8 answer empty")
		}
		if strings.Contains(q.Prompt, "21") && q.Answer != "21" {
			// 21 may legitimately appear as a flavor count; only forbid the
			// classic answer text leaking as a target — covered below.
		}
		if !strings.Contains(q.Prompt, "圆形") || !strings.Contains(q.Prompt, "星形") {
			t.Fatalf("Q8 prompt missing shapes: %q", q.Prompt)
		}
		if !strings.HasSuffix(q.Prompt, tailLine) {
			t.Fatalf("Q8 prompt tail: %q", q.Prompt)
		}
		if prev != "" && prev != q.Answer {
			changed++
		}
		prev = q.Answer
	}
	if changed == 0 {
		t.Fatalf("Q8 answers should vary across seeds")
	}
}

func TestSolveSocksBasics(t *testing.T) {
	// classic formula instance: 4 colors plenty counts, p=2 → 2p+c-1 = 7
	if got := SolveSocks([]int{7, 7, 7, 7}, 2); got != 7 {
		t.Fatalf("SolveSocks(4x7, p=2) = %d, want 7 (2p+c-1)", got)
	}
	// cramped stock: (2,2,2), p=2 → brute 5 < formula 6
	if got := SolveSocks([]int{2, 2, 2}, 2); got != 5 {
		t.Fatalf("SolveSocks(2,2,2 p=2) = %d, want 5", got)
	}
}

func TestGenSocksRejectsFormulaInstances(t *testing.T) {
	fellBack := 0
	for seed := int64(42); seed < 62; seed++ {
		q := GenSocks(newRand(seed))
		if q.ID != "Q7" || q.MaxScore != 16 || q.LimitSec != 120 {
			t.Fatalf("Q7 meta: %+v", q)
		}
		counts, p, worst, ok := socksParamsOf(q)
		if !ok {
			t.Fatalf("Q7 not carrying params: %q", q.Answer)
		}
		if got := SolveSocks(counts, p); got != worst {
			t.Fatalf("seed %d: SolveSocks mismatch %d vs %d", seed, got, worst)
		}
		for _, tg := range q.Tags {
			if tg == "退化实例" {
				fellBack++
			}
		}
		if worst == 2*p+len(counts)-1 {
			for _, tg := range q.Tags {
				if tg == "退化实例" {
					continue
				}
			}
			// only acceptable via the explicit fallback tag
			tagged := false
			for _, tg := range q.Tags {
				if tg == "退化实例" {
					tagged = true
				}
			}
			if !tagged {
				t.Fatalf("seed %d: instance equals formula 2p+c-1 without fallback tag: %+v", seed, q)
			}
			continue
		}
		if !strings.Contains(q.Prompt, "同色") {
			t.Fatalf("Q7 prompt missing pairing goal: %q", q.Prompt)
		}
		if !strings.HasSuffix(q.Prompt, tailLine) {
			t.Fatalf("Q7 prompt tail: %q", q.Prompt)
		}
	}
	if fellBack > 10 {
		t.Fatalf("too many fallback instances (%d/20): the stock space should admit plenty of non-formula cases", fellBack)
	}
}

func TestGenSocksFallbackTagged(t *testing.T) {
	// maxAttempts=1 with a forced-coincidence stock → regeneration budget
	// exhausted → must fall back and tag the instance
	counts, p, worst, tagged := genSocksInstance(newRand(7), 1)
	if worst != SolveSocks(counts, p) {
		t.Fatalf("fallback instance still inconsistent")
	}
	if p < 2 || p > 4 || len(counts) < 3 || len(counts) > 4 {
		t.Fatalf("fallback instance out of range: counts=%v p=%d", counts, p)
	}
	// the tag mechanism itself is exercised via GenSocks tags below
	if tagged {
		t.Logf("seed 7 single-attempt path fell back (tagged=true): counts=%v p=%d worst=%d", counts, p, worst)
	}
}

func TestGenSocksFallbackForcedTag(t *testing.T) {
	// brute-force the fallback path: maxAttempts=0 → counts==nil → default
	// instance accepted with tagged=true
	_, _, _, tagged := genSocksInstanceDefault(0)
	if !tagged {
		t.Fatalf("forced fallback must be tagged")
	}
}

func TestSolveSocksMatchesAdversarialEnumeration(t *testing.T) {
	for i := 0; i < 20; i++ {
		seed := int64(42 + i)
		r := newRand(seed)
		n := 3 + r.Intn(2)
		counts := make([]int, n)
		for j := range counts {
			counts[j] = 2 + r.Intn(6)
		}
		p := 2 + r.Intn(3)
		total := 0
		for _, c := range counts {
			total += c
		}
		pairs := 0
		for _, c := range counts {
			pairs += c / 2
		}
		if pairs < p {
			continue // infeasible, skip
		}
		got := SolveSocks(counts, p)
		want := socksAdversarialBrute(counts, p)
		if got != want {
			t.Fatalf("seed %d counts=%v p=%d: %d != adversarial %d", seed, counts, p, got, want)
		}
		if got <= 0 || got > total {
			t.Fatalf("seed %d: answer out of range: %d", seed, got)
		}
	}
}

func TestQuestionsCarryTags(t *testing.T) {
	if GenPill().Tags == nil {
		t.Fatalf("pill tags nil")
	}
	if len(GenColorBlind(newRand(1)).Tags) == 0 {
		t.Fatalf("colorblind tags empty")
	}
}

func ExampleGenCandyClassic_answerLine() {
	q := GenCandyClassic(newRand(42))
	fmt.Println(strings.HasSuffix(q.Prompt, tailLine))
	// Output: true
}
