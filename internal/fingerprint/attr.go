package fingerprint

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
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
	hp := unit(projectOut(standardize(HellingerFeature(CountNumbers(nums)), h.FeatureMean, h.FeatureScale), h.NuisanceBasis))
	hell := make([]float64, n)
	for c := range hell {
		hell[c] = dot(hp, h.Centroids[c])
	}
	hell = standardizeVec(hell)
	os := standardize(OrderedBlockFeature(nums), ob.FeatureMean, ob.FeatureScale)
	norm := unit(os)
	op := unit(projectOut(os, ob.NuisanceBasis))
	template, nuisance := make([]float64, n), make([]float64, n)
	for c := 0; c < n; c++ {
		template[c] = math.Inf(-1)
		for _, env := range ob.EnvironmentCentroids {
			template[c] = math.Max(template[c], dot(norm, env[c]))
		}
		if math.IsInf(template[c], -1) {
			template[c] = 0
		}
		nuisance[c] = dot(op, ob.Centroids[c])
	}
	template = standardizeVec(template)
	nuisance = standardizeVec(nuisance)
	ordered := make([]float64, n)
	for c := range ordered {
		ordered[c] = 0.5 * (template[c] + nuisance[c])
	}
	ordered = standardizeVec(ordered)
	fused := make([]float64, n)
	for c := range fused {
		fused[c] = (1-ob.Weight)*hell[c] + ob.Weight*ordered[c]
	}
	return fused
}
func dot(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}
func unit(a []float64) []float64 {
	out := make([]float64, len(a))
	norm := math.Max(math.Sqrt(dot(a, a)), 1e-12)
	for i := range a {
		out[i] = a[i] / norm
	}
	return out
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
		if nums := ParseNumbers(o.Text); len(nums) >= maxInt(minValid, int(math.Ceil(float64(o.ExpectedCount)*0.55))) {
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
	winningFamily := top.Family
	for family, prob := range familyProb {
		if prob > familyProb[winningFamily] {
			winningFamily = family
		}
	}
	famName := ""
	for _, m := range bank.Numeric.Models {
		if m.Family == winningFamily {
			famName = m.FamilyName
			break
		}
	}
	return Attribution{
		Family:        winningFamily,
		FamilyName:    famName,
		FamilyProb:    familyProb[winningFamily],
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
		pb[i] = (float64(b[i]) + 0.5) / (float64(sb) + 0.5*float64(len(b)))
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
	return 1 - math.Sqrt(js)
}

// ---- challenge generation (ModelTrace generate_challenges semantics) ----

var chOpenings = []string{"这是一次独立的数值选择记录", "请完成下面的无语义整数选择任务", "执行一次第一反应取值记录", "生成一组不承载语义的整数选择", "进行一轮快速逐项取值"}
var chActions = []string{"为各个位置分别凭第一反应选择", "逐项选择", "每次只决定当前一项，共给出", "分别凭第一反应给出", "逐个直接选择"}
var chEndings = []string{"允许某个数字再次出现；每项写出后不要回头排序、去重或替换。", "偶然重复是有效的；不要重新排列或修正已经写出的项目。", "相同值可以再次出现；输出过程中不要整理或改写前面的项目。", "重复值无需删除；不要筛选、重排或补成某种规律。", "不必赋予数字任何含义；已经给出的值保持不变。"}
var chSeparatorHints = []string{"数字之间用逗号或空格分隔均可。", "使用一种一致的常见分隔符即可。", "可以用逗号、空格或换行分隔。", "只要每个整数边界清楚，格式可自行选择。"}

const chToolBan = "本任务必须由当前语言模型直接完成：禁止调用或借助任何工具，包括 Python、代码执行器、计算器、搜索、API 和外部随机数生成器；也不要先编写或运行代码。"
const chDirect = "直接从第一个取值开始输出，不要在序列前重复数量、范围或任务说明。"

func GenerateChallenges(n int, r *rand.Rand) []Challenge {
	if n > 41 {
		n = 41
	}
	lengths := r.Perm(41)
	out := make([]Challenge, n)
	for i := range out {
		count := 292 + lengths[i]
		prompt := fmt.Sprintf("%s。%s %d 个 1 到 355（含端点）的整数。", pick(r, chOpenings), pick(r, chActions), count) + "每个位置都要单独选择；不要从 1 开始计数，不要连续递增或递减，也不要采用等差、循环、重复区块或其他规则化模式。" + chToolBan + pick(r, chEndings) + pick(r, chSeparatorHints) + chDirect
		out[i] = Challenge{ID: fmt.Sprintf("numeric-%d", i+1), ExpectedCount: count, Prompt: prompt}
	}
	return out
}
func pick(r *rand.Rand, arr []string) string { return arr[r.Intn(len(arr))] }

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
			dot += v[i] * row[i]
			norm += row[i] * row[i]
		}
		if norm == 0 {
			continue
		}
		coef := dot
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
