package knowledge

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"chanzhushui/internal/gateway"
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
	Answer     string `json:"answer"`
	Error      string `json:"error,omitempty"`
}

type Probe struct {
	ID, Quarter, HappenedAt, Question, Event string
	Needles                                  []string
	Tokens                                   []string
	Month                                    int
}

var (
	nowFn    = time.Now
	unsureRE = regexp.MustCompile(`(?i)不知道|不确定|无法确定|不清楚|无法确认|没有可靠|没有任何|查一下|不会去猜|不打算|无法回答|不能确定|没有信息|知识只到|知识截止|cutoff|don'?t know|not sure|unknown`)
	finalRE  = regexp.MustCompile(`(?i)(?:最终答案|最后答案|本题答案|final\s*answer|the\s+answer\s+is|answer\s+is)\s*[:：是为]?\s*([^\n]*)`)
	leadRE   = regexp.MustCompile(`(?i)^\s*(不知道|不确定|不清楚|无法确定|unknown|don'?t know)`)
)

const ladderWidth = 3

func (p Probe) prompt() string {
	return "凭你的训练记忆回答，禁止联网、禁止使用工具。如果你不知道，或事件发生在你的知识截止之后，就诚实回答不知道，不要猜测。\n" +
		p.Question + "\n最后单独一行写：最终答案: <短答案或不知道>"
}

func (p Probe) hit(focus string) bool {
	if leadRE.MatchString(focus) {
		return false
	}
	low := strings.ToLower(focus)
	for _, needle := range p.Needles {
		if needle != "" && strings.Contains(low, strings.ToLower(needle)) {
			return true
		}
	}
	for _, token := range p.Tokens {
		if hasToken(low, token) {
			return true
		}
	}
	return p.Month > 0 && hasMonth(low, p.Month)
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

func hasMonth(text string, month int) bool {
	n := 0
	in := false
	flush := func() bool {
		if in && n == month {
			return true
		}
		n = 0
		in = false
		return false
	}
	for i := 0; i < len(text); i++ {
		b := text[i]
		if b >= '0' && b <= '9' {
			in = true
			n = n*10 + int(b-'0')
			if n > 9999 {
				n = 0
				in = false
			}
			continue
		}
		if flush() {
			return true
		}
	}
	return flush()
}

func extractFinal(text string) (string, bool) {
	plain := strings.ReplaceAll(text, "\r", "")
	if all := finalRE.FindAllStringSubmatch(plain, -1); len(all) > 0 {
		val := strings.TrimSpace(all[len(all)-1][1])
		// A one-character fragment is a truncated "最终答案" line, not the answer.
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
	focus, explicit := extractFinal(content)
	refused := leadRE.MatchString(focus) || (explicit && unsureRE.MatchString(focus))
	ok := strings.TrimSpace(content) != "" && !refused && p.hit(focus)
	unsure := !ok && (strings.TrimSpace(content) == "" || unsureRE.MatchString(content) || leadRE.MatchString(focus))
	return Row{
		ID: p.ID, Quarter: p.Quarter, HappenedAt: p.HappenedAt, Event: p.Event,
		OK: ok, Unsure: unsure, Answer: clip(focus, 120),
	}
}

// Summarize uses the latest correctly answered quarter as the knowledge boundary.
// Correct/total counts every rung. A hit within 45 days is flagged as possible browsing.
func Summarize(rows []Row, now time.Time) Verdict {
	out := Verdict{Status: "unknown", Total: len(rows), Rows: rows, Reason: "按客观事件阶梯估计知识边界，不参与评分。"}
	type stamped struct {
		row Row
		at  time.Time
	}
	ordered := make([]stamped, 0, len(rows))
	for _, row := range rows {
		at, err := time.Parse("2006-01-02", row.HappenedAt)
		if err != nil {
			at = time.Time{}
		}
		ordered = append(ordered, stamped{row, at})
	}
	for i := 1; i < len(ordered); i++ {
		item := ordered[i]
		j := i
		for j > 0 && ordered[j-1].at.After(item.at) {
			ordered[j] = ordered[j-1]
			j--
		}
		ordered[j] = item
	}
	freshIdx := -1
	for i, item := range ordered {
		if !item.row.OK {
			continue
		}
		out.Correct++
		out.Cutoff = item.row.Quarter
		freshIdx = i
		days := now.Sub(item.at).Hours() / 24
		if days >= 0 && days <= 45 {
			out.WebSuspect = fmt.Sprintf("答对了 %.0f 天前的事件（%s），疑似联网", days, item.row.Event)
		} else {
			out.WebSuspect = ""
		}
	}
	if freshIdx > 0 {
		hard := 0
		for _, item := range ordered[:freshIdx] {
			if !item.row.OK && !item.row.Unsure {
				hard++
			}
		}
		if hard >= 2 {
			out.GapNote = fmt.Sprintf("更早的 %d 档答错，新鲜度估计可能偏高", hard)
		}
	}
	failed := 0
	for _, row := range rows {
		if row.Error != "" {
			failed++
		}
	}
	name := out.Cutoff
	if name == "" {
		name = "未知"
	}
	out.Label = fmt.Sprintf("知识≈%s（%d/%d）", name, out.Correct, out.Total)
	switch {
	case failed > 0 && failed == len(rows):
		out.Status = "error"
	case failed > 0:
		out.Status = "partial"
	case out.Correct > 0:
		out.Status = "estimated"
	default:
		out.Status = "unknown"
	}
	if out.GapNote != "" {
		out.Reason += out.GapNote + "。"
	}
	if out.WebSuspect != "" {
		out.Reason += out.WebSuspect + "。"
	}
	return out
}

// Check asks each dated event separately. The newest correct quarter is the cutoff.
func Check(ctx context.Context, c gateway.Client, model string, timeout int) (Verdict, error) {
	rows := make([]Row, len(Ladder))
	for i, p := range Ladder {
		rows[i] = Row{ID: p.ID, Quarter: p.Quarter, HappenedAt: p.HappenedAt, Event: p.Event, Unsure: true}
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, ladderWidth)
	attempts := 0
	protocol := c.ProtocolFor(model)
	for i := range Ladder {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := Ladder[i]
			row := Row{ID: p.ID, Quarter: p.Quarter, HappenedAt: p.HappenedAt, Event: p.Event, Unsure: true}
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				row.Error = "检测已停止"
				rows[i] = row
				return
			}
			if ctx.Err() != nil {
				row.Error = "检测已停止"
				rows[i] = row
				return
			}
			m, err := c.CallMessages(ctx, gateway.CallRequest{
				Model: model, Prompt: p.prompt(), MaxTokens: 512, TimeoutSeconds: timeout,
			})
			mu.Lock()
			attempts += m.Attempts
			if m.Protocol != "" {
				protocol = m.Protocol
			}
			mu.Unlock()
			if err != nil {
				row.Error = err.Error()
				rows[i] = row
				return
			}
			if m.Err != "" {
				row.Error = m.Err
				rows[i] = row
				return
			}
			rows[i] = judge(p, m.Answer)
		}(i)
	}
	wg.Wait()
	verdict := Summarize(rows, nowFn())
	verdict.Attempts = attempts
	verdict.Protocol = protocol
	if verdict.Status == "error" {
		return verdict, fmt.Errorf("知识阶梯没有拿到有效回答")
	}
	if ctx.Err() != nil && verdict.Correct == 0 {
		return verdict, ctx.Err()
	}
	return verdict, nil
}
