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

func indexRune(s string, r rune) int {
	for i, c := range s {
		if c == r {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// 数值等价

var numRe = regexp.MustCompile(`-?\d+(?:\.\d+)?`)

// parseNum extracts the leading answer number before explanatory breakdowns in s and parses it to float64.
// Chinese numerals (零..九十九, incl. 两=2, 二十一) are accepted when no
// arabic number is present.
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
		earned, half, tags = judgePill(text)
	case "Q2":
		earned, half, tags = judgeCarWash(text)
	case "Q3":
		earned, half, tags = judgeBatBall(text)
	case "Q4":
		earned, half, tags = judgeColorBlind(text)
	case "Q5":
		earned, half, tags = judgeAnchor(text, tail)
	case "Q6", "Q8":
		earned, half, tags = judgeCandy(q, text, tail)
	case "Q7":
		earned, half, tags = judgeSocks(q, text, tail)
	default:
		return 0, false, nil
	}
	if earned == 0 {
		if n, ok := fullForExpectedNumber(q, fullText, slot, tail); ok {
			return n, false, nil
		}
	}
	return earned, half, tags
}

// fullForExpectedNumber gives full marks when the published standard answer is
// a number and the model ended on that number. Earlier reasoning numbers do
// not count, and a wrong final slot is left at zero for the self-score round.
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

var carWashPassRe = regexp.MustCompile(`开车|驾车|驾驶|自驾|开过去`)
var carWashFailRe = regexp.MustCompile(`走路|步行|打车|出租|骑车|骑行|坐车`)

func judgeCarWash(text string) (int, bool, []string) {
	if carWashPassRe.MatchString(text) && !carWashFailRe.MatchString(text) {
		return 5, false, nil
	}
	return 0, false, nil
}

func judgeBatBall(text string) (int, bool, []string) {
	v, err := parseNum(text)
	if err == nil && numEq(v, 0.05) {
		return 6, false, nil
	}
	return 0, false, nil
}

var anchorDownRe = regexp.MustCompile(`下降|降低|下沉`)
var anchorReasonRe = regexp.MustCompile(`排水|浮力|体积|密度|重量`)
var anchorLevelDownRe = regexp.MustCompile(`水位(?:会|将会|将|应该|应|已经|已|明显|略)?(?:下降|降低|下沉)`)
var anchorUpRe = regexp.MustCompile(`上升|不变|不会下降|没有下降|未下降`)

func judgeAnchor(text, tail string) (int, bool, []string) {
	if anchorLevelDownRe.MatchString(text) || (anchorLevelDownRe.MatchString(tail) && !anchorUpRe.MatchString(text)) {
		return 10, false, nil
	}
	if !anchorDownRe.MatchString(text) {
		return 0, false, nil
	}
	if anchorReasonRe.MatchString(text) {
		return 10, false, nil
	}
	return 5, true, []string{"只答方向"}
}

var pillPassRe = regexp.MustCompile(`不叫醒|不喂|暂缓(?:给药|发放|发药|用药)|持药观察|无需服药|不给药|不发药|不發药|不必吃药|已经睡着就不用|暂不给药|无必要叫醒|没必要叫醒`)
var pillWakeRe = regexp.MustCompile(`叫醒|唤醒`)
var pillNegWakeRe = regexp.MustCompile(`(?:不|别|勿|未|莫|无法|不用|无需|不能|不可|不要|暂不|千万别|切莫)(?:叫醒|唤醒)`)

func judgePill(text string) (int, bool, []string) {
	text = regexp.MustCompile(`(?:无必要|没必要|无需|不必|不要|不能|不应|不可|不用|不|别|勿)(?:把|将)?[^，。；;：:
]{0,8}(?:叫醒|唤醒)[^，。；;：:
]*`).ReplaceAllString(text, "不叫醒")
	if regexp.MustCompile(`仍应喂药|应该喂药|强行喂药|必须喂药`).MatchString(text) {
		return 0, false, nil
	}
	if pillPassRe.MatchString(text) && !pillWakeRe.MatchString(pillNegWakeRe.ReplaceAllString(text, "")) {
		return 6, false, nil
	}
	return 0, false, nil
}

var (
	cbFullRe    = regexp.MustCompile(`非亲生|不是亲生|并非亲生|不可能是亲生|排除亲生`)
	cbChainRe   = regexp.MustCompile(`伴X|隔代|外公|外祖父|遗传|色盲基因`)
	cbAffirmRe  = regexp.MustCompile(`亲生|正常`)
	cbNegPrefix = regexp.MustCompile(`(?:不|非|并|未|无法|不可)(?:可能)?(亲生|正常)`)
)

func judgeColorBlind(text string) (int, bool, []string) {
	if regexp.MustCompile(`不能排除|无法排除|不一定|不能断定|无法断定`).MatchString(text) {
		return 0, false, nil
	}
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

func variantStockOf(q Question) (circ, star [3]int) {
	// stored in q.Tags as "库存:ca,ct,cm:sa,st,sm"
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
