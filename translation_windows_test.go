package main

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"cliprecorder/internal/translation"
	"github.com/egoist/mygo/ui"
)

func TestTranslationOverlayUsesNormalizedLayout(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 600, 200))
	first := translation.Line{Text: "你好，世界", X: 0.1, Y: 0.1, W: 0.4, H: 0.12, Color: "#224499"}
	second := translation.Line{Text: "这是下一行中文内容", X: 0.1, Y: 0.4, W: 0.7, H: 0.12, Color: "#000000"}
	o := &translationOverlay{image: img, scale: 1, lines: []translation.Line{first, second}}
	tt := ui.NewTester(o.view, 600, 200)
	if !tt.HasText(first.Text) || !tt.HasText(second.Text) {
		t.Fatal("overlay translation missing")
	}
	if p := tt.Image().RGBAAt(590, 190); p.R != 255 || p.G != 255 || p.B != 255 {
		t.Fatalf("overlay must be white: %+v", p)
	}
	start, ok := tt.Find(first.Text)
	if !ok {
		t.Fatal("first line not rendered")
	}
	next, ok := tt.Find(second.Text)
	if !ok {
		t.Fatal("second line not rendered")
	}
	// Normalized 0.1/0.1 on a 600x200 image lands at 60/20.
	if start.X < 50 || start.X > 75 || start.Y < 12 || start.Y > 34 {
		t.Fatalf("normalized position not applied: %+v", start)
	}
	if next.Y <= start.Y {
		t.Fatalf("reading order lost: %+v %+v", start, next)
	}
}

func TestTranslationWaitingTextIsCentered(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 400))
	o := &translationOverlay{image: img, scale: 1, status: "正在识别并翻译…"}
	tt := ui.NewTester(o.view, 800, 400)
	pos, ok := tt.Find("正在识别并翻译…")
	if !ok {
		t.Fatal("waiting text missing")
	}
	centerX, centerY := pos.X+pos.W/2, pos.Y+pos.H/2
	if centerX < 380 || centerX > 420 || centerY < 185 || centerY > 215 {
		t.Fatalf("waiting text must sit in the middle: %+v", pos)
	}
}

func TestTranslationWaitingTextIsTranslucentBlack(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 600, 300))
	o := &translationOverlay{image: img, scale: 1, status: "正在识别并翻译…"}
	tt := ui.NewTester(o.view, 600, 300)
	pos, ok := tt.Find("正在识别并翻译…")
	if !ok {
		t.Fatal("waiting text missing")
	}
	// A translucent black glyph over white stays light grey, never full black.
	var darkest uint8 = 255
	left, top := int(pos.X), int(pos.Y)
	for y := top - 6; y < top+24; y++ {
		for x := left - 4; x < left+200; x++ {
			if x < 0 || y < 0 || x >= 600 || y >= 300 {
				continue
			}
			if v := tt.Image().RGBAAt(x, y).R; v < darkest {
				darkest = v
			}
		}
	}
	if darkest < 120 || darkest > 245 {
		t.Fatalf("waiting text must be semi-transparent black, darkest R=%d", darkest)
	}
}

func TestTranslationSelectionHasNoConfirmButtons(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	s := &selection{translate: true, desktop: img.Bounds(), snapshot: img, bitmap: ui.NewBitmap(img), scale: 1}
	tt := ui.NewTester(s.view, 800, 600)
	if !tt.HasText("拖动框选英文区域，松开后自动翻译 · Esc / 右键取消") {
		t.Fatal("translation hint missing")
	}
	if tt.HasText("确认") || tt.HasText("开始录制") {
		t.Fatal("translation must not show confirmation buttons")
	}
	tt.Press(30, 30)
	tt.Move(320, 180)
	tt.Release(320, 180)
	if !s.selected || tt.HasText("确认") || tt.HasText("取消") {
		t.Fatal("translation selection must not expose confirmation controls")
	}
}

func TestTranslationCopiesAreWritten(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 120, 60))
	data, err := encodeJPEG(img)
	if err != nil || len(data) < 3 || data[0] != 0xff || data[1] != 0xd8 {
		t.Fatalf("JPEG copy: %v %x", err, data[:min(3, len(data))])
	}
	path := filepath.Join(t.TempDir(), "translate_test_zh.png")
	if err := writePNG(path, img); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("PNG copy missing: %v", err)
	}
}

func TestTranslationLightTextStaysVisible(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 100))
	line := translation.Line{Text: "白色原文", X: 0.1, Y: 0.2, W: 0.6, H: 0.4, Color: "#ffffff"}
	o := &translationOverlay{image: img, scale: 1, lines: []translation.Line{line}}
	tt := ui.NewTester(o.view, 400, 100)
	pos, ok := tt.Find(line.Text)
	if !ok {
		t.Fatal("translated line missing")
	}
	dark := 0
	for y := int(pos.Y); y < int(pos.Y+pos.H); y++ {
		for x := int(pos.X); x < int(pos.X+pos.W); x++ {
			if x < 0 || y < 0 || x >= 400 || y >= 100 {
				continue
			}
			if tt.Image().RGBAAt(x, y).R < 120 {
				dark++
			}
		}
	}
	if dark < 20 {
		t.Fatalf("white source text is invisible on the white overlay (dark pixels: %d)", dark)
	}
}

func leftmostDark(tt *ui.Tester, y0, y1, width int) int {
	best := width
	for y := y0; y < y1; y++ {
		for x := 0; x < width; x++ {
			if tt.Image().RGBAAt(x, y).R < 120 && x < best {
				best = x
			}
		}
	}
	return best
}

func TestTranslationCentersLinesThatWereCentered(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 600, 200))
	line := translation.Line{Text: "中间标题", X: 0.3, Y: 0.4, W: 0.4, H: 0.15, Color: "#000000"}
	o := &translationOverlay{image: img, scale: 1, lines: []translation.Line{line}}
	tt := ui.NewTester(o.view, 600, 200)
	left := leftmostDark(tt, 80, 110, 600)
	if left < 240 {
		t.Fatalf("centered line should not start at the box edge: leftmost pixel %d", left)
	}
}

func TestTranslationKeepsLeftAlignedLinesAtTheLeft(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 600, 200))
	line := translation.Line{Text: "左对齐正文", X: 0.05, Y: 0.4, W: 0.4, H: 0.15, Color: "#000000"}
	o := &translationOverlay{image: img, scale: 1, lines: []translation.Line{line}}
	tt := ui.NewTester(o.view, 600, 200)
	if left := leftmostDark(tt, 80, 110, 600); left > 50 {
		t.Fatalf("left aligned line moved away from its box: leftmost pixel %d", left)
	}
}

func TestModelImageCapsPixels(t *testing.T) {
	big := image.NewRGBA(image.Rect(0, 0, 3840, 2160))
	resized := modelImage(big)
	if resized == big {
		t.Fatal("4K selection must be downscaled before upload")
	}
	if pixels := resized.Bounds().Dx() * resized.Bounds().Dy(); pixels > modelPixelBudget {
		t.Fatalf("downscaled image still too large: %d pixels", pixels)
	}
	small := image.NewRGBA(image.Rect(0, 0, 800, 600))
	if modelImage(small) != small {
		t.Fatal("small selections must be sent untouched")
	}
}
