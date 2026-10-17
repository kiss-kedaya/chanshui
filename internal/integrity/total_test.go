package integrity

import "testing"

func full(key, label string, max int) Part {
	return Part{Key: key, Label: label, Applicable: true, Known: true, Points: max, MaxPoints: max}
}

func TestComposeFullClaudeWithCacheIs100(t *testing.T) {
	r := Compose([]Part{
		full("fingerprint", "模型指纹归因", 10),
		full("iq", "推理表现", 100),
		full("injection", "注入大小", 10),
		full("tools", "工具调用", 1),
		full("web", "联网检测", 1),
		full("thinking", "思维链回传", 10),
		full("cache", "缓存读写", 1),
	})
	if r.Score != 100 {
		t.Fatalf("score %d", r.Score)
	}
	got := map[string]int{}
	sum := 0
	for _, p := range r.Parts {
		got[p.Key] = p.Coefficient
		sum += p.Contribution
	}
	if sum != 100 || got["fingerprint"] != 33 || got["iq"] != 28 || got["injection"] != 13 || got["tools"] != 9 || got["web"] != 7 || got["thinking"] != 6 || got["cache"] != 4 {
		t.Fatalf("coeffs %+v sum %d", got, sum)
	}
}

func TestComposeDropsOptionalSectionsAndStillTotals100(t *testing.T) {
	r := Compose([]Part{
		full("fingerprint", "模型指纹归因", 10),
		full("iq", "推理表现", 100),
		full("injection", "注入大小", 10),
		full("tools", "工具调用", 1),
		full("web", "联网检测", 1),
		{Key: "thinking", Label: "思维链回传", Applicable: false, MaxPoints: 10},
		{Key: "cache", Label: "缓存读写", Applicable: false, MaxPoints: 1},
	})
	if r.Score != 100 {
		t.Fatalf("score %d parts %+v", r.Score, r.Parts)
	}
	sum := 0
	fp := 0
	for _, p := range r.Parts {
		if p.Applicable {
			sum += p.Coefficient
			if p.Key == "fingerprint" {
				fp = p.Coefficient
			}
		}
	}
	if sum != 100 || fp < 30 {
		t.Fatalf("renorm sum %d fingerprint %d", sum, fp)
	}
}

func TestComposeFingerprintMissAndPartialInjection(t *testing.T) {
	r := Compose([]Part{
		{Key: "fingerprint", Label: "模型指纹归因", Applicable: true, Known: true, Points: 0, MaxPoints: 10},
		full("iq", "推理表现", 100),
		{Key: "injection", Label: "注入大小", Applicable: true, Known: true, Points: 8, MaxPoints: 10},
		full("tools", "工具调用", 1),
		full("web", "联网检测", 1),
		full("thinking", "思维链回传", 10),
		full("cache", "缓存读写", 1),
	})
	var fp, inj int
	for _, p := range r.Parts {
		if p.Key == "fingerprint" {
			fp = p.Contribution
		}
		if p.Key == "injection" {
			inj = p.Contribution
		}
	}
	if fp != 0 || inj <= 0 || inj >= 13 || r.Score >= 100 {
		t.Fatalf("score %d fp %d inj %d", r.Score, fp, inj)
	}
}
