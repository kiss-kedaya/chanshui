package fingerprint

import "math"

// Number-range and feature-vector dimensions from the ModelTrace bank
// (method.range = [1, 355]; ordered block feature = 4x16 bins + 10 final digits).
const (
	NumMinValue     = 1
	NumMaxValue     = 355
	NumDim          = 355
	OrderedBlockDim = 74
)

// ParseNumbers extracts the valid number sequence from model output text:
// digit runs are split at alphabetic separators, the longest run wins, and
// only values in 1..355 are retained (matches the bank's parser description).
func ParseNumbers(text string) []int {
	var runs [][]int
	cur := []int{}
	start := -1
	for i := 0; i <= len(text); i++ {
		digit := i < len(text) && text[i] >= '0' && text[i] <= '9'
		alpha := i < len(text) && isAlphaSep(text[i])
		if digit {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			cur = append(cur, parseInt(text[start:i]))
			start = -1
		}
		if alpha && len(cur) > 0 {
			runs = append(runs, cur)
			cur = []int{}
		}
	}
	best := cur
	for _, r := range runs {
		if len(r) > len(best) {
			best = r
		}
	}
	if len(best) == 0 {
		return nil
	}
	out := make([]int, 0, len(best))
	for _, v := range best {
		if v >= NumMinValue && v <= NumMaxValue {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isAlphaSep(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func parseInt(s string) int {
	v := 0
	for i := 0; i < len(s); i++ {
		v = v*10 + int(s[i]-'0')
		if v > 1<<30 {
			return 1 << 30
		}
	}
	return v
}

// CountNumbers histograms parsed numbers into the 355-dim bank bin space.
func CountNumbers(nums []int) []int {
	counts := make([]int, NumDim)
	for _, v := range nums {
		if v >= NumMinValue && v <= NumMaxValue {
			counts[v-NumMinValue]++
		}
	}
	return counts
}

// HellingerFeature computes sqrt((counts+0.5)/sum), the 355-dim smoothed
// Hellinger embedding used by the bank's hellinger centroids.
func HellingerFeature(counts []int) []float64 {
	total := 0
	for _, c := range counts {
		total += c
	}
	f := make([]float64, len(counts))
	if total == 0 {
		return f
	}
	den := float64(total) + 0.5*float64(len(counts))
	for i, c := range counts {
		f[i] = math.Sqrt((float64(c) + 0.5) / den)
	}
	return f
}

// OrderedBlockFeature builds the 74-dim ordered-block feature: 4 position
// chunks x 16 value bins plus a 10-bin final-digit histogram, sqrt-smoothed.
func OrderedBlockFeature(nums []int) []float64 {
	f := make([]float64, OrderedBlockDim)
	chunk := len(nums) / 4
	if chunk == 0 {
		chunk = 1
	}
	for c := 0; c < 4; c++ {
		lo := c * chunk
		hi := lo + chunk
		if c == 3 || hi > len(nums) {
			hi = len(nums)
		}
		for i := lo; i < hi; i++ {
			f[c*16+(nums[i]-NumMinValue)%16]++
		}
	}
	for _, v := range nums {
		f[64+(v-NumMinValue)%10]++
	}
	sum := 0.0
	for _, v := range f {
		sum += v
	}
	if sum == 0 {
		return f
	}
	for i := range f {
		f[i] = math.Sqrt((f[i] + 0.5) / (sum + 0.5*float64(len(f))))
	}
	return f
}
