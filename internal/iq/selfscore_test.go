package iq

import "testing"

func TestParseSelfScoreClamps(t *testing.T) {
	n, ok := ParseSelfScore("16", 16)
	if !ok || n != 16 {
		t.Fatalf("%d %v", n, ok)
	}
	n, ok = ParseSelfScore("应给 14 分", 16)
	if !ok || n != 14 {
		t.Fatalf("%d %v", n, ok)
	}
	n, ok = ParseSelfScore("99", 16)
	if !ok || n != 16 {
		t.Fatalf("clamp %d", n)
	}
	n, ok = ParseSelfScore("不知道", 16)
	if ok || n != 0 {
		t.Fatalf("missing %d %v", n, ok)
	}
}

func TestParseSelfScoreIgnoresLeadingRange(t *testing.T) {
	n, ok := ParseSelfScore("first 0 then later\n16", 16)
	if !ok || n != 16 {
		t.Fatalf("trailing %d %v", n, ok)
	}
	n, ok = ParseSelfScore("only a 0 to 16 range", 16)
	if ok {
		t.Fatalf("range echo scored %d", n)
	}
	n, ok = ParseSelfScore("0", 16)
	if !ok || n != 0 {
		t.Fatalf("explicit zero %d %v", n, ok)
	}
}
