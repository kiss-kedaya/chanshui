package fingerprint

import (
	"encoding/json"
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
