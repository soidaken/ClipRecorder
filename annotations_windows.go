package main

import (
	"image"
	"math"
	"strconv"

	"github.com/egoist/mygo/ui"
)

// Annotation colors and sizes are fixed: the shot tools only need one pen, one
// blur strength and one marker size.
const (
	penWidth         = float32(5)
	markerRadius     = float32(12)
	blurCornerRadius = float32(8)
	// markCornerRadius and markWidth shape the red dashed region marker.
	markCornerRadius = float32(10)
	markWidth        = float32(6)
	// Segoe UI bold digits at this size ink 8px below the layout top and are
	// 14px tall; centering the layout box would push them low, because the box
	// also reserves descender space.
	markerFontSize  = float32(20)
	markerInkTop    = float32(8)
	markerInkHeight = float32(14)
	// markerDigitGap is the spacing between the ink of neighbouring digits.
	// Digits are placed by ink rather than by advance width, so "12" reads as
	// one label centered in the circle.
	markerDigitGap  = float32(1.5)
	markerSmallSize = float32(13)
	// blurStrength is applied to the shortest side of the blur rectangle, so a
	// wider box does not blur more than a tall one.
	blurStrength = 0.075
)

var (
	penColor    = ui.RGB(229, 57, 53)
	markerColor = ui.RGB(229, 57, 53)
)

type annotationKind int

const (
	annotationPen annotationKind = iota
	annotationBlur
	annotationNumber
	annotationMark
)

// annotation is one edit, stored in logical coordinates relative to the
// desktop, like selection.rect.
type annotation struct {
	kind   annotationKind
	points [][2]float32
	rect   ui.Rect
	number int
	blur   *ui.Bitmap
}

type annotationTool int

const (
	toolNone annotationTool = iota
	toolPen
	toolBlur
	toolNumber
	toolMark
)

func (s *selection) setTool(tool annotationTool) {
	if s.tool == tool {
		s.tool = toolNone
	} else {
		s.tool = tool
	}
	s.drawing = false
	s.blurDraft = ui.Rect{}
	if s.window != nil {
		s.window.Invalidate()
	}
}

func (s *selection) resetAnnotations() {
	s.annotations = nil
	s.numberCount = 0
	s.blurDraft = ui.Rect{}
	s.drawing = false
}

func (s *selection) hasAnnotations() bool { return len(s.annotations) > 0 }

func (s *selection) insideSelection(x, y float32) bool {
	return x >= s.rect.X && x <= s.rect.X+s.rect.W && y >= s.rect.Y && y <= s.rect.Y+s.rect.H
}

// handleAnnotationInput turns pointer activity into pen strokes, blur boxes and
// numbered markers while a tool is active.
func (s *selection) handleAnnotationInput(x, y float32, pressed, fresh bool) {
	if fresh && !s.drawing && s.insideSelection(x, y) {
		s.drawing = true
		switch s.tool {
		case toolPen:
			s.annotations = append(s.annotations, annotation{kind: annotationPen, points: [][2]float32{{x, y}}})
		case toolBlur, toolMark:
			s.anchorX, s.anchorY = x, y
			s.blurDraft = ui.Rect{X: x, Y: y}
		case toolNumber:
			s.numberCount++
			s.annotations = append(s.annotations, annotation{kind: annotationNumber, number: s.numberCount, rect: s.markerRect(x, y)})
		}
	}
	if !s.drawing {
		return
	}
	if pressed {
		switch s.tool {
		case toolPen:
			stroke := &s.annotations[len(s.annotations)-1]
			last := stroke.points[len(stroke.points)-1]
			if abs(x-last[0])+abs(y-last[1]) >= 1.5 {
				stroke.points = append(stroke.points, [2]float32{x, y})
			}
		case toolBlur, toolMark:
			s.blurDraft = s.clampToSelection(ui.Rect{X: min(s.anchorX, x), Y: min(s.anchorY, y), W: abs(s.anchorX - x), H: abs(s.anchorY - y)})
		}
		return
	}
	s.drawing = false
	switch s.tool {
	case toolBlur:
		if s.blurDraft.W >= 8 && s.blurDraft.H >= 8 {
			s.annotations = append(s.annotations, annotation{kind: annotationBlur, rect: s.blurDraft})
			s.prepareBlur(&s.annotations[len(s.annotations)-1])
		}
	case toolMark:
		// Any number of region markers, each kept where it was drawn.
		if s.blurDraft.W >= 8 && s.blurDraft.H >= 8 {
			s.annotations = append(s.annotations, annotation{kind: annotationMark, rect: s.blurDraft})
		}
	}
	s.blurDraft = ui.Rect{}
}

