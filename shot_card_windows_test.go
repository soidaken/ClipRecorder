package main

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func solidShot(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestShotCardGeometry(t *testing.T) {
	red := color.RGBA{220, 40, 40, 255}
	src := solidShot(400, 300, red)
	out := decorateShot(src)
	l := newCardLayout(400, 300)
	if out.Bounds().Dx() != l.left+400+l.right || out.Bounds().Dy() != l.top+300+l.bottom {
		t.Fatalf("card size: %v", out.Bounds())
	}
	if l.bottom <= l.top {
		t.Fatalf("the shadow falls downwards, so the bottom margin is larger: %+v", l)
	}
	if a := out.RGBAAt(0, 0).A; a != 0 {
		t.Fatalf("outer corner must be transparent, alpha %d", a)
	}
	center := out.RGBAAt(out.Bounds().Dx()/2, out.Bounds().Dy()/2)
	if center != red {
		t.Fatalf("center must keep the screenshot pixels: %+v", center)
	}
	// The corner of the screenshot box is rounded away: only shadow there.
	corner := out.RGBAAt(l.left, l.top)
	if corner.R > 10 || corner.A == 255 {
		t.Fatalf("screenshot corner is not rounded off: %+v", corner)
	}
	// Edges away from the corners keep every pixel.
	if p := out.RGBAAt(l.left, l.top+150); p != red {
		t.Fatalf("straight edge must stay sharp: %+v", p)
	}
}

func TestShotCardFloatsWithShadowOnly(t *testing.T) {
	src := solidShot(300, 300, color.RGBA{20, 120, 60, 255})
	out := decorateShot(src)
	l := newCardLayout(300, 300)
	midX := out.Bounds().Dx() / 2
	if a := out.RGBAAt(1, 1).A; a != 0 {
		t.Fatalf("corner must be transparent, alpha %d", a)
	}
	// The shadow sits below the shot, so the pixel under its bottom edge is
	// darker than the one above its top edge.
	below := out.RGBAAt(midX, l.top+300+6)
	above := out.RGBAAt(midX, l.top-6)
	if below.A == 0 {
		t.Fatal("no shadow below the screenshot")
	}
	if below.A <= above.A {
		t.Fatalf("shadow must fall downwards: above %d, below %d", above.A, below.A)
	}
	if below.A == 255 || below.R != 0 {
		t.Fatalf("shadow must stay translucent black: %+v", below)
	}
}

func TestPrepareCopyHonoursCardToggle(t *testing.T) {
	src := solidShot(800, 600, color.RGBA{10, 10, 200, 255})
	s := &selection{quality: qualityMedium, card: true}
	if got := prepareCopy(src, s.quality, s.card).Bounds().Dx(); got <= 800 {
		t.Fatalf("card must pad the copy: %d", got)
	}
	s.card = false
	if got := prepareCopy(src, s.quality, s.card).Bounds().Dx(); got != 800 {
		t.Fatalf("without the card the copy must stay 1:1: %d", got)
	}
	s.quality = qualityLow
	s.card = true
	wide := solidShot(3000, 1000, color.RGBA{10, 10, 200, 255})
	out := prepareCopy(wide, s.quality, s.card).Bounds()
	if out.Dx() <= lowQualityLongestSide || out.Dx() > lowQualityLongestSide+400 {
		t.Fatalf("low quality card size: %v", out)
	}
}

func TestShotShadowFadesBeforeTheCanvasEdge(t *testing.T) {
	for _, size := range []image.Point{{500, 320}, {120, 80}, {2400, 1500}} {
		out := decorateShot(solidShot(size.X, size.Y, color.RGBA{30, 90, 140, 255}))
		b := out.Bounds()
		edge := 0
		for x := 0; x < b.Dx(); x++ {
			edge = max(edge, int(out.RGBAAt(x, 0).A), int(out.RGBAAt(x, b.Dy()-1).A))
		}
		for y := 0; y < b.Dy(); y++ {
			edge = max(edge, int(out.RGBAAt(0, y).A), int(out.RGBAAt(b.Dx()-1, y).A))
		}
		if edge != 0 {
			t.Fatalf("%v: shadow is cut at the canvas edge, alpha %d", size, edge)
		}
	}
}

func TestShotPixelChannelsStayPremultiplied(t *testing.T) {
	white := color.RGBA{255, 255, 255, 255}
	out := decorateShot(solidShot(240, 160, white))
	for y := 0; y < out.Bounds().Dy(); y++ {
		for x := 0; x < out.Bounds().Dx(); x++ {
			p := out.RGBAAt(x, y)
			// Premultiplied colour never exceeds its alpha; breaking this makes
			// the PNG encoder unpremultiply into garbage, seen as stray dark
			// or coloured pixels along the rounded corners.
			if p.R > p.A || p.G > p.A || p.B > p.A {
				t.Fatalf("pixel at %d,%d is not premultiplied: %+v", x, y, p)
			}
		}
	}
}

// Over a white page the rounded edge must blend from the shot into the
// shadow. Edge pixels that left out the shadow behind them used to show a
// ring of light specks between the two, read as jagged corners.
func TestRoundedEdgeHasNoLightHalo(t *testing.T) {
	dark := color.RGBA{40, 40, 40, 255}
	out := decorateShot(solidShot(600, 400, dark))
	l := newCardLayout(600, 400)
	onWhite := func(x, y int) float64 {
		p := out.RGBAAt(x, y)
		return float64(p.R) + 255 - float64(p.A) // premultiplied over white
	}
	shape := cardShape{rect: image.Rect(l.left, l.top, l.left+600, l.top+400), extent: l.radius * cardCornerExtent}
	checked := 0
	for y := l.top - 4; y < l.top+30; y++ {
		for x := l.left - 4; x < l.left+30; x++ {
			if cov := shape.coverage(x, y); cov <= 0 || cov >= 1 {
				continue
			}
			checked++
			// Outward along the diagonal the page only gets lighter: an edge
			// pixel brighter than the shadow just outside it is a halo.
			if v, next := onWhite(x, y), onWhite(x-1, y-1); v > next+2 {
				t.Fatalf("light speck at %d,%d: %.0f next to %.0f", x, y, v, next)
			}
		}
	}
	if checked == 0 {
		t.Fatal("checked no edge pixels")
	}
}

// Coverage must change smoothly around the corner, not in the 1/16 steps of
// a 4×4 supersample, and follow the same curve at all four corners.
func TestCornersAreSmoothAndSymmetric(t *testing.T) {
	shape := cardShape{rect: image.Rect(0, 0, 200, 100), extent: 21}
	levels := map[float64]bool{}
	for y := 0; y < 25; y++ {
		for x := 0; x < 25; x++ {
			c := shape.coverage(x, y)
			if c > 0 && c < 1 && math.Abs(c*16-math.Round(c*16)) > 0.05 {
				levels[math.Round(c*255)] = true
			}
			for _, m := range [][2]int{{199 - x, y}, {x, 99 - y}, {199 - x, 99 - y}} {
				if d := math.Abs(shape.coverage(m[0], m[1]) - c); d > 1e-9 {
					t.Fatalf("corner %v differs from top-left at %d,%d by %v", m, x, y, d)
				}
			}
		}
	}
	if len(levels) < 10 {
		t.Fatalf("corner edge has only %d coverage levels", len(levels))
	}
}
