package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kiss-kedaya/chanshui/internal/gateway"
)

// Verdict is informational only. It never contributes to IQ, integrity, or attribution.
type Verdict struct {
	Status     string `json:"status"`
	Cutoff     string `json:"cutoff,omitempty"`
	Label      string `json:"label"`
	Correct    int    `json:"correct"`
	Total      int    `json:"total"`
	WebSuspect string `json:"web_suspect,omitempty"`
	GapNote    string `json:"gap_note,omitempty"`
	Reason     string `json:"reason"`
	Protocol   string `json:"protocol"`
	Attempts   int    `json:"attempts"`
	Rows       []Row  `json:"rows"`
}

type Row struct {
	ID         string `json:"id"`
	Quarter    string `json:"quarter"`
	HappenedAt string `json:"happened_at"`
	Event      string `json:"event"`
	OK         bool   `json:"ok"`
	Unsure     bool   `json:"unsure"`
	Kind       string `json:"kind,omitempty"`
	Answer     string `json:"answer"`
	Error      string `json:"error,omitempty"`
}

type Choice struct {
	Letter string
	Text   string
	Kind   string
}

type Probe struct {
	ID, HappenedAt, Question, Event string
	Accept                          [][]string
	Wrong                           [][]string
	Choices                         []Choice
}

const (
	kindKnow    = "know"
	kindUnknown = "unknown"
	kindWrong   = "wrong"
	kindError   = "error"
	kindInvalid = "invalid"
	maxAsks     = 10
)

var (
	nowFn     = time.Now
	unsureRE  = regexp.MustCompile(`(?i)不知道|不确定|无法确定|不清楚|无法确认|不能确定|没有可靠|没有任何|不会去猜|不打算|无法回答|没有信息|知识只到|知识截止|cutoff|don'?t know|not sure|unknown|只知道日程|不知道结果|只知道赛程|只知道会举办|只知道会选举|尚未发生|尚未举行|尚未闭幕|尚未结束|还未发生|还没发生|还没举行|未发生|没有发生|将举办|将举行|将于|预定于|计划于|预计将|结果未知|没有举行|未闭幕|没有闭幕|还没闭幕|将会|将要|将在|将去|打算于|计划在|预定在|预计在`)
	invalidRE = regexp.MustCompile(`(?i)我(刚刚|已经|刚才)?(联网|搜索了|查了)|刚才联网|调用了工具|使用了工具|searched the web|i looked it up|根据今天|当前日期是|今天是\s*20\d\d`)
	finalRE   = regexp.MustCompile(`(?i)(?:最终答案|最后答案|本题答案|final\s*answer|the\s+answer\s+is|answer\s+is)\s*[:：是为]?\s*([^\n]*)`)
	leadRE    = regexp.MustCompile(`(?i)^\s*(不知道|不确定|不清楚|无法确定|unknown|don'?t know|只知道日程)`)
	bareYesRE = regexp.MustCompile(`(?i)^\s*(是|是的|对|已经发生|发生了|yes|yeah)[。.!！\s]*$`)
)

