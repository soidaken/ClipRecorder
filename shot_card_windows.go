package main

import (
	"image"
	"image/color"
	"math"
)

// A confirmed shot floats: the capture keeps its pixels, gets rounded corners
// and a soft drop shadow, and everything around it stays transparent. There is
// no coloured plate behind it, so it sits on whatever page it is pasted into.

const (
	cardPaddingRatio = 0.08 // of the shorter side
	cardPaddingMin   = 24
	cardPaddingMax   = 96
	cardRadiusRatio  = 0.025
	cardRadiusMin    = 12
	cardRadiusMax    = 32
	cardShadowAlpha  = 0.45
)

func decorateShot(src image.Image) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return image.NewRGBA(image.Rect(0, 0, max(1, w), max(1, h)))
	}
	short := float32(min(w, h))
	pad := clampFloat(cardPaddingRatio*short, cardPaddingMin, cardPaddingMax)
	radius := clampFloat(cardRadiusRatio*short, cardRadiusMin, cardRadiusMax)
	canvasW, canvasH := w+int(math.Round(float64(pad)))*2, h+int(math.Round(float64(pad)))*2
	dst := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	shot := image.Rect(canvasW/2-w/2, canvasH/2-h/2, canvasW/2-w/2+w, canvasH/2-h/2+h)

	// The shadow has to fade to nothing inside the canvas, so its blur and
	// offset are derived from the margin: 3 sigma + offset stays under it.
	sigma := float64(pad) * 0.26
	shadowOffset := int(math.Round(float64(pad) * 0.10))
	mask := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	fillRoundRect(mask, shot, radius, color.RGBA{255, 255, 255, 255})
	shadow := blurRGBA(mask, sigma)

	for y := 0; y < canvasH; y++ {
		for x := 0; x < canvasW; x++ {
			cov := roundRectCoverage(shot, radius, x, y)
			s := shadowAt(shadow, x, y-shadowOffset) * cardShadowAlpha
			if cov <= 0 {
				if s > 0 {
					dst.SetRGBA(x, y, color.RGBA{A: toAlpha(s)})
				}
				continue
			}
			srcX, srcY := b.Min.X+x-shot.Min.X, b.Min.Y+y-shot.Min.Y
			r, g, bb, _ := src.At(srcX, srcY).RGBA()
			c := srcColor(r, g, bb)
			if cov >= 1 {
				dst.SetRGBA(x, y, c)
				continue
			}
			// Edge pixels keep the screenshot colour only: letting the shadow
			// show through a half covered pixel drew a dark fringe along the
			// rounded corners. image.RGBA is alpha premultiplied, so the colour
			// has to be scaled by the coverage: writing full white with a low
			// alpha overflowed when the PNG encoder unpremultiplied it, which
			// showed up as stray dark pixels in the corners.
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(float32(c.R) * cov),
				G: uint8(float32(c.G) * cov),
				B: uint8(float32(c.B) * cov),
				A: toAlpha(cov),
			})
		}
	}
	return dst
}

// roundRectCoverage supersamples the rounded rectangle so its edge fades
// smoothly instead of stepping pixel by pixel.
func roundRectCoverage(r image.Rectangle, radius float32, x, y int) float32 {
	const samples = 4
	// The distance changes by at most 1 per pixel moved, and every sample lies
	// within 0.53px of the pixel centre: pixels further than that from the edge
	// are fully in or out, which is almost all of them.
	if d := roundRectSDF(float32(x)+0.5, float32(y)+0.5, r, radius); d <= -0.75 {
		return 1
	} else if d >= 0.75 {
		return 0
	}
	var inside int
	for sy := 0; sy < samples; sy++ {
		for sx := 0; sx < samples; sx++ {
			px := float32(x) + (float32(sx)+0.5)/samples
			py := float32(y) + (float32(sy)+0.5)/samples
			if roundRectSDF(px, py, r, radius) <= 0 {
				inside++
			}
		}
	}
	return float32(inside) / (samples * samples)
}

// shadowAt samples the blurred mask, which holds the shadow's strength in red.
func shadowAt(shadow *image.RGBA, x, y int) float32 {
	if x < 0 || y < 0 || x >= shadow.Rect.Dx() || y >= shadow.Rect.Dy() {
		return 0
	}
	return float32(shadow.RGBAAt(x, y).R) / 255
}

// prepareCopy applies the copy resolution and, when enabled, the card look.
func prepareCopy(img image.Image, quality shotQuality, card bool) image.Image {
	out := quality.resizeCopy(img)
	if card {
		return decorateShot(out)
	}
	return out
}

func srcColor(r, g, b uint32) color.RGBA {
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}
}

// blend puts the opaque screenshot colour over the dark card by coverage.
func blend(c color.RGBA, cardAlpha, coverage float32) color.RGBA {
	alpha := coverage + cardAlpha*(1-coverage)
	if alpha <= 0 {
		return color.RGBA{}
	}
	mix := func(v uint8) uint8 {
		return uint8(clampFloat((float32(v)*coverage)/alpha, 0, 255))
	}
	return color.RGBA{R: mix(c.R), G: mix(c.G), B: mix(c.B), A: toAlpha(alpha)}
}

func toAlpha(a float32) uint8 { return uint8(clampFloat(a*255+0.5, 0, 255)) }

func clampFloat(v, low, high float32) float32 {
	return min(max(v, low), high)
}

// roundRectSDF is the signed distance to a rounded rectangle: negative inside.
func roundRectSDF(px, py float32, r image.Rectangle, radius float32) float32 {
	cx := float32(r.Min.X+r.Max.X) / 2
	cy := float32(r.Min.Y+r.Max.Y) / 2
	hx := float32(r.Dx())/2 - radius
	hy := float32(r.Dy())/2 - radius
	dx := abs(px-cx) - hx
	dy := abs(py-cy) - hy
	outside := float32(math.Hypot(float64(max(dx, 0)), float64(max(dy, 0))))
	inside := min(max(dx, dy), 0)
	return outside + inside - radius
}

func fillRoundRect(dst *image.RGBA, r image.Rectangle, radius float32, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			coverage := roundRectCoverage(r, radius, x, y)
			if coverage <= 0 {
				continue
			}
			dst.SetRGBA(x, y, color.RGBA{R: c.R, G: c.G, B: c.B, A: uint8(float32(c.A) * coverage)})
		}
	}
}
