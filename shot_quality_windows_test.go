package main

import (
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestQualityLevelsResizeCopies(t *testing.T) {
	wide := image.NewRGBA(image.Rect(0, 0, 3000, 1000))
	low := qualityLow.resizeCopy(wide)
	if low.Bounds().Dx() != lowQualityLongestSide || low.Bounds().Dy() >= 1000 {
		t.Fatalf("low quality should cap the longest side: %v", low.Bounds())
	}
	if got := qualityMedium.resizeCopy(wide); got != image.Image(wide) {
		t.Fatal("medium must keep the original pixels")
	}
	if got := qualityHigh.resizeCopy(wide); got != image.Image(wide) {
		t.Fatal("high must keep the original pixels")
	}
	small := image.NewRGBA(image.Rect(0, 0, 640, 360))
	if got := qualityLow.resizeCopy(small); got != image.Image(small) {
		t.Fatal("low quality must not enlarge small selections")
	}
}

func TestQualityDefaultsToMediumAndIsSelectable(t *testing.T) {
	if qualityMedium.label() != "中" {
		t.Fatalf("medium label: %s", qualityMedium.label())
	}
	s := testSelection()
	s.quality = qualityMedium
	tt := ui.NewTester(s.view, 400, 300)
	for _, label := range []string{"低", "中", "高"} {
		if !tt.HasText(label) {
			t.Fatalf("quality option %q missing", label)
		}
	}
	if err := tt.Click("图片质量 低"); err != nil {
		t.Fatal(err)
	}
	if s.quality != qualityLow {
		t.Fatalf("clicking 低 must select low quality, got %v", s.quality)
	}
}