func (s *selection) clampToSelection(r ui.Rect) ui.Rect {
	x0 := max(r.X, s.rect.X)
	y0 := max(r.Y, s.rect.Y)
	x1 := min(r.X+r.W, s.rect.X+s.rect.W)
	y1 := min(r.Y+r.H, s.rect.Y+s.rect.H)
	if x1 < x0 {
		x1 = x0
	}
	if y1 < y0 {
		y1 = y0
	}
	return ui.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

func (s *selection) markerRect(x, y float32) ui.Rect {
	r := markerRadius
	cx := min(max(x, s.rect.X+r), s.rect.X+s.rect.W-r)
	cy := min(max(y, s.rect.Y+r), s.rect.Y+s.rect.H-r)
	return ui.Rect{X: cx - r, Y: cy - r, W: 2 * r, H: 2 * r}
}

// prepareBlur blurs just this box: blurring the whole selection would stall
// the UI for hundreds of milliseconds on a 4K capture.
func (s *selection) prepareBlur(a *annotation) {
	scale := float32(s.scale)
	r := image.Rect(
		int(math.Round(float64(a.rect.X*scale))), int(math.Round(float64(a.rect.Y*scale))),
		int(math.Round(float64((a.rect.X+a.rect.W)*scale))), int(math.Round(float64((a.rect.Y+a.rect.H)*scale))),
	).Intersect(s.snapshot.Bounds())
	if r.Empty() {
		return
	}
	crop := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	region := s.snapshot.SubImage(r).(*image.RGBA)
	for y := 0; y < r.Dy(); y++ {
		copy(crop.Pix[y*crop.Stride:y*crop.Stride+r.Dx()*4], region.Pix[y*region.Stride:y*region.Stride+r.Dx()*4])
	}
	sigma := float64(min(r.Dx(), r.Dy())) * blurStrength
	a.blur = ui.NewBitmap(blurRGBA(crop, sigma))
}

// blurRGBA approximates a Gaussian blur with three box passes per axis, which
// is fast enough to run once per selection.
func blurRGBA(src *image.RGBA, sigma float64) *image.RGBA {
	if sigma < 1 {
		sigma = 1
	}
	radius := int(math.Ceil(sigma))
	buf := image.NewRGBA(src.Bounds())
	copy(buf.Pix, src.Pix)
	tmp := image.NewRGBA(src.Bounds())
	for i := 0; i < 3; i++ {
		boxBlurHorizontal(buf, tmp, radius)
		boxBlurVertical(tmp, buf, radius)
	}
	return buf
}

func boxBlurHorizontal(src, dst *image.RGBA, radius int) {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	window := radius*2 + 1
	for y := 0; y < h; y++ {
		row := y * src.Stride
		var r, g, b int
		for x := -radius; x <= radius; x++ {
			i := row + clampInt(x, 0, w-1)*4
			r += int(src.Pix[i])
			g += int(src.Pix[i+1])
			b += int(src.Pix[i+2])
		}
		for x := 0; x < w; x++ {
			i := row + x*4
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/window), uint8(g/window), uint8(b/window), 255
			out := row + clampInt(x-radius, 0, w-1)*4
			in := row + clampInt(x+radius+1, 0, w-1)*4
			r += int(src.Pix[in]) - int(src.Pix[out])
			g += int(src.Pix[in+1]) - int(src.Pix[out+1])
			b += int(src.Pix[in+2]) - int(src.Pix[out+2])
		}
	}
}

func boxBlurVertical(src, dst *image.RGBA, radius int) {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	window := radius*2 + 1
	for x := 0; x < w; x++ {
		var r, g, b int
		for y := -radius; y <= radius; y++ {
			i := clampInt(y, 0, h-1)*src.Stride + x*4
			r += int(src.Pix[i])
			g += int(src.Pix[i+1])
			b += int(src.Pix[i+2])
		}
		for y := 0; y < h; y++ {
			i := y*src.Stride + x*4
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/window), uint8(g/window), uint8(b/window), 255
			out := clampInt(y-radius, 0, h-1)*src.Stride + x*4
			in := clampInt(y+radius+1, 0, h-1)*src.Stride + x*4
			r += int(src.Pix[in]) - int(src.Pix[out])
			g += int(src.Pix[in+1]) - int(src.Pix[out+1])
			b += int(src.Pix[in+2]) - int(src.Pix[out+2])
		}
	}
}

