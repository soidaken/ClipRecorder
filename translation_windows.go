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
	if a.selection != nil {
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

// The overlay keeps the selection's own pixels, so a hairline frame marks
// where the translated region ends.
const translationBorderWidth = float32(2)

// translationHint is how long, after the translation appears, the overlay
// tells how to compare it with the source.
const translationHint = 3 * time.Second

type translationOverlay struct {
	window *mygo.Window
	cancel context.CancelFunc
	image  *image.RGBA
	scale  float64
	// source is the selection as captured, shown while translating and
	// while the pointer is held down; result has its English erased and
	// lines are the Chinese set over it.
	source *ui.Bitmap
	result *ui.Bitmap
	lines  []placedLine
	// reader lists the translation on a plain sheet when it could not be
	// placed over the screenshot (no Windows OCR, nothing matched).
	reader []translation.Block
	status string
	failed bool
	shown  time.Time
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
	o := &translationOverlay{image: img, cancel: cancel, status: "正在翻译…", scale: 1, source: ui.NewBitmap(img)}
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

	// Windows OCR finds where every word is while the model reads and
	// translates; both run at once, so placing the text costs no extra wait.
	type recognized struct {
		lines []translation.OCRLine
		err   error
	}
	ocr := make(chan recognized, 1)
	go func() {
		lines, err := translation.Recognize(ctx, img)
		ocr <- recognized{lines, err}
	}()
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
		var blocks []translation.Block
		if err == nil {
			var key string
			key, err = translation.LoadKey()
			if err == nil {
				blocks, err = (translation.Client{}).TranslateImage(ctx, key, jpegData)
			}
		}
		var erased *image.RGBA
		var regions []translation.Region
		if err == nil {
			if words := <-ocr; words.err != nil {
				log.Print("本机文字识别失败，改为列出译文: ", words.err)
			} else {
				erased, regions = translation.Plan(img, blocks, words.lines)
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
				o.status, o.failed = "翻译未完成："+err.Error(), true
				o.window.Invalidate()
				return
			}
			o.apply(blocks, erased, regions)
			o.window.Invalidate()
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

// apply shows the translation: set over the screenshot when it could be
// placed, otherwise as a list.
func (o *translationOverlay) apply(blocks []translation.Block, erased *image.RGBA, regions []translation.Region) {
	o.status = ""
	o.shown = time.Now()
	if erased == nil || len(regions) == 0 {
		o.reader = blocks
		return
	}
	o.result = ui.NewBitmap(erased)
	o.lines = layoutRegions(regions, float32(max(o.scale, 0.01)))
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
	// Holding the pointer down shows the source, to compare.
	peek := root.Pressed()
	waiting := o.status != "" && !o.failed
	root.Draw(func(p *ui.Painter, r ui.Rect) {
		if o.reader != nil && !peek {
			return
		}
		if o.result != nil && !peek {
			p.Image(o.result, r, ui.FillBox)
			for _, l := range o.lines {
				p.TextLayout(l.layout, l.x, l.y, l.color)
			}
		} else {
			p.Image(o.source, r, ui.FillBox)
		}
		if waiting {
			paintScan(p, r, time.Now())
		}
	})
	root.DrawOver(func(p *ui.Painter, r ui.Rect) {
		edge := ui.RGBA(0, 0, 0, 0.28)
		if waiting {
			edge = ui.RGBA(76, 154, 255, 0.85)
		}
		p.Stroke(r, edge, 0, translationBorderWidth)
	})
	root.Children(func() {
		switch {
		case o.reader != nil && !peek:
			o.readerView(c)
		case waiting:
			translationPill(c, o.status, false, ui.Center)
			c.After(16 * time.Millisecond) // the scan band moves
		case o.failed:
			translationPill(c, o.status+" · Esc 关闭", true, ui.Center)
		case !peek && !o.shown.IsZero() && time.Since(o.shown) < translationHint:
			// Held back until the result is stored, so the saved copy is clean.
			if time.Since(o.shown) > 600*time.Millisecond {
				translationPill(c, "按住鼠标查看原文 · Esc 关闭", false, ui.End)
			}
			c.After(100 * time.Millisecond)
		}
	})
}

// paintScan sweeps a soft band of light down the selection while it is
// being read, like a scanner passing over it.
func paintScan(p *ui.Painter, r ui.Rect, now time.Time) {
	const period = 1600 * time.Millisecond
	phase := float32(now.UnixNano()%int64(period)) / float32(period)
	band := max(float32(36), r.H*0.12)
	center := r.Y - band + phase*(r.H+2*band)
	const strips = 24
	for i := 0; i < strips; i++ {
		t := (float32(i)+0.5)/strips*2 - 1 // -1..1 across the band
		alpha := 0.16 * (1 - t*t)
		y := center + t*band/2
		p.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: band/strips + 0.5}, ui.RGBA(76, 154, 255, alpha), 0)
	}
	p.Fill(ui.Rect{X: r.X, Y: center - 0.5, W: r.W, H: 1}, ui.RGBA(76, 154, 255, 0.45), 0)
}

// translationPill is the small dark label of the overlay: progress, errors
// and the hint, at the bottom middle or the bottom right.
func translationPill(c *ui.Context, text string, alert bool, align ui.Align) {
	row := ui.Row(c).Absolute().Left(0).Right(0).Bottom(12).Padding(0, 12).Justify(align).PassThrough()
	row.Children(func() {
		pill := ui.Row(c).Height(30).Padding(0, 14).Gap(8).Radius(15).AlignItems(ui.Center).
			Background(ui.RGBA(20, 25, 32, 0.86)).Border(1, ui.RGBA(255, 255, 255, 0.14)).
			Shadow(0, 4, 14, 0, ui.RGBA(0, 0, 0, 0.22)).PassThrough()
		pill.Children(func() {
			dot := ui.RGB(110, 178, 255)
			if alert {
				dot = ui.RGB(255, 126, 138)
			}
			ui.Box(c).Size(6, 6).Radius(3).Background(dot).PassThrough()
			ui.Text(c, text).Font(uiFont).FontSize(12.5).TextColor(ui.RGB(242, 245, 248)).PassThrough()
		})
	})
}

// readerView lists the translation on a plain sheet, for when it could not
// be placed over the screenshot.
func (o *translationOverlay) readerView(c *ui.Context) {
	ui.Column(c).Fill().Padding(18, 20).Gap(10).Children(func() {
		for _, b := range o.reader {
			weight := 400
			if b.Bold {
				weight = 700
			}
			ui.Text(c, b.Text).Font(translationFont).FontSize(14).FontWeight(weight).
				TextColor(ui.RGB(31, 35, 40)).FillWidth()
		}
	})
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
