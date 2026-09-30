package iq

import (
	"reflect"
	"testing"
)

func item(id string, max, earned int, half bool, tags ...string) QItem {
	return QItem{ID: id, Title: id, Max: max, Earned: earned, Half: half, Tags: tags, LimitSec: 120}
}

// fiveBase builds the five real questions with their specced maxes:
// Q1=6, Q4=14, Q6=16, Q8=16, Q7=16 → 卷面 Σ = 68.
//
// NOTE on the plan's "卷面 58": 6+14+16+16+16 = 68, not 58. The 58 makes the
// two half/zero scenarios in the plan arithmetically unsatisfiable at the
// same time (socks-half numerator 52 needs Q7 max 6; Q6-zero numerator 52
// needs Q6 max 6; both with Σ=58 is impossible). Score therefore uses the
// honest sum ΣMax = 68; the tasks' 90 expectations become 76 under the same
// rule (52/68 → 76.47 → 76).
func fiveBase() []QItem {
	return []QItem{
		item("Q1", 6, 6, false),
		item("Q4", 14, 14, false),
		item("Q6", 16, 16, false),
		item("Q8", 16, 16, false),
		item("Q7", 16, 16, false),
	}
}

func TestScoreAllCorrect(t *testing.T) {
	res := Score(fiveBase())
	if res.Score != 100 {
		t.Fatalf("all-correct Score = %d, want 100", res.Score)
	}
	if res.Max != 68 {
		t.Fatalf("all-correct Max = %d, want 68", res.Max)
	}
	if len(res.PerQuestion) != 5 {
		t.Fatalf("PerQuestion len = %d, want 5", len(res.PerQuestion))
	}
}

func TestScoreSocksHalf(t *testing.T) {
	items := fiveBase()
	items[4] = item("Q7", 16, 8, true, "替代建模")
	res := Score(items)
	// numerator excludes half credit: 6+14+16+16 = 52; 52/68*100 = 76.47 → 76
	if res.Score != 76 {
		t.Fatalf("socks-half Score = %d, want 76 (52/68)", res.Score)
	}
	if res.Max != 68 {
		t.Fatalf("Max = %d, want 68", res.Max)
	}
}

func TestScoreQ6Zero(t *testing.T) {
	items := fiveBase()
	items[2] = item("Q6", 16, 0, false)
	res := Score(items)
	// passed-full = 6+14+16+16 = 52 (Q6 lost) → 52/68*100 = 76.47 → 76
	if res.Score != 76 {
		t.Fatalf("Q6-zero Score = %d, want 76", res.Score)
	}
}

func TestScoreHalfNeverInNumerator(t *testing.T) {
	// every question half except Q1: numerator = 6 → 6/68 = 8.8 → 9
	items := []QItem{
		item("Q1", 6, 6, false),
		item("Q4", 14, 7, true, "仅遗传链"),
		item("Q6", 16, 8, true, "替代建模"),
		item("Q8", 16, 8, true, "替代建模"),
		item("Q7", 16, 8, true, "替代建模"),
	}
	res := Score(items)
	if res.Score != 9 {
		t.Fatalf("half-only Score = %d, want 9 (6/68*100=8.82→9)", res.Score)
	}
}

func TestScoreAllZero(t *testing.T) {
	items := fiveBase()
	for i := range items {
		items[i].Earned = 0
	}
	res := Score(items)
	if res.Score != 0 || res.Max != 68 {
		t.Fatalf("all-zero Score/Max = %d/%d, want 0/68", res.Score, res.Max)
	}
}

func TestScoreTagsPropagate(t *testing.T) {
	items := fiveBase()
	items[0] = item("Q1", 6, 6, false, "安全判断")
	items[4] = item("Q7", 16, 8, true, "替代建模", "退化实例")
	res := Score(items)
	if !reflect.DeepEqual(res.PerQuestion[0].Tags, []string{"安全判断"}) {
		t.Fatalf("Q1 tags = %v", res.PerQuestion[0].Tags)
	}
	if !reflect.DeepEqual(res.PerQuestion[4].Tags, []string{"替代建模", "退化实例"}) {
		t.Fatalf("Q7 tags = %v", res.PerQuestion[4].Tags)
	}
	if !res.PerQuestion[4].Half {
		t.Fatalf("Q7 Half flag lost")
	}
	if res.PerQuestion[4].Earned != 8 || res.PerQuestion[4].Max != 16 {
		t.Fatalf("Q7 earned/max = %d/%d", res.PerQuestion[4].Earned, res.PerQuestion[4].Max)
	}
}

func TestScoreEmpty(t *testing.T) {
	res := Score(nil)
	if res.Score != 0 || res.Max != 0 {
		t.Fatalf("empty Score/Max = %d/%d, want 0/0", res.Score, res.Max)
	}
}

func TestScoreFromRealJudges(t *testing.T) {
	// integration: generate the five real questions, judge ideal answers,
	// score → must be 100; then make Q7 half → expect 76.
	var items []QItem
	for _, q := range []Question{
		GenPill(),
		GenColorBlind(newRand(42)),
		GenCandyClassic(newRand(42)),
		GenCandyVariant(newRand(42)),
		GenSocks(newRand(42)),
	} {
		full := "推理……\n最终答案: " + q.Answer
		earned, half, tags := Judge(q, full)
		items = append(items, QItem{ID: q.ID, Title: q.Title, Max: q.MaxScore, Earned: earned, Half: half, Tags: tags, LimitSec: q.LimitSec})
	}
	res := Score(items)
	if res.Score != 100 {
		t.Fatalf("ideal answers Score = %d, want 100 (%+v)", res.Score, res.PerQuestion)
	}
	items[4].Earned = 8
	items[4].Half = true
	items[4].Tags = []string{"替代建模"}
	if got := Score(items).Score; got != 76 {
		t.Fatalf("Q7-half Score = %d, want 76", got)
	}
}
