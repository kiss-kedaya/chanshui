package api

import (
	"testing"

	"github.com/kiss-kedaya/chanshui/internal/integrity"
)

// checks builds the JSON shape sanitize() hands back: a list of maps.
func checks(pass, fail, skip int) []any {
	out := []any{}
	for i := 0; i < pass; i++ {
		out = append(out, map[string]any{"name": "p", "pass": true})
	}
	for i := 0; i < fail; i++ {
		out = append(out, map[string]any{"name": "f", "pass": false})
	}
	for i := 0; i < skip; i++ {
		out = append(out, map[string]any{"name": "s", "pass": false, "skipped": true})
	}
	return out
}

func claudeVerdict() Verdict {
	return Verdict{
		Fingerprint: map[string]any{"family": "claude", "top_model": "claude-opus-5-5", "used_outputs": 3},
		IQ:          map[string]any{"complete": true, "score": 100},
		Injection:   map[string]any{"score": 10},
		Tools:       map[string]any{"pass": true},
		Web:         map[string]any{"status": "pass", "pass": true, "protocol": "messages"},
		Thinking:    map[string]any{"score": 10, "max_score": 10, "strictness": checks(10, 0, 0)},
	}
}

func findPart(parts []integrity.Part, key string) (integrity.Part, bool) {
	for _, p := range parts {
		if p.Key == key {
			return p, true
		}
	}
	return integrity.Part{}, false
}

func TestChannelSectionsFullPassStill100(t *testing.T) {
	v := claudeVerdict()
	v.ClaudeDeep = map[string]any{"checks": checks(6, 0, 0)}
	v.ForceCache = map[string]any{"forced": false, "tier_5m": true, "tier_1h": true}
	total := buildTotal(v, "claude-opus-5-5")
	if total.Score != 100 {
		t.Fatalf("score %d %+v", total.Score, total.Parts)
	}
	sum := 0
	for _, key := range []string{"strictness", "deep", "forcecache"} {
		part, ok := findPart(total.Parts, key)
		if !ok || !part.Applicable || part.Coefficient == 0 {
			t.Fatalf("%s missing or unweighted: %+v", key, part)
		}
		if part.Contribution != part.Coefficient {
			t.Fatalf("%s full pass lost points: %+v", key, part)
		}
	}
	for _, part := range total.Parts {
		sum += part.Coefficient
	}
	if sum != 100 {
		t.Fatalf("coefficients sum %d", sum)
	}
}

func TestStrictnessCountsOnlyAnswersAndCostsPoints(t *testing.T) {
	v := claudeVerdict()
	baseline := buildTotal(v, "claude-opus-5-5").Score
	v.Thinking = map[string]any{"score": 10, "max_score": 10, "strictness": checks(2, 5, 3)}
	total := buildTotal(v, "claude-opus-5-5")
	part, ok := findPart(total.Parts, "strictness")
	if !ok || part.Points != 2 || part.MaxPoints != 7 {
		t.Fatalf("strictness part %+v", part)
	}
	if total.Score >= baseline {
		t.Fatalf("a lenient channel must score lower: %d vs %d", total.Score, baseline)
	}
}

func TestSkippedEverywhereLeavesTheSectionOut(t *testing.T) {
	plain := claudeVerdict()
	plain.Thinking = map[string]any{"score": 10, "max_score": 10}
	want := buildTotal(plain, "claude-opus-5-5")
	v := claudeVerdict()
	v.Thinking = map[string]any{"score": 10, "max_score": 10, "strictness": checks(0, 0, 10)}
	v.ClaudeDeep = map[string]any{"checks": checks(0, 0, 6)}
	v.ForceCache = map[string]any{"conclusion": "HTTP 503，样本不定"}
	got := buildTotal(v, "claude-opus-5-5")
	for _, key := range []string{"strictness", "deep", "forcecache"} {
		if _, ok := findPart(got.Parts, key); ok {
			t.Fatalf("%s should be left out when nothing could be measured", key)
		}
	}
	if got.Score != want.Score || len(got.Parts) != len(want.Parts) {
		t.Fatalf("an outage changed the score: %d vs %d", got.Score, want.Score)
	}
}

