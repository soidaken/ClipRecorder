package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
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
	pad := 24 // 8% of 300
	if out.Bounds().Dx() != 400+2*pad || out.Bounds().Dy() != 300+2*pad {
		t.Fatalf("card size: %v", out.Bounds())
	}
	if a := out.RGBAAt(0, 0).A; a > 8 {
		t.Fatalf("outer corner must be transparent, alpha %d", a)
	}
	center := out.RGBAAt(out.Bounds().Dx()/2, out.Bounds().Dy()/2)
	if center != red {
		t.Fatalf("center must keep the screenshot pixels: %+v", center)
	}
	// The corner of the screenshot box is rounded away: card shows there.
	corner := out.RGBAAt(pad+1, pad+1)
	if corner.R > 120 || corner.G > 120 {
		t.Fatalf("screenshot corner is not rounded off: %+v", corner)
	}
	if corner.A == 0 {
		t.Fatal("card background must be opaque enough to read")
	}
}

func TestShotCardFloatsWithShadowOnly(t *testing.T) {
	src := solidShot(300, 300, color.RGBA{20, 120, 60, 255})
	out := decorateShot(src)
	pad := 24
	midX := out.Bounds().Dx() / 2
	// Nothing but a shadow outside the screenshot: far corners stay clear.
	if a := out.RGBAAt(1, 1).A; a > 6 {
		t.Fatalf("corner must be transparent, alpha %d", a)
	}
	// The shadow sits below the shot, so the pixel under its bottom edge is
	// darker than the one above its top edge.
	below := out.RGBAAt(midX, pad+300+3)
	above := out.RGBAAt(midX, pad-3)
	if below.A == 0 {
		t.Fatal("no shadow below the screenshot")
	}
	if below.A <= above.A {
		t.Fatalf("shadow must fall downwards: above %d, below %d", above.A, below.A)
	}
	if below.A == 255 {
		t.Fatal("shadow must stay translucent")
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
	if out.Dx() <= lowQualityLongestSide || out.Dx() > lowQualityLongestSide+2*cardPaddingMax {
		t.Fatalf("low quality card size: %v", out)
	}
}

func TestShotShadowFadesBeforeTheCanvasEdge(t *testing.T) {
	src := solidShot(500, 320, color.RGBA{30, 90, 140, 255})
	out := decorateShot(src)
	b := out.Bounds()
	bottom, right, top := 0, 0, 0
	for x := 0; x < b.Dx(); x++ {
		bottom = max(bottom, int(out.RGBAAt(x, b.Dy()-1).A))
		top = max(top, int(out.RGBAAt(x, 0).A))
	}
	for y := 0; y < b.Dy(); y++ {
		right = max(right, int(out.RGBAAt(b.Dx()-1, y).A))
	}
	if bottom != 0 || right != 0 || top != 0 {
		t.Fatalf("shadow is cut at the canvas edge: top %d bottom %d right %d", top, bottom, right)
	}
	// It must still be visible just under the screenshot.
	pad := int(clampFloat(cardPaddingRatio*float32(min(500, 320)), cardPaddingMin, cardPaddingMax))
	below := out.RGBAAt(b.Dx()/2, pad+320+3)
	if below.A == 0 {
		t.Fatal("shadow missing under the screenshot")
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

func TestRoundedCornersSurvivePNGRoundTrip(t *testing.T) {
	out := decorateShot(solidShot(240, 160, color.RGBA{255, 255, 255, 255}))
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	b := out.Bounds()
	pad := int(clampFloat(cardPaddingRatio*160, cardPaddingMin, cardPaddingMax))
	shot := image.Rect(pad, pad, pad+240, pad+160)
	radius := clampFloat(cardRadiusRatio*160, cardRadiusMin, cardRadiusMax)
	checked := 0
	// Around each rounded corner the edge fades out; every partially covered
	// pixel must still be white, never a dark or coloured speck.
	for _, corner := range []image.Point{{pad, pad}, {b.Dx() - pad - 1, pad}, {pad, b.Dy() - pad - 1}, {b.Dx() - pad - 1, b.Dy() - pad - 1}} {
		for dy := -12; dy <= 12; dy++ {
			for dx := -12; dx <= 12; dx++ {
				x, y := corner.X+dx, corner.Y+dy
				// Only the screenshot itself: the pixels outside it are shadow,
				// which is black by design.
				if x < pad || y < pad || x >= b.Dx()-pad || y >= b.Dy()-pad {
					continue
				}
				// Only the anti-aliased rim: outside the rounded rect the pixel
				// is shadow, which is black by design.
				if cov := roundRectCoverage(shot, radius, x, y); cov <= 0 || cov >= 1 {
					continue
				}
				checked++
				c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
				if c.A == 0 || c.A == 255 {
					continue
				}
				if c.R < 240 || c.G < 240 || c.B < 240 {
					t.Fatalf("dark speck at corner pixel %d,%d: %d,%d,%d a=%d", x, y, c.R, c.G, c.B, c.A)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("checked no edge pixels")
	}
}

// The early in/out answer must agree with supersampling every pixel.
func TestRoundRectCoverageShortcutMatchesSampling(t *testing.T) {
	r := image.Rect(7, 5, 140, 90)
	const radius = 13.5
	for y := 0; y < 100; y++ {
		for x := 0; x < 150; x++ {
			inside := 0
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					if roundRectSDF(float32(x)+(float32(sx)+0.5)/4, float32(y)+(float32(sy)+0.5)/4, r, radius) <= 0 {
						inside++
					}
				}
			}
			if got, want := roundRectCoverage(r, radius, x, y), float32(inside)/16; got != want {
				t.Fatalf("coverage at %d,%d: got %v want %v", x, y, got, want)
			}
		}
	}
}
