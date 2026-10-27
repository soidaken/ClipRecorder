package main

import (
	"image"
	"image/color"
	"math"
)

// A confirmed shot floats like a macOS window: the capture keeps its pixels,
// gets continuous corners, a hairline edge and a soft shadow that falls
// downwards, and everything around it stays transparent. There is no coloured
// plate behind it, so it sits on whatever page it is pasted into.

const (
	// The look is designed for a shot whose shorter side is cardScaleBase
	// pixels; other shots scale it within [cardScaleMin, cardScaleMax], so a
	// small crop does not drown in its own shadow.
	cardScaleBase = 700
	cardScaleMin  = 0.4
	cardScaleMax  = 1.4

	// cardRadius is the circular radius the corner reads like. The corner
	// is a superellipse that starts cardCornerExtent radii from the corner
	// and eases into the straight edges with no visible kink, like the
	// continuous corners of macOS windows.
	cardRadius         = 12
	cardCornerExtent   = 1.75
	cardCornerExponent = 4

	// The shadow follows an active macOS window: wide and soft, cast
	// downwards, with a dark hairline that keeps the edge crisp on light
	// pages.
	cardShadowSigma   = 26
	cardShadowOffset  = 20
	cardShadowSpread  = 2
	cardShadowAlpha   = 0.5
	cardHairlineAlpha = 0.16
)

// cardShape is the outline of the shot inside the canvas.
type cardShape struct {
	rect   image.Rectangle
	extent float64 // side of each corner region
}

// distance is the signed distance from (px, py) to the outline, negative
// inside. Along the sides the outline is the straight pixel-aligned edge; in
// the corners it is the superellipse |u|^n + |v|^n = 1, whose distance is the
// first-order estimate f/|∇f|, accurate in the few pixels around the edge
// where coverage and the shadow mask are decided.
func (s cardShape) distance(px, py float64) float64 {
	r := s.rect
	// Distances to the nearest vertical and horizontal edge, positive inside.
	a := min(px-float64(r.Min.X), float64(r.Max.X)-px)
	b := min(py-float64(r.Min.Y), float64(r.Max.Y)-py)
	e := s.extent
	if a >= e || b >= e {
		return -min(a, b)
	}
	u, v := (e-a)/e, (e-b)/e
	const n = cardCornerExponent
	un, vn := math.Pow(u, n), math.Pow(v, n)
	norm := math.Pow(un+vn, 1.0/n)
	if norm < 0.5 {
		return -(1 - norm) * e // deep inside the corner region
	}
	grad := math.Sqrt(un*un/(u*u)+vn*vn/(v*v)) / math.Pow(norm, n-1)
	if u == 0 || v == 0 {
		grad = 1
	}
	return e * (norm - 1) / grad
}

// coverage is the share of the pixel at (x, y) inside the outline.
func (s cardShape) coverage(x, y int) float64 {
	return clamp01(0.5 - s.distance(float64(x)+0.5, float64(y)+0.5))
}

func clamp01(v float64) float64 { return min(max(v, 0), 1) }

// cardLayout derives the sizes for a shot of w×h pixels.
type cardLayout struct {
	radius, sigma, offset, spread float64
	left, top, right, bottom      int
}

func newCardLayout(w, h int) cardLayout {
	k := float64(clampFloat(float32(min(w, h))/cardScaleBase, cardScaleMin, cardScaleMax))
	l := cardLayout{
		radius: max(6, cardRadius*k),
		sigma:  cardShadowSigma * k,
		offset: math.Round(cardShadowOffset * k),
		spread: cardShadowSpread * k,
	}
	// The shadow fades to nothing within 3 sigma of its spread outline, so
	// the margins hold all of it: less above the shot, more below.
	reach := 3*l.sigma + l.spread
	l.left = int(math.Ceil(reach))
	l.right = l.left
	l.top = int(math.Ceil(max(reach-l.offset, 2)))
	l.bottom = int(math.Ceil(reach + l.offset))
	return l
}

