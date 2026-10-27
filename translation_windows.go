package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cliprecorder/internal/capture"
	"cliprecorder/internal/translation"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
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
				ui.Text(c, "密钥加密保存在本机。框选内容以图片发送给 DeepSeek 识别并翻译。").FontSize(12).TextColor(muted)
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

// The overlay is a plain white sheet over the page, so a hairline frame marks
// where the translated region ends.
const translationBorderWidth = float32(2)

type translationOverlay struct {
	window *mygo.Window
	cancel context.CancelFunc
	image  *image.RGBA
	lines  []translation.Line
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
	o := &translationOverlay{image: img, cancel: cancel, status: "正在识别并翻译…", scale: 1}
	a.translation = o
	o.window = mygo.NewWindow(mygo.WindowOptions{Title: "ClipRecorder · 翻译", Width: max(1, screenRect.Dx()), Height: max(1, screenRect.Dy()), Hidden: true, Frameless: true, Panel: true,
		AlwaysOnTop: true, SkipTaskbar: true, DisableResize: true, DisableShadow: true, BackgroundColor: "#ffffff", Content: ui.View(func(c *ui.Context) { o.view(c) })})
	disableWindowAnimation(o.window)
	placePhysical(o.window, screenRect)
	o.scale = windowScale(o.window)
	o.window.OnClosed(func() {
		cancel()
		mygo.GlobalShortcut.Unregister("Escape")
		if a.translation == o {
			a.translation = nil
		}
	})
	o.window.Show()
	o.window.Focus()
	// Esc must close the overlay even when another window has the focus, so it
	// is registered as a global shortcut for as long as the overlay lives.
	if err := mygo.GlobalShortcut.Register("Escape", func() { a.closeTranslation() }); err != nil {
		log.Print("注册 Esc 关闭翻译失败: ", err)
	}

	// Keep both copies next to each other: the source crop and the rendered
	// translation, told apart by the trailing marker.
	base := filepath.Join(translationSaveDir(), "translate_"+time.Now().Format("20060102_150405"))
	originalPath := base + ".jpg"
	resultPath := base + "_zh.png"

	go func() {
		// The model shrinks images to roughly this budget itself, so sending
		// more pixels only makes the upload slower.
		payload := modelImage(img)
		jpegData, err := encodeJPEG(payload)
		if err == nil {
			// The crop is saved before the request so a failed translation still
			// leaves the source image behind.
			err = os.WriteFile(originalPath, jpegData, 0o644)
		}
		var lines []translation.Line
		if err == nil {
			var key string
			key, err = translation.LoadKey()
			if err == nil {
				lines, err = (translation.Client{}).TranslateImage(ctx, key, jpegData)
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
			}
			o.window.Invalidate()
			if err != nil {
				return
			}
			// Give the overlay a frame to paint, then store exactly what is shown.
			go func() {
				time.Sleep(450 * time.Millisecond)
				mygo.RunOnMain(func() {
					if a.translation != o {
						return
					}
					shot, shotErr := capture.Snapshot(screenRect)
					if shotErr == nil {
						shotErr = writePNG(resultPath, shot)
					}
					if shotErr != nil {
						log.Print("保存翻译结果失败: ", shotErr)
					}
				})
			}()
		})
	}()
}

func (o *translationOverlay) view(c *ui.Context) {
	if c.Shortcut(0, ui.KeyEscape) {
		o.window.Close()
		return
	}
	root := ui.Box(c).Fill().Background(ui.RGB(255, 255, 255)).
		Border(translationBorderWidth, ui.RGBA(0, 0, 0, 0.28)).Clip()
	if root.RightClicked() {
		o.window.Close()
		return
	}
	scale := float32(o.scale)
	if scale <= 0 {
		scale = 1
	}
	viewW := float32(o.image.Bounds().Dx()) / scale
	viewH := float32(o.image.Bounds().Dy()) / scale
	root.Children(func() {
		if o.status != "" {
			// Waiting text: centered in the selection, enlarged and slightly
			// transparent so the page underneath still shows through.
			size := min(max(float32(18), min(viewW, viewH)*0.09), 72)
			ui.Column(c).Fill().Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				ui.Text(c, o.status).Font(ui.LightTheme().Font).FontSize(size).TextColor(ui.RGBA(0, 0, 0, 0.45)).TextAlign(ui.Center).Width(viewW * 0.88)
			})
			return
		}
		o.renderLines(c, viewW, viewH)
	})
}

