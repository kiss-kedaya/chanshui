package knowledge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kiss-kedaya/chanzhushui/internal/gateway"
)

func TestFinalLineBeatsEarlierMentionAndEcho(t *testing.T) {
	p := anchorByID("mamdani")
	refused := judge(p, "佐兰·马姆达尼当选纽约市长。\n最终答案: 不知道")
	if refused.OK || !refused.Unsure || refused.Kind != kindUnknown {
		t.Fatalf("refusal = %+v", refused)
	}
	echo := judge(p, p.prompt()+"\n最终答案: 马姆达尼当选")
	if !echo.OK || echo.Kind != kindKnow {
		t.Fatalf("echo = %+v", echo)
	}
	schedule := judge(anchorByID("worldcup"), "西班牙是热门。\n最终答案: 只知道日程，不知道结果")
	if schedule.OK || schedule.Kind != kindUnknown {
		t.Fatalf("schedule = %+v", schedule)
	}
	spain := judge(anchorByID("worldcup"), "最终答案: 西班牙夺冠，费兰·托雷斯进球")
	if !spain.OK {
		t.Fatalf("spain = %+v", spain)
	}
	argentina := judge(anchorByID("worldcup"), "最终答案: 冠军是阿根廷")
	if argentina.OK || argentina.Unsure || argentina.Kind != kindWrong {
		t.Fatalf("argentina = %+v", argentina)
	}
	norway := judge(anchorByID("olympics"), "最终答案: 已经闭幕，奖牌榜第一是挪威")
	if !norway.OK {
		t.Fatalf("norway = %+v", norway)
	}
	held := judge(anchorByID("us250"), "最终答案: 已经举行，白宫举行了国庆庆典")
	if !held.OK || held.Kind != kindKnow {
		t.Fatalf("us250 held = %+v", held)
	}
	sched := judge(anchorByID("us250"), "最终答案: 只知道日程，不知道结果")
	if sched.OK || sched.Kind != kindUnknown {
		t.Fatalf("us250 schedule = %+v", sched)
	}
	cancelled := judge(anchorByID("us250"), "最终答案: 庆典取消")
	if cancelled.Kind != kindWrong {
		t.Fatalf("us250 cancelled = %+v", cancelled)
	}
	truncated := judge(p, "马姆达尼当选纽约市长。\n最")
	if !truncated.OK {
		t.Fatalf("truncated = %+v", truncated)
	}
	browsed := judge(p, "我刚刚联网查了，马姆达尼当选。\n最终答案: 马姆达尼当选")
	if browsed.Kind != kindInvalid {
		t.Fatalf("browsed = %+v", browsed)
	}
	bare := judge(p, "最终答案: 是")
	if bare.Kind != kindWrong {
		t.Fatalf("bare yes = %+v", bare)
	}
}

