package fingerprint

import (
	"math"
	"reflect"
	"testing"
)

func TestParseNumbers(t *testing.T) {
	got := ParseNumbers("3, 17, 42 100")
	if !reflect.DeepEqual(got, []int{3, 17, 42, 100}) {
		t.Fatalf("ParseNumbers simple: got %v", got)
	}

	got = ParseNumbers("356 0 5abc")
	if !reflect.DeepEqual(got, []int{5}) {
		t.Fatalf("ParseNumbers out-of-range and split: got %v", got)
	}

	// Alphabetic separators split runs; whitespace/punctuation do not.
	// "7 12a34" is therefore two runs: [7,12] and [34] — the longest wins.
	got = ParseNumbers("7 12a34")
	if !reflect.DeepEqual(got, []int{7, 12}) {
		t.Fatalf("ParseNumbers longest run picked: got %v", got)
	}

	// A single short run followed by a longer run.
	got = ParseNumbers("9a10 11 12")
	if !reflect.DeepEqual(got, []int{10, 11, 12}) {
		t.Fatalf("ParseNumbers longest run tie-break: got %v", got)
	}

	if got := ParseNumbers(""); got != nil {
		t.Fatalf("ParseNumbers empty: got %v", got)
	}
}

func TestCountNumbers(t *testing.T) {
	counts := CountNumbers([]int{3, 17, 3, 355, 356, 0})
	if len(counts) != 355 {
		t.Fatalf("CountNumbers length: %d", len(counts))
	}
	if counts[3-1] != 2 || counts[17-1] != 1 || counts[355-1] != 1 {
		t.Fatalf("CountNumbers binning wrong: %d %d %d", counts[2], counts[16], counts[354])
	}
	total := 0
	for _, c := range counts {
		total += c
	}
	if total != 4 {
		t.Fatalf("CountNumbers total: %d", total)
	}
}

func TestHellingerFeature(t *testing.T) {
	counts := make([]int, 355)
	counts[0] = 10
	counts[100] = 30
	counts[354] = 60
	f := HellingerFeature(counts)
	if len(f) != 355 {
		t.Fatalf("HellingerFeature length: %d", len(f))
	}
	sumSq := 0.0
	for _, v := range f {
		if v < 0 {
			t.Fatalf("HellingerFeature negative: %v", v)
		}
		sumSq += v * v
	}
	if math.Abs(sumSq-1.0) > 1e-9 {
		t.Fatalf("HellingerFeature squared sum: %v", sumSq)
	}
}

func TestOrderedBlockFeature(t *testing.T) {
	nums := make([]int, 0, 100)
	for i := 1; nums != nil && i <= 100; i++ {
		nums = append(nums, 1+i%355)
	}
	f := OrderedBlockFeature(nums)
	if len(f) != 74 {
		t.Fatalf("OrderedBlockFeature length: %d", len(f))
	}
	for _, v := range f {
		if v < 0 {
			t.Fatalf("OrderedBlockFeature negative: %v", v)
		}
	}
	// 4 chunks x 16 bins = 64, last-digit = 10
	if len(OrderedBlockFeature(nil)) != 74 {
		t.Fatalf("OrderedBlockFeature empty input length")
	}
}
