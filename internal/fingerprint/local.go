package fingerprint

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// LocalReference retains raw training sequences so centroids can be projected
// again after an upstream bank changes its feature normalization. Holdout
// responses never enter this structure.
type LocalReference struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name,omitempty"`
	Aliases     []string `json:"aliases"`
	Family      string   `json:"family"`
	FamilyName  string   `json:"family_name"`
	Source      string   `json:"source"`
	CapturedAt  string   `json:"captured_at"`
	Custom      bool     `json:"custom,omitempty"`
	Samples     [][]int  `json:"samples"`
}

//go:embed local.references.json
var localReferences []byte

func MergeLocalReferences(b *Bank) error {
	var refs []LocalReference
	if err := json.Unmarshal(localReferences, &refs); err != nil {
		return err
	}
	for _, ref := range refs {
		if err := AddLocalReference(b, ref); err != nil {
			return err
		}
	}
	return ValidateBank(b)
}

// AddLocalReference uses the existing feature space, without pretending that
// one provider supplies the bank's twelve independently trained environments.
// Its single observed environment template is shared across those slots.
func AddLocalReference(b *Bank, ref LocalReference) error {
	if err := ValidateBank(b); err != nil {
		return err
	}
	if ref.ID == "" || len(ref.Samples) < 30 {
		return fmt.Errorf("local reference requires ID and at least 30 training samples")
	}
	h, o := &b.Numeric.Robust.Hellinger, &b.Numeric.Robust.OrderedBlocks
	hc, oc, ec := make([]float64, 355), make([]float64, 74), make([]float64, 74)
	counts := make([]int, 355)
	add := func(dst, src []float64) {
		for i, v := range src {
			dst[i] += v
		}
	}
	for _, nums := range ref.Samples {
		if len(nums) < b.Numeric.MinimumValidNumbers {
			return fmt.Errorf("insufficient training numbers")
		}
		for _, n := range nums {
			if n < 1 || n > 355 {
				return fmt.Errorf("training number out of range")
			}
			counts[n-1]++
		}
		hv := unit(projectOut(standardize(HellingerFeature(CountNumbers(nums)), h.FeatureMean, h.FeatureScale), h.NuisanceBasis))
		ov := standardize(OrderedBlockFeature(nums), o.FeatureMean, o.FeatureScale)
		add(hc, hv)
		add(oc, unit(projectOut(ov, o.NuisanceBasis)))
		add(ec, unit(ov))
	}
	hc, oc, ec = unit(hc), unit(oc), unit(ec)
	displayName := ref.DisplayName
	if displayName == "" {
		displayName = ref.ID
	}
	model := ModelDef{ID: ref.ID, DisplayName: displayName, Family: ref.Family, FamilyName: ref.FamilyName, Counts: counts, Aliases: ref.Aliases, ReferenceSource: ref.Source, ReferenceSamples: len(ref.Samples), Custom: ref.Custom}
	index := -1
	for i, m := range b.Numeric.Models {
		if m.ID == ref.ID {
			index = i
			break
		}
		for _, alias := range ref.Aliases {
			if m.ID == alias {
				index = i
				break
			}
		}
	}
	if index >= 0 {
		b.Numeric.Models[index] = model
		b.Numeric.Robust.ModelOrder[index] = ref.ID
		h.Centroids[index] = hc
		o.Centroids[index] = oc
		for i := range o.EnvironmentCentroids {
			o.EnvironmentCentroids[i][index] = append([]float64(nil), ec...)
		}
	} else {
		b.Numeric.Models = append(b.Numeric.Models, model)
		b.Numeric.Robust.ModelOrder = append(b.Numeric.Robust.ModelOrder, ref.ID)
		h.Centroids = append(h.Centroids, hc)
		o.Centroids = append(o.Centroids, oc)
		for i := range o.EnvironmentCentroids {
			o.EnvironmentCentroids[i] = append(o.EnvironmentCentroids[i], append([]float64(nil), ec...))
		}
	}
	return ValidateBank(b)
}

// CloneBank returns a deep copy suitable for validating a candidate reference
// without mutating the bank used by running jobs.
func CloneBank(b *Bank) (*Bank, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	var out Bank
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RemoveLocalReference removes a custom reference and its aligned centroids.
// It refuses to remove an official/embedded model.
func RemoveLocalReference(b *Bank, id string) error {
	index := -1
	for i, m := range b.Numeric.Models {
		if m.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("reference not found")
	}
	if !b.Numeric.Models[index].Custom {
		return fmt.Errorf("refusing to remove a non-custom reference")
	}
	b.Numeric.Models = append(b.Numeric.Models[:index], b.Numeric.Models[index+1:]...)
	b.Numeric.Robust.ModelOrder = append(b.Numeric.Robust.ModelOrder[:index], b.Numeric.Robust.ModelOrder[index+1:]...)
	b.Numeric.Robust.Hellinger.Centroids = append(b.Numeric.Robust.Hellinger.Centroids[:index], b.Numeric.Robust.Hellinger.Centroids[index+1:]...)
	b.Numeric.Robust.OrderedBlocks.Centroids = append(b.Numeric.Robust.OrderedBlocks.Centroids[:index], b.Numeric.Robust.OrderedBlocks.Centroids[index+1:]...)
	for i := range b.Numeric.Robust.OrderedBlocks.EnvironmentCentroids {
		b.Numeric.Robust.OrderedBlocks.EnvironmentCentroids[i] = append(
			b.Numeric.Robust.OrderedBlocks.EnvironmentCentroids[i][:index],
			b.Numeric.Robust.OrderedBlocks.EnvironmentCentroids[i][index+1:]...,
		)
	}
	return ValidateBank(b)
}

// FamilyForModel derives the display family for a captured reference. Unknown
// request names remain usable as a custom family.
func FamilyForModel(model string) (string, string) {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, prefix := range []string{"openai/", "anthropic/", "moonshotai/"} {
		model = strings.TrimPrefix(model, prefix)
	}
	switch {
	case strings.HasPrefix(model, "gpt-"):
		return "gpt", "GPT"
	case strings.HasPrefix(model, "claude-"):
		return "claude", "Claude"
	case strings.HasPrefix(model, "kimi-"):
		return "kimi", "Kimi"
	case strings.HasPrefix(model, "deepseek-"):
		return "deepseek", "DeepSeek"
	case strings.HasPrefix(model, "grok-"):
		return "grok", "Grok"
	case strings.HasPrefix(model, "gemini-"):
		return "gemini", "Gemini"
	case strings.HasPrefix(model, "glm-"):
		return "glm", "GLM"
	case strings.HasPrefix(model, "qwen-"):
		return "qwen", "Qwen"
	default:
		return "custom", "Custom"
	}
}

func (b *Bank) NumericModel(id string) *ModelDef {
	for _, prefix := range []string{"openai/", "anthropic/", "moonshotai/"} {
		id = strings.TrimPrefix(id, prefix)
	}
	for i := range b.Numeric.Models {
		m := &b.Numeric.Models[i]
		if m.ID == id {
			return m
		}
		for _, alias := range m.Aliases {
			if alias == id {
				return m
			}
		}
	}
	return nil
}
