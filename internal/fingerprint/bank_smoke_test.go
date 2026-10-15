package fingerprint

import "testing"

func TestLoadBank(t *testing.T) {
	b, err := LoadBank()
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Numeric.Models) != 22 {
		t.Fatalf("numeric models: %d", len(b.Numeric.Models))
	}
	if len(b.Behavioral.Models) != 34 {
		t.Fatalf("behavioral models: %d", len(b.Behavioral.Models))
	}
	h := b.Numeric.Robust.Hellinger
	if len(h.Centroids) != 22 || len(h.Centroids[0]) != 355 {
		t.Fatalf("hellinger centroids %dx%d", len(h.Centroids), len(h.Centroids[0]))
	}
	if len(h.NuisanceBasis) != 2 || len(h.NuisanceBasis[0]) != 355 {
		t.Fatalf("hellinger nuisance %dx%d", len(h.NuisanceBasis), len(h.NuisanceBasis[0]))
	}
	ob := b.Numeric.Robust.OrderedBlocks
	if len(ob.Centroids) != 22 || len(ob.Centroids[0]) != 74 {
		t.Fatalf("ordered centroids %dx%d", len(ob.Centroids), len(ob.Centroids[0]))
	}
	if len(ob.EnvironmentCentroids) != 12 || len(ob.EnvironmentCentroids[0]) != 22 || len(ob.EnvironmentCentroids[0][0]) != 74 {
		t.Fatalf("env centroids %dx%dx%d", len(ob.EnvironmentCentroids), len(ob.EnvironmentCentroids[0]), len(ob.EnvironmentCentroids[0][0]))
	}
	if ob.Weight != 0.25 {
		t.Fatalf("ordered weight %v", ob.Weight)
	}
	if b.Numeric.Calibration["3"].Beta != 11.99999999999999 {
		t.Fatalf("calibration beta: %v", b.Numeric.Calibration["3"].Beta)
	}
}