func clampInt(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

// paintAnnotations draws every edit above the frozen screen, clipped to the
// selection so nothing leaks into the dimmed area.
func (s *selection) paintAnnotations(p *ui.Painter) {
	// The live draft is all there is during the first drag, before anything is
	// committed, so it has to keep the painter going too.
	if len(s.annotations) == 0 && !(s.drawing && (s.tool == toolBlur || s.tool == toolMark)) {
		return
	}
	p.Clip(s.rect, 0, func() {
		for _, a := range s.annotations {
			switch a.kind {
			case annotationPen:
				if len(a.points) == 1 {
					pt := a.points[0]
					p.Fill(ui.Rect{X: pt[0] - penWidth/2, Y: pt[1] - penWidth/2, W: penWidth, H: penWidth}, penColor, penWidth/2)
					continue
				}
				// Pointer events arrive a few milliseconds apart, so the raw
				// points are corners. A Catmull-Rom curve through them keeps the
				// stroke smooth without slowing the drawing down.
				p.StrokePath(smoothStroke(a.points), penWidth, penColor)
			case annotationBlur:
				if a.blur == nil {
					continue
				}
				w, h := a.blur.Size()
				src := ui.Rect{X: 0, Y: 0, W: float32(w), H: float32(h)}
				p.Clip(a.rect, blurCornerRadius, func() {
					p.ImageRegion(a.blur, src, a.rect, ui.ImageOptions{})
				})
			case annotationMark:
				p.StrokeDashed(a.rect, penColor, markCornerRadius, markWidth)
			case annotationNumber:
				// Red badge with a bold white number: highest contrast the
				// selection can carry, readable at a glance.
				p.Shadow(a.rect, markerRadius, 0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.28))
				p.Fill(a.rect, markerColor, markerRadius)
				drawMarkerDigits(p, a)
			}
		}
		if s.drawing && s.blurDraft.W > 0 {
			switch s.tool {
			case toolBlur:
				// Dotted, half transparent black: visible on any content without
				// claiming the area is already blurred.
				drawDottedRoundedRect(p, s.blurDraft, blurCornerRadius, 1.5, 4, ui.RGBA(0, 0, 0, 128.0/255.0))
			case toolMark:
				p.StrokeDashed(s.blurDraft, penColor, markCornerRadius, markWidth)
			}
		}
	})
}

// annotationButton is a square icon toggle; the active tool gets the accent
// tint of the toolbar so the current mode is obvious at a glance.
func annotationButton(c *ui.Context, tool annotationTool, active bool, badge string) ui.Element {
	b := ui.ButtonBase(c).Size(32, 32).Radius(9).Cursor(ui.CursorPointer).FocusRing(false)
	fill, edge := ui.Transparent, ui.Transparent
	color := muted
	if active {
		fill, edge, color = ui.RGBA(185, 221, 246, 0.16), ui.RGBA(185, 221, 246, 0.38), ice
	} else if b.Hovered() {
		fill = ui.RGBA(255, 255, 255, 0.08)
	}
	b.Background(fill).Border(1, edge)
	b.DrawOver(func(p *ui.Painter, r ui.Rect) {
		drawToolIcon(p, r, tool, color)
		if badge != "" {
			// A red pill pinned to the corner, separated from the icon by a dark
			// ring of the toolbar colour, with the count in bold white.
			font := ui.Font{Family: "Segoe UI", Size: 8, Weight: 700}
			layout := ui.ShapeText(badge, font, 0)
			width, _ := layout.Size()
			pillW, pillH := width+5, float32(10)
			pill := ui.Rect{X: r.X + r.W - pillW, Y: r.Y, W: pillW, H: pillH}
			p.Fill(ui.Rect{X: pill.X - 1.2, Y: pill.Y - 1.2, W: pill.W + 2.4, H: pill.H + 2.4}, ui.RGB(20, 26, 33), (pillH+2.4)/2)
			p.Fill(pill, markerColor, pillH/2)
			p.TextLayout(layout, pill.X+(pill.W-width)/2, pill.Y+pill.H/2-6, ui.RGB(255, 255, 255))
		}
	})
	return b
}