func TestDeepAndForcedCacheNeedOptIn(t *testing.T) {
	total := buildTotal(claudeVerdict(), "claude-opus-5-5")
	for _, key := range []string{"deep", "forcecache"} {
		if _, ok := findPart(total.Parts, key); ok {
			t.Fatalf("%s scored without being run", key)
		}
	}
	if _, ok := findPart(total.Parts, "strictness"); !ok {
		t.Fatal("strictness always rides along with the thinking probe")
	}
}

func TestForceCachePoints(t *testing.T) {
	cases := []struct {
		name       string
		in         map[string]any
		points     int
		max        int
		measurable bool
	}{
		{"forced", map[string]any{"forced": true}, 0, 3, true},
		{"passthrough both tiers", map[string]any{"forced": false, "tier_5m": true, "tier_1h": true}, 3, 3, true},
		{"passthrough five minutes only", map[string]any{"forced": false, "tier_5m": true, "tier_1h": false}, 2, 3, true},
		{"passthrough no tiers", map[string]any{"forced": false, "tier_5m": false, "tier_1h": false}, 1, 3, true},
		{"no verdict", map[string]any{"conclusion": "样本不定"}, 0, 0, false},
	}
	for _, tc := range cases {
		points, max, ok := forceCachePoints(tc.in)
		if points != tc.points || max != tc.max || ok != tc.measurable {
			t.Fatalf("%s: got %d/%d %v", tc.name, points, max, ok)
		}
	}
	if _, _, ok := forceCachePoints(nil); ok {
		t.Fatal("nil must not score")
	}
}

func TestChannelSectionsAreClaudeOnly(t *testing.T) {
	v := claudeVerdict()
	v.ClaudeDeep = map[string]any{"checks": checks(6, 0, 0)}
	v.ForceCache = map[string]any{"forced": true}
	for _, part := range buildTotal(v, "gpt-6-astra").Parts {
		switch part.Key {
		case "strictness", "deep", "forcecache":
			t.Fatalf("%s applied to a GPT model", part.Key)
		}
	}
}

func TestSelectedSectionsDecideWhetherStrictnessCounts(t *testing.T) {
	v := claudeVerdict()
	v.ClaudeDeep = map[string]any{"checks": checks(5, 1, 0)}
	total := buildTotalFor(v, "claude-opus-5-5", map[string]bool{"iq": true})
	if _, ok := findPart(total.Parts, "strictness"); ok {
		t.Fatal("strictness belongs to the thinking section, which was not selected")
	}
	part, ok := findPart(total.Parts, "deep")
	if !ok || part.Points != 5 || part.MaxPoints != 6 {
		t.Fatalf("the opt-in deep audit scores whatever sections were chosen: %+v", part)
	}
}

// The fingerprint is the one section that can prove a substitution, so a
// confirmed swap must not read as a decent score even when every other section
// passes. This guards the weight, not a cap: the scale stays plain percent.
func TestConfirmedSwapCannotLookDecent(t *testing.T) {
	v := claudeVerdict()
	v.Fingerprint = map[string]any{"basis": "numeric", "top_prob": 1.0, "family": "claude", "top_model": "claude-haiku-4-5", "used_outputs": 3}
	v.Vision = map[string]any{"score": 100}
	if got := buildTotal(v, "claude-opus-5-5").Score; got > 75 {
		t.Fatalf("swapped Claude run with everything else perfect scored %d, want at most 75", got)
	}
	g := Verdict{
		Fingerprint: map[string]any{"basis": "numeric", "top_prob": 1.0, "family": "gpt", "top_model": "gpt-6-astra", "used_outputs": 3},
		IQ:          map[string]any{"complete": true, "score": 100},
		Vision:      map[string]any{"score": 100},
		Injection:   map[string]any{"score": 10},
		Tools:       map[string]any{"pass": true},
		Web:         map[string]any{"status": "pass", "pass": true, "protocol": "messages"},
	}
	if got := buildTotal(g, "gpt-6.1-sol").Score; got > 70 {
		t.Fatalf("swapped GPT run with everything else perfect scored %d, want at most 70", got)
	}
	// And a clean run is still a full 100: nothing was capped or shifted.
	g.Fingerprint = map[string]any{"basis": "numeric", "top_prob": 1.0, "family": "gpt", "top_model": "gpt-6.1-sol", "used_outputs": 3}
	if got := buildTotal(g, "gpt-6.1-sol").Score; got != 100 {
		t.Fatalf("clean GPT run scored %d, want 100", got)
	}
}
