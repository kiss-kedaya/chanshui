package iq

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var selfScoreNumber = regexp.MustCompile(`\d+(?:\.\d+)?`)
var bareScoreLine = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)

// ParseSelfScore reads the score a model returned after seeing the standard
// answer. A trailing line that is only an integer wins. A short reply may
// contain one number ("应给 14 分"). The first number inside a long trace is
// ignored, so a quoted "0 到 16" cannot become a zero.
func ParseSelfScore(text string, max int) (int, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		line = strings.Trim(line, "`*_。．. \t")
		line = strings.TrimSuffix(line, "分")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if bareScoreLine.MatchString(line) {
			return clampScore(line, max)
		}
		break
	}
	if utf8.RuneCountInString(text) <= 40 {
		found := selfScoreNumber.FindAllString(text, -1)
		if len(found) == 1 {
			return clampScore(found[0], max)
		}
	}
	return 0, false
}

func clampScore(raw string, max int) (int, bool) {
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
