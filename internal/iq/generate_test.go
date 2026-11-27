package iq

import (
	"fmt"
	"strings"
	"testing"
)

const tailLine = "请在最后单独一行以“最终答案: ”开头给出短结论。"

func TestGenerateBatteryBalancedEncodingsAndSum100(t *testing.T) {
	for seed := int64(1); seed <= 25; seed++ {
		battery := GenerateBattery(newRand(seed))
		if len(battery) != 9 {
			t.Fatalf("seed %d: battery len = %d, want 9", seed, len(battery))
		}
		sum := 0
		encCounts := map[string]int{}
		for i, q := range battery {
			wantID := fmt.Sprintf("Q%d", i+1)
			if q.ID != wantID {
				t.Fatalf("seed %d: item %d ID = %q, want %q", seed, i, q.ID, wantID)
			}
			sum += q.MaxScore
			if !strings.HasSuffix(q.Prompt, tailLine) {
				t.Fatalf("seed %d %s: prompt missing answer tail: %q", seed, q.ID, q.Prompt)
			}
			decoded, err := DecodePrompt(q.Prompt)
			if err != nil {
				t.Fatalf("seed %d %s: DecodePrompt failed: %v\nprompt:\n%s", seed, q.ID, err, q.Prompt)
			}
			if !isPrintableASCII(decoded) || len(strings.TrimSpace(decoded)) < 30 {
				t.Fatalf("seed %d %s: decoded not valid ASCII (%d bytes): %q", seed, q.ID, len(decoded), decoded)
			}
			for _, tg := range q.Tags {
				if tg == "URL编码" || tg == "十六进制编码" || tg == "Base64编码" {
					encCounts[tg]++
				}
			}
		}
		if sum != 100 {
			t.Fatalf("seed %d: battery MaxScore sum = %d, want 100", seed, sum)
		}
		for _, enc := range []string{"URL编码", "十六进制编码", "Base64编码"} {
			if encCounts[enc] != 3 {
				t.Fatalf("seed %d: encoding %s count = %d, want 3 (all=%v)", seed, enc, encCounts[enc], encCounts)
			}
		}
	}
}

func TestEncodeRoundTripAllModes(t *testing.T) {
	setup := "2 identical ovens run in parallel; no job can be split."
	core := "Jobs (min): 9, 8, 7, 7, 6.\nMin total minutes?"
	for _, mode := range []Encoding{EncodingURL, EncodingHex, EncodingBase64} {
		prompt := FormatEncodedPrompt(setup, core, mode, tailLine)
		// Ensure raw digits from core do not leak unencoded inside encoded blocks.
		if strings.Contains(prompt, "9, 8, 7, 7, 6") {
			t.Fatalf("mode %s leaked plaintext digits: %s", mode, prompt)
		}
		decoded, err := DecodePrompt(prompt)
		if err != nil {
			t.Fatalf("mode %s DecodePrompt error: %v", mode, err)
		}
		norm := strings.Join(strings.Fields(decoded), " ")
		want := strings.Join(strings.Fields(setup+" "+core), " ")
		if norm != want {
			t.Fatalf("mode %s round-trip mismatch:\ngot:  %q\nwant: %q", mode, norm, want)
		}
	}
}

func TestGenTruthLiarUniqueAcrossSeeds(t *testing.T) {
	seen := map[string]int{}
	for seed := int64(42); seed < 72; seed++ {
		q := GenTruthLiar(newRand(seed))
		if q.ID != "Q1" || q.Title != "谎言排查" || q.MaxScore != 10 {
			t.Fatalf("Q1 meta: %+v", q)
		}
		if q.Answer < "1" || q.Answer > "5" || len(q.Answer) != 1 {
			t.Fatalf("Q1 invalid answer: %q", q.Answer)
		}
		decoded, err := DecodePrompt(q.Prompt)
		if err != nil {
			t.Fatalf("Q1 decode: %v", err)
		}
		if !strings.Contains(decoded, "suspects") || !strings.Contains(decoded, "thief") {
			t.Fatalf("Q1 decoded missing framing: %q", decoded)
		}
		seen[q.Answer]++
	}
	if len(seen) < 3 {
		t.Fatalf("Q1 culprits should vary across seeds: %v", seen)
	}
}

