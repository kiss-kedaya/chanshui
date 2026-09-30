package iq

import "math"

// QItem is one judged question line: what was asked, what it was worth, what
// was earned, whether the earn was half-credit, and the judge tags.
type QItem struct {
	ID       string
	Title    string
	Max      int
	Earned   int
	Half     bool
	Tags     []string
	LimitSec int
}

// IQResult is the 0-100 IQ verdict for one detection run.
type IQResult struct {
	Score       int
	Max         int
	PerQuestion []QItem
}

// Score aggregates per-question items into the 0-100 IQ score.
// 卷面 Max = Σ MaxScore of ALL questions (including failed ones); 半分只进
// 标记不进分子: the numerator sums Earned only where !Half.
//
// NOTE: with the specced maxes Q1=6 Q4=14 Q6=16 Q8=16 Q7=16 the 卷面 is 68,
// not the 58 written in the plan (6+14+16+16+16 = 68; 58 also makes the
// plan's own half/zero scenarios mutually unsatisfiable). See score_test.go.
func Score(items []QItem) IQResult {
	max := 0
	passedFull := 0
	for _, it := range items {
		max += it.Max
		if !it.Half {
			passedFull += it.Earned
		}
	}
	score := 0
	if max > 0 {
		score = int(math.Round(float64(passedFull) / float64(max) * 100))
	}
	return IQResult{Score: score, Max: max, PerQuestion: append([]QItem(nil), items...)}
}
