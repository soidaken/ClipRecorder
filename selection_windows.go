package main

import (
	"bytes"
	"cliprecorder/internal/capture"
	"cliprecorder/internal/recording"
	"fmt"
	"image"
	"image/png"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

type selection struct {
	app                *app
	window             *mygo.Window
	record             bool
	translate          bool
	aspect             int
	desktop            image.Rectangle
	snapshot           *image.RGBA
	bitmap             *ui.Bitmap
	scale              float64
	anchorX, anchorY   float32
	rect               ui.Rect
	dragging, selected bool
	options            recording.Options
	fpsIndex           int
	hardwareChecked    bool
	hardwareNames      []string
	hardwareError      error
	probeParams        encoderParams
	probeRevision      uint64
	probeRunning       bool
	probeTimer         *time.Timer
	tool               annotationTool
	annotations        []annotation
	numberCount        int
	drawing            bool
	blurDraft          ui.Rect
	capturing          bool
	quality            shotQuality
	pointerDown        bool
	hintBackdrop       *ui.Bitmap
	hintRect           ui.Rect
}

type encoderParams struct {
	size    image.Point
	options recording.Options
}

// Called after settings have handled input. Encoding itself is excluded from
// the key: the probe resolves whether hardware works for the other parameters.
func (s *selection) updateEncoderCheck() {
	if s.app == nil || s.window == nil || !s.selected || s.dragging {
		return
	}
	opts := s.options
	opts.Encoding = recording.EncodingAuto
	r := s.physicalRect()
	params := encoderParams{image.Pt(r.Dx()/2*2, r.Dy()/2*2), opts}
	if params == s.probeParams {
		return
	}
	s.probeParams = params
	s.probeRevision++
	s.hardwareChecked, s.hardwareNames, s.hardwareError = false, nil, nil
	s.scheduleEncoderCheck()
}

func (s *selection) scheduleEncoderCheck() {
	if s.probeTimer != nil {
		s.probeTimer.Stop()
	}
	revision := s.probeRevision
	s.probeTimer = time.AfterFunc(300*time.Millisecond, func() {
		mygo.RunOnMain(func() {
			if s.app.selection != s || revision != s.probeRevision || s.probeRunning {
				return
			}
			s.probeRunning = true
			params := s.probeParams
			go func() {
				info, err := recording.ProbeEncoder(params.size.X, params.size.Y, params.options)
				mygo.RunOnMain(func() {
					s.probeRunning = false
					if s.app.selection != s {
						return
					}
					if revision != s.probeRevision {
						s.scheduleEncoderCheck()
						return
					}
					s.applyEncoderCheck(info, err)
					s.window.Invalidate()
				})
			}()
		})
	})
}

func (s *selection) applyEncoderCheck(info recording.EncoderInfo, err error) {
	s.hardwareChecked, s.hardwareError, s.hardwareNames = true, err, nil
	if err == nil && info.Hardware {
		s.hardwareNames = []string{info.Name}
	} else if s.options.Encoding == recording.EncodingHardware {
		s.options.Encoding = recording.EncodingSoftware
	}
}

func (s *selection) encodingStatus() string {
	if !s.selected {
		return "框选区域后，按所选参数检测编码方式"
	}
	if !s.hardwareChecked {
		return "正在按所选参数检测编码方式…"
	}
	if s.hardwareError != nil {
		return "当前参数无法编码，请调整选区或画质"
	}
	switch s.options.Encoding {
	case recording.EncodingSoftware:
		return "已选择软件编码"
	case recording.EncodingHardware:
		return "已选择硬件编码"
	default:
		if len(s.hardwareNames) > 0 {
			return "自动 · 将优先使用硬件编码"
		}
		return "自动 · 当前参数使用软件编码"
	}
}

func (s *selection) view(c *ui.Context) {
	toolTheme(c)
	if s.capturing {
		// Copying: draw only the frozen screen and the edits, so the capture
		// holds no border, size chip or toolbar. Esc and right click are
		// ignored until the capture is done.
		clean := ui.Box(c).Fill()
		clean.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Image(s.bitmap, r, ui.FillBox)
			s.paintAnnotations(p)
		})
		return
	}
	if c.Shortcut(0, ui.KeyEscape) {
		s.window.Close()
		return
	}
	root := ui.Box(c).Fill().Cursor(ui.CursorCrosshair)
	x, y, over := root.PointerPosition()
	pressed := root.Pressed()
	// A press only counts where it started: dragging in from outside the
	// selection must not draw, place markers or start a new region.
	fresh := pressed && !s.pointerDown
	s.pointerDown = pressed
	if s.selected && s.tool != toolNone && s.insideSelection(x, y) {
		s.handleAnnotationInput(x, y, pressed, fresh)
	} else if s.dragging {
		if pressed {
			s.rect = ui.Rect{X: min(s.anchorX, x), Y: min(s.anchorY, y), W: abs(s.anchorX - x), H: abs(s.anchorY - y)}
		} else {
			s.dragging = false
			s.selected = s.rect.W >= 10 && s.rect.H >= 10
			if s.selected {
				s.applyAspect()
				if s.translate && s.app != nil {
					s.app.translateSelection(s)
					return
				}
			}
		}
	} else if fresh && over && (!s.selected || s.insideSelection(x, y)) {
		// Only a press that starts here counts: an existing region is not
		// replaced by clicks outside it, and dragging in from outside does
		// nothing at all.
		s.anchorX, s.anchorY = x, y
		s.dragging = true
		s.selected = false
		s.resetAnnotations()
		s.rect = ui.Rect{X: x, Y: y}
	}
	if root.RightClicked() {
		s.window.Close()
		return
	}
	root.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Image(s.bitmap, r, ui.FillBox)
		dim := ui.RGBA(6, 10, 16, 0.38)
		box := s.rect
		if !s.dragging && !s.selected {
			p.Fill(r, dim, 0)
		} else {
			p.Fill(ui.Rect{X: 0, Y: 0, W: r.W, H: box.Y}, dim, 0)
			p.Fill(ui.Rect{X: 0, Y: box.Y, W: box.X, H: box.H}, dim, 0)
			p.Fill(ui.Rect{X: box.X + box.W, Y: box.Y, W: max(0, r.W-box.X-box.W), H: box.H}, dim, 0)
			p.Fill(ui.Rect{X: 0, Y: box.Y + box.H, W: r.W, H: max(0, r.H-box.Y-box.H)}, dim, 0)
			p.Stroke(box, ui.RGBA(0, 0, 0, 0.55), 0, 3)
			p.Stroke(box, ice, 0, 1)
			// Short corner guides stay legible over both light and dark content.
			for _, x := range []float32{box.X, box.X + box.W} {
				for _, y := range []float32{box.Y, box.Y + box.H} {
					dx, dy := float32(1), float32(1)
					if x > box.X {
						dx = -1
					}
					if y > box.Y {
						dy = -1
					}
					p.Line(x, y, x+dx*min(float32(12), box.W/2), y, 2, ink)
					p.Line(x, y, x, y+dy*min(float32(12), box.H/2), 2, ink)
				}
			}
			physical := s.physicalRect()
			label := fmt.Sprintf("%d × %d", physical.Dx(), physical.Dy())
			font := ui.Font{Family: "Segoe UI", Size: 12}
			layout := ui.ShapeText(label, font, 0)
			lw, lh := layout.Size()
			chip := ui.Rect{X: box.X, Y: max(0, box.Y-34), W: lw + 20, H: 26}
			p.Fill(chip, ui.RGBA(22, 28, 35, 0.94), 6)
			p.Stroke(chip, rim, 6, 1)
			p.TextLayout(layout, chip.X+10, chip.Y+(26-lh)/2, ink)
		}
		s.paintAnnotations(p)
	})
	root.Children(func() {
		scale := float32(s.scale)
		width, height := float32(s.desktop.Dx())/scale, float32(s.desktop.Dy())/scale
		if !s.selected {
			if !s.dragging && !s.record {
				text := "拖动框选截图区域，松开后确认复制 · Esc / 右键取消"
				if s.translate {
					text = "拖动框选英文区域，松开后自动翻译 · Esc / 右键取消"
				}
				bottom := float32(24)
				ui.Row(c).Absolute().Bottom(bottom).Left(0).Right(0).Center().PassThrough().Children(func() {
					glassPanel(ui.Row(c).Padding(12, 20).PassThrough()).Children(func() {
						ui.Text(c, text).TextColor(ink).PassThrough()
					})
				})
			}
			if s.record && !s.dragging {
				panelWidth := min(float32(392), width-16)
				s.recordControls(c, max(float32(8), (width-panelWidth)/2), max(float32(8), height-recordingPanelHeight-24), panelWidth)
			}
			return
		}
		if s.translate {
			return
		}
		if s.record {
			panelWidth := min(float32(392), width-16)
			bx := max(float32(8), min(width-panelWidth-8, s.rect.X+s.rect.W-panelWidth))
			by := s.rect.Y + s.rect.H + 10
			if by+recordingPanelHeight > height-8 {
				by = max(float32(8), s.rect.Y-recordingPanelHeight-10)
			}
			s.recordControls(c, bx, by, panelWidth)
			return
		}
		panelWidth := min(float32(390), width-16)
		bx := max(float32(8), min(width-panelWidth-8, s.rect.X+s.rect.W-panelWidth))
		by := s.rect.Y + s.rect.H + 10
		if by+54 > height {
			by = max(float32(8), s.rect.Y-64)
		}
		// The hint sits in the same band as the toolbar, aligned to the left
		// edge of the chosen region.
		hintX := max(float32(8), s.rect.X)
		if limit := bx - 8 - hintChipWidth; hintX > limit {
			hintX = max(float32(8), limit)
		}

		glassPanel(ui.Row(c).Absolute().Left(bx).Top(by).Size(panelWidth, 54).Padding(9).Gap(6)).Children(func() {
			for _, tool := range []annotationTool{toolPen, toolBlur, toolNumber, toolMark} {
				badge := ""
				if tool == toolNumber && s.numberCount > 0 {
					badge = strconv.Itoa(s.numberCount)
				}
				if annotationButton(c, tool, s.tool == tool, badge).Label(toolLabel(tool)).Clicked() {
					s.setTool(tool)
				}
			}
			ui.Box(c).Size(1, 20).Shrink(0).Background(ui.RGBA(255, 255, 255, 0.14))
			qualityControl(c, &s.quality)
			if confirmButton(c, "确认", true).Grow(1).Clicked() {
				s.confirm()
			}
		})
		// Drawn last so it stays readable when a small selection makes the
		// toolbar and the hint share the same band.
		s.hintChip(c, hintX, by+(54-hintChipHeight)/2)
	})
	if s.selected && !s.translate && c.Shortcut(0, ui.KeyEnter) {
		s.confirm()
	}
}
func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func (s *selection) physicalRect() image.Rectangle {
	scale := s.scale
	px := func(v float32) int { return int(math.Round(float64(v) * scale)) }
	r := image.Rect(px(s.rect.X), px(s.rect.Y), px(s.rect.X+s.rect.W), px(s.rect.Y+s.rect.H))
	return r.Intersect(s.snapshot.Bounds())
}
func (s *selection) confirm() {
	if s.app.selection != s || !s.selected {
		return
	}
	if s.record && (!s.hardwareChecked || s.hardwareError != nil) {
		return
	}
	rect := s.physicalRect()
	if rect.Empty() {
		return
	}
	if s.record {
		s.window.Close()
		opts := s.options
		if opts.Encoding == recording.EncodingAuto && len(s.hardwareNames) == 0 {
			opts.Encoding = recording.EncodingSoftware
		}
		s.app.record(rect.Add(s.desktop.Min), opts)
		return
	}
	if s.hasAnnotations() {
		s.copyAnnotated(rect)
		return
	}
	s.window.Close()
	// Crop the frozen image shown during selection, avoiding overlay pixels.
	s.copyImage(s.snapshot.SubImage(rect))
}

