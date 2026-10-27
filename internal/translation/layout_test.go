package translation

import (
	"image"
	"image/color"
	"math"
	"strings"
	"testing"
)

// line builds an OCR line from words laid out left to right at a 13px font,
// each as tall as its letters reach.
func line(x, y float64, texts ...string) OCRLine {
	var l OCRLine
	for _, t := range texts {
		w := float64(len(t)) * 7
		top := xHeight
		if strings.ContainsAny(t, "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789bdfhiklt$") {
			top = capHeight
		}
		h := 13 * (top + descent(t))
		l.Words = append(l.Words, Word{Text: t, X: x, Y: y, W: w, H: h})
		x += w + 4
	}
	return l
}

func segmentTexts(segs []segment) map[string]int {
	out := map[string]int{}
	for _, s := range segs {
		out[s.text] = s.block
	}
	return out
}

func TestAssignSplitsLabelAndValueOnOneLine(t *testing.T) {
	blocks := []Block{{Src: "IP changes", Text: "IP 变更"}, {Src: "After the upgrade, a new address stays", Text: "升级后，新地址保留"}}
	got := segmentTexts(assign(blocks, []OCRLine{line(7, 170, "IP", "changes", "After", "the", "upgrade,", "a", "new", "address", "stays")}))
	if got["IP changes"] != 0 || len(got) != 2 {
		t.Fatalf("label and value must be told apart: %v", got)
	}
	if b, ok := got["After the upgrade, a new address stays"]; !ok || b != 1 {
		t.Fatalf("value missing: %v", got)
	}
}

func TestAssignPrefersTheBlockOfTheSameLength(t *testing.T) {
	blocks := []Block{{Src: "Free Later", Text: "稍后免费"}, {Src: "Free", Text: "免费"}, {Src: "IPV4", Text: "IPV4"}}
	got := segmentTexts(assign(blocks, []OCRLine{line(52, 140, "Free", "Later", "Free", "IPV4")}))
	if got["Free Later"] != 0 || got["Free"] != 1 || got["IPV4"] != 2 {
		t.Fatalf("badges matched to the wrong blocks: %v", got)
	}
}

func TestAssignKeepsAMisreadWordInItsLine(t *testing.T) {
	blocks := []Block{{Src: "Charged from the day of the upgrade, then with each service renewal", Text: "从升级当天起计费"}}
	segs := assign(blocks, []OCRLine{line(80, 147, "Charged", "frorn", "the", "day", "Of", "the", "upgrade,", "then", "with", "each", "service", "renewal")})
	if len(segs) != 1 || len(segs[0].words) != 12 {
		t.Fatalf("one misread word must not split the line: %+v", segs)
	}
}

func TestAssignTakesInAnUnreadableNeighbour(t *testing.T) {
	blocks := []Block{{Src: "$1.00 USD", Text: "1.00 美元"}}
	segs := assign(blocks, []OCRLine{line(449, 460, "$1.00", "1-JSD")})
	if len(segs) != 1 || len(segs[0].words) != 2 {
		t.Fatalf("misread word next to its run must be erased with it: %+v", segs)
	}
}

func TestStripIconsKeepsWordsButDropsBullets(t *testing.T) {
	words := []Word{{Text: "•", H: 3}, {Text: "A", H: 9}, {Text: "line", H: 9}}
	if got := stripIcons(words); len(got) != 2 || got[0].Text != "A" {
		t.Fatalf("bullet must go, the article stay: %+v", got)
	}
	icon := []Word{{Text: "O", H: 13}, {Text: "A", H: 9}, {Text: "reboot", H: 9}}
	if got := stripIcons(icon); len(got) != 2 || got[0].Text != "A" {
		t.Fatalf("an icon read as O must go: %+v", got)
	}
}

func TestWordEmReadsTheLettersOfTheWord(t *testing.T) {
	for _, w := range []Word{{Text: "Price", H: 9.4}, {Text: "Billing", H: 12.2}, {Text: "new", H: 6.8}} {
		em, ok := wordEm(w)
		if !ok || math.Abs(em-13) > 0.6 {
			t.Fatalf("%s: em %.2f, want about 13", w.Text, em)
		}
	}
}

// page paints a white page with dark "glyph" bars where the words are.
func page(w, h int, ink color.RGBA, lines ...OCRLine) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	for _, l := range lines {
		for _, wd := range l.Words {
			for y := int(wd.Y); y < int(wd.Y+wd.H); y++ {
				for x := int(wd.X); x < int(wd.X+wd.W); x++ {
					img.SetRGBA(x, y, ink)
				}
			}
		}
	}
	return img
}

