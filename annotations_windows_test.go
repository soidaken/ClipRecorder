package main

import (
	"image"
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

func testSelection() *selection {
	snap := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			g := uint8(40 + (x+y)%40)
			snap.SetRGBA(x, y, color.RGBA{g, g, g + 10, 255})
		}
	}
	return &selection{
		desktop:  image.Rect(0, 0, 400, 300),
		snapshot: snap,
		bitmap:   ui.NewBitmap(snap),
		scale:    1,
		selected: true,
		rect:     ui.Rect{X: 50, Y: 40, W: 300, H: 200},
	}
}

func TestNumberMarkersIncrement(t *testing.T) {
	s := testSelection()
	s.tool = toolNumber
	s.handleAnnotationInput(100, 100, true, true)
	s.handleAnnotationInput(100, 100, false, false)
	s.handleAnnotationInput(150, 120, true, true)
	s.handleAnnotationInput(150, 120, false, false)
	if s.numberCount != 2 || len(s.annotations) != 2 {
		t.Fatalf("markers: count %d, annotations %d", s.numberCount, len(s.annotations))
	}
	if s.annotations[0].number != 1 || s.annotations[1].number != 2 {
		t.Fatalf("numbers out of order: %+v", s.annotations)
	}
	first := s.annotations[0].rect
	if first.W != 2*markerRadius || first.X+first.W/2 != 100 {
		t.Fatalf("marker geometry wrong: %+v", first)
	}
}

func TestMarkersClampInsideSelection(t *testing.T) {
	s := testSelection()
	s.tool = toolNumber
	s.handleAnnotationInput(51, 41, true, true)
	s.handleAnnotationInput(51, 41, false, false)
	r := s.annotations[0].rect
	if r.X < s.rect.X || r.Y < s.rect.Y {
		t.Fatalf("marker spilled out of the selection: %+v", r)
	}
}

func TestPenCollectsStrokePoints(t *testing.T) {
	s := testSelection()
	s.tool = toolPen
	s.handleAnnotationInput(80, 80, true, true)
	s.handleAnnotationInput(110, 96, true, true)
	s.handleAnnotationInput(140, 120, true, true)
	s.handleAnnotationInput(140, 120, false, false)
	if len(s.annotations) != 1 || s.annotations[0].kind != annotationPen {
		t.Fatalf("pen stroke missing: %+v", s.annotations)
	}
	if got := len(s.annotations[0].points); got != 3 {
		t.Fatalf("pen points = %d, want 3", got)
	}
	if s.drawing {
		t.Fatal("pen must stop drawing on release")
	}
}

func TestBlurBoxIsAddedOnRelease(t *testing.T) {
	s := testSelection()
	s.tool = toolBlur
	s.handleAnnotationInput(120, 90, true, true)
	s.handleAnnotationInput(260, 180, true, true)
	if len(s.annotations) != 0 {
		t.Fatal("blur must not apply before release")
	}
	s.handleAnnotationInput(260, 180, false, false)
	if len(s.annotations) != 1 || s.annotations[0].kind != annotationBlur {
		t.Fatalf("blur box missing: %+v", s.annotations)
	}
	if s.annotations[0].blur == nil {
		t.Fatal("blurred bitmap not prepared")
	}
	box := s.annotations[0].rect
	if box.W != 140 || box.H != 90 {
		t.Fatalf("blur box = %+v", box)
	}
}

func TestBlurSoftensDetail(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 120, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 120; x++ {
			v := uint8(0)
			if (x/4+y/4)%2 == 0 {
				v = 255
			}
			img.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	blurred := blurRGBA(img, 6)
	if variance(blurred) >= variance(img) {
		t.Fatalf("blur did not soften detail: %v >= %v", variance(blurred), variance(img))
	}
	flat := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for i := range flat.Pix {
		flat.Pix[i] = 200
	}
	if got := blurRGBA(flat, 4).RGBAAt(10, 10); got.R != 200 || got.G != 200 || got.B != 200 {
		t.Fatalf("flat color changed: %+v", got)
	}
}

func variance(img *image.RGBA) float64 {
	var sum, sumSq float64
	n := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			v := float64(img.RGBAAt(x, y).R)
			sum += v
			sumSq += v * v
			n++
		}
	}
	mean := sum / float64(n)
	return sumSq/float64(n) - mean*mean
}

func TestAnnotationsResetWithNewSelection(t *testing.T) {
	s := testSelection()
	s.tool = toolNumber
	s.handleAnnotationInput(100, 100, true, true)
	s.handleAnnotationInput(100, 100, false, false)
	s.handleAnnotationInput(140, 140, true, true)
	s.handleAnnotationInput(140, 140, false, false)
	s.tool = toolBlur
	s.handleAnnotationInput(120, 90, true, true)
	s.handleAnnotationInput(200, 160, false, false)
	s.resetAnnotations()
	if len(s.annotations) != 0 || s.numberCount != 0 {
		t.Fatalf("annotations survived a new selection: %+v", s)
	}
}

