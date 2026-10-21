package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"cliprecorder/internal/translation"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type keyDialog struct {
	window       *mygo.Window
	key, message string
	resume       func()
}

func (a *app) beginTranslate() {
	if a.selection != nil || a.job != nil {
		return
	}
	if a.keyDialog != nil {
		a.keyDialog.window.Focus()
		return
	}
	key, err := translation.LoadKey()
	if err != nil || key == "" {
		a.showKeyDialog(a.beginTranslate)
		return
	}
	a.beginSelection(false, true)
}

func (a *app) showKeyDialog(resume func()) {
	if a.keyDialog != nil {
		a.keyDialog.window.Focus()
		return
	}
	d := &keyDialog{resume: resume}
	a.keyDialog = d
	d.window = mygo.NewWindow(mygo.WindowOptions{Title: "DeepSeek 翻译设置", Width: 460, Height: 260, Hidden: true, AlwaysOnTop: true, DisableResize: true,
		Content: ui.View(func(c *ui.Context) {
			toolTheme(c)
			ui.Column(c).Fill().Padding(24).Gap(14).Background(ui.RGB(25, 31, 38)).Children(func() {
				ui.Text(c, "输入 DeepSeek API Key").FontSize(18).FontWeight(600)
				ui.Text(c, "密钥加密保存在本机。框选文字将发送给 DeepSeek 翻译。").FontSize(12).TextColor(muted)
				input := ui.TextInput(c, &d.key).Password().Placeholder("DeepSeek API Key").AutoFocus().Height(38).FillWidth()
				if d.message != "" {
					ui.Text(c, d.message).FontSize(12).TextColor(ui.RGB(255, 160, 160))
				}
				ui.Row(c).Gap(10).Children(func() {
					if toolButton(c, "确认并保存", true, strings.TrimSpace(d.key) != "").Clicked() || input.Submitted() {
						if err := translation.SaveKey(d.key); err != nil {
							d.message = err.Error()
							d.window.Invalidate()
							return
						}
						d.key = ""
						d.window.Close()
						if d.resume != nil {
							d.resume()
						}
					}
					if toolButton(c, "取消", false, true).Clicked() {
						d.window.Close()
					}
				})
			})
		}),
	})
	d.window.OnClosed(func() {
		d.key = ""
		if a.keyDialog == d {
			a.keyDialog = nil
		}
	})
	d.window.Show()
	d.window.Focus()
}

type translationOverlay struct {
	window *mygo.Window
	cancel context.CancelFunc
	image  *image.RGBA
	lines  []translation.Line
	colors []color.RGBA
	status string
	scale  float64
}

func (a *app) closeTranslation() {
	if a.translation != nil {
		a.translation.window.Close()
	}
}

func (a *app) translateSelection(s *selection) {
	r := s.physicalRect()
	if r.Empty() || a.selection != s {
		return
	}
	// Normalize crop coordinates so OCR bounds and color samples share an origin.
	img := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		copy(img.Pix[y*img.Stride:y*img.Stride+r.Dx()*4], s.snapshot.Pix[(r.Min.Y+y)*s.snapshot.Stride+r.Min.X*4:(r.Min.Y+y)*s.snapshot.Stride+r.Max.X*4])
	}
	s.window.Close()
	a.startTranslation(img, r.Add(s.desktop.Min))
}

