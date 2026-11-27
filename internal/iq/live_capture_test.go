package iq

import "testing"

func TestFullBatteryIdealAnswersScore100(t *testing.T) {
	battery := GenerateBattery(newRand(2026))
	var items []QItem
	for _, q := range battery {
		earned, half, tags := Judge(q, "心算解码并推理完毕。\n最终答案: "+q.Answer)
		if earned != q.MaxScore || half {
			t.Fatalf("%s (%s) ideal answer %q -> earned=%d/%d half=%v tags=%v", q.ID, q.Title, q.Answer, earned, q.MaxScore, half, tags)
		}
		items = append(items, QItem{ID: q.ID, Title: q.Title, Max: q.MaxScore, Earned: earned, Half: half, Tags: tags, LimitSec: q.LimitSec})
	}
	res := Score(items)
	if res.Score != 100 || res.Max != 100 {
		t.Fatalf("full battery ideal score = %d/%d, want 100/100", res.Score, res.Max)
	}
}
