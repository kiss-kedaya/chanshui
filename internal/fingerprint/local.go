package fingerprint

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// LocalReference retains raw training sequences so centroids can be projected
// again after an upstream bank changes its feature normalization. Holdout
// responses never enter this structure.
type LocalReference struct {
	ID         string   `json:"id"`
	Aliases    []string `json:"aliases"`
	Family     string   `json:"family"`
	FamilyName string   `json:"family_name"`
	Source     string   `json:"source"`
	CapturedAt string   `json:"captured_at"`
	Samples    [][]int  `json:"samples"`
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
	model := ModelDef{ID: ref.ID, DisplayName: ref.ID, Family: ref.Family, FamilyName: ref.FamilyName, Counts: counts, Aliases: ref.Aliases, ReferenceSource: ref.Source, ReferenceSamples: len(ref.Samples)}
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

func (b *Bank) NumericModel(id string) *ModelDef {
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
