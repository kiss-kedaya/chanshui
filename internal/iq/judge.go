package iq

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// 答案抽取

// stripMarkdown removes code fences/inline code and the common emphasis and
// heading markers so that slots wrapped in ** or fenced blocks still match.
func stripMarkdown(s string) string {
	s = regexp.MustCompile("(?m)^\x60\x60\x60[^\n]*\n?").ReplaceAllString(s, "")
	return strings.NewReplacer("\x60", "", "**", "", "__", "", "##", "").Replace(s)
}

var slotRe = regexp.MustCompile(`(?i)(最终答案|最后答案|本题答案|答案[:：]|final answer|the answer is)`)

// ExtractAnswer finds the LAST answer slot in the text. slot is the remainder
// of that line after the marker (leading colon/space removed); chainTail is
// the last 2000 runes of the whole stripped text, used as fallback when no
// slot (or an empty slot) is found.
func ExtractAnswer(text string) (slot, chainTail string) {
	cleaned := stripMarkdown(text)
	runes := []rune(cleaned)
	if len(runes) > 2000 {
		runes = runes[len(runes)-2000:]
	}
	chainTail = string(runes)
	locs := slotRe.FindAllStringIndex(cleaned, -1)
	if len(locs) == 0 {
		return "", chainTail
	}
	loc := locs[len(locs)-1]
	rest := cleaned[loc[1]:]
	if nl := indexRune(rest, '\n'); nl >= 0 {
		rest = rest[:nl]
	}
	rest = strings.Trim(rest, " \t\r*：:。、，")
	return rest, chainTail
}

// NumericAnswer reports whether the answer slot or the tail contains a
// number. The vision section uses it to tell "saw the chart" from "request
// failed" without reimplementing the answer parser.
func NumericAnswer(fullText string) bool {
	slot, tail := ExtractAnswer(fullText)
	if slot != "" {
		if _, err := parseNum(slot); err == nil {
			return true
		}
	}
	_, err := parseNum(tail)
	return err == nil
}