func decorateShot(src image.Image) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return image.NewRGBA(image.Rect(0, 0, max(1, w), max(1, h)))
	}
	l := newCardLayout(w, h)
	canvasW, canvasH := l.left+w+l.right, l.top+h+l.bottom
	dst := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	shape := cardShape{
		rect:   image.Rect(l.left, l.top, l.left+w, l.top+h),
		extent: min(l.radius*cardCornerExtent, float64(min(w, h))/2),
	}

	// The shadow is the outline grown by the spread, moved down and blurred.
	// It stays in float32 throughout so its long tail fades without bands.
	mask := make([]float32, canvasW*canvasH)
	for y := 0; y < canvasH; y++ {
		py := float64(y) + 0.5 - l.offset
		for x := 0; x < canvasW; x++ {
			mask[y*canvasW+x] = float32(clamp01(0.5 - (shape.distance(float64(x)+0.5, py) - l.spread)))
		}
	}
	shadow := gaussianBlur(mask, canvasW, canvasH, l.sigma)

	for y := 0; y < canvasH; y++ {
		for x := 0; x < canvasW; x++ {
			d := shape.distance(float64(x)+0.5, float64(y)+0.5)
			cov := clamp01(0.5 - d)
			if cov >= 1 {
				r, g, bb, _ := src.At(b.Min.X+x-l.left, b.Min.Y+y-l.top).RGBA()
				dst.SetRGBA(x, y, srcColor(r, g, bb))
				continue
			}
			// Behind the shot: the shadow and, one pixel wide around the
			// outline, the hairline, both black.
			hair := (clamp01(1.5-d) - cov) * cardHairlineAlpha
			behind := 1 - (1-float64(shadow[y*canvasW+x])*cardShadowAlpha)*(1-hair)
			alpha := cov + behind*(1-cov)
			if cov <= 0 {
				if behind > 0 {
					dst.SetRGBA(x, y, color.RGBA{A: toAlpha(alpha)})
				}
				continue
			}
			// The edge pixel lays the screenshot colour over what is behind
			// it. image.RGBA is alpha premultiplied and the backdrop is black,
			// so the colour channels are the screenshot scaled by coverage.
			r, g, bb, _ := src.At(b.Min.X+x-l.left, b.Min.Y+y-l.top).RGBA()
			c := srcColor(r, g, bb)
			a := toAlpha(alpha)
			premul := func(v uint8) uint8 { return min(uint8(float64(v)*cov+0.5), a) }
			dst.SetRGBA(x, y, color.RGBA{R: premul(c.R), G: premul(c.G), B: premul(c.B), A: a})
		}
	}
	return dst
}

// gaussianBlur approximates a Gaussian of the given sigma with three box
// passes per axis, in float32 so faint values keep their precision.
func gaussianBlur(src []float32, w, h int, sigma float64) []float32 {
	// Three boxes of radius r have a variance of r(r+1).
	radius := max(1, int(math.Round((math.Sqrt(1+4*sigma*sigma)-1)/2)))
	buf := append([]float32(nil), src...)
	tmp := make([]float32, len(src))
	for i := 0; i < 3; i++ {
		boxBlur(buf, tmp, w, h, 1, w, radius)
		boxBlur(tmp, buf, h, w, w, 1, radius)
	}
	return buf
}

// boxBlur averages runs of n values spaced step apart; lines are line apart.
// Values beyond the ends count as zero: the canvas is clear there.
func boxBlur(src, dst []float32, n, lines, step, line, radius int) {
	scale := 1 / float32(2*radius+1)
	for l := 0; l < lines; l++ {
		base := l * line
		var sum float32
		for i := 0; i < min(radius, n); i++ {
			sum += src[base+i*step]
		}
		for i := 0; i < n; i++ {
			if in := i + radius; in < n {
				sum += src[base+in*step]
			}
			if out := i - radius - 1; out >= 0 {
				sum -= src[base+out*step]
			}
			dst[base+i*step] = max(sum*scale, 0)
		}
	}
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

func toAlpha(a float64) uint8 { return uint8(clamp01(a)*255 + 0.5) }

func clampFloat(v, low, high float32) float32 {
	return min(max(v, low), high)
}
