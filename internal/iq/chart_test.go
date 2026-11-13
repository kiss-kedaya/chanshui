package iq

import (
	"bytes"
	"image/png"
	"math/rand"
	"strconv"
	"testing"
)

func TestGenChartMeta(t *testing.T) {
	q := GenChart(rand.New(rand.NewSource(7)))
	if q.ID != "vision" || q.Title != "K线读数" || q.MaxScore != 20 || q.LimitSec != 150 {
		t.Fatalf("vision meta: %+v", q)
	}
	if len(q.ImagePNG) == 0 {
		t.Fatal("vision has no image")
	}
	img, err := png.Decode(bytes.NewReader(q.ImagePNG))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() != chartW || b.Dy() != chartH {
		t.Fatalf("image %dx%d, want %dx%d", b.Dx(), b.Dy(), chartW, chartH)
	}
	sum := 0
	for _, c := range q.Answer {
		sum = sum*10 + int(c-'0')
	}
	if sum < 0 || sum > 9*chartBars {
		t.Fatalf("answer %q out of range", q.Answer)
	}
}

func TestJudgeChart(t *testing.T) {
	q := GenChart(rand.New(rand.NewSource(11)))
	want := 0
	for _, c := range q.Answer {
		want = want*10 + int(c-'0')
	}
	near := want + 2
	far := want + 5
	cases := []struct {
		text string
		earn int
		half bool
	}{
		{"最终答案: " + q.Answer, 20, false},
		{q.Answer, 20, false},
		{"最终答案: " + strconv.Itoa(near), 10, true},
		{"最终答案: " + strconv.Itoa(far), 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		earned, half, _ := Judge(q, c.text)
		if earned != c.earn || half != c.half {
			t.Fatalf("%q -> (%d,%v), want (%d,%v)", c.text, earned, half, c.earn, c.half)
		}
	}
}

func TestChartRebuildsFromSeed(t *testing.T) {
	q := GenChart(rand.New(rand.NewSource(3)))
	q.ImagePNG = nil
	again := WithChartImage(q)
	if len(again.ImagePNG) == 0 {
		t.Fatal("image not rebuilt")
	}
	if again.Answer != q.Answer {
		t.Fatalf("answer %q -> %q", q.Answer, again.Answer)
	}
	if !bytes.Equal(again.ImagePNG, WithChartImage(q).ImagePNG) {
		t.Fatal("rebuild is not deterministic")
	}
}

func TestChartBarsAreContinuous(t *testing.T) {
	bars := randomChartBars(42)
	if len(bars) != chartBars {
		t.Fatalf("bars=%d", len(bars))
	}
	for i := 1; i < len(bars); i++ {
		if bars[i][0] != bars[i-1][3] {
			t.Fatalf("bar %d opens at %d, previous close %d", i+1, bars[i][0], bars[i-1][3])
		}
	}
}