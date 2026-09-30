package fingerprint

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
)

// Output is one probe response for numeric attribution.
type Output struct {
	Text          string
	ExpectedCount int
}

// Challenge is one generated numeric fingerprint probe.
type Challenge struct {
	ID            string
	ExpectedCount int
	Prompt        string
}

// Result is the per-model attribution score for one model.
type Result struct {
	ModelID      string  `json:"modelId"`
	DisplayName  string  `json:"displayName"`
	Family       string  `json:"family"`
	Score        float64 `json:"score"`
	Prob         float64 `json:"prob"`
	JSSimilarity float64 `json:"jsSimilarity"`
}

// Attribution is the aggregated numeric fingerprint verdict.
type Attribution struct {
	Family        string   `json:"family"`
	FamilyName    string   `json:"familyName"`
	FamilyProb    float64  `json:"familyProb"`
	TopModel      string   `json:"topModel"`
	TopProb       float64  `json:"topProb"`
	Results       []Result `json:"results"`
	UsedOutputs   int      `json:"usedOutputs"`
	LowConfidence bool     `json:"lowConfidence"`
}

const jsAlpha = 0.5

func calibKey(n int) string {
	k := n
	if k > 3 {
		k = 3
	}
	return fmt.Sprintf("%d", k)
}

// RobustScoreNumbers computes the 16 fused per-model z-scores for one number
// sequence, porting the ModelTrace robust scoring semantics:
// hellinger side (standardize -> project out nuisance -> cosine with
// centroids -> standardize) and ordered side (same flow plus per-environment
// template max), fused with the bank's ordered-block weight.
func RobustScoreNumbers(nums []int, bank *Bank) []float64 {
	if len(nums) == 0 {
		return nil
	}
	n := len(bank.Numeric.Models)
	h := bank.Numeric.Robust.Hellinger
	ob := bank.Numeric.Robust.OrderedBlocks

	// Hellinger side.
	hf := HellingerFeature(CountNumbers(nums))
	hStd := standardize(hf, h.FeatureMean, h.FeatureScale)
	hProj := projectOut(hStd, h.NuisanceBasis)
	hCos := make([]float64, n)
	for c := 0; c < n; c++ {
		cv := projectOut(h.Centroids[c], h.NuisanceBasis)
		hCos[c] = cosine(hProj, cv)
	}
	hell := standardizeVec(hCos)

	// Ordered side: direct centroids + per-environment template max, fused 0.5/0.5.
	of := OrderedBlockFeature(nums)
	oStd := standardize(of, ob.FeatureMean, ob.FeatureScale)
	oProj := projectOut(oStd, ob.NuisanceBasis)
	oCos := make([]float64, n)
	for c := 0; c < n; c++ {
		cv := projectOut(ob.Centroids[c], ob.NuisanceBasis)
		oCos[c] = cosine(oProj, cv)
	}
	oZ := standardizeVec(oCos)

	envZ := make([][]float64, len(ob.EnvironmentCentroids))
	for e, env := range ob.EnvironmentCentroids {
		eCos := make([]float64, n)
		for c := 0; c < n; c++ {
			ev := projectOut(env[c], ob.NuisanceBasis)
			eCos[c] = cosine(oProj, ev)
		}
		envZ[e] = standardizeVec(eCos)
	}
	envMax := make([]float64, n)
	for c := 0; c < n; c++ {
		m := envZ[0][c]
		for e := 1; e < len(envZ); e++ {
			if envZ[e][c] > m {
				m = envZ[e][c]
			}
		}
		envMax[c] = m
	}

	ordered := make([]float64, n)
	for c := 0; c < n; c++ {
		ordered[c] = 0.5*oZ[c] + 0.5*envMax[c]
	}

	// Final fusion: (1-w) hellinger + w ordered; weight from the bank.
	w := ob.Weight
	fused := make([]float64, n)
	for c := 0; c < n; c++ {
		fused[c] = (1-w)*hell[c] + w*ordered[c]
	}
	return fused
}