func (p Probe) at() time.Time {
	t, err := time.Parse("2006-01-02", p.HappenedAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (p Probe) prompt() string {
	var b strings.Builder
	b.WriteString("请仅凭参数记忆作答。禁止联网，禁止调用工具，禁止使用当前日期，禁止推测。\n")
	b.WriteString("只输出一个 JSON 对象，不要 Markdown，不要解释，不要输出选项原文。\n")
	b.WriteString("格式：{\"choice\":\"<字母>\"}\n")
	b.WriteString("choice 必须是 A、B、C、D 之一。选已经发生的那一项；只知道日程或不能确认时选对应项。\n")
	b.WriteString("题目：" + p.Question + "\n")
	for _, c := range p.Choices {
		fmt.Fprintf(&b, "%s. %s\n", c.Letter, c.Text)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p Probe) lookupLetter(letter string) (Choice, bool) {
	letter = strings.ToUpper(strings.TrimSpace(letter))
	for _, c := range p.Choices {
		if strings.EqualFold(c.Letter, letter) {
			return c, true
		}
	}
	return Choice{}, false
}

func asLetter(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if len(v) != 1 {
		return "", false
	}
	u := strings.ToUpper(v)
	if u < "A" || u > "D" {
		return "", false
	}
	return u, true
}

func (p Probe) lookupChoice(v string) (Choice, bool) {
	if letter, ok := asLetter(v); ok {
		return p.lookupLetter(letter)
	}
	s := strings.TrimSpace(v)
	for _, c := range p.Choices {
		if s == c.Text || strings.EqualFold(s, c.Letter+". "+c.Text) {
			return c, true
		}
	}
	return Choice{}, false
}

func (p Probe) choiceFrom(content string) (Choice, bool) {
	var hit Choice
	found := false
	for i := 0; i < len(content); i++ {
		if content[i] != '{' {
			continue
		}
		depth := 0
		inStr := false
		esc := false
		end := -1
		for j := i; j < len(content); j++ {
			c := content[j]
			if inStr {
				if esc {
					esc = false
					continue
				}
				if c == '\\' {
					esc = true
					continue
				}
				if c == '"' {
					inStr = false
				}
				continue
			}
			switch c {
			case '"':
				inStr = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					end = j
				}
			}
			if end == j {
				break
			}
		}
		if end < 0 {
			continue
		}
		var obj map[string]any
		if json.Unmarshal([]byte(content[i:end+1]), &obj) == nil {
			if v, ok := obj["choice"]; ok {
				if ch, ok := p.lookupChoice(fmt.Sprint(v)); ok {
					hit = ch
					found = true
				}
			}
		}
		i = end
	}
	return hit, found
}

func (p Probe) positive(text string) bool { return matchGroups(text, p.Accept) }
func (p Probe) negative(text string) bool { return matchGroups(text, p.Wrong) }

func matchGroups(text string, sets [][]string) bool {
	low := strings.ToLower(text)
	for _, set := range sets {
		if len(set) == 0 {
			continue
		}
		ok := true
		for _, needle := range set {
			if !matchNeedle(low, needle) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func matchNeedle(low, needle string) bool {
	n := strings.ToLower(strings.TrimSpace(needle))
	if n == "" {
		return false
	}
	if isASCII(n) && !strings.Contains(n, " ") && len(n) <= 5 {
		return hasToken(low, n)
	}
	return strings.Contains(low, n)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}

func hasToken(text, token string) bool {
	token = strings.ToLower(token)
	if token == "" {
		return false
	}
	for i := 0; i+len(token) <= len(text); {
		j := strings.Index(text[i:], token)
		if j < 0 {
			return false
		}
		j += i
		before := j == 0 || !isASCIIWord(text[j-1])
		after := j+len(token) == len(text) || !isASCIIWord(text[j+len(token)])
		if before && after {
			return true
		}
		i = j + 1
	}
	return false
}

func isASCIIWord(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func extractFinal(text string) (string, bool) {
	plain := strings.ReplaceAll(text, "\r", "")
	if all := finalRE.FindAllStringSubmatch(plain, -1); len(all) > 0 {
		val := strings.TrimSpace(all[len(all)-1][1])
		if utf8.RuneCountInString(val) >= 2 {
			return val, true
		}
	}
	return tail(plain, 800), false
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func judge(p Probe, content string) Row {
	kind, focus := classify(p, content)
	row := Row{
		ID: p.ID, Quarter: p.HappenedAt, HappenedAt: p.HappenedAt, Event: p.Event,
		Kind: kind, Answer: clip(focus, 120),
	}
	switch kind {
	case kindKnow:
		row.OK = true
	case kindUnknown, kindInvalid:
		row.Unsure = true
	}
	return row
}

func classify(p Probe, content string) (string, string) {
	content = strings.ReplaceAll(content, p.prompt(), "\n")
	content = strings.ReplaceAll(content, p.Question, " ")
	if invalidRE.MatchString(content) {
		focus, _ := extractFinal(content)
		return kindInvalid, focus
	}
	if ch, ok := p.choiceFrom(content); ok {
		return ch.Kind, ch.Letter + " " + ch.Text
	}
	focus, explicit := extractFinal(content)
	if explicit {
		if ch, ok := p.lookupChoice(focus); ok {
			return ch.Kind, ch.Letter + " " + ch.Text
		}
	}
	if explicit {
		switch decide(p, focus, true) {
		case kindKnow, kindUnknown, kindWrong:
			return decide(p, focus, true), focus
		}
	}
	return decide(p, content, false), focus
}

func decide(p Probe, text string, explicit bool) string {
	pos := p.positive(text)
	neg := p.negative(text)
	unknown := unsureRE.MatchString(text) || leadRE.MatchString(text)
	if explicit && unknown {
		return kindUnknown
	}
	if pos && !neg && !unknown {
		return kindKnow
	}
	if unknown && !pos {
		return kindUnknown
	}
	if neg {
		return kindWrong
	}
	if pos && unknown {
		return kindUnknown
	}
	if strings.TrimSpace(text) == "" {
		return kindUnknown
	}
	if bareYesRE.MatchString(strings.TrimSpace(text)) {
		return kindWrong
	}
	if explicit {
		return kindWrong
	}
	return kindWrong
}

type search struct {
	lo, hi  time.Time
	asked   int
	correct int
	wrongs  int
	errors  int
	invalid bool
	rows    []Row
	seen    map[string]bool
	kinds   map[string]string
}

func newSearch() *search {
	return &search{seen: map[string]bool{}, kinds: map[string]string{}}
}

const (
	modeAsk = iota
	modeFallback
	modeStop
)

func (s *search) next() (Probe, bool) {
	if s.invalid || s.asked >= maxAsks {
		return Probe{}, false
	}
	id, mode := s.script()
	if mode == modeStop {
		return Probe{}, false
	}
	if mode == modeFallback {
		id = s.fallback()
		if id == "" {
			return Probe{}, false
		}
	}
	return anchorByID(id), true
}

func (s *search) script() (string, int) {
	switch s.gate("mamdani") {
	case gateAsk:
		return "mamdani", modeAsk
	case gateBad:
		return "", modeFallback
	case gateKnow:
		return s.afterMamdani()
	case gateUnknown:
		return s.beforeMamdani()
	default:
		return "", modeFallback
	}
}

func (s *search) afterMamdani() (string, int) {
	switch s.gate("olympics") {
	case gateAsk:
		return "olympics", modeAsk
	case gateBad:
		return "", modeFallback
	case gateKnow:
		switch s.gate("worldcup") {
		case gateAsk:
			return "worldcup", modeAsk
		case gateBad:
			return "", modeFallback
		case gateUnknown:
			switch s.gate("us250") {
			case gateAsk:
				return "us250", modeAsk
			case gateBad:
				return "", modeFallback
			default:
				return "", modeStop
			}
		default:
			return "", modeStop
		}
	case gateUnknown:
		switch s.gate("bulgaria") {
		case gateAsk:
			return "bulgaria", modeAsk
		case gateBad:
			return "", modeFallback
		default:
			return "", modeStop
		}
	default:
		return "", modeStop
	}
}

func (s *search) beforeMamdani() (string, int) {
	switch s.gate("leo") {
	case gateAsk:
		return "leo", modeAsk
	case gateBad:
		return "", modeFallback
	case gateKnow:
		return s.afterLeo()
	case gateUnknown:
		return s.beforeLeo()
	default:
		return "", modeFallback
	}
}

func (s *search) afterLeo() (string, int) {
	switch s.gate("japan") {
	case gateAsk:
		return "japan", modeAsk
	case gateBad:
		return "", modeFallback
	case gateUnknown:
		switch s.gate("lee") {
		case gateAsk:
			return "lee", modeAsk
		case gateBad:
			return "", modeFallback
		default:
			return "", modeStop
		}
	case gateKnow:
		switch s.gate("kirk") {
		case gateAsk:
			return "kirk", modeAsk
		case gateBad:
			return "", modeFallback
		case gateUnknown:
			return "", modeStop
		case gateKnow:
			switch s.gate("takaichi_ldp") {
			case gateAsk:
				return "takaichi_ldp", modeAsk
			case gateBad:
				return "", modeFallback
			case gateUnknown:
				return "", modeStop
			case gateKnow:
				switch s.gate("takaichi_pm") {
				case gateAsk:
					return "takaichi_pm", modeAsk
				case gateBad:
					return "", modeFallback
				default:
					return "", modeStop
				}
			default:
				return "", modeStop
			}
		default:
			return "", modeStop
		}
	default:
		return "", modeStop
	}
}

func (s *search) beforeLeo() (string, int) {
	switch s.gate("germany") {
	case gateAsk:
		return "germany", modeAsk
	case gateBad:
		return "", modeFallback
	case gateUnknown:
		switch s.gate("trump") {
		case gateAsk:
			return "trump", modeAsk
		case gateBad:
			return "", modeFallback
		default:
			return "", modeStop
		}
	case gateKnow:
		switch s.gate("yoon") {
		case gateAsk:
			return "yoon", modeAsk
		case gateBad:
			return "", modeFallback
		case gateUnknown:
			switch s.gate("carney") {
			case gateAsk:
				return "carney", modeAsk
			case gateBad:
				return "", modeFallback
			default:
				return "", modeStop
			}
		case gateKnow:
			switch s.gate("francis") {
			case gateAsk:
				return "francis", modeAsk
			case gateBad:
				return "", modeFallback
			default:
				return "", modeStop
			}
		default:
			return "", modeStop
		}
	default:
		return "", modeStop
	}
}

const (
	gateAsk = iota
	gateKnow
	gateUnknown
	gateBad
	gateSkip
)

func (s *search) gate(id string) int {
	if !s.seen[id] {
		if !s.open(id) {
			return gateSkip
		}
		return gateAsk
	}
	switch s.kinds[id] {
	case kindKnow:
		return gateKnow
	case kindUnknown:
		return gateUnknown
	default:
		return gateBad
	}
}

func (s *search) open(id string) bool {
	at := anchorByID(id).at()
	if !s.lo.IsZero() && !at.After(s.lo) {
		return false
	}
	if !s.hi.IsZero() && !at.Before(s.hi) {
		return false
	}
	return true
}

func (s *search) fallback() string {
	lo := s.lo
	hi := s.hi
	if lo.IsZero() {
		lo = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	if hi.IsZero() {
		hi = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	}
	mid := lo.Add(hi.Sub(lo) / 2)
	best := ""
	var bestDist time.Duration = 1<<63 - 1
	for _, p := range Anchors {
		if s.seen[p.ID] || !s.open(p.ID) {
			continue
		}
		d := p.at().Sub(mid)
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			best = p.ID
		}
	}
	return best
}

func (s *search) apply(p Probe, kind, answer string) {
	s.asked++
	s.seen[p.ID] = true
	s.kinds[p.ID] = kind
	row := Row{
		ID: p.ID, Quarter: p.HappenedAt, HappenedAt: p.HappenedAt, Event: p.Event,
		Kind: kind, Answer: clip(answer, 120),
	}
	switch kind {
	case kindKnow:
		row.OK = true
		at := p.at()
		if s.lo.IsZero() || at.After(s.lo) {
			s.lo = at
		}
		s.correct++
	case kindUnknown:
		row.Unsure = true
		at := p.at()
		if s.hi.IsZero() || at.Before(s.hi) {
			s.hi = at
		}
	case kindInvalid:
		row.Unsure = true
		s.invalid = true
	case kindError:
		row.Unsure = true
		row.Error = answer
		s.errors++
	default:
		s.wrongs++
	}
	s.rows = append(s.rows, row)
}

func (s *search) verdict(now time.Time) Verdict {
	out := Verdict{
		Status:  "unknown",
		Label:   s.label(),
		Correct: s.correct,
		Total:   s.asked,
		Rows:    s.rows,
		Reason:  "按 2025–2026 客观事件串行二分估计知识边界，不参与评分。",
	}
	if !s.lo.IsZero() {
		out.Cutoff = s.lo.Format("2006-01-02")
	}
	if s.invalid {
		out.Status = "invalid"
		out.Reason = "回答里出现联网、工具或当前日期，这次知识截止测试无效。"
	} else if s.asked > 0 && s.errors == s.asked {
		out.Status = "error"
	} else if s.errors > 0 {
		out.Status = "partial"
	} else if s.correct > 0 || !s.hi.IsZero() {
		out.Status = "estimated"
	}
	if s.wrongs > 0 {
		out.GapNote = fmt.Sprintf("有 %d 题结果对不上，未用它们移动边界", s.wrongs)
		out.Reason += out.GapNote + "。"
	}
	for _, row := range s.rows {
		if !row.OK {
			continue
		}
		at, err := time.Parse("2006-01-02", row.HappenedAt)
		if err != nil {
			continue
		}
		days := now.Sub(at).Hours() / 24
		if days >= 0 && days <= 45 {
			out.WebSuspect = fmt.Sprintf("答对了 %.0f 天前的事件（%s），疑似联网", days, row.Event)
		}
	}
	if out.WebSuspect != "" {
		out.Reason += out.WebSuspect + "。"
	}
	return out
}

func (s *search) label() string {
	if s.invalid {
		return "知识截止测试无效"
	}
	lo, hi := "", ""
	if !s.lo.IsZero() {
		lo = s.lo.Format("2006-01-02")
	}
	if !s.hi.IsZero() {
		hi = s.hi.Format("2006-01-02")
	}
	switch {
	case lo != "" && hi != "":
		return fmt.Sprintf("知识≈%s 至 %s（%d 问）", lo, hi, s.asked)
	case lo != "":
		return "知识≥" + lo
	case hi != "":
		return "知识＜" + hi
	default:
		return "知识≈未知"
	}
}

// Check asks dated events one at a time. The first question is always the 2025-11-04 mayoral election.
func Check(ctx context.Context, c gateway.Client, model string, timeout int) (Verdict, error) {
	s := newSearch()
	attempts := 0
	protocol := c.ProtocolFor(model)
	for {
		if ctx.Err() != nil {
			break
		}
		p, ok := s.next()
		if !ok {
			break
		}
		m, err := c.CallMessages(ctx, gateway.CallRequest{
			Model: model, Prompt: p.prompt(), MaxTokens: 512, TimeoutSeconds: timeout,
		})
		attempts += m.Attempts
		if m.Protocol != "" {
			protocol = m.Protocol
		}
		if err != nil || m.Err != "" {
			msg := m.Err
			if err != nil {
				msg = err.Error()
			}
			s.apply(p, kindError, msg)
			continue
		}
		kind, focus := classify(p, m.Answer)
		s.apply(p, kind, focus)
		if kind == kindInvalid {
			break
		}
	}
	verdict := s.verdict(nowFn())
	verdict.Attempts = attempts
	verdict.Protocol = protocol
	if verdict.Status == "error" {
		return verdict, fmt.Errorf("知识截止探针没有拿到有效回答")
	}
	if ctx.Err() != nil && verdict.Correct == 0 && verdict.Total == 0 {
		return verdict, ctx.Err()
	}
	return verdict, nil
}
