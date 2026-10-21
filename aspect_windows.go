package main

import (
	"math"

	"github.com/egoist/mygo/ui"
)

var recordingRatios = []struct {
	label string
	w, h  float32
}{{"9:16", 9, 16}, {"16:9", 16, 9}, {"3:4", 3, 4}, {"4:3", 4, 3}, {"1:1", 1, 1}}

// Keep one dimension, choosing the smaller change to the current rectangle.
// Fit to the desktop and keep the top-left corner whenever possible.
func fitAspect(r ui.Rect, ratio, width, height float32) ui.Rect {
	if r.W <= 0 || r.H <= 0 || ratio <= 0 {
		return r
	}
	w, h := r.W, r.W/ratio
	if math.Abs(float64(r.H*ratio-r.W)) < math.Abs(float64(h-r.H)) {
		w, h = r.H*ratio, r.H
	}
	factor := min(float32(1), width/w, height/h)
	w, h = w*factor, h*factor
	return ui.Rect{X: max(float32(0), min(r.X, width-w)), Y: max(float32(0), min(r.Y, height-h)), W: w, H: h}
}

func (s *selection) applyAspect() {
	if !s.record || s.aspect == 0 || s.rect.W <= 0 || s.rect.H <= 0 {
		return
	}
	r := recordingRatios[s.aspect-1]
	s.rect = fitAspect(s.rect, r.w/r.h, float32(s.desktop.Dx())/float32(s.scale), float32(s.desktop.Dy())/float32(s.scale))
	// Exact ratio with even physical dimensions, as required by H.264.
	unit := max(float32(2), float32(math.Floor(float64(min(s.rect.W*float32(s.scale)/r.w, s.rect.H*float32(s.scale)/r.h)/2)))*2)
	s.rect.W, s.rect.H = unit*r.w/float32(s.scale), unit*r.h/float32(s.scale)
}

func (s *selection) aspectControl(c *ui.Context) {
	ui.Row(c).Gap(10).Children(func() {
		ui.Text(c, "比例").TextColor(muted).Grow(1).Tooltip("默认自由框选；再次点击已选比例可取消限制")
		ui.Row(c).Padding(3).Gap(3).Radius(9).Background(ui.RGBA(0, 0, 0, 0.18)).Children(func() {
			for i, ratio := range recordingRatios {
				b := ui.ButtonBase(c.Key(ratio.label)).Size(48, 28).Radius(6).Cursor(ui.CursorPointer)
				if b.Clicked() {
					if s.aspect == i+1 {
						s.aspect = 0
					} else {
						s.aspect = i + 1
						s.applyAspect()
					}
				}
				color := muted
				if s.aspect == i+1 {
					b.Background(ui.RGBA(255, 255, 255, 0.13)).Border(1, rim)
					color = ink
				}
				b.Role(ui.RoleRadio).Checked(s.aspect == i+1).Children(func() {
					ui.Text(c, ratio.label).Font("Segoe UI").FontSize(12).TextColor(color)
				})
			}
		})
	})
}