// copyAnnotated captures the selection as it is drawn, so pen strokes, blur
// boxes and markers are included without a second rasterizer.
func (s *selection) copyAnnotated(rect image.Rectangle) {
	if s.capturing {
		return
	}
	s.capturing = true
	s.window.Invalidate()
	screen := rect.Add(s.desktop.Min)
	time.AfterFunc(160*time.Millisecond, func() {
		mygo.RunOnMain(func() {
			if !s.capturing {
				return
			}
			shot, err := capture.Snapshot(screen)
			s.capturing = false
			s.window.Close()
			if err != nil {
				s.copyImage(s.snapshot.SubImage(rect))
				s.app.notify("标注未能随截图复制", err.Error())
				return
			}
			s.copyImage(shot)
		})
	})
}

func (s *selection) copyImage(img image.Image) {
	copied := s.quality.resizeCopy(img)
	go func() {
		var out bytes.Buffer
		if err := png.Encode(&out, copied); err != nil {
			s.app.notify("截图复制失败", err.Error())
			return
		}
		pngBytes := out.Bytes()
		data := transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, pngBytes)))
		if s.app != nil && s.app.shotSaveFile {
			path, err := saveShot(pngBytes)
			if err != nil {
				s.app.notify("截图保存失败", err.Error())
			} else {
				data = shotClipboard(pngBytes, path)
			}
		}
		if err := mygo.Clipboard.Write(data); err != nil {
			s.app.notify("截图复制失败", err.Error())
		}
	}()
}

