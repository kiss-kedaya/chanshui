package api

import (
	"encoding/json"
	"strings"
	"testing"
)

const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func TestTrimVisionImageMovesPNGToEndpoint(t *testing.T) {
	verdict := json.RawMessage(`{"vision":{"question":{"image":"data:image/png;base64,` + onePixelPNG + `"}}}`)
	out := trimVisionImage(verdict, "abc123")
	if strings.Contains(string(out), "data:image") {
		t.Fatalf("inline image left in report: %s", out)
	}
	var doc struct {
		Vision struct {
			Question struct {
				Image string `json:"image"`
			} `json:"question"`
		} `json:"vision"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if got := doc.Vision.Question.Image; got != "/api/reports/abc123/vision.png" {
		t.Fatalf("image = %q", got)
	}
	if png := visionPNG(verdict); len(png) == 0 {
		t.Fatal("image endpoint would have no bytes")
	}
}
