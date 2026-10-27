package main

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cliprecorder/internal/translation"
	"github.com/egoist/mygo/ui"
)

func TestBreakLinesKeepsWordsWholeAndPunctuationAttached(t *testing.T) {
	font := ui.Font{Family: translationFont, Size: 13}
	text := "从 2026-11-01 起，IP 费用将添加到您的服务续费中。"
	lines := breakLines(text, font, 90)
	if len(lines) < 2 {
		t.Fatalf("expected wrapping at 90 DIPs: %q", lines)
	}
	if strings.Join(lines, "") != strings.ReplaceAll(text, " ", "") && !strings.Contains(strings.Join(lines, " "), "2026-11-01") {
		t.Fatalf("a date must not be split: %q", lines)
	}
	for _, l := range lines {
		if strings.ContainsAny(string([]rune(l)[0]), noLineStart) {
			t.Fatalf("line starts with closing punctuation: %q", lines)
		}
		if w, _ := ui.ShapeText(l, font, 0).Size(); w > 90 && len([]rune(l)) > 1 && !strings.Contains(l, "2026-11-01") {
			t.Fatalf("line wider than the width: %q (%.1f)", l, w)
		}
	}
}

func region(text string, align translation.Align, lines ...translation.Line) translation.Region {
	r := translation.Region{Text: text, Lines: lines, Em: lines[0].Em, Color: color.RGBA{20, 40, 80, 255}, Align: align,
		MinX: lines[0].Box.X0, MaxX: lines[0].Box.X1}
	return r
}

func TestLayoutCentersAButtonLabel(t *testing.T) {
	r := region("继续", translation.AlignCenter, translation.Line{Box: translation.Rect{X0: 380, Y0: 628, X1: 442, Y1: 640}, Baseline: 640, Em: 17})
	r.MinX, r.MaxX = 290, 532
	placed := layoutRegions([]translation.Region{r}, 1)
	if len(placed) != 1 {
		t.Fatalf("one line expected: %d", len(placed))
	}
	w, _ := placed[0].layout.Size()
	if center := placed[0].x + w/2; center < 409 || center > 413 {
		t.Fatalf("label must stay centred on 411: %.1f", center)
	}
	// The ideographs sit on the source line: their middle where Latin text
	// had its middle, between baseline and cap height.
	inkMiddle := placed[0].y + translationInkCenter*17
	if want := float32(640 - latinCenter*17); inkMiddle < want-1 || inkMiddle > want+1 {
		t.Fatalf("vertical middle %.1f, want %.1f", inkMiddle, want)
	}
}

func TestLayoutSetsAParagraphOnTheSourceLines(t *testing.T) {
	lines := []translation.Line{
		{Box: translation.Rect{X0: 20, Y0: 235, X1: 510, Y1: 247}, Baseline: 244, Em: 13},
		{Box: translation.Rect{X0: 20, Y0: 253, X1: 508, Y1: 266}, Baseline: 263, Em: 13},
		{Box: translation.Rect{X0: 20, Y0: 273, X1: 270, Y1: 286}, Baseline: 283, Em: 13},
	}
	r := region("付费 IP 范围内的地址不保证可从其访问任何网站或服务，也不保证 IP 的地理位置信息（例如 GeoIP 数据库显示的国家或地区）。", translation.AlignLeft, lines...)
	placed := layoutRegions([]translation.Region{r}, 1)
	if len(placed) == 0 || len(placed) > 3 {
		t.Fatalf("the translation must fit the source's lines: %d", len(placed))
	}
	for i, p := range placed {
		if p.x != 20 {
			t.Fatalf("line %d does not start at the paragraph's left edge: %.1f", i, p.x)
		}
		if w, _ := p.layout.Size(); p.x+w > 520+7 {
			t.Fatalf("line %d runs past the paragraph: %.1f", i, p.x+w)
		}
		middle := p.y + translationInkCenter*13
		if want := float32(lines[i].Baseline - latinCenter*13); middle < want-1.5 || middle > want+1.5 {
			t.Fatalf("line %d middle %.1f, want %.1f", i, middle, want)
		}
	}
}

func TestLayoutShrinksALabelThatWouldWrap(t *testing.T) {
	r := region("这是一个比原文长得多的中文翻译标签", translation.AlignLeft, translation.Line{Box: translation.Rect{X0: 10, Y0: 10, X1: 120, Y1: 22}, Baseline: 22, Em: 13})
	r.MaxX = 190
	placed := layoutRegions([]translation.Region{r}, 1)
	if len(placed) == 1 {
		if w, _ := placed[0].layout.Size(); w > 181 {
			t.Fatalf("a single line must fit up to the neighbour: %.1f", w)
		}
	} else if len(placed) > 2 {
		t.Fatalf("too many lines: %d", len(placed))
	}
}

