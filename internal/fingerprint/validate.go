package fingerprint

import (
	"fmt"
	"math"
)

// ValidateBank rejects malformed remote matrices before they can reach any running job.
func ValidateBank(b *Bank) error {
	n := len(b.Numeric.Models)
	if n < 1 || n > 500 || len(b.Numeric.Robust.ModelOrder) != n {
		return fmt.Errorf("invalid model count")
	}
	if b.Numeric.MinimumValidNumbers < 80 || b.Numeric.MinimumValidNumbers > 333 {
		return fmt.Errorf("invalid sample minimum")
	}
	method := b.Numeric.Method
	if len(method.Range) != 2 || method.Range[0] != 1 || method.Range[1] != 355 || method.Alpha != 0.5 {
		return fmt.Errorf("unsupported fingerprint method")
	}
	validVector := func(row []float64, d int, positive bool) bool {
		if len(row) != d {
			return false
		}
		for _, v := range row {
			if math.IsNaN(v) || math.IsInf(v, 0) || (positive && v <= 0) {
				return false
			}
		}
		return true
	}
	validMatrix := func(rows [][]float64, n, d int) bool {
		if len(rows) != n {
			return false
		}
		for _, row := range rows {
			if !validVector(row, d, false) {
				return false
			}
		}
		return true
	}
	seen := map[string]bool{}
	for i, m := range b.Numeric.Models {
		if m.ID == "" || m.Family == "" || seen[m.ID] || m.ID != b.Numeric.Robust.ModelOrder[i] || len(m.Counts) != 355 {
			return fmt.Errorf("invalid model or order")
		}
		seen[m.ID] = true
		total := 0
		for _, v := range m.Counts {
			if v < 0 {
				return fmt.Errorf("negative count")
			}
			total += v
		}
		if total == 0 {
			return fmt.Errorf("empty fingerprint")
		}
	}
	h := b.Numeric.Robust.Hellinger
	o := b.Numeric.Robust.OrderedBlocks
	if !validVector(h.FeatureMean, 355, false) || !validVector(h.FeatureScale, 355, true) || !validMatrix(h.Centroids, n, 355) || !validMatrix(h.NuisanceBasis, len(h.NuisanceBasis), 355) {
		return fmt.Errorf("invalid Hellinger matrix")
	}
	if !validVector(o.FeatureMean, 74, false) || !validVector(o.FeatureScale, 74, true) || !validMatrix(o.Centroids, n, 74) || !validMatrix(o.NuisanceBasis, len(o.NuisanceBasis), 74) || len(o.EnvironmentCentroids) == 0 || o.Weight < 0 || o.Weight > 1 || math.IsNaN(o.Weight) {
		return fmt.Errorf("invalid ordered matrix")
	}
	for _, env := range o.EnvironmentCentroids {
		if !validMatrix(env, n, 74) {
			return fmt.Errorf("invalid environment matrix")
		}
	}
	for _, k := range []string{"1", "2", "3"} {
		c, ok := b.Numeric.Calibration[k]
		if !ok || c.Beta <= 0 || math.IsNaN(c.Beta) || math.IsInf(c.Beta, 0) {
			return fmt.Errorf("invalid calibration")
		}
	}
	return nil
}
