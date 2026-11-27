package iq

import (
	"fmt"
	"strings"
	"testing"
)

func TestGenBridgeParameterized(t *testing.T) {
	seen := map[string]int{}
	for seed := int64(1); seed <= 30; seed++ {
		q := GenBridge(newRand(seed))
		if q.ID != "Q9" || q.Title != "夜间过桥" || q.MaxScore != 12 || q.LimitSec != 75 {
			t.Fatalf("Q9 meta: %+v", q)
		}
		times, ok := bridgeTimesOf(q)
		if !ok {
			t.Fatalf("Q9 missing times tag: %+v", q.Tags)
		}
		if times[0]+times[1]+times[2] > times[3] {
			t.Fatalf("Q9 invalid bridge spec %v: a+b+c > d", times)
		}
		if q.Answer != fmt.Sprint(times[3]) {
			t.Fatalf("Q9 answer = %q, want %d", q.Answer, times[3])
		}
		decoded, err := DecodePrompt(q.Prompt)
		if err != nil {
			t.Fatalf("Q9 decode: %v", err)
		}
		flat := strings.Join(strings.Fields(decoded), " ")
		if !strings.Contains(flat, "fixed stand") {
			t.Fatalf("Q9 decoded missing fixed stand: %q", flat)
		}
		seen[q.Answer]++
	}
	if len(seen) < 3 {
		t.Fatalf("Q9 speeds should vary across seeds: %v", seen)
	}
}

func TestJudgeBridge(t *testing.T) {
	q := GenBridge(nil, EncodingURL) // default {1,2,7,10} -> ans=10, trap=17
	cases := []struct {
		text string
		want int
		half bool
		tag  string
	}{
		{"10", 12, false, ""},
		{"10分钟", 12, false, ""},
		{"最终答案: 10", 12, false, ""},
		{"**10**", 12, false, ""},
		{"17", 0, false, "忽略定向支架"},
		{"最终答案: 17", 0, false, "忽略定向支架"},
		{"13", 0, false, ""},
		{"", 0, false, ""},
	}
	for _, c := range cases {
		earned, half, tags := Judge(q, c.text)
		if earned != c.want || half != c.half {
			t.Fatalf("%q -> (%d,%v), want (%d,%v)", c.text, earned, half, c.want, c.half)
		}
		if c.tag == "" {
			continue
		}
		found := false
		for _, tg := range tags {
			if tg == c.tag {
				found = true
			}
		}
		if !found {
			t.Fatalf("%q tags = %v, want %q", c.text, tags, c.tag)
		}
	}
	// Verify carry-back trap adapts to randomized speed tuple.
	q2 := GenBridge(newRand(7))
	times, _ := bridgeTimesOf(q2)
	trap := bridgeCarryBackAnswer(times)
	earned, _, tags := Judge(q2, fmt.Sprintf("最终答案: %d", trap))
	if earned != 0 || len(tags) == 0 || tags[0] != "忽略定向支架" {
		t.Fatalf("random bridge trap %d -> (%d,%v)", trap, earned, tags)
	}
}
