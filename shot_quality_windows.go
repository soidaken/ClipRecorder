package main

import (
	"image"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
)

// shotQuality is how much of the selection is copied. The clipboard only
// carries lossless PNG (mygo writes the registered PNG format), so the levels
// differ in resolution: medium and high keep every physical pixel.
type shotQuality int

const (
	qualityLow shotQuality = iota
	qualityMedium
	qualityHigh
)

// lowQualityLongestSide is the longest side a low quality copy keeps.
const lowQualityLongestSide = 1280

func (q shotQuality) label() string {
	switch q {
	case qualityLow:
		return "低"
	case qualityHigh:
		return "高"
	default:
		return "中"
	}
}

// resizeCopy returns img as it should be copied at this quality level.
func (q shotQuality) resizeCopy(img image.Image) image.Image {
	if q != qualityLow {
		return img
	}
	b := img.Bounds()
	longest := max(b.Dx(), b.Dy())
	if longest <= lowQualityLongestSide {
		return img
	}
	scale := float64(lowQualityLongestSide) / float64(longest)
	dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(b.Dx())*scale)), max(1, int(float64(b.Dy())*scale))))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// qualityControl is the compact three-way segment for the copy resolution.
func qualityControl(c *ui.Context, current *shotQuality) {
	group := ui.Row(c).Padding(3).Gap(2).Radius(9).Background(ui.RGBA(255, 255, 255, 0.05)).
		Border(1, ui.RGBA(255, 255, 255, 0.08)).Shrink(0)
	group.Children(func() {
		for _, level := range []shotQuality{qualityLow, qualityMedium, qualityHigh} {
			active := *current == level
			segment := ui.ButtonBase(c).Size(32, 28).Radius(6).Cursor(ui.CursorPointer).FocusRing(false).
				Label("图片质量 " + level.label())
			fill, color := ui.Transparent, muted
			if active {
				fill, color = ice, ui.RGB(24, 35, 45)
			} else if segment.Hovered() {
				fill = ui.RGBA(255, 255, 255, 0.08)
			}
			segment.Background(fill)
			segment.Children(func() {
				ui.Text(c, level.label()).FontSize(12).FontWeight(500).TextColor(color)
			})
			if segment.Clicked() {
				*current = level
			}
		}
	})
}
