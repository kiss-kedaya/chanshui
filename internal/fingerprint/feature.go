package fingerprint

import (
	"math"
	"regexp"
	"strconv"
	"unicode"
)

const (
	NumMinValue     = 1
	NumMaxValue     = 355
	NumDim          = 355
	OrderedBlockDim = 74
)

var numberPattern = regexp.MustCompile("[0-9]+")

func ParseNumbers(text string) []int {
	var best, cur []int
	end := 0
	for _, loc := range numberPattern.FindAllStringIndex(text, -1) {
		sep := text[end:loc[0]]
		hasLetter := false
		for _, r := range sep {
			if unicode.IsLetter(r) {
				hasLetter = true
				break
			}
		}
		if hasLetter && len(cur) > 0 {
			if len(cur) > len(best) {
				best = cur
			}
			cur = nil
		}
		v, err := strconv.Atoi(text[loc[0]:loc[1]])
		if err == nil && v >= 1 && v <= 355 {
			cur = append(cur, v)
		}
		end = loc[1]
	}
	if len(cur) > len(best) {
		best = cur
	}
	return best
}
func CountNumbers(nums []int) []int {
	c := make([]int, 355)
	for _, v := range nums {
		if v >= 1 && v <= 355 {
			c[v-1]++
		}
	}
	return c
}
func HellingerFeature(c []int) []float64 {
	f := make([]float64, len(c))
	sum := 0.5 * float64(len(c))
	for _, v := range c {
		sum += float64(v)
	}
	for i, v := range c {
		f[i] = math.Sqrt((float64(v) + 0.5) / sum)
	}
	return f
}
func OrderedBlockFeature(nums []int) []float64 {
	f := make([]float64, 74)
	pos := 0
	for b := 0; b < 4; b++ {
		size := len(nums) / 4
		if b < len(nums)%4 {
			size++
		}
		for _, v := range nums[pos : pos+size] {
			if v >= 1 && v <= 355 {
				f[b*16+(v-1)*16/355]++
			}
		}
		for j := 0; j < 16; j++ {
			f[b*16+j] = math.Sqrt((f[b*16+j] + 0.5) / (float64(size) + 8))
		}
		pos += size
	}
	for _, v := range nums {
		if v >= 1 && v <= 355 {
			f[64+v%10]++
		}
	}
	for j := 64; j < 74; j++ {
		f[j] = math.Sqrt((f[j] + 0.5) / (float64(len(nums)) + 5))
	}
	return f
}