func TestAnnotationToolTogglesOff(t *testing.T) {
	s := testSelection()
	s.setTool(toolPen)
	if s.tool != toolPen {
		t.Fatal("pen not selected")
	}
	s.setTool(toolPen)
	if s.tool != toolNone {
		t.Fatal("second click must return to plain selection")
	}
}

func TestShotToolbarHasNoCancelButton(t *testing.T) {
	s := testSelection()
	tt := ui.NewTester(s.view, 400, 300)
	if !tt.HasText("确认") {
		t.Fatal("copy button missing")
	}
	if !tt.HasText("Esc 取消") && !hasWhitePixels(tt, 400, 300) {
		t.Fatal("bottom hint missing")
	}
	for _, text := range tt.Texts() {
		if text == "取消" {
			t.Fatal("cancel button must be gone from the shot toolbar")
		}
	}
	if err := tt.Click("画笔"); err != nil {
		t.Fatal(err)
	}
	if s.tool != toolPen {
		t.Fatalf("pen button did not activate the tool: %v", s.tool)
	}
	if err := tt.Click("画笔"); err != nil {
		t.Fatal(err)
	}
	if s.tool != toolNone {
		t.Fatalf("pen button did not toggle off: %v", s.tool)
	}
}

func TestMarkersAndPenAreVisible(t *testing.T) {
	s := testSelection()
	s.tool = toolNumber
	s.handleAnnotationInput(200, 140, true, true)
	s.handleAnnotationInput(200, 140, false, false)
	s.tool = toolPen
	s.handleAnnotationInput(80, 200, true, true)
	s.handleAnnotationInput(200, 240, true, true)
	s.handleAnnotationInput(200, 240, false, false)
	tt := ui.NewTester(s.view, 400, 300)
	if p := tt.Image().RGBAAt(206, 140); p.R < 180 || p.G > 110 || p.B > 110 {
		t.Fatalf("marker is not red: %+v", p)
	}
	if p := tt.Image().RGBAAt(140, 220); p.R < 150 || p.G > 120 || p.B > 120 {
		t.Fatalf("pen stroke is not red: %+v", p)
	}
}

func TestMarkerIsRedWithWhiteDigits(t *testing.T) {
	s := testSelection()
	s.tool = toolNumber
	s.handleAnnotationInput(200, 140, true, true)
	s.handleAnnotationInput(200, 140, false, false)
	tt := ui.NewTester(s.view, 400, 300)
	white, red := 0, 0
	for y := 128; y < 152; y++ {
		for x := 188; x < 212; x++ {
			p := tt.Image().RGBAAt(x, y)
			if p.R > 240 && p.G > 240 && p.B > 240 {
				white++
			}
			if p.R > 180 && p.G < 110 && p.B < 110 {
				red++
			}
		}
	}
	if white == 0 {
		t.Fatal("marker digit is not white")
	}
	if red == 0 {
		t.Fatal("marker has no red background")
	}
}

func TestBlurBoxHasRoundedCorners(t *testing.T) {
	s := testSelection()
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			v := uint8(0)
			if (x/4+y/4)%2 == 0 {
				v = 255
			}
			s.snapshot.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	s.bitmap = ui.NewBitmap(s.snapshot)
	s.tool = toolBlur
	s.handleAnnotationInput(120, 90, true, true)
	s.handleAnnotationInput(260, 190, true, true)
	s.handleAnnotationInput(260, 190, false, false)
	box := s.annotations[0].rect
	tt := ui.NewTester(s.view, 400, 300)
	// The center is blurred, so the checker loses its contrast.
	center := tt.Image().RGBAAt(int(box.X+box.W/2), int(box.Y+box.H/2))
	if center.R < 90 || center.R > 165 {
		t.Fatalf("blur center not blurred: %+v", center)
	}
	// The sharp corner of the box stays outside the rounded corner radius.
	corner := tt.Image().RGBAAt(int(box.X), int(box.Y))
	if corner.R < 250 {
		t.Fatalf("rounded corner must leave the original pixel: %+v", corner)
	}
}

