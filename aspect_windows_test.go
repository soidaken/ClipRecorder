package main

import (
	"image"
	"math"
	"testing"

	"cliprecorder/internal/recording"
	"github.com/egoist/mygo/ui"
)

func TestAspectChoices(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1000, 700))
	s := &selection{record: true, selected: true, rect: ui.Rect{X: 100, Y: 30, W: 360, H: 200}, desktop: img.Bounds(), snapshot: img, bitmap: ui.NewBitmap(img), scale: 1, options: recording.DefaultOptions(), fpsIndex: 1, hardwareChecked: true}
	tt := ui.NewTester(s.view, 1000, 700)
	if s.aspect != 0 {
		t.Fatal("ratio must start unselected")
	}
	for i, ratio := range recordingRatios {
		if err := tt.Click(ratio.label); err != nil {
			t.Fatal(err)
		}
		r := s.physicalRect()
		if s.aspect != i+1 || r.Dx()%2 != 0 || r.Dy()%2 != 0 || r.Dx()*int(ratio.h) != r.Dy()*int(ratio.w) {
			t.Fatalf("ratio %s: %+v %v", ratio.label, s.rect, r)
		}
		if !r.In(img.Bounds()) {
			t.Fatalf("ratio outside screen: %v", r)
		}
	}
	before := s.rect
	if err := tt.Click("1:1"); err != nil {
		t.Fatal(err)
	}
	if s.aspect != 0 || s.rect != before {
		t.Fatal("clicking selected ratio must restore free selection without resizing")
	}
	for _, ratio := range recordingRatios {
		r := fitAspect(ui.Rect{X: 950, Y: 650, W: 200, H: 200}, ratio.w/ratio.h, 1000, 700)
		if r.X < 0 || r.Y < 0 || r.X+r.W > 1000.01 || r.Y+r.H > 700.01 || math.Abs(float64(r.W/r.H-ratio.w/ratio.h)) > 0.001 {
			t.Fatalf("ratio fit: %+v", r)
		}
	}
}
