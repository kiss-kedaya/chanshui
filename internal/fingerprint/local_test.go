package fingerprint

import (
	"encoding/json"
	"os"
	"testing"
)

func TestLocalReferenceValidationAndIdempotence(t *testing.T) {
	var b Bank
	if err := json.Unmarshal(bankGenerated, &b); err != nil {
		t.Fatal(err)
	}
	n := len(b.Numeric.Models)
	ref := LocalReference{ID: "fixture-local", Aliases: []string{"fixture-alias"}, Family: "deepseek", FamilyName: "DeepSeek", Source: "https://fixture.invalid"}
	nums := make([]int, 300)
	for i := range nums {
		nums[i] = 1 + (i*71)%355
	}
	for i := 0; i < 36; i++ {
		ref.Samples = append(ref.Samples, nums)
	}
	if err := AddLocalReference(&b, ref); err != nil {
		t.Fatal(err)
	}
	if err := AddLocalReference(&b, ref); err != nil {
		t.Fatal(err)
	}
	if len(b.Numeric.Models) != n+1 || b.NumericModel("fixture-alias").ID != "fixture-local" {
		t.Fatal("alias duplicate or missing")
	}
	ref.Samples[0] = []int{999}
	if err := AddLocalReference(&b, ref); err == nil {
		t.Fatal("invalid sample accepted")
	}
}

func TestDeepSeekHeldOutTriples(t *testing.T) {
	raw, err := os.ReadFile("testdata/deepseek-flash-holdout.json")
	if err != nil {
		t.Fatal(err)
	}
	var outputs []Output
	if err = json.Unmarshal(raw, &outputs); err != nil {
		t.Fatal(err)
	}
	if len(outputs) != 12 {
		t.Fatalf("holdout size %d", len(outputs))
	}
	bank, err := LoadBank()
	if err != nil {
		t.Fatal(err)
	}
	ref := bank.NumericModel("deepseek-flash")
	if ref == nil || ref.ID != "deepseek-v4.1-flash" || ref.ReferenceSamples != 36 {
		t.Fatal("missing trained alias")
	}
	for i := 0; i < len(outputs); i += 3 {
		a := Attribute(outputs[i:i+3], bank)
		if a.TopModel != "deepseek-v4.1-flash" || a.UsedOutputs != 3 {
			t.Fatalf("holdout %d: %+v", i/3, a)
		}
	}
}
