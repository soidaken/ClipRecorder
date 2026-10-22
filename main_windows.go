package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"time"

	"cliprecorder/internal/autostart"
	"cliprecorder/internal/capture"
	"cliprecorder/internal/recording"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type app struct {
	tray        *mygo.Tray
	selection   *selection
	job         *recordJob
	quitting    bool
	translation *translationOverlay
	keyDialog   *keyDialog
	// shotSaveFile writes every confirmed shot to disk and adds its path to
	// the clipboard next to the image.
	shotSaveFile bool
}
type recordJob struct {
	cancel   context.CancelFunc
	window   *mygo.Window
	start    time.Time
	stopping bool
	options  recording.Options
	encoder  recording.EncoderInfo
}

func main() {
	mygo.App.SetName("ClipRecorder")
	mygo.Theme.SetSource(mygo.ThemeDark)
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	// The installer asks a running instance to quit this way: it is a tray app
	// with no main window, so closing its windows would never end the process.
	// Reaching this point means we own the lock, so there is nothing to quit.
	if hasArg(os.Args[1:], "--quit") {
		return
	}
	a := &app{}
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		if hasArg(args, "--quit") {
			mygo.App.Quit() // Finishes an active recording before quitting.
		}
	})
	mygo.App.OnWindowAllClosed(func() {}) // Remain in the tray between captures.
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if a.job != nil {
			e.PreventDefault()
			a.quitting = true
			a.stop()
		}
	})
	mygo.App.WhenReady(func() {
		a.shotSaveFile = true
		startupItem := a.autostartMenuItem()
		menu := mygo.NewMenu([]*mygo.MenuItem{
			{Label: "截图  ·  Ctrl+I", Click: func(*mygo.MenuItem, *mygo.Window) { a.begin(false) }},
			{Label: "开始录制  ·  Ctrl+R", Click: func(*mygo.MenuItem, *mygo.Window) { a.begin(true) }},
			{Label: "框选翻译  ·  Ctrl+T", Click: func(*mygo.MenuItem, *mygo.Window) { a.beginTranslate() }},
			{Label: "停止录制  ·  Ctrl+Shift+T", Click: func(*mygo.MenuItem, *mygo.Window) { a.stop() }},
			mygo.Separator(),
			startupItem,
			saveShotItem(a),
			{Label: "设置 DeepSeek Key…", Click: func(*mygo.MenuItem, *mygo.Window) { a.showKeyDialog(nil) }},
			mygo.Separator(),
			{Label: "退出", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
		})
		var err error
		a.tray, err = mygo.NewTray(mygo.TrayOptions{Icon: trayPNG(), ToolTip: "ClipRecorder · Ctrl+I 截图 / Ctrl+R 录制 / Ctrl+T 翻译", Menu: menu})
		if err != nil {
			log.Print(err)
			mygo.App.Quit()
			return
		}
		a.tray.OnRightClick(func() {
			if enabled, err := autostart.New().Enabled(); err == nil {
				startupItem.SetChecked(enabled)
			}
		})
		for _, shortcut := range []struct {
			key    string
			action func()
		}{
			{"Ctrl+I", func() { a.begin(false) }},
			{"Ctrl+R", func() { a.begin(true) }},
			{"Ctrl+T", a.beginTranslate},
			{"Ctrl+Shift+T", a.stop},
		} {
			if err = mygo.GlobalShortcut.Register(shortcut.key, shortcut.action); err != nil {
				a.notify("快捷键注册失败", fmt.Sprintf("%s: %v，可从托盘菜单操作", shortcut.key, err))
			}
		}
		// A bounded real-window smoke check for local development.
		if len(os.Args) > 1 && (os.Args[1] == "--smoke" || os.Args[1] == "--smoke-record") {
			a.begin(os.Args[1] == "--smoke-record")
			go func() { time.Sleep(2 * time.Second); mygo.App.Quit() }()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *app) notify(title, body string) {
	log.Printf("%s: %s", title, body)
	go func() {
		if err := mygo.NewNotification(mygo.NotificationOptions{Title: title, Body: body, Silent: true}).Show(); err != nil {
			log.Print(err)
		}
	}()
}
func (a *app) begin(record bool) {
	a.beginSelection(record, false)
}

func (a *app) beginSelection(record, translate bool) {
	if a.selection != nil || a.job != nil || a.keyDialog != nil {
		return
	}
	a.closeTranslation()
	bounds := capture.Desktop()
	snapshot, err := capture.Snapshot(bounds)
	if err != nil {
		a.notify("截图失败", err.Error())
		return
	}
	s := &selection{app: a, record: record, translate: translate, desktop: bounds, snapshot: snapshot, bitmap: ui.NewBitmap(snapshot), options: recording.DefaultOptions(), fpsIndex: 1, quality: qualityMedium}
	a.selection = s
	s.window = mygo.NewWindow(mygo.WindowOptions{
		Title: "ClipRecorder · 选择区域", Content: ui.View(s.view), Hidden: true,
		Frameless: true, AlwaysOnTop: true, SkipTaskbar: true, DisableResize: true, DisableShadow: true,
		Width: 800, Height: 600, BackgroundColor: "#101419",
	})
	disableWindowAnimation(s.window)
	placePhysical(s.window, bounds)
	s.scale = windowScale(s.window)
	s.window.OnClosed(func() {
		if s.probeTimer != nil {
			s.probeTimer.Stop()
		}
		if a.selection == s {
			a.selection = nil
		}
	})
	s.window.Show()
	s.window.Focus()
}
func (a *app) record(rect image.Rectangle, options recording.Options) {
	path, err := recording.OutputPath()
	if err != nil {
		a.notify("录屏失败", err.Error())
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &recordJob{cancel: cancel, options: options}
	a.job = j
	j.window = mygo.NewWindow(mygo.WindowOptions{
		Title: "ClipRecorder · 正在录制", Content: ui.View(func(c *ui.Context) { a.recordView(c, j) }),
		Hidden: true, Width: recordingBarWidth, Height: recordingBarHeight, Frameless: true, Panel: true, AlwaysOnTop: true, SkipTaskbar: true,
		DisableResize: true, DisableMaximize: true, BackgroundColor: "#1c2229", Vibrancy: mygo.VibrancyAcrylic,
	})
	j.window.SetContentProtection(true)
	placeRecordingBar(j.window, rect)
	j.window.OnClose(func(e *mygo.CloseEvent) { e.PreventDefault(); a.stop() })
	j.window.ShowInactive()
	go func() {
		err := recording.Record(ctx, rect, path, options, func(info recording.EncoderInfo) {
			mygo.RunOnMain(func() { j.encoder = info; j.start = time.Now(); j.window.Invalidate() })
		})
		cancel()
		mygo.RunOnMain(func() {
			a.job = nil
			// Destroy bypasses the close listener that asks an active recording to stop.
			j.window.Destroy()
			if err != nil {
				a.notify("录屏失败", err.Error())
			} else {
				a.notify("录屏已保存", path)
				if openErr := mygo.Shell.OpenPath(filepath.Dir(path)); openErr != nil {
					a.notify("打开存放目录失败", fmt.Sprintf("文件已保存至 %s：%v", path, openErr))
				}
			}
			if a.quitting {
				mygo.App.Quit()
			}
		})
	}()
}
func (a *app) stop() {
	if a.job == nil || a.job.stopping {
		return
	}
	a.job.stopping = true
	a.job.cancel()
	a.job.window.Invalidate()
}
func (a *app) recordView(c *ui.Context, j *recordJob) {
	toolTheme(c)
	top, bottom := ui.RGBA(43, 52, 62, 0.34), ui.RGBA(17, 23, 30, 0.24)
	if !c.Vibrancy() {
		top, bottom = ui.RGB(41, 48, 57), ui.RGB(26, 32, 40)
	}
	root := ui.Row(c).Fill().Padding(13, 18).Gap(18).Radius(12).
		Gradient(top, bottom, 180).Border(1, ui.RGBA(255, 255, 255, 0.22)).
		InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.12)).DragWindow()
	root.Children(func() {
		label := "准备录制…"
		status := "准备中"
		if !j.start.IsZero() {
			status = "录制中 · " + j.encoder.Label()
			seconds := int(time.Since(j.start).Seconds())
			label = fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
			c.After(time.Second)
		}
		if j.stopping {
			label = "正在保存…"
			status = "收尾中"
		}
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Row(c).Gap(7).Children(func() {
				color := ui.RGB(255, 126, 138)
				if j.stopping || j.start.IsZero() {
					color = muted
				}
				ui.Box(c).Size(6, 6).Radius(3).Background(color)
				ui.Text(c, status).FontSize(11).TextColor(muted).Tooltip(j.encoder.Name)
			})
			ui.Text(c, label).Font("Segoe UI, Microsoft YaHei UI").FontSize(24).FontWeight(500).FontFeatures("tnum").TextColor(ink)
			ui.Text(c, fmt.Sprintf("%d fps · %s · %s", j.options.FPS, j.options.Quality.Label(), j.options.AudioLabel())).FontSize(11).TextColor(muted)
		})
		ui.Box(c).Width(1).Height(42).Background(ui.RGBA(255, 255, 255, 0.12)).PassThrough()
		if recordingStopButton(c, !j.stopping).Clicked() {
			a.stop()
		}
	})
}

func trayPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	ink := color.RGBA{76, 194, 255, 255}
	for y := 4; y < 28; y++ {
		for x := 4; x < 28; x++ {
			if (x < 7 || x >= 25) && (y < 12 || y >= 20) || (y < 7 || y >= 25) && (x < 12 || x >= 20) {
				img.SetRGBA(x, y, ink)
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

// saveShotItem toggles writing a file copy (and its path on the clipboard)
// for every confirmed shot.
func saveShotItem(a *app) *mygo.MenuItem {
	item := &mygo.MenuItem{Label: "截图同时保存文件并复制路径", Checked: a.shotSaveFile}
	item.Click = func(*mygo.MenuItem, *mygo.Window) {
		a.shotSaveFile = !a.shotSaveFile
		item.SetChecked(a.shotSaveFile)
	}
	return item
}
