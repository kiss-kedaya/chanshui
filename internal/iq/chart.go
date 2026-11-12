package iq

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"strconv"
	"strings"
)

const (
	chartW, chartH = 1240, 760
	chartBars      = 12
)

var (
	chartUp   = color.RGBA{0, 146, 88, 255}
	chartDown = color.RGBA{214, 55, 55, 255}
	chartGrid = color.RGBA{226, 230, 234, 255}
	chartAxis = color.RGBA{58, 64, 70, 255}
	chartInk  = color.RGBA{34, 38, 42, 255}
)

// 5x7 bitmap digits: no font file on the server, and no dependency.
var digitGlyphs = [10][7]uint8{
	{0b11111, 0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b11111},
	{0b00100, 0b01100, 0b00100, 0b00100, 0b00100, 0b00100, 0b01110},
	{0b11111, 0b00001, 0b00001, 0b11111, 0b10000, 0b10000, 0b11111},
	{0b11111, 0b00001, 0b00001, 0b01111, 0b00001, 0b00001, 0b11111},
	{0b10001, 0b10001, 0b10001, 0b11111, 0b00001, 0b00001, 0b00001},
	{0b11111, 0b10000, 0b10000, 0b11111, 0b00001, 0b00001, 0b11111},
	{0b01111, 0b10000, 0b10000, 0b11111, 0b10001, 0b10001, 0b11111},
	{0b11111, 0b00001, 0b00010, 0b00100, 0b01000, 0b01000, 0b01000},
	{0b11111, 0b10001, 0b10001, 0b11111, 0b10001, 0b10001, 0b11111},
	{0b11111, 0b10001, 0b10001, 0b11111, 0b00001, 0b00001, 0b11111},
}

func fillRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	b := img.Bounds()
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 >= b.Max.X {
		x1 = b.Max.X - 1
	}
	if y1 >= b.Max.Y {
		y1 = b.Max.Y - 1
	}
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func drawDigits(img *image.RGBA, x, y int, s string, scale int, c color.RGBA) int {
	for _, r := range s {
		if r < '0' || r > '9' {
			x += 6 * scale
			continue
		}
		g := digitGlyphs[r-'0']
		for row := 0; row < 7; row++ {
			for col := 0; col < 5; col++ {
				if g[row]&(1<<(4-col)) != 0 {
					fillRect(img, x+col*scale, y+row*scale, x+col*scale+scale-1, y+row*scale+scale-1, c)
				}
			}
		}
		x += 6 * scale
	}
	return x
}

func digitWidth(s string, scale int) int {
	if s == "" {
		return 0
	}
	return len(s)*6*scale - scale
}

// chartBars walks a random price path and rounds every candle to whole units,
// so the closing prices read back exactly off the axis.
func randomChartBars(seed int64) [][4]int {
	r := rand.New(rand.NewSource(seed))
	price := 60 + r.Float64()*80
	drift := 0.0
	out := make([][4]int, 0, chartBars)
	for i := 0; i < chartBars; i++ {
		if r.Float64() < 0.22 {
			drift = r.Float64()*3.6 - 1.8
		}
		price += drift + r.NormFloat64()*1.9
		if price < 20 {
			price = 20
		}
		closeP := int(math.Round(price))
		openP := int(math.Round(price - drift*0.6 + r.NormFloat64()*1.2))
		if closeP == openP {
			closeP++
		}
		hi := max(openP, closeP) + 1 + r.Intn(2)
		lo := min(openP, closeP) - 1 - r.Intn(2)
		out = append(out, [4]int{openP, hi, lo, closeP})
		price = float64(closeP)
	}
	return out
}

func drawChart(bars [][4]int) []byte {
	const (
		left, right = 110, 1200
		top, bottom = 60, 650
	)
	ymin := bars[0][2]
	ymax := bars[0][1]
	for _, b := range bars {
		ymin = min(ymin, b[2])
		ymax = max(ymax, b[1])
	}
	ymin -= 2
	ymax += 2
	span := float64(ymax - ymin)
	yOf := func(p float64) int {
		return int(math.Round(float64(bottom) - (p-float64(ymin))/span*float64(bottom-top)))
	}

	every := 1
	pxPerUnit := float64(bottom-top) / span
	for _, e := range []int{1, 2, 5, 10, 20} {
		every = e
		if float64(e)*pxPerUnit >= 26 {
			break
		}
	}

	img := image.NewRGBA(image.Rect(0, 0, chartW, chartH))
	fillRect(img, 0, 0, chartW-1, chartH-1, color.RGBA{255, 255, 255, 255})

	for v := ymin - ymin%every; v <= ymax; v += every {
		if v < ymin {
			continue
		}
		y := yOf(float64(v))
		fillRect(img, left, y, right, y, chartGrid)
		label := strconv.Itoa(v)
		drawDigits(img, left-14-digitWidth(label, 2), y-7, label, 2, chartInk)
	}
	fillRect(img, left, top, left+1, bottom, chartAxis)
	fillRect(img, left, bottom, right, bottom+1, chartAxis)

	slot := float64(right-left) / float64(len(bars))
	bodyW := int(slot * 0.5)
	if bodyW > 44 {
		bodyW = 44
	}
	if bodyW < 10 {
		bodyW = 10
	}
	for i, b := range bars {
		o, hi, lo, c := b[0], b[1], b[2], b[3]
		cx := int(float64(left) + slot*(float64(i)+0.5))
		col := chartUp
		if c < o {
			col = chartDown
		}
		fillRect(img, cx-1, yOf(float64(hi)), cx+1, yOf(float64(lo)), col)
		topY, botY := yOf(float64(max(o, c))), yOf(float64(min(o, c)))
		if botY-topY < 6 {
			topY, botY = topY-3, topY+3
		}
		fillRect(img, cx-bodyW/2, topY, cx+bodyW/2, botY, col)
		label := strconv.Itoa(i + 1)
		drawDigits(img, cx-digitWidth(label, 2)/2, bottom+16, label, 2, chartInk)
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// GenChart is Q10: a freshly randomized K-line chart plus the sum of the
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
	prompt := fmt.Sprintf("图中是一段 %d 根 K 线的收盘走势：绿色为上涨，红色为下跌；粗实体是开盘到收盘，上下细线是最高价与最低价，左侧纵轴是价格刻度。绿色 K 线的收盘价在实体上端，红色 K 线的收盘价在实体下端，横轴下方是 K 线编号 1-%d。\n"+
		"把图中 %d 根 K 线「收盘价的个位数字」全部相加。禁止联网、使用工具或写代码计算。只回答本题，不请求提示。最后单独一行以“最终答案: ”开头只写这个整数，不要单位、解释、标点或代码块。",
		len(bars), len(bars), len(bars))
	return Question{
		ID:       "Q10",
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
	if len(q.ImagePNG) > 0 || q.ID != "Q10" {
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
