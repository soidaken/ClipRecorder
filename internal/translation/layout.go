package translation

import (
	"image"
	"image/color"
	"math"
	"slices"
	"strings"
	"unicode"
)

// The overlay is the screenshot itself with the English taken out and the
// Chinese set in its place. Windows OCR says precisely where every word is;
// the model says which words form a paragraph and what they mean. This file
// joins the two and measures, on the pixels, everything the text is set by:
// size, baseline, colour, background and alignment.

// Align is how a region's text sat in the space around it.
type Align int

const (
	AlignLeft Align = iota
	AlignCenter
	AlignRight
)

// Rect is a box in image pixels.
type Rect struct{ X0, Y0, X1, Y1 float64 }

func (r Rect) union(o Rect) Rect {
	return Rect{min(r.X0, o.X0), min(r.Y0, o.Y0), max(r.X1, o.X1), max(r.Y1, o.Y1)}
}

// Line is one line of source text as it sat on the screen.
type Line struct {
	Box      Rect    // the words, tight
	Baseline float64 // y of the baseline
	Em       float64 // font size in pixels
}

// Region is one block placed on the screenshot: the translation goes into
// the space its source lines took.
type Region struct {
	Block int
	Text  string
	Bold  bool
	Lines []Line
	Em    float64
	Color color.RGBA
	Align Align
	// MinX and MaxX bound how far a single line may grow sideways before it
	// would run into a neighbour or the edge of its container.
	MinX, MaxX float64

	background color.RGBA // what the erased lines are filled from
}

// Plan places blocks over the words OCR found. It returns the screenshot with
// the placed source text erased and the regions to draw on it. Blocks whose
// translation equals the source (numbers, dates, addresses) keep their
// original pixels, as do words no block matches.
func Plan(img *image.RGBA, blocks []Block, lines []OCRLine) (*image.RGBA, []Region) {
	segments := assign(blocks, lines)
	var regions []Region
	for block, segs := range groupByBlock(segments) {
		segs = joinRows(segs)
		b := blocks[block]
		if sameText(b.Src, b.Text) {
			continue
		}
		for _, cluster := range clusters(segs) {
			if covered(cluster, b.Src) < 0.5 && len(normalize(b.Src)) > 20 {
				continue // a stray piece of a long paragraph
			}
			regions = append(regions, measure(img, b, block, cluster))
		}
	}
	slices.SortFunc(regions, func(a, b Region) int {
		return cmpFloat(a.Lines[0].Baseline, b.Lines[0].Baseline)
	})
	out := image.NewRGBA(img.Bounds())
	copy(out.Pix, img.Pix)
	for _, r := range regions {
		// A paragraph is cleared as one area, gaps between its lines too.
		area := eraseBox(r.Lines[0])
		for _, l := range r.Lines[1:] {
			area = area.Union(eraseBox(l))
		}
		erase(out, img, area, r.background)
	}
	return out, regions
}

// segment is a run of words of one OCR line matched to one block.
type segment struct {
	block    int
	words    []Word
	box      Rect
	em       float64
	baseline float64
	text     string
}

