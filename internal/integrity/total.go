package integrity

import "math"

// Part is one scored section. Coefficient is its share of 100 after
// inapplicable sections are removed. Contribution is the points that
// actually enter the total.
type Part struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	Applicable   bool   `json:"applicable"`
	Known        bool   `json:"known"`
	Points       int    `json:"points"`
	MaxPoints    int    `json:"max_points"`
	Weight       int    `json:"weight"`
	Coefficient  int    `json:"coefficient"`
	Contribution int    `json:"contribution"`
}

// Total is the report score. A full pass of every applicable section is 100.
type Total struct {
	Score  int    `json:"score"`
	Max    int    `json:"max"`
	Parts  []Part `json:"parts"`
	Reason string `json:"reason"`
}

// Raw weights. Fingerprint is the largest share. The seven weights sum to 100
// when cache and thinking both apply; otherwise they are rescaled.
var sectionWeight = map[string]int{
	"fingerprint": 30,
	"iq":          25,
	"injection":   12,
	"tools":       8,
	"web":         6,
	"thinking":    5,
	"cache":       4,
}

// Compose rescales applicable weights so they sum to 100, then scores each
// section by points/max. Unknown applicable sections contribute 0.
func Compose(parts []Part) Total {
	out := append([]Part(nil), parts...)
	weights := make([]int, 0, len(out))
	index := make([]int, 0, len(out))
	for i := range out {
		p := &out[i]
		p.Weight = sectionWeight[p.Key]
		if p.MaxPoints <= 0 {
			p.MaxPoints = 1
		}
		if p.Points < 0 {
			p.Points = 0
		}
		if p.Points > p.MaxPoints {
			p.Points = p.MaxPoints
		}
		if !p.Applicable {
			p.Coefficient = 0
			p.Contribution = 0
			continue
		}
		weights = append(weights, p.Weight)
		index = append(index, i)
	}
	coeffs := distribute(weights, 100)
	score := 0
	for n, coeff := range coeffs {
		p := &out[index[n]]
		p.Coefficient = coeff
		if !p.Known || p.Points == 0 {
			p.Contribution = 0
			continue
		}
		if p.Points == p.MaxPoints {
			p.Contribution = coeff
		} else {
			p.Contribution = coeff * p.Points / p.MaxPoints
		}
		score += p.Contribution
	}
	return Total{
		Score:  score,
		Max:    100,
		Parts:  out,
		Reason: "总分 100。指纹系数最高。缓存未开启、非 Claude 思维链、Chat Completions 无法核联网时，这些项不占分，其余按原权重重分配，全通过仍是 100。未完成的适用项按 0 分计入。知识截止日期只展示，不计入。",
	}
}

func distribute(weights []int, target int) []int {
	out := make([]int, len(weights))
	if len(weights) == 0 || target <= 0 {
		return out
	}
	sum := 0
	for _, w := range weights {
		if w > 0 {
			sum += w
		}
	}
	if sum == 0 {
		return out
	}
	type frac struct {
		i    int
		rest float64
		raw  int
	}
	used := 0
	rests := make([]frac, 0, len(weights))
	for i, w := range weights {
		exact := float64(w) * float64(target) / float64(sum)
		base := int(math.Floor(exact + 1e-9))
		out[i] = base
		used += base
		rests = append(rests, frac{i: i, rest: exact - float64(base), raw: w})
	}
	for i := 1; i < len(rests); i++ {
		j := i
		for j > 0 && (rests[j].rest > rests[j-1].rest || (rests[j].rest == rests[j-1].rest && rests[j].raw > rests[j-1].raw) || (rests[j].rest == rests[j-1].rest && rests[j].raw == rests[j-1].raw && rests[j].i < rests[j-1].i)) {
			rests[j], rests[j-1] = rests[j-1], rests[j]
			j--
		}
	}
	for n := 0; used < target; n++ {
		out[rests[n%len(rests)].i]++
		used++
	}
	return out
}
