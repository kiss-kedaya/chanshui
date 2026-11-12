package iq

import (
	"strings"
	"testing"
)

func TestGenBridge(t *testing.T) {
	q := GenBridge()
	if q.ID != "Q9" || q.Title != "夜间过桥" || q.MaxScore != 8 || q.LimitSec != 75 {
		t.Fatalf("Q9 meta: %+v", q)
	}
	if q.Answer != "10" {
		t.Fatalf("Q9 answer = %q, want 10", q.Answer)
	}
	for _, want := range []string{"1、2、7、10", "定向支架", "光束足以照亮全桥", "只回答一个整数"} {
		if !strings.Contains(q.Prompt, want) {
			t.Fatalf("Q9 prompt missing %q: %q", want, q.Prompt)
		}
	}
}

func TestJudgeBridge(t *testing.T) {
	cases := []struct {
		text string
		want int
		half bool
		tag  string
	}{
		{"10", 8, false, ""},
		{"10分钟", 8, false, ""},
		{"最终答案: 10", 8, false, ""},
		{"**10**", 8, false, ""},
		{"17", 0, false, "忽略定向支架"},
		{"最终答案: 17", 0, false, "忽略定向支架"},
		{"13", 0, false, ""},
		{"", 0, false, ""},
	}
	for _, c := range cases {
		earned, half, tags := Judge(GenBridge(), c.text)
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
}
