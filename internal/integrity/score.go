package integrity

import "math"

// Signal is one independently measured interface capability. Unknown signals
// reduce coverage but do not silently become failures.
type Signal struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Weight     int    `json:"weight"`
	Applicable bool   `json:"applicable"`
	Known      bool   `json:"known"`
	Pass       *bool  `json:"pass,omitempty"`
	Points     *int   `json:"points,omitempty"`
	MaxPoints  int    `json:"max_points"`
	Reason     string `json:"reason"`
}

type Inputs struct {
	Protocol        string
	InjectionScore  *int
	CachePass       *bool
	CacheApplicable bool
	ToolsPass       *bool
	ToolsApplicable bool
	WebPass         *bool
	WebApplicable   bool
	ThinkingScore   *int
	ThinkingApply   bool
	StreamPass      *bool
}

type Result struct {
	Score            *int     `json:"score"`
	Coverage         float64  `json:"coverage"`
	AvailableWeight  int      `json:"available_weight"`
	ApplicableWeight int      `json:"applicable_weight"`
	Status           string   `json:"status"`
	Reason           string   `json:"reason"`
	Signals          []Signal `json:"signals"`
}

const (
	weightInjection = 20
	weightCache     = 20
	weightTools     = 20
	weightWeb       = 15
	weightStream    = 15
	weightThinking  = 10
)

// Calculate normalizes only known, applicable signals. It is intentionally a
// separate score from IQ: protocol quality is not a claim about intelligence.
func Calculate(in Inputs) Result {
	protocol := in.Protocol
	signals := []Signal{
		{Key: "injection", Label: "注入大小", Weight: weightInjection, MaxPoints: 10, Applicable: true, Known: in.InjectionScore != nil, Points: in.InjectionScore, Reason: "两次独立请求的线性估计"},
		boolSignal("cache", "缓存读写", weightCache, in.CacheApplicable, in.CachePass != nil, boolPoints(in.CachePass), "三次请求衔接"),
		boolSignal("tools", "工具调用", weightTools, in.ToolsApplicable, in.ToolsPass != nil, boolPoints(in.ToolsPass), "结构化调用与结果回传"),
		boolSignal("web", "联网证据", weightWeb, in.WebApplicable, in.WebPass != nil, boolPoints(in.WebPass), "搜索工具与来源"),
		boolSignal("stream", "流式完整性", weightStream, in.StreamPass != nil, in.StreamPass != nil, boolPoints(in.StreamPass), "结束事件与响应完整"),
		{Key: "thinking", Label: "思维链回传", Weight: weightThinking, MaxPoints: 10, Applicable: in.ThinkingApply, Known: in.ThinkingScore != nil, Points: in.ThinkingScore, Reason: "思维摘要 5 分，signature 5 分"},
	}

	applicable, available, earned := 0, 0, 0
	for i := range signals {
		s := &signals[i]
		if !s.Applicable {
			continue
		}
		applicable += s.Weight
		if !s.Known || s.Points == nil {
			continue
		}
		available += s.Weight
		points := *s.Points
		if points < 0 {
			points = 0
		}
		if points > s.MaxPoints {
			points = s.MaxPoints
		}
		earned += s.Weight * points / s.MaxPoints
	}
	result := Result{ApplicableWeight: applicable, AvailableWeight: available, Signals: signals}
	if applicable > 0 {
		result.Coverage = math.Round(float64(available)/float64(applicable)*10000) / 100
	}
	if available == 0 {
		result.Status = "unknown"
		result.Reason = "没有足够的接口证据，未生成完整性分数。"
		return result
	}
	score := int(math.Round(float64(earned) / float64(available) * 100))
	result.Score = &score
	if available == applicable {
		result.Status = "complete"
		result.Reason = "所有适用接口证据均已完成。"
	} else {
		result.Status = "partial"
		result.Reason = "分数按已获得证据归一化；覆盖率不足，不能与完整报告直接横向比较。"
	}
	_ = protocol // retained in Inputs for future protocol-specific weighting.
	return result
}

func boolSignal(key, label string, weight int, applicable, known bool, points *int, reason string) Signal {
	return Signal{Key: key, Label: label, Weight: weight, MaxPoints: 1, Applicable: applicable, Known: known, Points: points, Reason: reason}
}

func boolPoints(v *bool) *int {
	if v == nil {
		return nil
	}
	n := 0
	if *v {
		n = 1
	}
	return &n
}
