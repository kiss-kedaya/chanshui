package iq

import (
	"math"
	"regexp"
	"strconv"
)

var selfScoreNumber = regexp.MustCompile(`\d+(?:\.\d+)?`)

// ParseSelfScore reads the only number a model returned after seeing the
// standard answer. Values outside 0..max are clamped. No number is not a score.
func ParseSelfScore(text string, max int) (int, bool) {
	raw := selfScoreNumber.FindString(text)
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	n := int(math.Round(value))
	if n < 0 {
		n = 0
	}
	if max > 0 && n > max {
		n = max
	}
	return n, true
}
