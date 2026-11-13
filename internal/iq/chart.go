package iq

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Same canvas and layout as the reference generator (make_test.py).
const (
	chartW, chartH = 1360, 1000
	chartBars      = 40

	chartLeft, chartRight      = 150, chartW - 40
	chartTop, chartBottom      = 292, 890
	chartBodyMax, chartBodyMin = 18, 3
)

var (
	chartUp   = color.RGBA{0, 146, 88, 255}
	chartDown = color.RGBA{214, 55, 55, 255}
	chartFlat = color.RGBA{120, 124, 130, 255}
	chartGrid = color.RGBA{226, 230, 234, 255}
	chartAxis = color.RGBA{58, 64, 70, 255}
	chartInk  = color.RGBA{34, 38, 42, 255}
	chartSub  = color.RGBA{95, 101, 108, 255}
	chartBox  = color.RGBA{24, 92, 190, 255}
	chartBan  = color.RGBA{198, 40, 40, 255}
)

// Subset CJK faces (only the glyphs printed on the card) so the image renders
// identically on a server with no system fonts installed.
//
//go:embed fonts/bold.ttf
var chartBoldTTF []byte

//go:embed fonts/reg.ttf
var chartRegTTF []byte

type chartFaceSet struct {
	en, big, ban, sub, sm, title, tick font.Face
}

// opentype.Face owns a rasterizer, so it cannot be shared between concurrent
// chart jobs; only the parsed font is safe to reuse.
var (
	chartParseOnce sync.Once
	chartBoldFont  *opentype.Font
	chartRegFont   *opentype.Font
)

func chartFaces() *chartFaceSet {
	chartParseOnce.Do(func() {
		var err error
		if chartBoldFont, err = opentype.Parse(chartBoldTTF); err != nil {
			panic(err)
		}
		if chartRegFont, err = opentype.Parse(chartRegTTF); err != nil {
			panic(err)
		}
	})
	mk := func(f *opentype.Font, size float64) font.Face {
		face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
		if err != nil {
			panic(err)
		}
		return face
	}
	return &chartFaceSet{
		en: mk(chartBoldFont, 24), big: mk(chartBoldFont, 29), ban: mk(chartBoldFont, 21),
		sub: mk(chartRegFont, 18), sm: mk(chartRegFont, 13), title: mk(chartBoldFont, 24), tick: mk(chartRegFont, 12),
	}
}

func fillBox(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	b := img.Bounds()
	x0, y0 = max(x0, b.Min.X), max(y0, b.Min.Y)
	x1, y1 = min(x1, b.Max.X-1), min(y1, b.Max.Y-1)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func strokeBox(img *image.RGBA, x0, y0, x1, y1, w int, c color.RGBA) {
	for i := 0; i < w; i++ {
		fillBox(img, x0+i, y0+i, x1-i, y0+i, c)
		fillBox(img, x0+i, y1-i, x1-i, y1-i, c)
		fillBox(img, x0+i, y0+i, x0+i, y1-i, c)
		fillBox(img, x1-i, y0+i, x1-i, y1-i, c)
	}
}

// drawStr anchors text by its top-left corner, matching Pillow's default
// "la" anchor instead of Go's baseline anchor.
func drawStr(img *image.RGBA, f font.Face, x, y float64, s string, c color.RGBA) {
	ascent := float64(f.Metrics().Ascent) / 64
	d := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: f,
		Dot: fixed.Point26_6{
			X: fixed.Int26_6(math.Round(x * 64)),
			Y: fixed.Int26_6(math.Round((y + ascent) * 64)),
		},
	}
	d.DrawString(s)
}

func strW(f font.Face, s string) float64 {
	return float64(font.MeasureString(f, s)) / 64
}

// randomChartBars walks a random price path and rounds every candle to whole
// units, mirroring the reference generator exactly.
func randomChartBars(seed int64) [][4]int {
	r := rand.New(rand.NewSource(seed))
	openP := 96 + r.Intn(40)
	drift := 0.0
	out := make([][4]int, 0, chartBars)
	for i := 0; i < chartBars; i++ {
		if r.Float64() < 0.22 {
			drift = r.Float64()*3.6 - 1.8
		}
		// 每根开盘接上一根收盘，图上才不会出现毫无道理的跳空。
		closeP := openP + int(math.Round(drift+r.NormFloat64()*1.9))
		if closeP == openP {
			closeP += 1 - 2*r.Intn(2)
		}
		if closeP < 20 {
			closeP = 20
			if closeP == openP {
				closeP++
			}
		}
		hi := max(openP, closeP) + 1 + r.Intn(2)
		lo := min(openP, closeP) - 1 - r.Intn(2)
		out = append(out, [4]int{openP, hi, lo, closeP})
		openP = closeP
	}
	return out
}

