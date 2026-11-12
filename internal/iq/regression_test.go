package iq

import (
	"math/rand"
	"testing"
)

func TestFormattedFinalAnswer(t *testing.T) {
	q := GenCandyClassic(rand.New(rand.NewSource(1)))
	for _, s := range []string{"最终答案: `21`", "```text\n最终答案: 21\n```", "**最终答案：21**"} {
		e, _, _ := Judge(q, s)
		if e != 16 {
			t.Errorf("lost answer %q -> %d", s, e)
		}
	}
}
func TestNoPositiveFalseMatch(t *testing.T) {
	q := GenColorBlind(rand.New(rand.NewSource(1)))
	for _, s := range []string{"最终答案：不能排除亲生的可能性", "最终答案：不一定是非亲生", "最终答案：可能是亲生"} {
		e, _, _ := Judge(q, s)
		if e == 14 {
			t.Errorf("false full: %s", s)
		}
	}
	q = GenPill()
	e, _, _ := Judge(q, "最终答案：虽然已经睡着，但仍应喂药。")
	if e != 0 {
		t.Fatal("unsafe contradiction scored")
	}
}
func TestVariantNoMagicTwentyFive(t *testing.T) {
	q := Question{ID: "Q8", Answer: "29", Tags: []string{"库存:10,10,8:10,10,8"}}
	e, half, _ := Judge(q, "最终答案: 25")
	if e != 0 || half {
		t.Fatal("unrelated classic answer got half")
	}
}

func TestPillNaturalLanguageNegation(t *testing.T) {
	for _, answer := range []string{"最终答案: 暂缓发放、报告医生请示，不得擅自更改医嘱，也无必要叫醒病人强行喂药。", "最终答案: 不必叫醒病人，暂缓发药并记录。", "最终答案: 不发药，不要把病人叫醒。"} {
		earned, _, _ := Judge(GenPill(), answer)
		if earned != 6 {
			t.Errorf("natural correct answer rejected: %s", answer)
		}
	}
}

func TestFinalTotalBeforeBreakdown(t *testing.T) {
	q := GenCandyClassic(rand.New(rand.NewSource(1)))
	for _, text := range []string{"最终答案: 21（9 圆 + 12 星）", "最终答案: 21 颗，不是29颗"} {
		e, _, _ := Judge(q, text)
		if e != 16 {
			t.Errorf("total lost: %s", text)
		}
	}
}