type lineBox struct{ x, y, w, h float32 }

// renderLines lays the translated text into the boxes the model reported.
// Lines of similar height share one font size, and a line that already sat in
// the middle of the selection stays centered, so a translated block reads as a
// block instead of a stack of independently sized rows.
func (o *translationOverlay) renderLines(c *ui.Context, viewW, viewH float32) {
	boxes := make([]lineBox, len(o.lines))
	for i, line := range o.lines {
		x := float32(line.X) * viewW
		y := float32(line.Y) * viewH
		boxes[i] = lineBox{x: x, y: y, w: min(float32(line.W)*viewW+4, viewW-x), h: min(float32(line.H)*viewH*1.35, viewH-y)}
		for _, next := range o.lines {
			if next.Y > line.Y+line.H/2 && next.X < line.X+line.W && next.X+next.W > line.X {
				boxes[i].h = min(boxes[i].h, float32((next.Y-line.Y)*float64(viewH))-2)
			}
		}
	}
	sizes := groupSizes(boxes)
	for i, line := range o.lines {
		b := boxes[i]
		if b.w <= 0 || b.h <= 0 {
			continue
		}
		centered := math.Abs(float64(b.x+b.w/2)-float64(viewW)/2) <= float64(viewW)*0.04
		font := ui.Font{Family: ui.LightTheme().Font, Size: max(float32(8), sizes[i])}
		size := font.Size
		for size > 8 {
			layout := ui.ShapeText(line.Text, ui.Font{Family: font.Family, Size: size}, b.w)
			if _, lineHeight := layout.Size(); lineHeight <= b.h {
				break
			}
			size -= 0.5
		}
		_, textHeight := ui.ShapeText(line.Text, ui.Font{Family: font.Family, Size: size}, b.w).Size()
		top := b.y
		if textHeight < b.h {
			top = b.y + (b.h-textHeight)/2
		}
		col := translation.ContrastOnWhite(translation.ParseColor(line.Color))
		align := ui.Start
		if centered {
			align = ui.Center
		}
		ui.Box(c).Absolute().Left(b.x).Top(top).Size(b.w, min(b.h, textHeight)+2).Clip().Children(func() {
			ui.Text(c, line.Text).Font(font.Family).FontSize(size).TextColor(ui.RGB(col.R, col.G, col.B)).TextAlign(align).Width(b.w)
		})
	}
}

// groupSizes gives every run of similar-height lines the same starting size.
func groupSizes(boxes []lineBox) []float32 {
	sizes := make([]float32, len(boxes))
	for i := range boxes {
		if sizes[i] != 0 || boxes[i].h <= 0 {
			continue
		}
		group := []int{i}
		tallest := boxes[i].h
		for j := i + 1; j < len(boxes); j++ {
			if sizes[j] == 0 && boxes[j].h > 0 && math.Abs(float64(boxes[j].h-boxes[i].h)) <= float64(boxes[i].h)*0.15 {
				group = append(group, j)
				tallest = max(tallest, boxes[j].h)
			}
		}
		for _, k := range group {
			sizes[k] = tallest * 0.95
		}
	}
	return sizes
}

// translationSaveDir is the Downloads folder, so both copies are easy to find.
func translationSaveDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads")
}

// modelPixelBudget is roughly the pixel count the model keeps after its own
// downscaling; sending more only slows the upload down.
const modelPixelBudget = 1_700_000

func modelImage(img *image.RGBA) *image.RGBA {
	b := img.Bounds()
	pixels := float64(b.Dx()) * float64(b.Dy())
	if pixels <= modelPixelBudget {
		return img
	}
	scale := math.Sqrt(modelPixelBudget / pixels)
	w := max(1, int(float64(b.Dx())*scale))
	h := max(1, int(float64(b.Dy())*scale))
	resized := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(resized, resized.Bounds(), img, b, draw.Src, nil)
	return resized
}

func encodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 82}); err != nil {
		return nil, fmt.Errorf("无法生成截图副本：%w", err)
	}
	return buf.Bytes(), nil
}

func writePNG(path string, img image.Image) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("无法生成翻译图片：%w", err)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