func drawToolIcon(p *ui.Painter, r ui.Rect, tool annotationTool, color ui.Color) {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	switch tool {
	case toolPen:
		var nib ui.Path
		nib.MoveTo(cx-6.5, cy+6.5).LineTo(cx-5, cy+1).LineTo(cx+3, cy-6.5).LineTo(cx+6.5, cy-3).LineTo(cx-1.5, cy+5).Close()
		p.StrokePath(&nib, 1.5, color)
		var tip ui.Path
		tip.MoveTo(cx-7.5, cy+7.5).LineTo(cx-5.5, cy+7).LineTo(cx-7, cy+5.5).Close()
		p.FillPath(&tip, color)
	case toolBlur:
		cell := float32(6)
		gap := float32(1.6)
		for i := 0; i < 2; i++ {
			for j := 0; j < 2; j++ {
				r := ui.Rect{X: cx - cell - gap/2 + float32(i)*(cell+gap), Y: cy - cell - gap/2 + float32(j)*(cell+gap), W: cell, H: cell}
				alpha := float32(0.9)
				if i+j == 1 {
					alpha = 0.45
				} else if i+j == 2 {
					alpha = 0.6
				}
				p.Fill(r, ui.RGBA(242, 245, 248, alpha), 1.5)
			}
		}
	case toolMark:
		p.StrokeDashed(ui.Rect{X: cx - 8, Y: cy - 6.5, W: 16, H: 13}, color, 4, 1.6)
	case toolNumber:
		ring := ui.Rect{X: cx - 8, Y: cy - 8, W: 16, H: 16}
		p.Stroke(ring, color, 8, 1.6)
		// The "1" is drawn as strokes instead of text, so its ink box is exactly
		// known and can be centered in the ring: stem, flag and a short base.
		stemX := cx + 0.9
		p.Fill(ui.Rect{X: stemX - 1.1, Y: cy - 5.4, W: 2.2, H: 10.8}, color, 1.1)
		var flag ui.Path
		flag.MoveTo(stemX-3, cy-2.6).LineTo(stemX, cy-5.4)
		p.StrokePath(&flag, 1.8, color)
		var base ui.Path
		base.MoveTo(stemX-2.4, cy+5.4).LineTo(stemX+1.6, cy+5.4)
		p.StrokePath(&base, 1.6, color)
	}
}

func toolLabel(tool annotationTool) string {
	switch tool {
	case toolPen:
		return "画笔"
	case toolBlur:
		return "模糊"
	case toolNumber:
		return "序号标记"
	case toolMark:
		return "标记区域"
	}
	return ""
}

// markerDigitInk is the ink box of each Segoe UI bold digit at markerFontSize:
// how far the ink starts from the drawing origin and how wide it is. Digits are
// drawn one by one from this table, so a label is spaced and centered by what
// the eye sees rather than by the font's advance width.
var markerDigitInk = [10]struct{ Left, Width float32 }{
	{0, 11}, {2, 6}, {1, 9}, {1, 9}, {0, 11},
	{1, 9}, {1, 10}, {1, 10}, {1, 10}, {1, 10},
}

type markerDigit struct {
	text  string
	left  float32
	width float32
}

func drawMarkerDigits(p *ui.Painter, a annotation) {
	text := strconv.Itoa(a.number)
	size := markerFontSize
	if len(text) > 2 {
		// Three digits would not fit the 24px circle.
		size = markerSmallSize
	}
	markerDigitLabel(p, text, a.rect.X+a.rect.W/2, a.rect.Y+a.rect.H/2, size, ui.RGB(255, 255, 255))
}