// assign splits every OCR line into runs of words, each matched to the block
// whose source text contains it. A line can hold several blocks (a label and
// its value), and a block several lines (a paragraph).
func assign(blocks []Block, lines []OCRLine) []segment {
	grams := make([]map[string]bool, len(blocks))
	lengths := make([]int, len(blocks))
	for i, b := range blocks {
		n := normalize(b.Src)
		grams[i], lengths[i] = trigrams(n), len(n)
	}
	var out []segment
	for _, line := range lines {
		words := stripIcons(line.Words)
		n := len(words)
		if n == 0 {
			continue
		}
		// best[i] is the best score for the first i words; a run of words
		// scores by how much of it a block contains, longer runs more.
		type step struct {
			score       float64
			from, block int
		}
		best := make([]step, n+1)
		for i := 1; i <= n; i++ {
			// Leaving a word out costs as much as a matched character earns,
			// so a line with one misread word stays one run instead of two
			// around the word, which would stay on screen.
			skip := float64(len(normalize(words[i-1].Text))) * 0.6
			best[i] = step{score: best[i-1].score - skip, from: i - 1, block: -1}
			for j := 0; j < i; j++ {
				text := normalize(joinWords(words[j:i]))
				if text == "" {
					continue
				}
				g := trigrams(text)
				for k := range blocks {
					c := containment(g, grams[k])
					if c < 0.6 {
						continue
					}
					// Prefer the block closest in length when several contain
					// the run alike ("Free" over "Free Later").
					mismatch := 1 - float64(min(len(text), lengths[k]))/float64(max(len(text), lengths[k]))
					score := best[j].score + float64(len(text))*(c-0.5-0.05*mismatch) - 0.5
					if score > best[i].score {
						best[i] = step{score: score, from: j, block: k}
					}
				}
			}
		}
		type run struct{ from, to, block int }
		var runs []run
		for i := n; i > 0; i = best[i].from {
			if s := best[i]; s.block >= 0 {
				runs = append([]run{{s.from, i, s.block}}, runs...)
			}
		}
		// A word OCR misread badly ("USD" as "1-JSD") matches nothing; one
		// right next to a run whose block still has text unaccounted for is
		// part of it.
		taken := make([]bool, n)
		for _, r := range runs {
			for i := r.from; i < r.to; i++ {
				taken[i] = true
			}
		}
		near := func(a, b Word) bool { return b.X-(a.X+a.W) <= 0.8*max(a.H, b.H) }
		for k := range runs {
			r := &runs[k]
			short := func() bool {
				return len(normalize(joinWords(words[r.from:r.to]))) < lengths[r.block]
			}
			for r.to < n && !taken[r.to] && near(words[r.to-1], words[r.to]) && short() {
				taken[r.to] = true
				r.to++
			}
			for r.from > 0 && !taken[r.from-1] && near(words[r.from-1], words[r.from]) && short() {
				taken[r.from-1] = true
				r.from--
			}
			out = append(out, newSegment(r.block, words[r.from:r.to]))
		}
	}
	return out
}

func newSegment(block int, words []Word) segment {
	s := segment{block: block, words: slices.Clone(words), text: joinWords(words)}
	s.box = Rect{words[0].X, words[0].Y, words[0].X + words[0].W, words[0].Y + words[0].H}
	var ems []float64
	for _, w := range words {
		s.box = s.box.union(Rect{w.X, w.Y, w.X + w.W, w.Y + w.H})
		if em, ok := wordEm(w); ok {
			ems = append(ems, em)
		}
	}
	s.em = median(ems)
	if s.em == 0 {
		s.em = (s.box.Y1 - s.box.Y0) / 0.94
	}
	var baselines []float64
	for _, w := range words {
		if _, ok := wordEm(w); ok {
			baselines = append(baselines, w.Y+w.H-descent(w.Text)*s.em)
		}
	}
	s.baseline = median(baselines)
	if s.baseline == 0 {
		s.baseline = s.box.Y1 - 0.22*s.em
	}
	return s
}

// Segoe UI proportions of the em: the height letters reach above the
// baseline and how far descenders go below it.
const (
	capHeight = 0.72
	xHeight   = 0.52
	descender = 0.22
)

// wordEm estimates the font size from a word's box and which letters it
// has: "new" only spans the x-height, "Billing" caps and descenders.
func wordEm(w Word) (float64, bool) {
	top, letters := 0.0, 0
	for _, r := range w.Text {
		switch {
		case unicode.IsUpper(r) || unicode.IsDigit(r) || strings.ContainsRune("bdfhiklt'\"$%&#@!?/\\|()[]{}", r):
			top = capHeight
			letters++
		case unicode.IsLetter(r):
			top = max(top, xHeight)
			letters++
		}
	}
	if letters == 0 || w.H <= 0 {
		return 0, false
	}
	return w.H / (top + descent(w.Text)), true
}

func descent(text string) float64 {
	if strings.ContainsAny(text, "gjpqy(),;[]{}|$") {
		return descender
	}
	return 0
}

// stripIcons drops leading bullets and icons OCR read as letters: a single
// character that is no letter, or one much taller than the words after it.
// They stay on screen untouched.
func stripIcons(words []Word) []Word {
	for len(words) > 1 {
		w := words[0]
		if len([]rune(w.Text)) != 1 {
			break
		}
		r := []rune(w.Text)[0]
		rest := median(func() []float64 {
			var hs []float64
			for _, o := range words[1:] {
				hs = append(hs, o.H)
			}
			return hs
		}())
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if !(strings.ContainsRune("oO0", r) && w.H > rest*1.25) {
				break
			}
		}
		words = words[1:]
	}
	return words
}

