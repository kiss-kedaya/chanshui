package iq

import (
	"math/rand"
	"testing"
)

func TestFormattedFinalAnswer(t *testing.T) {
	q := GenCandyClassic(rand.New(rand.NewSource(1)), EncodingURL)
	for _, s := range []string{"最终答案: `21`", "```text\n最终答案: 21\n```", "**最终答案：21**"} {
		e, _, _ := Judge(q, s)
		if e != 12 {
			t.Errorf("lost answer %q -> %d", s, e)
		}
	}
}

func TestVariantNoMagicTwentyFive(t *testing.T) {
	q := Question{ID: "Q8", MaxScore: 12, Answer: "29", Tags: []string{"库存:10,10,8:10,10,8"}}
	e, half, _ := Judge(q, "最终答案: 25")
	if e != 0 || half {
		t.Fatal("unrelated classic answer got half")
	}
}

func TestFinalTotalBeforeBreakdown(t *testing.T) {
	q := GenCandyClassic(rand.New(rand.NewSource(1)), EncodingURL)
	for _, text := range []string{"最终答案: 21（9 圆 + 12 星）", "最终答案: 21 颗，不是29颗"} {
		e, _, _ := Judge(q, text)
		if e != 12 {
			t.Errorf("total lost: %s", text)
		}
	}
}