func TestPlanErasesSourceAndMeasuresIt(t *testing.T) {
	ink := color.RGBA{30, 60, 120, 255}
	first := line(20, 40, "Addresses", "in", "the", "paid", "range")
	second := line(20, 60, "come", "with", "no", "guarantee")
	img := page(400, 120, ink, first, second)
	blocks := []Block{{Src: "Addresses in the paid range come with no guarantee", Text: "付费范围内的地址不作保证"}}
	erased, regions := Plan(img, blocks, []OCRLine{first, second})
	if len(regions) != 1 || len(regions[0].Lines) != 2 {
		t.Fatalf("a paragraph is one region of two lines: %+v", regions)
	}
	r := regions[0]
	if r.Color != ink {
		t.Fatalf("text colour %v, want %v", r.Color, ink)
	}
	if math.Abs(r.Em-13) > 1 || r.Align != AlignLeft {
		t.Fatalf("em %.2f align %d", r.Em, r.Align)
	}
	for _, l := range []OCRLine{first, second} {
		for _, w := range l.Words {
			if p := erased.RGBAAt(int(w.X+w.W/2), int(w.Y+w.H/2)); p != (color.RGBA{255, 255, 255, 255}) {
				t.Fatalf("%s not erased: %v", w.Text, p)
			}
		}
	}
	if img.RGBAAt(25, 45) != ink {
		t.Fatal("the source image must stay untouched")
	}
}

func TestPlanLeavesUntranslatedTextAlone(t *testing.T) {
	l := line(20, 40, "2026-10-31")
	img := page(200, 80, color.RGBA{A: 255}, l)
	erased, regions := Plan(img, []Block{{Src: "2026-10-31", Text: "2026-10-31"}}, []OCRLine{l})
	if len(regions) != 0 || erased.RGBAAt(25, 45) != (color.RGBA{A: 255}) {
		t.Fatal("text the translation keeps must keep its pixels")
	}
}

func TestAlignmentFindsButtonsAndRightColumns(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 60))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	// A grey button from x 100 to 220 with its label in the middle.
	for y := 10; y < 50; y++ {
		img.SetRGBA(100, y, color.RGBA{120, 120, 120, 255})
		img.SetRGBA(220, y, color.RGBA{120, 120, 120, 255})
	}
	bg := color.RGBA{255, 255, 255, 255}
	label := Line{Box: Rect{140, 24, 180, 33}, Baseline: 33, Em: 13}
	if a, _, _ := alignment(img, label, bg); a != AlignCenter {
		t.Fatalf("button label: align %d", a)
	}
	// A value pressed against the button's left border from the right, far
	// from anything on its left.
	value := Line{Box: Rect{60, 24, 96, 33}, Baseline: 33, Em: 13}
	for y := 10; y < 50; y++ {
		img.SetRGBA(2, y, color.RGBA{120, 120, 120, 255})
	}
	if a, _, _ := alignment(img, value, bg); a != AlignRight {
		t.Fatalf("right column value: align %d", a)
	}
}

func TestEraseBlendsAGradient(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 60, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 60; x++ {
			v := uint8(100 + y*2)
			img.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	dst := image.NewRGBA(img.Bounds())
	copy(dst.Pix, img.Pix)
	erase(dst, img, image.Rect(10, 20, 50, 40), img.RGBAAt(30, 30))
	prev := 0
	for y := 20; y < 40; y++ {
		v := int(dst.RGBAAt(30, y).R)
		if v < prev {
			t.Fatalf("gradient must keep running down the erased area: %d after %d", v, prev)
		}
		prev = v
	}
}

func TestParseOCRScalesBackAndSkipsBadWords(t *testing.T) {
	out := append([]byte{0xef, 0xbb, 0xbf}, `{"lines":[{"words":[["Price",14,250,56,18],["bad",1,2]]},{"words":[]}]}`...)
	lines, err := parseOCR(out, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || len(lines[0].Words) != 1 {
		t.Fatalf("lines: %+v", lines)
	}
	if w := lines[0].Words[0]; w.X != 7 || w.Y != 125 || w.W != 28 || w.H != 9 {
		t.Fatalf("coordinates not scaled back: %+v", w)
	}
}