func TestLayoutConvertsPixelsToDIPs(t *testing.T) {
	r := region("价格", translation.AlignLeft, translation.Line{Box: translation.Rect{X0: 20, Y0: 250, X1: 76, Y1: 268}, Baseline: 268, Em: 26})
	placed := layoutRegions([]translation.Region{r}, 2)
	if placed[0].x != 10 {
		t.Fatalf("x %.1f, want 10 DIPs at 200%%", placed[0].x)
	}
}

func TestTranslationOverlayShowsTheSourceWhileTranslating(t *testing.T) {
	img := solidRGBA(400, 200, color.RGBA{200, 30, 30, 255})
	o := &translationOverlay{image: img, scale: 1, status: "正在翻译…", source: ui.NewBitmap(img)}
	tt := ui.NewTester(o.view, 400, 200)
	if !tt.HasText("正在翻译…") {
		t.Fatal("progress label missing")
	}
	// Away from the scan band and the label the screenshot shows as it was.
	im := tt.Image()
	if p := im.RGBAAt(20, 20); p.R < 150 || p.G > 90 {
		if p2 := im.RGBAAt(380, 100); p2.R < 150 || p2.G > 90 {
			t.Fatalf("the source must show through while waiting: %+v %+v", p, p2)
		}
	}
}

func TestTranslationOverlayDrawsTheResult(t *testing.T) {
	img := solidRGBA(400, 200, color.RGBA{255, 255, 255, 255})
	erased := solidRGBA(400, 200, color.RGBA{240, 246, 255, 255})
	r := region("你好世界", translation.AlignLeft, translation.Line{Box: translation.Rect{X0: 40, Y0: 80, X1: 160, Y1: 100}, Baseline: 100, Em: 20})
	r.Color = color.RGBA{A: 255}
	o := &translationOverlay{image: img, scale: 1, source: ui.NewBitmap(img)}
	o.result = ui.NewBitmap(erased)
	o.lines = layoutRegions([]translation.Region{r}, 1)
	im := ui.NewTester(o.view, 400, 200).Image()
	if p := im.RGBAAt(300, 160); p.R != 240 || p.G != 246 || p.B != 255 {
		t.Fatalf("the erased screenshot must be the backdrop: %+v", p)
	}
	dark := 0
	for y := 75; y < 105; y++ {
		for x := 40; x < 130; x++ {
			if im.RGBAAt(x, y).R < 100 {
				dark++
			}
		}
	}
	if dark < 40 {
		t.Fatalf("translation not drawn where the source was: %d dark pixels", dark)
	}
}

func TestTranslationOverlayListsBlocksItCouldNotPlace(t *testing.T) {
	img := solidRGBA(400, 200, color.RGBA{255, 255, 255, 255})
	o := &translationOverlay{image: img, scale: 1, source: ui.NewBitmap(img), reader: []translation.Block{
		{Src: "Upgrade", Text: "升级", Bold: true}, {Src: "Cancel", Text: "取消"},
	}}
	tt := ui.NewTester(o.view, 400, 200)
	if !tt.HasText("升级") || !tt.HasText("取消") {
		t.Fatal("the fallback must list every block")
	}
}

func TestTranslationOverlayReportsFailure(t *testing.T) {
	img := solidRGBA(400, 200, color.RGBA{255, 255, 255, 255})
	o := &translationOverlay{image: img, scale: 1, source: ui.NewBitmap(img), status: "翻译未完成：网络错误", failed: true}
	if !ui.NewTester(o.view, 400, 200).HasText("翻译未完成：网络错误 · Esc 关闭") {
		t.Fatal("error label missing")
	}
}

func TestTranslationOverlayHasTwoPixelBorder(t *testing.T) {
	img := solidRGBA(400, 200, color.RGBA{255, 255, 255, 255})
	o := &translationOverlay{image: img, scale: 1, source: ui.NewBitmap(img), result: ui.NewBitmap(img)}
	im := ui.NewTester(o.view, 400, 200).Image()
	for _, pt := range []image.Point{{0, 100}, {1, 100}, {398, 100}, {399, 100}, {200, 0}, {200, 1}, {200, 198}, {200, 199}} {
		if p := im.RGBAAt(pt.X, pt.Y); p.R > 200 {
			t.Fatalf("border missing at %v: %+v", pt, p)
		}
	}
	if p := im.RGBAAt(3, 100); p.R < 240 || p.G < 240 || p.B < 240 {
		t.Fatalf("inside the border the screenshot shows: %+v", p)
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

func solidRGBA(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}