func TestGenScheduleStrictlyBeatsLowerBoundAndLPT(t *testing.T) {
	for seed := int64(42); seed < 72; seed++ {
		q := GenSchedule(newRand(seed))
		if q.ID != "Q2" || q.Title != "并行调度" || q.MaxScore != 10 {
			t.Fatalf("Q2 meta: %+v", q)
		}
		jobs, ok := scheduleParamsOf(q)
		if !ok {
			t.Fatalf("Q2 missing job params: %+v", q.Tags)
		}
		total := 0
		for _, d := range jobs {
			total += d
		}
		lb := (total + 1) / 2
		opt := SolveSchedule(jobs)
		lpt := lptSchedule(jobs)
		if fmt.Sprint(opt) != q.Answer {
			t.Fatalf("seed %d: SolveSchedule=%d != Answer=%q", seed, opt, q.Answer)
		}
		if opt <= lb || lpt <= opt {
			t.Fatalf("seed %d jobs=%v: want lb(%d) < opt(%d) < lpt(%d)", seed, jobs, lb, opt, lpt)
		}
	}
}

func TestGenCRTMatchesResiduesAndExceedsLowerBound(t *testing.T) {
	seen := map[string]int{}
	for seed := int64(42); seed < 72; seed++ {
		q := GenCRT(newRand(seed))
		if q.ID != "Q3" || q.Title != "余数锁定" || q.MaxScore != 10 {
			t.Fatalf("Q3 meta: %+v", q)
		}
		base, ok := crtBaseOf(q)
		if !ok {
			t.Fatalf("Q3 missing base tag: %+v", q.Tags)
		}
		ans, err := strconvAtoi(q.Answer)
		if err != nil || ans <= base {
			t.Fatalf("seed %d: ans=%q must exceed base=%d", seed, q.Answer, base)
		}
		decoded, err := DecodePrompt(q.Prompt)
		if err != nil {
			t.Fatalf("Q3 decode: %v", err)
		}
		if !strings.Contains(decoded, "mod") {
			t.Fatalf("Q3 decoded missing mod: %q", decoded)
		}
		seen[q.Answer]++
	}
	if len(seen) < 10 {
		t.Fatalf("Q3 answers should vary widely across seeds: %v", seen)
	}
}

func TestGenGeneticsVariants(t *testing.T) {
	seen := map[string]int{}
	for seed := int64(42); seed < 82; seed++ {
		q := GenGenetics(newRand(seed))
		if q.ID != "Q4" || q.Title != "伴X遗传" || q.MaxScore != 12 {
			t.Fatalf("Q4 meta: %+v", q)
		}
		gc, ok := geneticsCaseOf(q)
		if !ok || gc.answer != q.Answer {
			t.Fatalf("Q4 case mismatch: %+v", q)
		}
		if _, err := parseProb(q.Answer, false); err != nil {
			t.Fatalf("Q4 unparseable answer %q: %v", q.Answer, err)
		}
		decoded, err := DecodePrompt(q.Prompt)
		if err != nil {
			t.Fatalf("Q4 decode: %v", err)
		}
		if !strings.Contains(decoded, "X-linked") {
			t.Fatalf("Q4 decoded missing X-linked: %q", decoded)
		}
		seen[gc.instance]++
	}
	if len(seen) < 5 {
		t.Fatalf("Q4 should span both Bayesian and obligate-carrier cases: %v", seen)
	}
}

func TestGenForbidStepsStrictlyLengthensPath(t *testing.T) {
	if got := SolveForbidSteps(3, 47, nil); got != 5 {
		t.Fatalf("3->47 unconstrained = %d, want 5", got)
	}
	if got := SolveForbidSteps(3, 47, []int{24}); got != 7 {
		t.Fatalf("3->47 forbid 24 = %d, want 7", got)
	}
	for seed := int64(42); seed < 72; seed++ {
		q := GenForbidSteps(newRand(seed))
		if q.ID != "Q5" || q.Title != "禁区步数" || q.MaxScore != 10 {
			t.Fatalf("Q5 meta: %+v", q)
		}
		base, ok := forbidBaseDistOf(q)
		if !ok {
			t.Fatalf("Q5 missing base dist: %+v", q.Tags)
		}
		ans, err := strconvAtoi(q.Answer)
		if err != nil || ans <= base {
			t.Fatalf("seed %d: Q5 ans=%d must exceed unconstrained base=%d", seed, ans, base)
		}
	}
}

