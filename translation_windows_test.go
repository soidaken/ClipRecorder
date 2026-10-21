package main

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"cliprecorder/internal/translation"
	"github.com/egoist/mygo/ui"
)

func TestTranslationOverlayLayoutAndColor(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 600, 200))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(20, 20, 150, 35), image.NewUniform(color.RGBA{34, 68, 153, 255}), image.Point{}, draw.Src)
	line := translation.Line{Text: "你好，世界", X: 20, Y: 18, W: 200, H: 25}
	col := sourceTextColor(img, line)
	if col.B < 120 || col.R > 70 {
		t.Fatalf("source blue not retained: %+v", col)
	}
	o := &translationOverlay{image: img, scale: 1, lines: []translation.Line{line, {Text: "这是下一行中文内容", X: 20, Y: 65, W: 400, H: 24}}, colors: []color.RGBA{col, {0, 0, 0, 255}}}
	tt := ui.NewTester(o.view, 600, 200)
	if !tt.HasText(line.Text) || !tt.HasText(o.lines[1].Text) {
		t.Fatal("overlay translation missing")
	}
	if p := tt.Image().RGBAAt(590, 190); p.R != 255 || p.G != 255 || p.B != 255 {
		t.Fatalf("overlay must be white: %+v", p)
	}
	first, _ := tt.Find(line.Text)
	second, _ := tt.Find(o.lines[1].Text)
	if first.X != 20 || second.Y <= first.Y {
		t.Fatalf("original line positions not retained: %+v %+v", first, second)
	}
}

func TestTranslationSelectionHasNoConfirmButtons(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	s := &selection{translate: true, desktop: img.Bounds(), snapshot: img, bitmap: ui.NewBitmap(img), scale: 1}
	tt := ui.NewTester(s.view, 800, 600)
	if !tt.HasText("拖动框选英文区域，松开后自动翻译 · Esc / 右键取消") {
		t.Fatal("translation hint missing")
	}
	if tt.HasText("复制截图") || tt.HasText("开始录制") {
		t.Fatal("translation must not show confirmation buttons")
	}
	tt.Press(30, 30)
	tt.Move(320, 180)
	tt.Release(320, 180)
	if !s.selected || tt.HasText("复制截图") || tt.HasText("取消") {
		t.Fatal("translation selection must not expose confirmation controls")
	}
}
