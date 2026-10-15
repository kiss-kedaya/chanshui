package fingerprint

import (
	"encoding/json"
	"os"
	"testing"
)

// TestSolAndFableHeldOutAgainstJointBank enumerates every holdout triple rather
// than the four fixed groups a single 3-probe live run happens to draw. The
// live run only ever samples one triple, so a group-based check hides exactly
// the failure a user would see: gpt-6.1-sol and gpt-6-astra sit close enough on
// this endpoint that some triples cross over. The thresholds below are set from
// measured behaviour to keep that visible instead of passing silently.
func TestSolAndFableHeldOutAgainstJointBank(t *testing.T) {
	bank, err := LoadBank()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		model, alias, family string
		wantSamples          int
		minHitRate           float64
	}{
		{"gpt-6.1-sol", "openai/gpt-6.1-sol", "gpt", 72, 0.95},
		{"claude-fable-5", "anthropic/claude-fable-5", "claude", 36, 1.00},
	} {
		t.Run(tc.model, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/" + tc.model + "-holdout.json")
			if err != nil {
				t.Fatal(err)
			}
			var outputs []Output
			if err := json.Unmarshal(raw, &outputs); err != nil {
				t.Fatal(err)
			}
			ref := bank.NumericModel(tc.alias)
			if len(outputs) != 12 || ref == nil || ref.ID != tc.model || ref.ReferenceSamples != tc.wantSamples || ref.Family != tc.family {
				t.Fatal("incomplete reference or alias")
			}
			misattributed := map[string]int{}
			hits, total := 0, 0
			for i := 0; i < len(outputs); i++ {
				for j := i + 1; j < len(outputs); j++ {
					for k := j + 1; k < len(outputs); k++ {
						a := Attribute([]Output{outputs[i], outputs[j], outputs[k]}, bank)
						total++
						if a.TopModel == tc.model {
							hits++
						} else {
							misattributed[a.TopModel]++
						}
					}
				}
			}
			rate := float64(hits) / float64(total)
			if rate < tc.minHitRate {
				t.Fatalf("exhaustive triple hit rate %.3f below %.2f (%d/%d); misattributed to %v", rate, tc.minHitRate, hits, total, misattributed)
			}
			t.Logf("%s: %d/%d triples = %.1f%%; misattributed to %v", tc.model, hits, total, rate*100, misattributed)
		})
	}
}