// shotDir is where a copy of every confirmed shot is written.
func shotDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Pictures", "ClipRecorder")
	return dir, os.MkdirAll(dir, 0o755)
}

func saveShot(pngBytes []byte) (string, error) {
	dir, err := shotDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "shot_"+time.Now().Format("20060102_150405.000")+".png")
	return path, os.WriteFile(path, pngBytes, 0o644)
}

// shotClipboard carries the image, the file and its absolute path in one
// payload: image apps take the bitmap, editors take the text, and apps that
// accept files take the dropped file.
func shotClipboard(pngBytes []byte, path string) transfer.Data {
	return transfer.New(transfer.NewItem(
		transfer.Bytes(transfer.PNG, pngBytes),
		transfer.Bytes(transfer.Text, []byte(path)),
		transfer.Bytes(transfer.FileList, []byte(fileURI(path))),
		transfer.Bytes(transfer.URIList, []byte(fileURI(path))),
	))
}

// fileURI renders an absolute path as the file URL the clipboard formats use.
func fileURI(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

func (s *selection) recordControls(c *ui.Context, x, y, width float32) {
	glassPanel(ui.Column(c.Key("record-settings")).Absolute().Left(x).Top(y).Width(width).Height(recordingPanelHeight).
		Padding(16).Gap(12)).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, "屏幕录制").FontSize(15).FontWeight(600)
				text := "拖动框选区域，再开始录制"
				if s.selected {
					text = "区域已选定，准备就绪"
				}
				ui.Text(c, text).FontSize(12).TextColor(muted)
			})
			ui.Text(c, "Esc 取消").FontSize(11).TextColor(muted)
		})
		ui.Row(c).Gap(10).Children(func() {
			audioCheckbox(c.Key("system-audio"), &s.options.SystemAudio, "系统声音")
			audioCheckbox(c.Key("microphone"), &s.options.Microphone, "麦克风")
		})
		ui.Row(c).Gap(12).Children(func() {
			ui.Text(c, "帧率").TextColor(muted).Grow(1)
			frameRateControl(c, &s.fpsIndex)
			s.options.FPS = []int{30, 60}[s.fpsIndex]
		})
		ui.Row(c).Gap(12).Children(func() {
			ui.Text(c, "画质").TextColor(muted).Grow(1)
			index := int(s.options.Quality)
			choiceControl(c, &index, 64, "标准", "高清", "极清")
			s.options.Quality = recording.Quality(index)
		})
		s.aspectControl(c)
		ui.Column(c.Key("encoding-settings")).Gap(6).Children(func() {
			ui.Row(c).Gap(12).Children(func() {
				ui.Text(c, "编码方式").TextColor(muted).Grow(1).
					Tooltip("自动：按当前参数优先使用硬件；软件：始终使用软件；硬件：仅在检测通过后可选，参数不再支持时切到软件")
				s.updateEncoderCheck()
				index := int(s.options.Encoding)
				choiceControlEnabled(c, &index, 64, []bool{true, s.hardwareChecked && s.hardwareError == nil && len(s.hardwareNames) > 0, true}, "自动", "硬件", "软件")
				s.options.Encoding = recording.EncodingMode(index)
			})
			detail := strings.Join(s.hardwareNames, "、")
			if s.hardwareError != nil {
				detail = s.hardwareError.Error()
			}
			ui.Text(c, s.encodingStatus()).FontSize(11).TextColor(muted).Tooltip(detail)
		})
		text := "按原始像素录制，画质越高文件越大"
		if s.selected {
			r := s.physicalRect()
			w, h := r.Dx()/2*2, r.Dy()/2*2
			text = fmt.Sprintf("%d × %d · 目标 %.1f Mbps · H.264 High", w, h, float64(s.options.VideoBitrate(w, h))/1000000)
		}
		ui.Text(c, text).FontSize(11).TextColor(muted)
		ui.Box(c).Height(1).Background(ui.RGBA(255, 255, 255, 0.08)).PassThrough()
		ui.Row(c).Gap(8).Children(func() {
			label := "开始录制"
			if !s.selected {
				label = "请先框选区域"
			} else if !s.hardwareChecked {
				label = "正在检测编码…"
			}
			if toolButton(c, label, true, s.selected && s.hardwareChecked && s.hardwareError == nil).Grow(1).Clicked() {
				s.confirm()
			}
			if toolButton(c, "取消", false, true).Clicked() {
				s.window.Close()
			}
		})
	})
}