func TestBinaryPaths(t *testing.T) {
	cases := []struct {
		name  string
		ans   map[string]string
		label string
		order []string
	}{
		{"world cup", map[string]string{"mamdani": kindKnow, "olympics": kindKnow, "worldcup": kindKnow}, "知识≥2026-07-19", []string{"mamdani", "olympics", "worldcup"}},
		{"between olympics and semiquincentennial", map[string]string{"mamdani": kindKnow, "olympics": kindKnow, "worldcup": kindUnknown, "us250": kindUnknown}, "知识≈2026-02-22 至 2026-07-04（4 问）", []string{"mamdani", "olympics", "worldcup", "us250"}},
		{"between semiquincentennial and cup", map[string]string{"mamdani": kindKnow, "olympics": kindKnow, "worldcup": kindUnknown, "us250": kindKnow}, "知识≈2026-07-04 至 2026-07-19（4 问）", []string{"mamdani", "olympics", "worldcup", "us250"}},
		{"wrong semiquincentennial stays wide", map[string]string{"mamdani": kindKnow, "olympics": kindKnow, "worldcup": kindUnknown, "us250": kindWrong}, "知识≈2026-02-22 至 2026-07-19（4 问）", []string{"mamdani", "olympics", "worldcup", "us250"}},
		{"bulgaria inside", map[string]string{"mamdani": kindKnow, "olympics": kindUnknown, "bulgaria": kindKnow}, "知识≈2026-01-01 至 2026-02-22（3 问）", []string{"mamdani", "olympics", "bulgaria"}},
		{"before bulgaria", map[string]string{"mamdani": kindKnow, "olympics": kindUnknown, "bulgaria": kindUnknown}, "知识≈2025-11-04 至 2026-01-01（3 问）", []string{"mamdani", "olympics", "bulgaria"}},
		{"takaichi window", map[string]string{"mamdani": kindUnknown, "leo": kindKnow, "japan": kindKnow, "kirk": kindKnow, "takaichi_ldp": kindKnow, "takaichi_pm": kindKnow}, "知识≈2025-10-21 至 2025-11-04（6 问）", []string{"mamdani", "leo", "japan", "kirk", "takaichi_ldp", "takaichi_pm"}},
		{"lee split", map[string]string{"mamdani": kindUnknown, "leo": kindKnow, "japan": kindUnknown, "lee": kindKnow}, "知识≈2025-06-03 至 2025-07-20（4 问）", []string{"mamdani", "leo", "japan", "lee"}},
		{"before lee", map[string]string{"mamdani": kindUnknown, "leo": kindKnow, "japan": kindUnknown, "lee": kindUnknown}, "知识≈2025-05-08 至 2025-06-03（4 问）", []string{"mamdani", "leo", "japan", "lee"}},
		{"francis window", map[string]string{"mamdani": kindUnknown, "leo": kindUnknown, "germany": kindKnow, "yoon": kindKnow, "francis": kindKnow}, "知识≈2025-04-21 至 2025-05-08（5 问）", []string{"mamdani", "leo", "germany", "yoon", "francis"}},
		{"carney split", map[string]string{"mamdani": kindUnknown, "leo": kindUnknown, "germany": kindKnow, "yoon": kindUnknown, "carney": kindKnow}, "知识≈2025-03-14 至 2025-04-04（5 问）", []string{"mamdani", "leo", "germany", "yoon", "carney"}},
		{"trump window", map[string]string{"mamdani": kindUnknown, "leo": kindUnknown, "germany": kindUnknown, "trump": kindKnow}, "知识≈2025-01-20 至 2025-02-23（4 问）", []string{"mamdani", "leo", "germany", "trump"}},
		{"before trump", map[string]string{"mamdani": kindUnknown, "leo": kindUnknown, "germany": kindUnknown, "trump": kindUnknown}, "知识＜2025-01-20", []string{"mamdani", "leo", "germany", "trump"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, order := walk(tc.ans)
			if v.Label != tc.label {
				t.Fatalf("label %q want %q order %v", v.Label, tc.label, order)
			}
			if strings.Join(order, ",") != strings.Join(tc.order, ",") {
				t.Fatalf("order %v want %v", order, tc.order)
			}
			if v.Total != len(tc.order) {
				t.Fatalf("total %d", v.Total)
			}
		})
	}
}

func TestWrongMamdaniDoesNotMoveBound(t *testing.T) {
	v, order := walk(map[string]string{
		"mamdani":     kindWrong,
		"takaichi_pm": kindKnow,
		"olympics":    kindUnknown,
		"bulgaria":    kindUnknown,
	})
	if len(order) == 0 || order[0] != "mamdani" {
		t.Fatalf("order %v", order)
	}
	if order[1] != "takaichi_pm" {
		t.Fatalf("fallback order %v", order)
	}
	if strings.Contains(v.Label, "2025-11-04") {
		t.Fatalf("wrong answer moved the november bound: %s", v.Label)
	}
	if v.GapNote == "" {
		t.Fatal("expected gap note")
	}
}

func TestCheckIsSerialAndSkipsHTTPFailure(t *testing.T) {
	var inflight int32
	var maxSeen int32
	var order []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&inflight, 1)
		defer atomic.AddInt32(&inflight, -1)
		for {
			old := atomic.LoadInt32(&maxSeen)
			if n <= old || atomic.CompareAndSwapInt32(&maxSeen, old, n) {
				break
			}
		}
		body, _ := io.ReadAll(r.Body)
		q := string(body)
		text := "最终答案: 不知道"
		switch {
		case strings.Contains(q, "纽约市长"):
			text = "最终答案: 马姆达尼当选"
		case strings.Contains(q, "冬奥会"):
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":{"message":"upstream"}}`))
			return
		case strings.Contains(q, "建国250") || strings.Contains(q, "250周年"):
			text = "最终答案: 已经举行，白宫举行了庆典"
		case strings.Contains(q, "世界杯"):
			text = "最终答案: 西班牙夺冠"
		}
		order = append(order, text)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []any{map[string]any{"type": "text", "text": text}},
			"usage":   map[string]int{"input_tokens": 20, "output_tokens": 4},
		})
	}))
	defer s.Close()
	prev := nowFn
	nowFn = func() time.Time { return time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC) }
	defer func() { nowFn = prev }()
	v, err := Check(context.Background(), gateway.Client{BaseURL: s.URL, NoRetry: true}, "claude-opus-5", 3)
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&maxSeen) != 1 {
		t.Fatalf("concurrent requests: %d", maxSeen)
	}
	if v.Label != "知识≥2026-07-19" {
		t.Fatalf("label %s rows %+v", v.Label, v.Rows)
	}
	if v.Rows[1].Kind != kindError {
		t.Fatalf("olympics = %+v", v.Rows[1])
	}
	if strings.Contains(v.Label, "2026-02-22") {
		t.Fatal("http failure moved the olympics bound")
	}
}