func indexRune(s string, r rune) int {
	for i, c := range s {
		if c == r {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// 数值与分数等价

var numRe = regexp.MustCompile(`-?\d+(?:\.\d+)?`)
var fracRe = regexp.MustCompile(`(\d+)\s*/\s*(\d+)`)
var pctRe = regexp.MustCompile(`(-?\d+(?:\.\d+)?)\s*%`)

// parseNum extracts the leading answer number before explanatory breakdowns in
// s and parses it to float64. Chinese numerals (零..九十九, incl. 两=2, 二十一)
// are accepted when no arabic number is present.
func parseNum(s string) (float64, error) {
	s = regexp.MustCompile(`(?:不是|并非|不等于|不应为|而非)\s*-?\d+(?:\.\d+)?`).ReplaceAllString(s, "")
	cleaned := strings.NewReplacer(",", "", "，", "", " ", "", "\t", "").Replace(stripMarkdown(s))
	if m := numRe.FindAllString(cleaned, -1); len(m) > 0 {
		return strconv.ParseFloat(m[0], 64)
	}
	if v, ok := parseCNNum(cleaned); ok {
		return v, nil
	}
	return 0, errNoNumber
}

// parseProb parses a probability expressed as a fraction (p/q), percentage
// (x%), or decimal (0.xxx) into float64 in [0, 1]. When last is true, takes
// the final probability in s (used for slotless chain-tail fallback).
func parseProb(s string, last bool) (float64, error) {
	s = regexp.MustCompile(`(?:不是|并非|不等于|不应为|而非)\s*(?:\d+\s*/\s*\d+|\d+(?:\.\d+)?%?)`).ReplaceAllString(s, "")
	cleaned := strings.NewReplacer(",", "", "，", "", "$", "", "\\frac{", "", "}{", "/", "}", "").Replace(stripMarkdown(s))
	if ms := fracRe.FindAllStringSubmatch(cleaned, -1); len(ms) > 0 {
		m := ms[0]
		if last {
			m = ms[len(ms)-1]
		}
		num, e1 := strconv.ParseFloat(m[1], 64)
		den, e2 := strconv.ParseFloat(m[2], 64)
		if e1 == nil && e2 == nil && den > 0 {
			return num / den, nil
		}
	}
	if ms := pctRe.FindAllStringSubmatch(cleaned, -1); len(ms) > 0 {
		m := ms[0]
		if last {
			m = ms[len(ms)-1]
		}
		v, err := strconv.ParseFloat(m[1], 64)
		if err == nil {
			return v / 100.0, nil
		}
	}
	if ms := numRe.FindAllString(strings.ReplaceAll(cleaned, " ", ""), -1); len(ms) > 0 {
		if last {
			for i := len(ms) - 1; i >= 0; i-- {
				if v, err := strconv.ParseFloat(ms[i], 64); err == nil && v >= 0 && v <= 1 {
					return v, nil
				}
			}
		} else {
			if v, err := strconv.ParseFloat(ms[0], 64); err == nil && v >= 0 && v <= 1 {
				return v, nil
			}
		}
	}
	return 0, errNoNumber
}

var errNoNumber = &numError{}

type numError struct{}

func (*numError) Error() string { return "no number found in answer slot" }

var cnDigits = map[rune]int{
	'零': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5,
	'六': 6, '七': 7, '八': 8, '九': 9,
}

func parseCNNum(s string) (float64, bool) {
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		j := i
		for j < len(runes) && isCNNumRune(runes[j]) {
			j++
		}
		if j > i {
			if v, ok := cnPhraseValue(runes[i:j]); ok {
				return float64(v), true
			}
		}
		i = j
	}
	return 0, false
}

func isCNNumRune(r rune) bool {
	_, digit := cnDigits[r]
	return digit || r == '十'
}

func cnPhraseValue(rs []rune) (int, bool) {
	if len(rs) == 0 {
		return 0, false
	}
	if len(rs) == 1 {
		if v, ok := cnDigits[rs[0]]; ok && !(v == 0 && string(rs) != "零") {
			return v, true
		}
		if rs[0] == '十' {
			return 10, true
		}
		return 0, false
	}
	tenIdx := -1
	for i, r := range rs {
		if r == '十' {
			tenIdx = i
			break
		}
	}
	if tenIdx < 0 {
		return 0, false
	}
	tens := 1
	if tenIdx > 0 {
		v, ok := cnDigits[rs[tenIdx-1]]
		if !ok || v == 0 {
			return 0, false
		}
		tens = v
	}
	units := 0
	if tenIdx+1 < len(rs) {
		v, ok := cnDigits[rs[tenIdx+1]]
		if !ok {
			return 0, false
		}
		units = v
	}
	if tenIdx+2 < len(rs) {
		return 0, false
	}
	return tens*10 + units, true
}

func numEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func probEq(a, b float64) bool {
	return math.Abs(a-b) < 5e-4
}

// ---------------------------------------------------------------------------
// 判分

// Judge scores a full model response against its question. earned follows the
// per-question rule; half marks half-credit (counted as a flag, NOT into the
// 0-100 numerator); tags carry diagnostic labels like 替代建模 or 忽略禁区.
func Judge(q Question, fullText string) (earned int, half bool, tags []string) {
	slot, tail := ExtractAnswer(fullText)
	switch q.ID {
	case "Q1":
		earned, half, tags = judgeTruthLiar(q, slot, tail)
	case "Q2":
		earned, half, tags = judgeSchedule(q, slot, tail)
	case "Q3":
		earned, half, tags = judgeCRT(q, slot, tail)
	case "Q4":
		earned, half, tags = judgeGenetics(q, slot, tail)
	case "Q5":
		earned, half, tags = judgeForbidSteps(q, slot, tail)
	case "Q6", "Q8":
		earned, half, tags = judgeCandy(q, slot, tail)
	case "Q7":
		earned, half, tags = judgeSocks(q, slot, tail)
	case "Q9":
		earned, half, tags = judgeBridge(q, slot, tail)
	case "vision":
		earned, half, tags = judgeChart(q, slot, tail)
	default:
		return 0, false, nil
	}
	if earned == 0 && len(tags) == 0 {
		if n, ok := fullForExpectedNumber(q, fullText, slot, tail); ok {
			return n, false, nil
		}
	}
	return earned, half, tags
}

// fullForExpectedNumber gives full marks when the published standard answer is
// a number and the model ended on that number. Earlier reasoning numbers do
// not count, and a wrong final slot is left at zero.
func fullForExpectedNumber(q Question, full, slot, tail string) (int, bool) {
	want, err := strconv.ParseFloat(strings.TrimSpace(q.Answer), 64)
	if err != nil || q.MaxScore <= 0 {
		return 0, false
	}
	if slot != "" {
		if v, e := parseNum(slot); e == nil && numEq(v, want) {
			return q.MaxScore, true
		}
		return 0, false
	}
	lines := strings.Split(stripMarkdown(full), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if v, e := lastNum(line); e == nil && numEq(v, want) {
			return q.MaxScore, true
		}
		return 0, false
	}
	if v, e := lastNum(tail); e == nil && numEq(v, want) {
		return q.MaxScore, true
	}
	return 0, false
}

func lastNum(s string) (float64, error) {
	s = regexp.MustCompile(`(?:不是|并非|不等于|不应为|而非)\s*-?\d+(?:\.\d+)?`).ReplaceAllString(s, "")
	cleaned := strings.NewReplacer(",", "", "，", "", " ", "", "\t", "").Replace(stripMarkdown(s))
	if m := numRe.FindAllString(cleaned, -1); len(m) > 0 {
		return strconv.ParseFloat(m[len(m)-1], 64)
	}
	if v, ok := parseCNNum(cleaned); ok {
		return v, nil
	}
	return 0, errNoNumber
}

func maxOr(q Question, def int) int {
	if q.MaxScore > 0 {
		return q.MaxScore
	}
	return def
}

func parseSlotOrTail(slot, tail string) (float64, bool) {
	if slot != "" {
		if v, err := parseNum(slot); err == nil {
			return v, true
		}
	}
	if v, err := lastNum(tail); err == nil {
		return v, true
	}
	return 0, false
}

func judgeTruthLiar(q Question, text, tail string) (int, bool, []string) {
	want, err := strconv.Atoi(strings.TrimSpace(q.Answer))
	if err != nil {
		return 0, false, nil
	}
	v, ok := parseSlotOrTail(text, tail)
	if !ok {
		return 0, false, nil
	}
	if numEq(v, float64(want)) {
		return maxOr(q, 10), false, nil
	}
	return 0, false, nil
}

func judgeSchedule(q Question, text, tail string) (int, bool, []string) {
	want, err := strconv.Atoi(strings.TrimSpace(q.Answer))
	if err != nil {
		return 0, false, nil
	}
	v, ok := parseSlotOrTail(text, tail)
	if !ok {
		return 0, false, nil
	}
	full := maxOr(q, 10)
	if numEq(v, float64(want)) {
		return full, false, nil
	}
	if jobs, has := scheduleParamsOf(q); has {
		total := 0
		for _, d := range jobs {
			total += d
		}
		lb := (total + 1) / 2
		lpt := lptSchedule(jobs)
		if numEq(v, float64(lb)) {
			return 0, false, []string{"忽略不可拆分"}
		}
		if lpt != want && numEq(v, float64(lpt)) {
			return full / 2, true, []string{"贪心次优"}
		}
	}
	return 0, false, nil
}

func judgeCRT(q Question, text, tail string) (int, bool, []string) {
	want, err := strconv.Atoi(strings.TrimSpace(q.Answer))
	if err != nil {
		return 0, false, nil
	}
	v, ok := parseSlotOrTail(text, tail)
	if !ok {
		return 0, false, nil
	}
	if numEq(v, float64(want)) {
		return maxOr(q, 10), false, nil
	}
	if base, has := crtBaseOf(q); has && base != want && numEq(v, float64(base)) {
		return 0, false, []string{"忽略下界"}
	}
	return 0, false, nil
}

func judgeGenetics(q Question, slot, tail string) (int, bool, []string) {
	want, err := parseProb(q.Answer, false)
	if err != nil {
		return 0, false, nil
	}
	var v float64
	var perr error
	if slot != "" {
		v, perr = parseProb(slot, false)
	}
	if slot == "" || perr != nil {
		v, perr = parseProb(tail, true)
		if perr != nil {
			return 0, false, nil
		}
	}
	if probEq(v, want) {
		return maxOr(q, 12), false, nil
	}
	if gc, ok := geneticsCaseOf(q); ok && gc.trap != "" {
		if trapV, err := parseProb(gc.trap, false); err == nil && probEq(v, trapV) {
			return 0, false, []string{gc.trapTag}
		}
	}
	return 0, false, nil
}

func judgeForbidSteps(q Question, text, tail string) (int, bool, []string) {
	want, err := strconv.Atoi(strings.TrimSpace(q.Answer))
	if err != nil {
		return 0, false, nil
	}
	v, ok := parseSlotOrTail(text, tail)
	if !ok {
		return 0, false, nil
	}
	if numEq(v, float64(want)) {
		return maxOr(q, 10), false, nil
	}
	if base, has := forbidBaseDistOf(q); has && base != want && numEq(v, float64(base)) {
		return 0, false, []string{"忽略禁区"}
	}
	return 0, false, nil
}

func judgeCandy(q Question, text, tail string) (int, bool, []string) {
	var solver, blind int
	switch q.ID {
	case "Q6":
		circ := [3]int{7, 9, 8}
		star := [3]int{7, 6, 4}
		solver = SolveCandy(circ, star)
		blind = blindCandyAnswer(circ, star)
	case "Q8":
		circ, star := variantStockOf(q)
		solver = SolveCandy(circ, star)
		blind = blindCandyAnswer(circ, star)
	}
	v, ok := parseSlotOrTail(text, tail)
	if !ok {
		return 0, false, nil
	}
	full := maxOr(q, 12)
	if numEq(v, float64(solver)) {
		return full, false, nil
	}
	if q.ID == "Q8" && numEq(v, 21) {
		return 0, false, []string{"疑似套用经典答案"}
	}
	if numEq(v, float64(blind)) {
		return full / 2, true, []string{"替代建模"}
	}
	return 0, false, nil
}

func judgeBridge(q Question, text, tail string) (int, bool, []string) {
	want, err := strconv.Atoi(strings.TrimSpace(q.Answer))
	if err != nil {
		want = 10
	}
	times, _ := bridgeTimesOf(q)
	trap := bridgeCarryBackAnswer(times)
	v, ok := parseSlotOrTail(text, tail)
	if !ok {
		return 0, false, nil
	}
	if numEq(v, float64(want)) {
		return maxOr(q, 12), false, nil
	}
	if numEq(v, float64(trap)) {
		return 0, false, []string{"忽略定向支架"}
	}
	return 0, false, nil
}

func judgeSocks(q Question, text, tail string) (int, bool, []string) {
	counts, p, worst, ok := socksParamsOf(q)
	if !ok {
		return 0, false, nil
	}
	formula := 2*p + len(counts) - 1
	v, vok := parseSlotOrTail(text, tail)
	if !vok {
		return 0, false, nil
	}
	full := maxOr(q, 12)
	if numEq(v, float64(worst)) {
		return full, false, nil
	}
	if numEq(v, float64(formula)) {
		return full / 2, true, []string{"替代建模"}
	}
	return 0, false, nil
}

func variantStockOf(q Question) (circ, star [3]int) {
	for _, tg := range q.Tags {
		if !strings.HasPrefix(tg, "库存:") {
			continue
		}
		body := strings.TrimPrefix(tg, "库存:")
		segs := strings.Split(body, ":")
		if len(segs) != 2 {
			continue
		}
		read := func(s string, out *[3]int) bool {
			parts := strings.Split(s, ",")
			if len(parts) != 3 {
				return false
			}
			for i, p := range parts {
				v, e := strconv.Atoi(p)
				if e != nil {
					return false
				}
				out[i] = v
			}
			return true
		}
		if read(segs[0], &circ) && read(segs[1], &star) {
			return circ, star
		}
	}
	return [3]int{}, [3]int{}
}

func judgeChart(q Question, text, tail string) (int, bool, []string) {
	want, err := strconv.Atoi(q.Answer)
	if err != nil {
		return 0, false, nil
	}
	v, ok := parseSlotOrTail(text, tail)
	if !ok {
		return 0, false, nil
	}
	if numEq(v, float64(want)) {
		return q.MaxScore, false, nil
	}
	if math.Abs(v-float64(want)) <= 2 {
		return q.MaxScore / 2, true, []string{"读数误差"}
	}
	return 0, false, nil
}