func TestGenCandyClassicFixed(t *testing.T) {
	q := GenCandyClassic(newRand(42), EncodingURL)
	if q.ID != "Q6" || q.MaxScore != 12 || q.LimitSec != 120 {
		t.Fatalf("Q6 meta: %+v", q)
	}
	if q.Answer != "21" {
		t.Fatalf("Q6 classic answer must be 21, got %q", q.Answer)
	}
	decoded, err := DecodePrompt(q.Prompt)
	if err != nil {
		t.Fatalf("Q6 decode: %v", err)
	}
	for _, want := range []string{"7 apple", "9 peach", "8 melon", "6 peach", "4 melon", "Circle", "Star"} {
		if !strings.Contains(decoded, want) {
			t.Fatalf("Q6 decoded missing %q: %q", want, decoded)
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
		if q.ID != "Q8" || q.MaxScore != 12 || q.LimitSec != 120 {
			t.Fatalf("Q8 meta: %+v", q)
		}
		if q.Answer == "" || q.Answer == "21" {
			t.Fatalf("Q8 invalid answer: %q", q.Answer)
		}
		decoded, err := DecodePrompt(q.Prompt)
		if err != nil {
			t.Fatalf("Q8 decode: %v", err)
		}
		if !strings.Contains(decoded, "Circle") || !strings.Contains(decoded, "Star") {
			t.Fatalf("Q8 decoded missing shapes: %q", decoded)
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
	if got := SolveSocks([]int{7, 7, 7, 7}, 2); got != 7 {
		t.Fatalf("SolveSocks(4x7, p=2) = %d, want 7 (2p+c-1)", got)
	}
	if got := SolveSocks([]int{2, 2, 2}, 2); got != 5 {
		t.Fatalf("SolveSocks(2,2,2 p=2) = %d, want 5", got)
	}
}

func TestGenSocksRejectsFormulaInstances(t *testing.T) {
	fellBack := 0
	for seed := int64(42); seed < 62; seed++ {
		q := GenSocks(newRand(seed))
		if q.ID != "Q7" || q.MaxScore != 12 || q.LimitSec != 120 {
			t.Fatalf("Q7 meta: %+v", q)
		}
		counts, p, worst, ok := socksParamsOf(q)
		if !ok {
			t.Fatalf("Q7 not carrying params: %q", q.Answer)
		}
		if got := SolveSocks(counts, p); got != worst {
			t.Fatalf("seed %d: SolveSocks mismatch %d vs %d", seed, got, worst)
		}
		tagged := false
		for _, tg := range q.Tags {
			if tg == "退化实例" {
				fellBack++
				tagged = true
			}
		}
		if worst == 2*p+len(counts)-1 && !tagged {
			t.Fatalf("seed %d: instance equals formula 2p+c-1 without fallback tag: %+v", seed, q)
		}
		decoded, err := DecodePrompt(q.Prompt)
		if err != nil {
			t.Fatalf("Q7 decode: %v", err)
		}
		if !strings.Contains(decoded, "same-color pairs") {
			t.Fatalf("Q7 decoded missing pairing goal: %q", decoded)
		}
		if !strings.HasSuffix(q.Prompt, tailLine) {
			t.Fatalf("Q7 prompt tail: %q", q.Prompt)
		}
	}
	if fellBack > 10 {
		t.Fatalf("too many fallback instances (%d/20)", fellBack)
	}
}

func TestGenSocksFallbackForcedTag(t *testing.T) {
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
		total, pairs := 0, 0
		for _, c := range counts {
			total += c
			pairs += c / 2
		}
		if pairs < p {
			continue
		}
		got := SolveSocks(counts, p)
		want := socksAdversarialBrute(counts, p)
		if got != want || got <= 0 || got > total {
			t.Fatalf("seed %d counts=%v p=%d: got=%d want=%d", seed, counts, p, got, want)
		}
	}
}

func ExampleGenCandyClassic_answerLine() {
	q := GenCandyClassic(newRand(42), EncodingURL)
	fmt.Println(strings.HasSuffix(q.Prompt, tailLine))
	// Output: true
}
