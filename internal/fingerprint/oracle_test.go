package fingerprint

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

func TestPythonReferenceOracle(t *testing.T) {
	b, e := os.ReadFile("testdata/reference-oracle.json")
	if e != nil {
		t.Fatal(e)
	}
	var data struct {
		Cases []struct {
			Numbers         []int
			Feature, Scores []float64
		}
	}
	if e = json.Unmarshal(b, &data); e != nil {
		t.Fatal(e)
	}
	bank, e := LoadBank()
	if e != nil {
		t.Fatal(e)
	}
	for k, c := range data.Cases {
		for name, pair := range map[string][2][]float64{"feature": {OrderedBlockFeature(c.Numbers), c.Feature}, "score": {RobustScoreNumbers(c.Numbers, bank), c.Scores}} {
			for i, v := range pair[0] {
				if math.Abs(v-pair[1][i]) > 1e-9 {
					t.Errorf("case %d %s[%d] got %.12f want %.12f", k, name, i, v, pair[1][i])
				}
			}
		}
	}
}