// Attribute aggregates per-response scores across valid outputs, applies the
// calibration beta, and aggregates family probabilities.
func Attribute(outputs []Output, bank *Bank) Attribution {
	minValid := bank.Numeric.MinimumValidNumbers
	var fused [][]float64
	for _, o := range outputs {
		nums := ParseNumbers(o.Text)
		if len(nums) < maxInt(minValid, int(math.Ceil(float64(o.ExpectedCount)*0.55))) {
			continue
		}
		if s := RobustScoreNumbers(nums, bank); s != nil {
			fused = append(fused, s)
		}
	}
	n := len(bank.Numeric.Models)
	if len(fused) == 0 {
		return Attribution{Results: []Result{}}
	}

	score := make([]float64, n)
	for _, s := range fused {
		for c := 0; c < n; c++ {
			score[c] += s[c]
		}
	}
	for c := range score {
		score[c] /= float64(len(fused))
	}

	beta := bank.Numeric.Calibration[calibKey(len(fused))].Beta
	probs := softmax(beta, score)

	// Aggregate query counts across valid outputs for js_similarity.
	queryCounts := make([]int, NumDim)
	for _, o := range outputs {
		if nums := ParseNumbers(o.Text); len(nums) >= minValid {
			for _, v := range nums {
				queryCounts[v-NumMinValue]++
			}
		}
	}

	results := make([]Result, 0, n)
	familyProb := map[string]float64{}
	for c, m := range bank.Numeric.Models {
		results = append(results, Result{
			ModelID:      m.ID,
			DisplayName:  m.DisplayName,
			Family:       m.Family,
			Score:        score[c],
			Prob:         probs[c],
			JSSimilarity: jsSimilarity(queryCounts, m.Counts),
		})
		familyProb[m.Family] += probs[c]
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Prob != results[j].Prob {
			return results[i].Prob > results[j].Prob
		}
		return results[i].ModelID < results[j].ModelID
	})

	top := results[0]
	famName := ""
	for _, m := range bank.Numeric.Models {
		if m.Family == top.Family {
			famName = m.FamilyName
			break
		}
	}
	return Attribution{
		Family:        top.Family,
		FamilyName:    famName,
		FamilyProb:    familyProb[top.Family],
		TopModel:      top.ModelID,
		TopProb:       top.Prob,
		Results:       results,
		UsedOutputs:   len(fused),
		LowConfidence: top.Prob < 0.6,
	}
}