func TestMarkerDigitsLookCentered(t *testing.T) {
	for _, scale := range []float64{1, 1.5} {
		s := testSelection()
		s.scale = scale
		s.tool = toolNumber
		cx, cy := 200, 140
		for _, n := range []int{1, 8, 12, 25, 108} {
			s.annotations = nil
			s.numberCount = n
			s.handleAnnotationInput(float32(cx), float32(cy), true, true)
			s.handleAnnotationInput(float32(cx), float32(cy), false, false)
			tt := ui.NewTester(s.view, 400, 300)
			img := tt.Image()
			inkX0, inkY0, inkX1, inkY1 := 9999, 9999, -1, -1
			redX0, redY0, redX1, redY1 := 9999, 9999, -1, -1
			for y := cy - 30; y < cy+30; y++ {
				for x := cx - 30; x < cx+30; x++ {
					p := img.RGBAAt(x, y)
					if p.R > 250 && p.G > 250 && p.B > 250 {
						inkX0, inkY0 = min(inkX0, x), min(inkY0, y)
						inkX1, inkY1 = max(inkX1, x), max(inkY1, y)
					}
					if p.R > 180 && p.G < 90 && p.B < 70 {
						redX0, redY0 = min(redX0, x), min(redY0, y)
						redX1, redY1 = max(redX1, x), max(redY1, y)
					}
				}
			}
			if inkX1 < 0 || redX1 < 0 {
				t.Fatalf("scale %v: marker %d not rendered", scale, n)
			}
			inkCX, inkCY := float64(inkX0+inkX1)/2, float64(inkY0+inkY1)/2
			redCX, redCY := float64(redX0+redX1)/2, float64(redY0+redY1)/2
			if abs(float32(inkCX-redCX)) > 1.6 || abs(float32(inkCY-redCY)) > 1.6 {
				t.Fatalf("scale %v, marker %d: digits off center by (%.1f, %.1f)", scale, n, inkCX-redCX, inkCY-redCY)
			}
			if inkX0 <= redX0 || inkX1 >= redX1 || inkY0 <= redY0 || inkY1 >= redY1 {
				t.Fatalf("scale %v, marker %d: digits reach the circle edge (ink %d-%d, circle %d-%d)", scale, n, inkX0, inkX1, redX0, redX1)
			}
		}
	}
}

func TestDragInFromOutsideAddsNoMarker(t *testing.T) {
	s := testSelection()
	s.tool = toolNumber
	tt := ui.NewTester(s.view, 400, 300)
	tt.Press(20, 20)     // outside the region
	tt.Move(120, 120)    // dragged into it
	tt.Release(120, 120) // released inside
	if len(s.annotations) != 0 || s.numberCount != 0 {
		t.Fatalf("dragging in from outside must do nothing: %+v", s.annotations)
	}
	// A press that starts inside still places one.
	tt.Press(120, 120)
	tt.Release(120, 120)
	if s.numberCount != 1 {
		t.Fatalf("click inside must place a marker, got %d", s.numberCount)
	}
}

func TestDragInFromOutsideAddsNoBlurBox(t *testing.T) {
	s := testSelection()
	s.tool = toolBlur
	tt := ui.NewTester(s.view, 400, 300)
	tt.Press(20, 20)
	tt.Move(200, 160)
	tt.Release(200, 160)
	if len(s.annotations) != 0 {
		t.Fatalf("blur box must not appear from an outside drag: %+v", s.annotations)
	}
}

func TestBlurDraftUsesDottedOutline(t *testing.T) {
	s := testSelection()
	s.tool = toolBlur
	s.drawing = true
	s.blurDraft = ui.Rect{X: 100, Y: 80, W: 200, H: 120}
	tt := ui.NewTester(s.view, 400, 300)
	img := tt.Image()
	runs, dark := 0, 0
	wasDark := false
	for x := 100; x <= 300; x++ {
		p := img.RGBAAt(x, 80)
		// Half transparent black over the page: clearly darker than the page.
		isDark := p.R < 60 && p.G < 60 && p.B < 60
		if isDark && !wasDark {
			runs++
		}
		if isDark {
			dark++
		}
		wasDark = isDark
	}
	if dark == 0 {
		t.Fatal("no dots drawn")
	}
	if runs < 5 {
		t.Fatalf("outline is not dotted: %d runs over %d dark pixels", runs, dark)
	}
}