func groupByBlock(segs []segment) map[int][]segment {
	out := map[int][]segment{}
	for _, s := range segs {
		out[s.block] = append(out[s.block], s)
	}
	return out
}

// joinRows merges a block's segments that share a line into one, so a line
// OCR read in pieces is erased and set as a whole.
func joinRows(segs []segment) []segment {
	slices.SortFunc(segs, func(a, b segment) int {
		if math.Abs(a.baseline-b.baseline) > 0.3*max(a.em, b.em) {
			return cmpFloat(a.baseline, b.baseline)
		}
		return cmpFloat(a.box.X0, b.box.X0)
	})
	var out []segment
	for _, s := range segs {
		if n := len(out); n > 0 {
			last := out[n-1]
			if math.Abs(s.baseline-last.baseline) <= 0.3*max(s.em, last.em) && s.box.X0-last.box.X1 <= 3*max(s.em, last.em) {
				out[n-1] = newSegment(s.block, append(slices.Clone(last.words), s.words...))
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

// clusters splits a block's segments into groups of consecutive lines. A
// block appears in several places when the same text repeats ("IPV4" in two
// badges); each place gets its own copy of the translation.
func clusters(segs []segment) [][]segment {
	slices.SortFunc(segs, func(a, b segment) int { return cmpFloat(a.baseline, b.baseline) })
	var out [][]segment
	for _, s := range segs {
		if n := len(out); n > 0 {
			last := out[n-1][len(out[n-1])-1]
			first := out[n-1][0]
			gap := s.baseline - last.baseline
			if gap > 0.3*s.em && gap <= 2.2*max(s.em, last.em) && math.Abs(s.box.X0-first.box.X0) <= 3*s.em {
				out[n-1] = append(out[n-1], s)
				continue
			}
		}
		out = append(out, []segment{s})
	}
	return out
}

// covered is the share of the block's source the cluster accounts for.
func covered(cluster []segment, src string) float64 {
	n := 0
	for _, s := range cluster {
		n += len(normalize(s.text))
	}
	return float64(n) / float64(max(1, len(normalize(src))))
}

func measure(img *image.RGBA, b Block, block int, cluster []segment) Region {
	r := Region{Block: block, Text: b.Text, Bold: b.Bold}
	var ems []float64
	inkiest := 0
	for _, s := range cluster {
		r.Lines = append(r.Lines, Line{Box: s.box, Baseline: s.baseline, Em: s.em})
		ems = append(ems, s.em)
		bg := ringColor(img, eraseBox(r.Lines[len(r.Lines)-1]))
		ink, count := inkColor(img, s.box, bg)
		if count > inkiest {
			inkiest, r.Color, r.background = count, ink, bg
		}
	}
	if inkiest == 0 {
		r.Color, r.background = color.RGBA{A: 255}, ringColor(img, eraseBox(r.Lines[0]))
	}
	r.Em = median(ems)
	first := r.Lines[0]
	r.MinX, r.MaxX = first.Box.X0, first.Box.X1
	if len(r.Lines) == 1 {
		r.Align, r.MinX, r.MaxX = alignment(img, first, r.background)
	} else {
		for _, l := range r.Lines {
			r.MaxX = max(r.MaxX, l.Box.X1)
		}
	}
	return r
}

// alignment looks along the line, outwards from both ends, for where the
// background stops: at a border, another element or the selection's edge.
// Text with as much room on either side was centered (a button); text with
// a far wider gap on its left than its right was right-aligned.
func alignment(img *image.RGBA, l Line, bg color.RGBA) (Align, float64, float64) {
	b := img.Bounds()
	y := int(l.Baseline - 0.3*l.Em)
	if y < b.Min.Y || y >= b.Max.Y {
		return AlignLeft, l.Box.X0, l.Box.X1
	}
	open := func(x int) bool {
		// A pixel counts as background when the rows above and below it at
		// the text's height agree, so a stray antialiased pixel does not end
		// the run.
		for _, dy := range []int{-int(0.25 * l.Em), 0, int(0.2 * l.Em)} {
			yy := min(max(y+dy, b.Min.Y), b.Max.Y-1)
			if colorDistance(img.RGBAAt(x, yy), bg) > 40 {
				return false
			}
		}
		return true
	}
	left, right := 0, 0
	x := int(math.Floor(l.Box.X0)) - 1
	for ; x >= b.Min.X && open(x); x-- {
		left++
	}
	leftEdge := x < b.Min.X
	x = int(math.Ceil(l.Box.X1))
	for ; x < b.Max.X && open(x); x++ {
		right++
	}
	rightEdge := x >= b.Max.X
	minX := l.Box.X0 - float64(left) + 0.4*l.Em
	maxX := l.Box.X1 + float64(right) - 0.4*l.Em
	l1, r1 := float64(left), float64(right)
	switch {
	case !leftEdge && !rightEdge && l1 >= 0.4*l.Em && r1 >= 0.4*l.Em && math.Abs(l1-r1) <= max(3, 0.15*max(l1, r1)):
		return AlignCenter, minX, maxX
	case !rightEdge && r1 < 0.25*l1 && r1 < 2.5*l.Em && l1 > 4*l.Em:
		return AlignRight, minX, maxX
	}
	return AlignLeft, minX, maxX
}

// eraseBox is the area cleared for a line: the words plus room for the
// ascenders, descenders and antialiasing their boxes leave out.
func eraseBox(l Line) image.Rectangle {
	pad := max(1.5, 0.12*l.Em)
	return image.Rect(
		int(math.Floor(l.Box.X0-pad)),
		int(math.Floor(min(l.Box.Y0, l.Baseline-0.8*l.Em)-pad)),
		int(math.Ceil(l.Box.X1+pad)),
		int(math.Ceil(max(l.Box.Y1, l.Baseline+0.24*l.Em)+pad)),
	)
}

// ringColor is the most common colour on the one-pixel ring around r: the
// background the text sat on.
func ringColor(img *image.RGBA, r image.Rectangle) color.RGBA {
	type bin struct {
		n       int
		r, g, b int
	}
	bins := map[uint32]*bin{}
	var best *bin
	visit := func(x, y int) {
		if !(image.Point{x, y}.In(img.Bounds())) {
			return
		}
		c := img.RGBAAt(x, y)
		key := uint32(c.R>>3)<<10 | uint32(c.G>>3)<<5 | uint32(c.B>>3)
		e := bins[key]
		if e == nil {
			e = &bin{}
			bins[key] = e
		}
		e.n++
		e.r += int(c.R)
		e.g += int(c.G)
		e.b += int(c.B)
		if best == nil || e.n > best.n {
			best = e
		}
	}
	for x := r.Min.X - 1; x <= r.Max.X; x++ {
		visit(x, r.Min.Y-1)
		visit(x, r.Max.Y)
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		visit(r.Min.X-1, y)
		visit(r.Max.X, y)
	}
	if best == nil {
		return color.RGBA{255, 255, 255, 255}
	}
	return color.RGBA{uint8(best.r / best.n), uint8(best.g / best.n), uint8(best.b / best.n), 255}
}

// inkColor is the text colour inside box: the pixels furthest from the
// background, where antialiasing has not mixed the two.
func inkColor(img *image.RGBA, box Rect, bg color.RGBA) (color.RGBA, int) {
	r := image.Rect(int(box.X0), int(box.Y0), int(math.Ceil(box.X1)), int(math.Ceil(box.Y1))).Intersect(img.Bounds())
	far := 0.0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			far = max(far, colorDistance(img.RGBAAt(x, y), bg))
		}
	}
	if far < 30 {
		return color.RGBA{A: 255}, 0
	}
	var sr, sg, sb, n int
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if c := img.RGBAAt(x, y); colorDistance(c, bg) >= 0.75*far {
				sr += int(c.R)
				sg += int(c.G)
				sb += int(c.B)
				n++
			}
		}
	}
	return color.RGBA{uint8(sr / n), uint8(sg / n), uint8(sb / n), 255}, n
}