func softmax(beta float64, score []float64) []float64 {
	mx := score[0]
	for _, s := range score[1:] {
		if s > mx {
			mx = s
		}
	}
	out := make([]float64, len(score))
	sum := 0.0
	for i, s := range score {
		out[i] = math.Exp(beta * (s - mx))
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

// jsSimilarity ports the ModelTrace js_similarity (ALPHA=0.5) between two
// count histograms, returning 1 - Jensen-Shannon distance in bits.
func jsSimilarity(a, b []int) float64 {
	if len(a) != len(b) {
		return 0
	}
	pa := make([]float64, len(a))
	pb := make([]float64, len(b))
	sa, sb := 0, 0
	for i := range a {
		sa += a[i]
		sb += b[i]
	}
	if sa == 0 || sb == 0 {
		return 0
	}
	for i := range a {
		pa[i] = float64(a[i]) / float64(sa)
		pb[i] = float64(b[i]) / float64(sb)
	}
	js := 0.0
	for i := range a {
		pm := 0.5 * (pa[i] + pb[i])
		if pa[i] > 0 {
			js += jsAlpha * pa[i] * math.Log2(pa[i]/pm)
		}
		if pb[i] > 0 {
			js += (1 - jsAlpha) * pb[i] * math.Log2(pb[i]/pm)
		}
	}
	return 1 - js
}

// ---- challenge generation (ModelTrace generate_challenges semantics) ----

var chOpenings = []string{
	"请仔细完成以下任务。",
	"我们来进行一个小测试。",
	"现在开始一个简单的序列任务。",
	"请按照要求处理下面的数字任务。",
	"接下来是一个纯文本的数字任务。",
}

var chActions = []string{
	"请根据下方的规则，在 1 到 355 的范围内挑选数字并逐个输出。",
	"请从 1 到 355 的范围内选取符合描述的数字并按顺序写出。",
	"请在 1 到 355 的整数范围内生成一列数字并全部列出。",
	"下面请你以 1 到 355 为取值范围生成一列数字并完整输出。",
	"请在 1 到 355 范围内按要求生成数字并依次罗列出来。",
}

var chEndings = []string{
	"请一次性给完整序列。",
	"输出请保持连贯，不要中断。",
	"把整列数字完整写在回复里。",
	"全部数字请写在同一个回复中。",
	"请连续输出，不要分多段。",
}

var chSeparatorHints = []string{
	"数字之间用单个空格分隔。",
	"数字之间用逗号加空格分隔。",
	"数字之间用换行分隔。",
	"数字之间用顿号分隔。",
}

const (
	chToolBan  = "禁止使用任何工具或联网功能。"
	chDirect   = "直接输出数字序列本身，不要附加任何解释或说明。"
	chCountAsk = "总共输出 300 个数字。"
	chPadding  = "请注意保持输出的稳定性与一致性。"
)

// GenerateChallenges builds n numeric fingerprint prompts. Prompt length is
// randomized in [292, 333] runes, matching the ModelTrace challenge lengths.
func GenerateChallenges(n int, r *rand.Rand) []Challenge {
	chs := make([]Challenge, n)
	for i := 0; i < n; i++ {
		chs[i] = Challenge{
			ID:            fmt.Sprintf("numeric-%d", i+1),
			ExpectedCount: 300,
			Prompt:        buildChallengePrompt(r),
		}
	}
	return chs
}

func buildChallengePrompt(r *rand.Rand) string {
	for attempt := 0; attempt < 64; attempt++ {
		var b strings.Builder
		b.WriteString(pick(r, chOpenings))
		b.WriteString(pick(r, chActions))
		b.WriteString(chCountAsk)
		b.WriteString(pick(r, chSeparatorHints))
		b.WriteString(pick(r, chEndings))
		b.WriteString(chToolBan)
		b.WriteString(chDirect)
		s := b.String()
		l := len([]rune(s))
		if l >= 292 && l <= 333 {
			return s
		}
		if l < 292 {
			s = strings.Replace(s, pick(r, chEndings), pick(r, chEndings)+chPadding, 1)
			if ll := len([]rune(s)); ll >= 292 && ll <= 333 {
				return s
			}
			continue
		}
		// too long: try again with different picks
	}
	// Deterministic fallback inside the range.
	base := pick(r, chOpenings) + pick(r, chActions) + chCountAsk + pick(r, chSeparatorHints) + chToolBan + chDirect
	pad := 292 - len([]rune(base))
	if pad > 0 {
		base += strings.Repeat("。", pad)
	}
	return base
}

func pick(r *rand.Rand, arr []string) string {
	return arr[r.Intn(len(arr))]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---- linear algebra helpers shared by feature/attr ----

func standardize(v, mean, scale []float64) []float64 {
	out := make([]float64, len(v))
	for i := range v {
		s := scale[i]
		if s == 0 {
			s = 1
		}
		out[i] = (v[i] - mean[i]) / s
	}
	return out
}

// standardizeVec z-scores a vector across its own entries.
func standardizeVec(v []float64) []float64 {
	n := len(v)
	mean := 0.0
	for _, x := range v {
		mean += x
	}
	mean /= float64(n)
	var std float64
	for _, x := range v {
		std += (x - mean) * (x - mean)
	}
	std = math.Sqrt(std / float64(n))
	if std == 0 {
		std = 1
	}
	out := make([]float64, n)
	for i, x := range v {
		out[i] = (x - mean) / std
	}
	return out
}

// projectOut removes each basis row's component from v (Gram-Schmidt style,
// matching the nuisance projection of the bank).
func projectOut(v []float64, basis [][]float64) []float64 {
	out := make([]float64, len(v))
	copy(out, v)
	for _, row := range basis {
		dot := 0.0
		norm := 0.0
		for i := range out {
			dot += out[i] * row[i]
			norm += row[i] * row[i]
		}
		if norm == 0 {
			continue
		}
		coef := dot / norm
		for i := range out {
			out[i] -= coef * row[i]
		}
	}
	return out
}

func cosine(a, b []float64) float64 {
	dot, na, nb := 0.0, 0.0, 0.0
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}