func (a *app) startTranslation(img *image.RGBA, screenRect image.Rectangle) {
	if a.selection != nil || a.translation != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	o := &translationOverlay{image: img, cancel: cancel, status: "正在识别文字…", scale: 1}
	a.translation = o
	o.window = mygo.NewWindow(mygo.WindowOptions{Title: "ClipRecorder · 翻译", Width: max(1, screenRect.Dx()), Height: max(1, screenRect.Dy()), Hidden: true, Frameless: true, Panel: true,
		AlwaysOnTop: true, SkipTaskbar: true, DisableResize: true, DisableShadow: true, BackgroundColor: "#ffffff", Content: ui.View(func(c *ui.Context) { o.view(c) })})
	disableWindowAnimation(o.window)
	placePhysical(o.window, screenRect)
	o.scale = windowScale(o.window)
	o.window.OnClosed(func() {
		cancel()
		if a.translation == o {
			a.translation = nil
		}
	})
	o.window.Show()
	o.window.Focus()
	go func() {
		lines, err := translation.Recognize(ctx, img)
		if err == nil && len(lines) == 0 {
			err = fmt.Errorf("未识别到文字，请按 Esc 后重新框选")
		}
		if err == nil {
			mygo.RunOnMain(func() {
				if a.translation == o {
					o.status = "正在翻译…"
					o.window.Invalidate()
				}
			})
			var key string
			key, err = translation.LoadKey()
			if err == nil {
				lines, err = (translation.Client{}).Translate(ctx, key, lines)
			}
		}
		mygo.RunOnMain(func() {
			if a.translation != o {
				return
			}
			if errors.Is(err, translation.ErrInvalidKey) {
				o.window.Close()
				a.showKeyDialog(func() { a.startTranslation(img, screenRect) })
				return
			}
			if err != nil {
				o.status = "翻译未完成：" + err.Error() + " · Esc 关闭"
			} else {
				o.status = ""
				o.lines = lines
				for _, line := range lines {
					o.colors = append(o.colors, sourceTextColor(img, line))
				}
			}
			o.window.Invalidate()
		})
	}()
}

func (o *translationOverlay) view(c *ui.Context) {
	if c.Shortcut(0, ui.KeyEscape) {
		o.window.Close()
		return
	}
	root := ui.Box(c).Fill().Background(ui.RGB(255, 255, 255)).Clip()
	if root.RightClicked() {
		o.window.Close()
		return
	}
	root.Children(func() {
		if o.status != "" {
			ui.Text(c, o.status).Font(ui.LightTheme().Font).FontSize(13).TextColor(ui.RGB(25, 25, 25)).Padding(8)
			return
		}
		for i, line := range o.lines {
			x, y := float32(line.X/o.scale), float32(line.Y/o.scale)
			w := min(float32(line.W/o.scale)+4, float32(o.image.Bounds().Dx())/float32(o.scale)-x)
			h := float32(o.image.Bounds().Dy())/float32(o.scale) - y
			for _, next := range o.lines {
				if next.Y > line.Y+line.H/2 && next.X < line.X+line.W && next.X+next.W > line.X {
					h = min(h, float32((next.Y-line.Y)/o.scale)-1)
				}
			}
			if w <= 0 || h <= 0 {
				continue
			}
			size := max(float32(8), float32(line.H/o.scale))
			for size > 8 {
				layout := ui.ShapeText(line.Text, ui.Font{Family: ui.LightTheme().Font, Size: size}, w)
				_, lh := layout.Size()
				if lh <= h {
					break
				}
				size -= 0.5
			}
			col := o.colors[i]
			ui.Box(c).Absolute().Left(x).Top(y).Size(w, h).Clip().Children(func() {
				ui.Text(c, line.Text).Font(ui.LightTheme().Font).FontSize(size).TextColor(ui.RGB(col.R, col.G, col.B)).Width(w)
			})
		}
	})
}

func sourceTextColor(img *image.RGBA, line translation.Line) color.RGBA {
	r := image.Rect(int(line.X), int(line.Y), int(math.Ceil(line.X+line.W)), int(math.Ceil(line.Y+line.H))).Intersect(img.Bounds())
	hist := map[uint16]int{}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			p := img.RGBAAt(x, y)
			hist[uint16(p.R>>4)<<8|uint16(p.G>>4)<<4|uint16(p.B>>4)]++
		}
	}
	var background uint16
	count := 0
	for k, n := range hist {
		if n > count {
			background, count = k, n
		}
	}
	decode := func(k uint16) color.RGBA {
		return color.RGBA{uint8((k>>8)&15) * 17, uint8((k>>4)&15) * 17, uint8(k&15) * 17, 255}
	}
	bg := decode(background)
	best := color.RGBA{0, 0, 0, 255}
	count = 0
	for k, n := range hist {
		p := decode(k)
		d := abs(float32(p.R)-float32(bg.R)) + abs(float32(p.G)-float32(bg.G)) + abs(float32(p.B)-float32(bg.B))
		if d > 100 && n > count {
			best, count = p, n
		}
	}
	if count < 2 || (int(best.R)*299+int(best.G)*587+int(best.B)*114)/1000 > 210 {
		return color.RGBA{0, 0, 0, 255}
	}
	return best
}