// erase fills r on dst from the pixels around it on src. A plain background
// is filled flat. One that changes across the area (a gradient, a tinted
// panel's edge) is blended in from the four sides, each smoothed first so
// compression noise and stray glyph pixels do not draw streaks; pixels that
// are clearly not background (a neighbour's glyph, a border) are replaced by
// the background colour before that.
func erase(dst, src *image.RGBA, r image.Rectangle, bg color.RGBA) {
	b := src.Bounds()
	r = r.Intersect(b)
	if r.Empty() {
		return
	}
	sample := func(x, y int) [3]float64 {
		c := bg
		if (image.Point{x, y}.In(b)) {
			if px := src.RGBAAt(x, y); colorDistance(px, bg) <= 24 {
				c = px
			}
		}
		return [3]float64{float64(c.R), float64(c.G), float64(c.B)}
	}
	side := func(n int, at func(i int) [3]float64) [][3]float64 {
		raw := make([][3]float64, n)
		for i := range raw {
			raw[i] = at(i)
		}
		const radius = 6
		out := make([][3]float64, n)
		for i := range out {
			var sum [3]float64
			k := 0
			for j := max(0, i-radius); j <= min(n-1, i+radius); j++ {
				for c := range sum {
					sum[c] += raw[j][c]
				}
				k++
			}
			for c := range sum {
				out[i][c] = sum[c] / float64(k)
			}
		}
		return out
	}
	top := side(r.Dx(), func(i int) [3]float64 { return sample(r.Min.X+i, r.Min.Y-1) })
	bottom := side(r.Dx(), func(i int) [3]float64 { return sample(r.Min.X+i, r.Max.Y) })
	left := side(r.Dy(), func(i int) [3]float64 { return sample(r.Min.X-1, r.Min.Y+i) })
	right := side(r.Dy(), func(i int) [3]float64 { return sample(r.Max.X, r.Min.Y+i) })
	flat := true
	for _, s := range [][][3]float64{top, bottom, left, right} {
		for _, v := range s {
			d := color.RGBA{uint8(v[0]), uint8(v[1]), uint8(v[2]), 255}
			if colorDistance(d, bg) > 6 {
				flat = false
			}
		}
	}
	w, h := float64(r.Dx()+1), float64(r.Dy()+1)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		ty := float64(y-r.Min.Y+1) / h
		for x := r.Min.X; x < r.Max.X; x++ {
			if flat {
				dst.SetRGBA(x, y, bg)
				continue
			}
			tx := float64(x-r.Min.X+1) / w
			i, j := x-r.Min.X, y-r.Min.Y
			var c [3]uint8
			for k := range c {
				v := (top[i][k]*(1-ty) + bottom[i][k]*ty + left[j][k]*(1-tx) + right[j][k]*tx) / 2
				c[k] = uint8(min(max(v+0.5, 0), 255))
			}
			dst.SetRGBA(x, y, color.RGBA{c[0], c[1], c[2], 255})
		}
	}
}

