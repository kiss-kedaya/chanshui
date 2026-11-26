package integrity

import "testing"

func full(key, label string, max int) Part {
	return Part{Key: key, Label: label, Applicable: true, Known: true, Points: max, MaxPoints: max}
}

func TestComposeFullClaudeWithCacheIs100(t *testing.T) {
	r := Compose([]Part{
		full("fingerprint", "模型指纹归因", 10),
		full("iq", "推理表现", 100),
		full("vision", "识图能力", 100),
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
	if sum != 100 || got["fingerprint"] != 31 || got["iq"] != 20 || got["vision"] != 13 || got["injection"] != 11 || got["tools"] != 8 || got["web"] != 7 || got["thinking"] != 6 || got["cache"] != 4 {
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
	if sum != 100 || fp != 40 {
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
	if fp != 0 || inj <= 0 || inj >= 22 || r.Score >= 100 {
		t.Fatalf("score %d fp %d inj %d", r.Score, fp, inj)
	}
}

func TestComposeClaudeChannelSectionsShareTheHundred(t *testing.T) {
	r := Compose([]Part{
		full("fingerprint", "模型指纹归因", 10),
		full("iq", "推理表现", 100),
		full("vision", "识图能力", 100),
		full("injection", "注入大小", 10),
		full("tools", "工具调用", 1),
		full("web", "联网检测", 1),
		full("thinking", "思维链回传", 12),
		full("cache", "缓存读写", 1),
		full("strictness", "协议校验", 10),
		full("deep", "深度体检", 6),
		full("forcecache", "强制缓存", 3),
	})
	if r.Score != 100 {
		t.Fatalf("score %d", r.Score)
	}
	got := map[string]int{}
	sum := 0
	for _, p := range r.Parts {
		got[p.Key] = p.Coefficient
		sum += p.Coefficient
	}
	if sum != 100 {
		t.Fatalf("coefficients sum %d: %+v", sum, got)
	}
	// 8 + 10 + 5 of 123 raw weight: deep outweighs strictness, which outweighs forced cache.
	if !(got["deep"] > got["strictness"] && got["strictness"] > got["forcecache"] && got["forcecache"] > 0) {
		t.Fatalf("channel coefficients out of order: %+v", got)
	}
	// Adding the channel sections must take weight from the old ones, not add to 100.
	if got["iq"] >= 20 || got["fingerprint"] >= 31 {
		t.Fatalf("old sections did not shrink: %+v", got)
	}
}

func TestComposeLenientChannelCostsOnlyItsOwnShare(t *testing.T) {
	r := Compose([]Part{
		full("fingerprint", "模型指纹归因", 10),
		full("iq", "推理表现", 100),
		full("injection", "注入大小", 10),
		full("tools", "工具调用", 1),
		full("web", "联网检测", 1),
		{Key: "strictness", Label: "协议校验", Applicable: true, Known: true, Points: 2, MaxPoints: 10},
	})
	var strict Part
	for _, p := range r.Parts {
		if p.Key == "strictness" {
			strict = p
		}
	}
	lost := strict.Coefficient - strict.Contribution
	if lost <= 0 || r.Score != 100-lost {
		t.Fatalf("score %d should be 100 minus the strictness shortfall %d: %+v", r.Score, lost, strict)
	}
}
