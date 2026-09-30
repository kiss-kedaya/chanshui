package fingerprint

import (
	"math"
	"math/rand"
	"strings"
	"testing"
)

func bankForTest(t *testing.T) *Bank {
	t.Helper()
	b, err := LoadBank()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sampleFromCounts(r *rand.Rand, counts []int, n int) []int {
	total := 0
	for _, c := range counts {
		total += c
	}
	pick := func() int {
		x := r.Intn(total)
		for i, c := range counts {
			if x < c {
				return i + NumMinValue
			}
			x -= c
		}
		return NumMinValue
	}
	out := make([]int, n)
	for i := range out {
		out[i] = pick()
	}
	return out
}

func numbersToText(nums []int) string {
	parts := make([]string, len(nums))
	for i, v := range nums {
		parts[i] = itoa(v)
	}
	return strings.Join(parts, ", ")
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [24]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func TestAttributionSelfConsistency(t *testing.T) {
	b := bankForTest(t)
	rng := rand.New(rand.NewSource(42))
	picks := []int{0, 8, 15} // one gpt, one claude haiku, one claude opus
	for _, mi := range picks {
		m := b.Numeric.Models[mi]
		var outputs []Output
		for k := 0; k < 3; k++ {
			nums := sampleFromCounts(rng, m.Counts, 260)
			outputs = append(outputs, Output{Text: numbersToProbeText(nums), ExpectedCount: 260})
		}
		attr := Attribute(outputs, b)
		if len(attr.Results) != len(b.Numeric.Models) {
			t.Fatalf("%s: results %d", m.ID, len(attr.Results))
		}
		if attr.Family != m.Family {
			t.Errorf("%s: family got %s want %s", m.ID, attr.Family, m.Family)
		}
		if attr.TopModel != m.ID {
			t.Errorf("%s: top1 got %s (p=%.3f) lowConf=%v", m.ID, attr.TopModel, attr.TopProb, attr.LowConfidence)
		}
		if math.Abs(attr.Results[0].Prob-attr.TopProb) > 1e-9 {
			t.Errorf("%s: top prob mismatch %v vs %v", m.ID, attr.Results[0].Prob, attr.TopProb)
		}
		if attr.UsedOutputs != 3 {
			t.Errorf("%s: used outputs %d", m.ID, attr.UsedOutputs)
		}
		if attr.LowConfidence {
			t.Errorf("%s: unexpected low confidence (top prob %.3f)", m.ID, attr.TopProb)
		}
	}
}

func TestAttributionValidityGate(t *testing.T) {
	b := bankForTest(t)
	// expected 260 → threshold max(80, ceil(143)) = 143; 100 valid numbers is rejected.
	nums := sampleFromCounts(rand.New(rand.NewSource(1)), b.Numeric.Models[0].Counts, 100)
	attr := Attribute([]Output{{Text: numbersToProbeText(nums), ExpectedCount: 260}}, b)
	if attr.UsedOutputs != 0 || len(attr.Results) != 0 {
		t.Fatalf("expected invalid outputs to be rejected, got used=%d results=%d", attr.UsedOutputs, len(attr.Results))
	}
}

func TestAttributionLowConfidenceFlag(t *testing.T) {
	b := bankForTest(t)
	// 1 output → calibration key "1", beta 7.28; should still usually resolve, but
	// the flag must exist and track top1 prob < 0.6.
	nums := sampleFromCounts(rand.New(rand.NewSource(7)), b.Numeric.Models[3].Counts, 300)
	attr := Attribute([]Output{{Text: numbersToProbeText(nums), ExpectedCount: 300}}, b)
	if attr.TopModel == "" {
		t.Fatal("no top model")
	}
	if attr.LowConfidence != (attr.TopProb < 0.6) {
		t.Fatalf("LowConfidence %v but topProb %.3f", attr.LowConfidence, attr.TopProb)
	}
}

func TestGenerateChallenges(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	chs := GenerateChallenges(3, rng)
	if len(chs) != 3 {
		t.Fatalf("challenges: %d", len(chs))
	}
	seen := map[string]bool{}
	for _, c := range chs {
		if c.ExpectedCount != 300 {
			t.Fatalf("challenge expected count: %d", c.ExpectedCount)
		}
		if l := len([]rune(c.Prompt)); l < 292 || l > 333 {
			t.Fatalf("challenge length %d out of [292,333]: %q", l, c.Prompt)
		}
		if !strings.Contains(c.Prompt, "1 到 355") {
			t.Fatalf("challenge missing range text: %q", c.Prompt)
		}
		if !strings.Contains(c.Prompt, "禁止") {
			t.Fatalf("challenge missing tool ban: %q", c.Prompt)
		}
		if c.ID == "" {
			t.Fatal("challenge id empty")
		}
		seen[c.Prompt] = true
	}
	if len(seen) < 2 {
		t.Fatalf("challenges are all identical: %d distinct of 3", len(seen))
	}
}

// numbersToProbeText wraps numbers in a probe-like instruction wrapper so the
// attribution path exercises real prompt text.
func numbersToProbeText(nums []int) string {
	return "输出数字序列：" + numbersToText(nums) + "。"
}