// hintBlurSigma is the backdrop blur of the hint chip: strength 75 of a 20px
// base, so the chip reads as frosted glass over any content.
const hintBlurSigma = 3.5

const hintChipHeight = float32(28)

// hintChipWidth fits "Esc 取消" at 12px with 10px padding on each side.
var hintChipWidth = func() float32 {
	w, _ := ui.ShapeText("Esc 取消", ui.Font{Family: uiFont, Size: 12}, 0).Size()
	return w + 20
}()

// hintChip shows "Esc 取消" in white on a rounded chip whose backdrop is the
// frozen screen blurred and dimmed exactly like the rest of the overlay, so it
// blends in instead of looking like a flat coloured block.
func (s *selection) hintChip(c *ui.Context, x, y float32) {
	text := "Esc 取消"
	font := ui.Font{Family: uiFont, Size: 12}
	layout := ui.ShapeText(text, font, 0)
	textW, textH := layout.Size()
	padding := float32(10)
	chipW := textW + 2*padding
	chip := ui.Box(c).Absolute().Left(x).Top(y).Size(chipW, hintChipHeight).PassThrough()
	chip.Draw(func(p *ui.Painter, r ui.Rect) {
		s.hintBackdropFor(r)
		if s.hintBackdrop != nil {
			w, h := s.hintBackdrop.Size()
			p.ImageRegion(s.hintBackdrop, ui.Rect{W: float32(w), H: float32(h)}, r, ui.ImageOptions{})
		}
		// The same dark glass tint as the toolbar, over the blurred pixels, so
		// the chip reads as part of the overlay instead of a lighter plate
		// pasted on top of it.
		p.Fill(r, ui.RGBA(22, 28, 35, 0.5), 8)
		p.Stroke(r, ui.RGBA(255, 255, 255, 0.16), 8, 1)
		textX := r.X + padding
		textY := r.Y + (r.H-textH)/2
		p.TextLayout(layout, textX, textY+1, ui.RGBA(0, 0, 0, 0.6))
		p.TextLayout(layout, textX, textY, ui.RGB(255, 255, 255))
	})
}

// hintBackdropFor caches the blurred pixels under the hint chip.
func (s *selection) hintBackdropFor(r ui.Rect) {
	if s.hintBackdrop != nil && s.hintRect == r {
		return
	}
	scale := float32(s.scale)
	src := image.Rect(int(r.X*scale), int(r.Y*scale), int((r.X+r.W)*scale), int((r.Y+r.H)*scale)).Intersect(s.snapshot.Bounds())
	if src.Empty() {
		s.hintBackdrop = nil
		return
	}
	crop := image.NewRGBA(image.Rect(0, 0, src.Dx(), src.Dy()))
	region := s.snapshot.SubImage(src).(*image.RGBA)
	for y := 0; y < src.Dy(); y++ {
		copy(crop.Pix[y*crop.Stride:y*crop.Stride+src.Dx()*4], region.Pix[y*region.Stride:y*region.Stride+src.Dx()*4])
	}
	s.hintBackdrop = ui.NewBitmap(blurRGBA(crop, hintBlurSigma*s.scale))
	s.hintRect = r
}