// markerDigitLabel draws digits centered by their ink, one digit at a time so
// "12" reads as one label instead of drifting off the circle.
func markerDigitLabel(p *ui.Painter, text string, cx, cy, size float32, color ui.Color) {
	ratio := size / markerFontSize
	gap := markerDigitGap * ratio
	digits := make([]markerDigit, 0, len(text))
	var total float32
	for _, r := range text {
		index := int(r - '0')
		if index < 0 || index > 9 {
			return
		}
		ink := markerDigitInk[index]
		digits = append(digits, markerDigit{text: string(r), left: ink.Left * ratio, width: ink.Width * ratio})
		total += ink.Width * ratio
	}
	if len(digits) == 0 {
		return
	}
	total += gap * float32(len(digits)-1)
	cursor := cx - total/2
	top := cy - markerInkTop*ratio - markerInkHeight*ratio/2
	font := ui.Font{Family: "Segoe UI", Size: size, Weight: 700}
	for _, d := range digits {
		layout := ui.ShapeText(d.text, font, 0)
		p.TextLayout(layout, cursor-d.left, top, color)
		cursor += d.width + gap
	}
}

// drawDottedRoundedRect walks the rounded rectangle and paints evenly spaced
// dots along it, the dotted outline used while a blur box is dragged.
func drawDottedRoundedRect(p *ui.Painter, r ui.Rect, radius, dotRadius, spacing float32, c ui.Color) {
	if r.W <= 0 || r.H <= 0 {
		return
	}
	radius = min(radius, min(r.W, r.H)/2)
	step := 2*dotRadius + spacing
	perimeter := 2*(r.W+r.H-4*radius) + 2*math.Pi*radius
	if perimeter <= step {
		return
	}
	for d := float32(0); d < perimeter; d += step {
		x, y := roundedRectPoint(r, radius, d)
		p.Fill(ui.Rect{X: x - dotRadius, Y: y - dotRadius, W: 2 * dotRadius, H: 2 * dotRadius}, c, dotRadius)
	}
}

// roundedRectPoint maps a distance along the perimeter of a rounded rectangle,
// clockwise from the top left corner, to a point on it.
func roundedRectPoint(r ui.Rect, radius, d float32) (float32, float32) {
	arc := math.Pi * radius / 2
	top, right, bottom, left := r.W-2*radius, r.H-2*radius, r.W-2*radius, r.H-2*radius
	segments := []struct {
		length float32
		at     func(t float32) (float32, float32)
	}{
		{top, func(t float32) (float32, float32) { return r.X + radius + t, r.Y }},
		{arc, func(t float32) (float32, float32) {
			a := float64(t/radius) - math.Pi/2
			return r.X + r.W - radius + radius*float32(math.Cos(a)), r.Y + radius + radius*float32(math.Sin(a))
		}},
		{right, func(t float32) (float32, float32) { return r.X + r.W, r.Y + radius + t }},
		{arc, func(t float32) (float32, float32) {
			a := float64(t / radius)
			return r.X + r.W - radius + radius*float32(math.Cos(a)), r.Y + r.H - radius + radius*float32(math.Sin(a))
		}},
		{bottom, func(t float32) (float32, float32) { return r.X + r.W - radius - t, r.Y + r.H }},
		{arc, func(t float32) (float32, float32) {
			a := float64(t/radius) + math.Pi/2
			return r.X + radius + radius*float32(math.Cos(a)), r.Y + r.H - radius + radius*float32(math.Sin(a))
		}},
		{left, func(t float32) (float32, float32) { return r.X, r.Y + r.H - radius - t }},
		{arc, func(t float32) (float32, float32) {
			a := float64(t/radius) + math.Pi
			return r.X + radius + radius*float32(math.Cos(a)), r.Y + radius + radius*float32(math.Sin(a))
		}},
	}
	for _, segment := range segments {
		if d < segment.length {
			return segment.at(d)
		}
		d -= segment.length
	}
	return r.X + radius, r.Y
}

// smoothStroke turns sampled pen points into quadratic curves through the mid
// points of the samples. Unlike a Catmull-Rom spline it never overshoots the
// points, so a fast drag reads as one smooth line and a sharp corner is only
// rounded, not ballooned.
func smoothStroke(points [][2]float32) *ui.Path {
	path := &ui.Path{}
	path.MoveTo(points[0][0], points[0][1])
	if len(points) == 2 {
		path.LineTo(points[1][0], points[1][1])
		return path
	}
	for i := 1; i < len(points)-1; i++ {
		midX := (points[i][0] + points[i+1][0]) / 2
		midY := (points[i][1] + points[i+1][1]) / 2
		path.QuadTo(points[i][0], points[i][1], midX, midY)
	}
	last := points[len(points)-1]
	path.LineTo(last[0], last[1])
	return path
}