func walk(answers map[string]string) (Verdict, []string) {
	s := newSearch()
	var order []string
	for {
		p, ok := s.next()
		if !ok {
			break
		}
		order = append(order, p.ID)
		kind := answers[p.ID]
		if kind == "" {
			kind = kindUnknown
		}
		s.apply(p, kind, kind)
	}
	return s.verdict(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)), order
}

func TestQuestionsAreOneChoice(t *testing.T) {
	banned := []string{"制胜球", "石破", "请回答", "如果只知道", "只知道日程，不知道结果", "是否获胜", "是否辞职"}
	for _, p := range Anchors {
		if strings.Count(p.Question, "？") != 1 {
			t.Fatalf("%s stem = %s", p.ID, p.Question)
		}
		for _, b := range banned {
			if strings.Contains(p.Question, b) {
				t.Fatalf("%s stem has %s", p.ID, b)
			}
		}
		if len(p.Choices) != 4 {
			t.Fatalf("%s choices %d", p.ID, len(p.Choices))
		}
		seen := map[string]bool{}
		know, wrong, unknown := 0, 0, 0
		for _, c := range p.Choices {
			if c.Letter < "A" || c.Letter > "D" || seen[c.Letter] || c.Text == "" {
				t.Fatalf("%s choice %+v", p.ID, c)
			}
			seen[c.Letter] = true
			switch c.Kind {
			case kindKnow:
				know++
			case kindWrong:
				wrong++
			case kindUnknown:
				unknown++
			default:
				t.Fatalf("%s kind %s", p.ID, c.Kind)
			}
		}
		if know == 0 || wrong == 0 || unknown == 0 || len(seen) != 4 {
			t.Fatalf("%s balance %d %d %d", p.ID, know, wrong, unknown)
		}
		if !strings.Contains(p.prompt(), `{"choice":"<字母>"}`) || !strings.Contains(p.prompt(), p.Question) {
			t.Fatalf("%s prompt missing json contract", p.ID)
		}
	}
	closed := judge(anchorByID("olympics"), "最终答案: 已经闭幕")
	if closed.OK || closed.Kind == kindKnow {
		t.Fatalf("closed only = %+v", closed)
	}
	liu := judge(anchorByID("olympics"), "最终答案: 女子单人滑冠军是刘美贤")
	if !liu.OK || liu.Kind != kindKnow {
		t.Fatalf("liu = %+v", liu)
	}
	alysa := judge(anchorByID("olympics"), "最终答案: Alysa Liu")
	if !alysa.OK {
		t.Fatalf("alysa = %+v", alysa)
	}
	oly := anchorByID("olympics")
	var knowLetter, unkLetter string
	for _, c := range oly.Choices {
		if c.Kind == kindKnow && strings.Contains(c.Text, "挪威") {
			knowLetter = c.Letter
		}
		if c.Kind == kindUnknown && unkLetter == "" {
			unkLetter = c.Letter
		}
	}
	hedged := judge(oly, "前文说不确定，也说将于下月闭幕。\n{\"choice\":\""+knowLetter+"\",\"note\":\"不确定\"}")
	if !hedged.OK || hedged.Kind != kindKnow {
		t.Fatalf("json choice = %+v", hedged)
	}
	unk := judge(oly, "{\"choice\":\""+unkLetter+"\"}")
	if unk.OK || unk.Kind != kindUnknown {
		t.Fatalf("json unknown = %+v", unk)
	}
	browsed := judge(oly, "我刚刚联网查了。\n{\"choice\":\""+knowLetter+"\"}")
	if browsed.Kind != kindInvalid {
		t.Fatalf("browsed json = %+v", browsed)
	}
	cup := anchorByID("worldcup")
	var wrongLetter string
	for _, c := range cup.Choices {
		if strings.Contains(c.Text, "阿根廷") {
			wrongLetter = c.Letter
		}
	}
	arg := judge(cup, "{\"choice\":\""+wrongLetter+"\"}")
	if arg.OK || arg.Unsure || arg.Kind != kindWrong {
		t.Fatalf("json argentina = %+v", arg)
	}
}