func colorDistance(a, b color.RGBA) float64 {
	dr, dg, db := float64(a.R)-float64(b.R), float64(a.G)-float64(b.G), float64(a.B)-float64(b.B)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

// normalize folds text for matching OCR words against the model's reading:
// case, spacing and punctuation go, and the letters OCR confuses are merged.
func normalize(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), "rn", "m")
	var b strings.Builder
	for _, r := range s {
		switch {
		case strings.ContainsRune("l1|!í", r):
			b.WriteRune('i')
		case strings.ContainsRune("0øº°", r):
			b.WriteRune('o')
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
		case r >= unicode.MaxASCII && unicode.IsLetter(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

func trigrams(s string) map[string]bool {
	out := map[string]bool{}
	r := []rune(s)
	if len(r) < 3 {
		if len(r) > 0 {
			out[s] = true
		}
		return out
	}
	for i := 0; i+3 <= len(r); i++ {
		out[string(r[i:i+3])] = true
	}
	return out
}

// containment is the share of a's trigrams that b also has.
func containment(a, b map[string]bool) float64 {
	if len(a) == 0 {
		return 0
	}
	n := 0
	for g := range a {
		if b[g] {
			n++
		}
	}
	return float64(n) / float64(len(a))
}

func sameText(a, b string) bool {
	return strings.Join(strings.Fields(a), " ") == strings.Join(strings.Fields(b), " ")
}

func joinWords(words []Word) string {
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = w.Text
	}
	return strings.Join(parts, " ")
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := slices.Clone(v)
	slices.Sort(s)
	return s[len(s)/2]
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
