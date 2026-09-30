package iq

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// 答案抽取

var mdNoise = regexp.MustCompile("(?s)```.*?```|`[^`]*`|\\*\\*|__|##|>>?")

// stripMarkdown removes code fences/inline code and the common emphasis and
// heading markers so that slots wrapped in ** or fenced blocks still match.
func stripMarkdown(s string) string {
	return mdNoise.ReplaceAllString(s, "")
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

func indexRune(s string, r rune) int {
	for i, c := range s {
		if c == '\n' && i >= 0 {
			_ = c
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// 数值等价

var numRe = regexp.MustCompile(`-?\d+(?:\.\d+)?`)

// parseNum extracts the LAST numeric token in s and parses it to float64.
// Chinese numerals (零..九十九, incl. 两=2, 二十一) are accepted when no
// arabic number is present.
func parseNum(s string) (float64, error) {
	cleaned := strings.NewReplacer(",", "", "，", "", " ", "", "\t", "").Replace(stripMarkdown(s))
	if m := numRe.FindAllString(cleaned, -1); len(m) > 0 {
		return strconv.ParseFloat(m[len(m)-1], 64)
	}
	if v, ok := parseCNNum(cleaned); ok {
		return v, nil
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

// parseCNNum reads the FIRST Chinese numeral phrase worth ≤99 in s.
func parseCNNum(s string) (float64, bool) {
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		// phrase window: consecutive chars in {零一二两三四五六七八九十}
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

// cnPhraseValue parses 零..九十九: handles 十(10), 十X(11..19), X十(20,30..90),
// X十Y(21..99).
func cnPhraseValue(rs []rune) (int, bool) {
	// reject all-zero / empty
	if len(rs) == 0 {
		return 0, false
	}
	// simple single digit
	if len(rs) == 1 {
		if v, ok := cnDigits[rs[0]]; ok && !(v == 0 && string(rs) != "零") {
			return v, true
		}
		if rs[0] == '十' {
			return 10, true
		}
		return 0, false
	}
	// long phrases: only 十-containing structures supported
	tenIdx := -1
	for i, r := range rs {
		if r == '十' {
			tenIdx = i
			break
		}
	}
	if tenIdx < 0 {
		// e.g. 九九 (unusual); not supported
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
		// trailing junk inside the window (e.g. 十二十五) — unsupported
		return 0, false
	}
	return tens*10 + units, true
}

func numEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// ---------------------------------------------------------------------------
// 判分

// Judge scores a full model response against its question. earned follows the
// per-question rule; half marks half-credit (counted as a flag, NOT into the
// 0-100 numerator); tags carry diagnostic labels like 替代建模.
func Judge(q Question, fullText string) (earned int, half bool, tags []string) {
	slot, tail := ExtractAnswer(fullText)
	text := slot
	if text == "" {
		text = tail
	}
	switch q.ID {
	case "Q1":
		return judgePill(text)
	case "Q4":
		return judgeColorBlind(text)
	case "Q6", "Q8":
		return judgeCandy(q, text, tail)
	case "Q7":
		return judgeSocks(q, text, tail)
	}
	return 0, false, nil
}

var pillPassRe = regexp.MustCompile(`不叫醒|不喂|暂缓给药|持药观察|无需服药|不给药|已经睡着|暂不给药`)
var pillWakeRe = regexp.MustCompile(`叫醒|唤醒`)
var pillNegWakeRe = regexp.MustCompile(`(?:不|别|勿|未|莫|无法|不用|无需|不能|不可|不要|暂不|千万别|切莫)(?:叫醒|唤醒)`)

func judgePill(text string) (int, bool, []string) {
	if pillPassRe.MatchString(text) && !pillWakeRe.MatchString(pillNegWakeRe.ReplaceAllString(text, "")) {
		return 6, false, nil
	}
	return 0, false, nil
}

var (
	cbFullRe    = regexp.MustCompile(`非亲生|不是亲生|并非亲生|不可能是亲生|排除亲生|亲生的可能性`)
	cbChainRe   = regexp.MustCompile(`伴X|隔代|外公|外祖父|遗传|色盲基因`)
	cbAffirmRe  = regexp.MustCompile(`亲生|正常`)
	cbNegPrefix = regexp.MustCompile(`(?:不|非|并|未|无法|不可)(?:可能)?(亲生|正常)`)
)

func judgeColorBlind(text string) (int, bool, []string) {
	if cbFullRe.MatchString(text) {
		return 14, false, nil
	}
	if cbChainRe.MatchString(text) {
		return 7, true, []string{"仅遗传链"}
	}
	// (亲生|正常) without negation → 0 (affirmation is wrong)
	stripped := cbNegPrefix.ReplaceAllString(text, "")
	if cbAffirmRe.MatchString(stripped) {
		return 0, false, nil
	}
	return 0, false, nil
}

// judgeCandy handles Q6 (classic, solver answer 21, blind answer 29) and Q8
// (variant, solver answer from the generated stock, blind answer from
// blindCandyAnswer).
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
	usedTail := false
	v, err := parseNum(text)
	if err != nil {
		// slot empty or non-numeric → 思维链兜底: scan the last 2000 chars
		tv, terr := parseNum(tail)
		if terr != nil {
			return 0, false, nil
		}
		v, usedTail = tv, true
		_ = usedTail
	}
	if numEq(v, float64(solver)) {
		return 16, false, nil
	}
	if q.ID == "Q8" && numEq(v, 21) {
		// 21 is the classic's (often non-optimal here) guarantee — parroting
		// the textbook answer earns nothing
		return 0, false, []string{"疑似套用经典答案"}
	}
	if numEq(v, float64(blind)) {
		return 8, true, []string{"替代建模"}
	}
	if q.ID == "Q8" && numEq(v, 25) {
		// 25 is the classic blind-draw answer — blind sampling on the variant
		return 8, true, []string{"替代建模"}
	}
	return 0, false, nil
}

func judgeSocks(q Question, text, tail string) (int, bool, []string) {
	counts, p, worst, ok := socksParamsOf(q)
	if !ok {
		return 0, false, nil
	}
	formula := 2*p + len(counts) - 1
	v, err := parseNum(text)
	if err != nil {
		tv, terr := parseNum(tail)
		if terr != nil {
			return 0, false, nil
		}
		v = tv
	}
	if numEq(v, float64(worst)) {
		return 16, false, nil
	}
	if numEq(v, float64(formula)) {
		return 8, true, []string{"替代建模"}
	}
	return 0, false, nil
}
