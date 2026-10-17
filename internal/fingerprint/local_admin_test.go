package fingerprint

import "testing"

func syntheticReference(id string) LocalReference {
	samples := make([][]int, 36)
	for i := range samples {
		seq := make([]int, 304)
		for j := range seq {
			seq[j] = (i*17+j*29)%355 + 1
		}
		samples[i] = seq
	}
	return LocalReference{
		ID: id, DisplayName: id, Family: "custom", FamilyName: "Custom",
		Source: "https://fixture.invalid", CapturedAt: "2026-10-18T00:00:00Z",
		Custom: true, Samples: samples,
	}
}

func TestCustomReferenceCloneAndRemove(t *testing.T) {
	bank, err := LoadBank()
	if err != nil {
		t.Fatal(err)
	}
	before := len(bank.Numeric.Models)
	ref := syntheticReference("custom-fixture-model")
	if err = AddLocalReference(bank, ref); err != nil {
		t.Fatal(err)
	}
	if len(bank.Numeric.Models) != before+1 {
		t.Fatalf("model count = %d, want %d", len(bank.Numeric.Models), before+1)
	}
	if got := bank.NumericModel(ref.ID); got == nil || !got.Custom {
		t.Fatal("custom model not marked")
	}

	clone, err := CloneBank(bank)
	if err != nil {
		t.Fatal(err)
	}
	if err = RemoveLocalReference(clone, ref.ID); err != nil {
		t.Fatal(err)
	}
	if len(clone.Numeric.Models) != before {
		t.Fatalf("clone count = %d, want %d", len(clone.Numeric.Models), before)
	}
	if len(clone.Numeric.Robust.Hellinger.Centroids) != before ||
		len(clone.Numeric.Robust.OrderedBlocks.Centroids) != before {
		t.Fatal("centroids not aligned after removal")
	}
	for _, env := range clone.Numeric.Robust.OrderedBlocks.EnvironmentCentroids {
		if len(env) != before {
			t.Fatal("environment centroids not aligned after removal")
		}
	}
	if err = ValidateBank(clone); err != nil {
		t.Fatal(err)
	}
	if len(bank.Numeric.Models) != before+1 {
		t.Fatal("clone removal mutated the source bank")
	}

	clone2, _ := CloneBank(clone)
	official := clone.Numeric.Models[0].ID
	if err = RemoveLocalReference(clone2, official); err == nil {
		t.Fatal("removed a non-custom reference")
	}
}

func TestFamilyForModel(t *testing.T) {
	cases := map[string][2]string{
		"claude-opus-5-5": {"claude", "Claude"},
		"openai/gpt-5.6":  {"gpt", "GPT"},
		"deepseek-flash":  {"deepseek", "DeepSeek"},
		"x-custom-model":  {"custom", "Custom"},
	}
	for model, want := range cases {
		family, name := FamilyForModel(model)
		if family != want[0] || name != want[1] {
			t.Errorf("%s => %s/%s, want %v", model, family, name, want)
		}
	}
}