func drawChart(bars [][4]int) []byte {
	n := len(bars)
	ymin, ymax := bars[0][2], bars[0][1]
	for _, b := range bars {
		ymin = min(ymin, b[2])
		ymax = max(ymax, b[1])
	}
	ymin -= 2
	ymax += 2

	f := chartFaces()
	img := image.NewRGBA(image.Rect(0, 0, chartW, chartH))
	fillBox(img, 0, 0, chartW-1, chartH-1, color.RGBA{255, 255, 255, 255})

	// ---- question panel, identical wording to the reference card ----
	strokeBox(img, 30, 22, chartW-30, 236, 3, chartBox)
	drawStr(img, f.en, 52, 34, "ONE INTEGER ONLY / 只回复一个整数", chartBox)
	drawStr(img, f.big, 52, 78, fmt.Sprintf("把下图 %d 根K线的「收盘价个位数字」全部相加，只返回这个和，不要解释。", n), chartInk)
	drawStr(img, f.sub, 52, 126, fmt.Sprintf("Sum the units digits of the %d closing prices. Reply with the number only.", n), chartSub)
	drawStr(img, f.ban, 52, 156, "禁止联网、使用工具或写代码计算。只回答一个整数，不要单位、解释、标点或代码块。", chartBan)
	drawStr(img, f.sm, 52, 196, "No web, no tools, no code. Reply with a single integer only -", chartBan)
	drawStr(img, f.sm, 52, 214, "no units, no explanation, no punctuation, no code block.   Example: close 127 -> 7", chartBan)
	drawStr(img, f.title, chartLeft, chartTop-32, fmt.Sprintf("Daily K-Line Chart (%d bars)", n), chartInk)

	yOf := func(p float64) float64 {
		return float64(chartBottom) - (p-float64(ymin))/(float64(ymax-ymin))*float64(chartBottom-chartTop)
	}

	// ---- grid + value axis ----
	step := 2
	if ymax-ymin <= 10 {
		step = 1
	}
	labelEvery := 1
	if ymax-ymin > 30 {
		labelEvery = 2
	}
	idx := 0
	for y := ymin - ymin%step; y <= ymax; y += step {
		if y >= ymin {
			yy := int(math.Round(yOf(float64(y))))
			fillBox(img, chartLeft, yy, chartRight, yy, chartGrid)
			if idx%labelEvery == 0 {
				lab := strconv.Itoa(y)
				drawStr(img, f.tick, float64(chartLeft)-10-strW(f.tick, lab), float64(yy)-7, lab, chartInk)
			}
		}
		idx++
	}
	fillBox(img, chartLeft, chartTop, chartLeft+1, chartBottom, chartAxis)
	fillBox(img, chartLeft, chartBottom, chartRight, chartBottom+1, chartAxis)

	// ---- candles ----
	slot := float64(chartRight-chartLeft) / float64(n)
	bodyW := int(slot * 0.62)
	bodyW = min(max(bodyW, chartBodyMin), chartBodyMax)
	wick := 1
	if slot >= 12 {
		wick = 2
	}
	for i, b := range bars {
		o, hi, lo, c := b[0], b[1], b[2], b[3]
		cx := float64(chartLeft) + slot*(float64(i)+0.5)
		col := chartUp
		if c < o {
			col = chartDown
		} else if c == o {
			col = chartFlat
		}
		x := int(math.Round(cx))
		fillBox(img, x, int(math.Round(yOf(float64(hi)))), x+wick-1, int(math.Round(yOf(float64(lo)))), col)
		top, bot := yOf(float64(max(o, c))), yOf(float64(min(o, c)))
		if bot-top < 1.5 {
			top -= 0.75
			bot = top + 1.5
		}
		fillBox(img, int(math.Round(cx-float64(bodyW)/2)), int(math.Round(top)),
			int(math.Round(cx+float64(bodyW)/2)), int(math.Round(bot)), col)
	}

	// ---- index axis ----
	labelStep := max(1, int(math.Round(float64(n)/12)))
	for i := 0; i < n; i += labelStep {
		cx := float64(chartLeft) + slot*(float64(i)+0.5)
		lab := fmt.Sprintf("D%02d", i+1)
		drawStr(img, f.tick, cx-strW(f.tick, lab)/2, float64(chartBottom+10), lab, chartInk)
	}
	drawStr(img, f.sm, chartLeft, chartBottom+38, fmt.Sprintf("Bar index D01-D%02d (left to right)", n), chartSub)

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// GenChart is the standalone vision section: a freshly randomized K-line chart plus the sum of the
// closing prices' units digits. The answer is only in the image, so a model
// that cannot see it cannot shortcut the item.
func GenChart(r *rand.Rand) Question {
	return chartFromSeed(r.Int63())
}

func chartFromSeed(seed int64) Question {
	bars := randomChartBars(seed)
	closes := make([]int, 0, len(bars))
	sum := 0
	for _, b := range bars {
		closes = append(closes, b[3])
		sum += b[3] % 10
	}
	prompt := fmt.Sprintf("图中是一段 %d 根 K 线的收盘走势：绿色为上涨，红色为下跌；粗实体是开盘到收盘，上下细线是最高价与最低价，左侧纵轴是价格刻度。绿色 K 线的收盘价在实体上端，红色 K 线的收盘价在实体下端，横轴下方是 K 线编号 D01-D%02d。\n"+
		"把图中 %d 根 K 线「收盘价的个位数字」全部相加。禁止联网、使用工具或写代码计算。只回答本题，不请求提示。最后单独一行以“最终答案: ”开头只写这个整数，不要单位、解释、标点或代码块。",
		len(bars), len(bars), len(bars))
	return Question{
		ID:       "vision",
		Title:    "K线读数",
		Prompt:   prompt,
		MaxScore: 20,
		LimitSec: 150,
		Answer:   strconv.Itoa(sum),
		Tags:     []string{"识图+算术", "随机出图", fmt.Sprintf("种子:%d", seed)},
		ImagePNG: drawChart(bars),
	}
}

// WithChartImage restores the image after a report is reloaded from the
// database (the PNG is not persisted; the seed is).
func WithChartImage(q Question) Question {
	if len(q.ImagePNG) > 0 || q.ID != "vision" {
		return q
	}
	for _, tag := range q.Tags {
		if !strings.HasPrefix(tag, "种子:") {
			continue
		}
		if seed, err := strconv.ParseInt(strings.TrimPrefix(tag, "种子:"), 10, 64); err == nil {
			return chartFromSeed(seed)
		}
	}
	return q
}