func TestHintChipIsSolidAndReadable(t *testing.T) {
	s := testSelection()
	tt := ui.NewTester(s.view, 400, 300)
	toolbarY := s.rect.Y + s.rect.H + 10
	if toolbarY+54 > 300 {
		toolbarY = max(8, s.rect.Y-64)
	}
	toolbarX := max(float32(8), min(400-352-8, s.rect.X+s.rect.W-352))
	chip := s.hintChipAt(toolbarX, toolbarY)
	white, minR, maxR := 0, 999, -1
	for y := int(chip.Y) + 5; y < int(chip.Y+chip.H)-5; y++ {
		for x := int(chip.X) + 3; x < int(chip.X)+8; x++ {
			if x < 0 || y < 0 || x >= 400 || y >= 300 {
				continue
			}
			p := tt.Image().RGBAAt(x, y)
			minR = min(minR, int(p.R))
			maxR = max(maxR, int(p.R))
		}
	}
	for y := int(chip.Y); y < int(chip.Y+chip.H); y++ {
		for x := int(chip.X); x < int(chip.X+chip.W); x++ {
			if x < 0 || y < 0 || x >= 400 || y >= 300 {
				continue
			}
			p := tt.Image().RGBAAt(x, y)
			if p.R > 240 && p.G > 240 && p.B > 240 {
				white++
			}
		}
	}
	if white < 10 {
		t.Fatal("hint text is not white")
	}
	if maxR-minR > 6 {
		t.Fatalf("hint chip background is not a flat colour: %d..%d", minR, maxR)
	}
}

func TestSmoothStrokeKeepsStraightLinesStraight(t *testing.T) {
	points := [][2]float32{{100, 100}, {200, 100}, {300, 100}, {400, 100}}
	b := smoothStroke(points).Bounds()
	if b.Y < 99.5 || b.Y+b.H > 100.5 || b.H > 1 {
		t.Fatalf("straight drag must stay straight: %+v", b)
	}
	if b.X > 100.5 || b.X+b.W < 399.5 {
		t.Fatalf("stroke must span the drag: %+v", b)
	}
	// A right angle is rounded instead of showing a hard corner.
	corner := smoothStroke([][2]float32{{100, 100}, {300, 100}, {300, 220}})
	cb := corner.Bounds()
	if cb.X+cb.W > 300.5 || cb.Y+cb.H > 220.5 {
		t.Fatalf("smoothed corner overshoots: %+v", cb)
	}
}

func TestRegionMarkToolDrawsMultipleDashedBoxes(t *testing.T) {
	s := testSelection()
	s.tool = toolMark
	tt := ui.NewTester(s.view, 400, 300)
	tt.Press(100, 90)
	tt.Move(180, 140)
	tt.Release(180, 140)
	tt.Press(200, 150)
	tt.Move(320, 210)
	tt.Release(320, 210)
	if len(s.annotations) != 2 || s.annotations[0].kind != annotationMark {
		t.Fatalf("region marks missing: %+v", s.annotations)
	}
	if r := s.annotations[0].rect; r.W != 80 || r.H != 50 {
		t.Fatalf("first mark geometry: %+v", r)
	}
	img := tt.Image()
	dark := 0
	for x := 100; x <= 180; x++ {
		for y := 88; y <= 94; y++ {
			if p := img.RGBAAt(x, y); p.R > 180 && p.G < 110 {
				dark++
			}
		}
	}
	if dark == 0 {
		t.Fatal("region mark is not drawn in red")
	}
}

func TestRegionMarkDraftShowsWhileDragging(t *testing.T) {
	s := testSelection()
	s.tool = toolMark
	tt := ui.NewTester(s.view, 400, 300)
	tt.Press(100, 90)
	tt.Move(220, 160)
	if len(s.annotations) != 0 {
		t.Fatal("mark must not commit before release")
	}
	red := 0
	img := tt.Image()
	for x := 100; x <= 220; x++ {
		for y := 86; y <= 96; y++ {
			if p := img.RGBAAt(x, y); p.R > 180 && p.G < 110 {
				red++
			}
		}
	}
	if red == 0 {
		t.Fatal("draft mark is not visible while dragging")
	}
	tt.Release(220, 160)
	if len(s.annotations) != 1 {
		t.Fatalf("mark not committed: %+v", s.annotations)
	}
}

func TestRegionMarkDraftShowsOnFirstDrag(t *testing.T) {
	s := testSelection()
	if len(s.annotations) != 0 {
		t.Fatal("test setup must start empty")
	}
	s.tool = toolMark
	tt := ui.NewTester(s.view, 400, 300)
	tt.Press(120, 100)
	tt.Move(260, 170)
	img := tt.Image()
	hits := 0
	for x := 120; x <= 260; x++ {
		for y := 96; y <= 108; y++ {
			p := img.RGBAAt(x, y)
			if p.R > 180 && p.G < 110 && p.B < 110 {
				hits++
			}
		}
	}
	if hits == 0 {
		t.Fatal("first drag shows no dashed outline")
	}
	if len(s.annotations) != 0 {
		t.Fatal("mark must not commit before release")
	}
	tt.Release(260, 170)
	if len(s.annotations) != 1 {
		t.Fatal("mark was not committed")
	}
}

func hasWhitePixels(tt *ui.Tester, w, h int) bool {
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := tt.Image().RGBAAt(x, y)
			if p.R > 240 && p.G > 240 && p.B > 240 {
				return true
			}
		}
	}
	return false
}
