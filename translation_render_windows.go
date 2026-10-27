package main

import (
	"strings"
	"unicode"

	"cliprecorder/internal/translation"
	"github.com/egoist/mygo/ui"
)

// translationFont sets the Latin left in a translation in Segoe UI and the
// Chinese in Microsoft YaHei UI, as Windows sets its own interface.
const translationFont = "Segoe UI, Microsoft YaHei UI"

// translationInkCenter is how far below the top of a shaped line the middle
// of the ideographs sits, as a share of the font size (measured on YaHei UI).
const translationInkCenter = 0.72

// latinCenter is how far above the baseline the middle of Latin text sits,
// between its baseline and cap height, as a share of the em.
const latinCenter = 0.36

// translationMinScale is how far a translation may shrink below the source
// size to keep to the lines the source took.
const translationMinScale = 0.8

// placedLine is one shaped line of translation at its place in the overlay.
type placedLine struct {
	layout *ui.TextLayout
	x, y   float32
	color  ui.Color
}

// layoutRegions sets every region's translation into the lines its source
// took, in DIPs of the overlay. scale is the overlay's pixels per DIP.
func layoutRegions(regions []translation.Region, scale float32) []placedLine {
	var out []placedLine
	for _, r := range regions {
		out = append(out, layoutRegion(r, scale)...)
	}
	return out
}

func layoutRegion(r translation.Region, scale float32) []placedLine {
	px := func(v float64) float32 { return float32(v) / scale }
	em := px(r.Em)
	weight := 400
	if r.Bold {
		weight = 700
	}
	centers := make([]float32, len(r.Lines))
	x0, x1 := px(r.Lines[0].Box.X0), px(r.Lines[0].Box.X1)
	for i, l := range r.Lines {
		centers[i] = px(l.Baseline - latinCenter*l.Em)
		x0, x1 = min(x0, px(l.Box.X0)), max(x1, px(l.Box.X1))
	}
	pitch := em * 1.4
	if n := len(centers); n > 1 {
		pitch = (centers[n-1] - centers[0]) / float32(n-1)
	}
	// The width the translation may take: the paragraph's for several
	// lines; for one, up to the neighbours its alignment grows towards.
	minX, maxX := px(r.MinX), px(r.MaxX)
	width := x1 - x0 + 0.5*em
	cx := (x0 + x1) / 2
	if len(r.Lines) == 1 {
		switch r.Align {
		case translation.AlignCenter:
			width = 2 * min(cx-minX, maxX-cx)
		case translation.AlignRight:
			width = x1 - minX
		default:
			width = maxX - x0
		}
		width = max(width, x1-x0)
	}
	ink := ui.RGB(r.Color.R, r.Color.G, r.Color.B)
	size := em
	var lines []string
	for {
		lines = breakLines(r.Text, ui.Font{Family: translationFont, Size: size, Weight: weight}, width)
		if len(lines) <= len(r.Lines) || size <= em*translationMinScale {
			break
		}
		size = max(em*translationMinScale, size-0.5)
	}
	font := ui.Font{Family: translationFont, Size: size, Weight: weight}
	if len(r.Lines) == 1 && len(lines) > 1 {
		// A label that still needs several lines spreads them around the
		// line it replaces.
		pitch = size * 1.3
		start := centers[0] - pitch*float32(len(lines)-1)/2
		centers = centers[:0]
		for i := range lines {
			centers = append(centers, start+pitch*float32(i))
		}
	}
	out := make([]placedLine, 0, len(lines))
	for i, text := range lines {
		layout := ui.ShapeText(text, font, 0)
		w, _ := layout.Size()
		center := centers[min(i, len(centers)-1)]
		if i >= len(centers) {
			center += pitch * float32(i-len(centers)+1)
		}
		x := x0
		switch r.Align {
		case translation.AlignCenter:
			x = cx - w/2
		case translation.AlignRight:
			x = x1 - w
		}
		out = append(out, placedLine{layout: layout, x: x, y: center - translationInkCenter*size, color: ink})
	}
	return out
}

// breakLines wraps text greedily at width. Chinese breaks between any two
// characters, Latin words, numbers and addresses stay whole, and closing
// punctuation never starts a line nor opening punctuation ends one.
func breakLines(text string, font ui.Font, width float32) []string {
	units := lineUnits(text)
	var lines []string
	var current strings.Builder
	fits := func(s string) bool {
		w, _ := ui.ShapeText(s, font, 0).Size()
		return w <= width
	}
	for _, u := range units {
		candidate := current.String() + u
		if current.Len() == 0 || fits(strings.TrimRight(candidate, " ")) {
			current.WriteString(u)
			continue
		}
		lines = append(lines, strings.TrimRight(current.String(), " "))
		current.Reset()
		current.WriteString(strings.TrimLeft(u, " "))
	}
	if s := strings.TrimRight(current.String(), " "); s != "" {
		lines = append(lines, s)
	}
	return lines
}

const (
	noLineStart = "，。、；：！？）》」』】”’%…·,.;:!?)]}"
	noLineEnd   = "（《「『【“‘([{$"
)

// lineUnits splits text into the pieces a line may break between.
func lineUnits(text string) []string {
	var units []string
	runes := []rune(text)
	for i := 0; i < len(runes); {
		j := i + 1
		if isWordRune(runes[i]) {
			for j < len(runes) && isWordRune(runes[j]) {
				j++
			}
		} else if runes[i] == ' ' {
			for j < len(runes) && runes[j] == ' ' {
				j++
			}
		}
		unit := string(runes[i:j])
		switch {
		case len(units) > 0 && strings.ContainsRune(noLineStart, runes[i]):
			units[len(units)-1] += unit
		case len(units) > 0 && strings.ContainsAny(units[len(units)-1], noLineEnd) && strings.ContainsRune(noLineEnd, []rune(units[len(units)-1])[len([]rune(units[len(units)-1]))-1]):
			units[len(units)-1] += unit
		default:
			units = append(units, unit)
		}
		i = j
	}
	return units
}

func isWordRune(r rune) bool {
	return r < 0x2E80 && (unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(".-_/@:#$%&+'×=~", r))
}
