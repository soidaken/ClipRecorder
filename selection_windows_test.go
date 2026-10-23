package main

import (
	"cliprecorder/internal/recording"
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSelectionDragAndRedraw(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		img := image.NewRGBA(image.Rect(0, 0, 1600, 1000))
		s := &selection{desktop: img.Bounds(), snapshot: img, bitmap: ui.NewBitmap(img), scale: scale}
		tt := ui.NewTester(s.view, 800, 500)
		tt.Press(100, 100)
		tt.Move(400, 300)
		tt.Release(400, 300)
		if !s.selected || s.dragging {
			t.Fatalf("selection not completed at scale %v: %+v", scale, s)
		}
		want := image.Rect(int(100*scale), int(100*scale), int(400*scale), int(300*scale))
		if got := s.physicalRect(); got != want {
			t.Fatalf("physical region: got %v want %v", got, want)
		}
		if !tt.HasText("确认") {
			t.Fatal("confirmation controls missing")
		}
		// A click outside the chosen region must not start over.
		before := s.physicalRect()
		tt.Press(450, 150)
		tt.Move(300, 50)
		tt.Release(300, 50)
		if got := s.physicalRect(); got != before {
			t.Fatalf("outside click changed the region: %v", got)
		}
		// Dragging from inside still redraws the region.
		tt.Press(200, 200)
		tt.Move(300, 50)
		tt.Release(300, 50)
		if got := s.physicalRect(); got != image.Rect(int(200*scale), int(50*scale), int(300*scale), int(200*scale)) {
			t.Fatalf("reverse redraw: %v", got)
		}
	}
}

func TestRecordingSettingsAndSelection(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	s := &selection{record: true, desktop: img.Bounds(), snapshot: img, bitmap: ui.NewBitmap(img), scale: 1,
		options: recording.DefaultOptions(), fpsIndex: 1, hardwareChecked: true, hardwareNames: []string{"测试硬件编码器"}}
	tt := ui.NewTester(s.view, 800, 600)
	tt.SetDark(true)
	if path := os.Getenv("CLIPRECORDER_UI_SNAPSHOT"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if s.options != recording.DefaultOptions() {
		t.Fatalf("defaults: %+v", s.options)
	}
	for _, text := range []string{"系统声音", "麦克风", "30 fps", "60 fps", "标准", "高清", "极清", "编码方式", "自动", "硬件", "软件", "请先框选区域"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q", text)
		}
	}
	checkboxLabel, _ := tt.Find("系统声音")
	checkboxX, checkboxY := int(checkboxLabel.X-16), int(checkboxLabel.Y+checkboxLabel.H/2)
	checkedPixel := tt.Image().RGBAAt(checkboxX, checkboxY)
	if err := tt.Click("系统声音"); err != nil {
		t.Fatal(err)
	}
	if s.options.SystemAudio || s.options.Microphone {
		t.Fatal("must be able to disable all audio before selecting a region")
	}
	if tt.Image().RGBAAt(checkboxX, checkboxY) == checkedPixel {
		t.Fatal("unchecked state must be painted immediately after clicking")
	}
	if err := tt.Click("系统声音"); err != nil {
		t.Fatal(err)
	}
	r, ok := tt.Find("系统声音")
	if !ok {
		t.Fatal("system audio checkbox missing")
	}
	tt.ClickAt(r.X-16, r.Y+r.H/2)
	if s.options.SystemAudio {
		t.Fatal("checkbox square must turn system audio off")
	}
	tt.ClickAt(r.X-16, r.Y+r.H/2)
	if err := tt.Click("麦克风"); err != nil {
		t.Fatal(err)
	}
	if !s.options.Microphone || !s.options.SystemAudio {
		t.Fatal("microphone should not deselect system audio")
	}
	if err := tt.Click("30 fps"); err != nil {
		t.Fatal(err)
	}
	if s.options.FPS != 30 {
		t.Fatal("frame rate not updated")
	}
	if err := tt.Click("极清"); err != nil {
		t.Fatal(err)
	}
	if s.options.Quality != recording.QualityUltra {
		t.Fatal("quality not updated")
	}
	if err := tt.Click("硬件"); err != nil {
		t.Fatal(err)
	}
	if s.options.Encoding != recording.EncodingHardware {
		t.Fatal("encoder mode not updated")
	}
	tt.Press(30, 40)
	tt.Move(350, 250)
	tt.Release(350, 250)
	if !s.selected || !tt.HasText("开始录制") {
		t.Fatal("selection did not expose start control")
	}
	if !s.options.Microphone || s.options.FPS != 30 || s.options.Quality != recording.QualityUltra || s.options.Encoding != recording.EncodingHardware {
		t.Fatal("selection reset settings")
	}
	if err := tt.Click("系统声音"); err != nil {
		t.Fatal(err)
	}
	if s.options.SystemAudio || !s.options.Microphone || !s.selected {
		t.Fatal("settings click changed the selected region")
	}
}

func TestHardwareChoiceDetectionStates(t *testing.T) {
	for _, checked := range []bool{false, true} {
		img := image.NewRGBA(image.Rect(0, 0, 800, 600))
		s := &selection{record: true, desktop: img.Bounds(), snapshot: img, bitmap: ui.NewBitmap(img), scale: 1,
			options: recording.DefaultOptions(), fpsIndex: 1, hardwareChecked: checked}
		tt := ui.NewTester(s.view, 800, 600)
		status := "框选区域后，按所选参数检测编码方式"
		if !tt.HasText(status) {
			t.Fatalf("missing detection status: %s", status)
		}
		r, ok := tt.Find("硬件")
		if !ok {
			t.Fatal("hardware choice missing")
		}
		tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
		if s.options.Encoding != recording.EncodingAuto {
			t.Fatal("unavailable hardware choice must not be selectable")
		}
		if err := tt.Click("软件"); err != nil {
			t.Fatal(err)
		}
		if s.options.Encoding != recording.EncodingSoftware {
			t.Fatal("software must remain selectable")
		}
	}
}

func TestEncodingStatusMatchesUserChoice(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	s := &selection{record: true, selected: true, rect: ui.Rect{X: 20, Y: 20, W: 300, H: 100},
		desktop: img.Bounds(), snapshot: img, bitmap: ui.NewBitmap(img), scale: 1,
		options: recording.DefaultOptions(), fpsIndex: 1, hardwareChecked: true, hardwareNames: []string{"test encoder"}}
	tt := ui.NewTester(s.view, 800, 600)
	for _, choice := range []struct {
		label, status string
		mode          recording.EncodingMode
	}{
		{"软件", "已选择软件编码", recording.EncodingSoftware},
		{"硬件", "已选择硬件编码", recording.EncodingHardware},
		{"自动", "自动 · 将优先使用硬件编码", recording.EncodingAuto},
	} {
		if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		if s.options.Encoding != choice.mode || !tt.HasText(choice.status) {
			t.Fatalf("choice %s not reflected in settings/status", choice.label)
		}
		if err := tt.Click("30 fps"); err != nil {
			t.Fatal(err)
		}
		if s.options.Encoding != choice.mode {
			t.Fatal("frame rate change overwrote encoding preference")
		}
	}
}

func TestEncoderCheckUpdatesHardwareChoice(t *testing.T) {
	s := &selection{options: recording.DefaultOptions()}
	s.options.Encoding = recording.EncodingHardware
	s.applyEncoderCheck(recording.EncoderInfo{Hardware: false, Name: "software"}, nil)
	if s.options.Encoding != recording.EncodingSoftware || !s.hardwareChecked || len(s.hardwareNames) != 0 {
		t.Fatal("unsupported parameters must switch a hardware selection to software")
	}
	s.applyEncoderCheck(recording.EncoderInfo{Hardware: true, Name: "test encoder"}, nil)
	if len(s.hardwareNames) != 1 || s.hardwareNames[0] != "test encoder" {
		t.Fatal("new parameters must restore hardware availability")
	}
	if s.options.Encoding != recording.EncodingSoftware {
		t.Fatal("an explicit software choice should be retained")
	}
}

func TestEncoderBadgeFollowsResolvedMode(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	base := func() *selection {
		s := &selection{record: true, desktop: img.Bounds(), snapshot: img, bitmap: ui.NewBitmap(img), scale: 1,
			selected: true, rect: ui.Rect{X: 100, Y: 100, W: 400, H: 300},
			options: recording.DefaultOptions(), fpsIndex: 1, hardwareChecked: true, hardwareNames: []string{"NVIDIA H.264 Encoder MFT"}}
		s.options.FPS = 60
		return s
	}
	s := base()
	tt := ui.NewTester(s.view, 800, 600)
	if !tt.HasText("硬编") || tt.HasText("软编") {
		t.Fatalf("auto with hardware available must show 硬编")
	}
	if !tt.HasText("目标") {
		t.Fatal("bitrate line missing")
	}

	s = base()
	s.options.Encoding = recording.EncodingSoftware
	tt = ui.NewTester(s.view, 800, 600)
	if !tt.HasText("软编") || tt.HasText("硬编") {
		t.Fatalf("forced software must show 软编")
	}

	s = base()
	s.hardwareNames = nil
	tt = ui.NewTester(s.view, 800, 600)
	if !tt.HasText("软编") {
		t.Fatalf("auto without hardware must show 软编")
	}

	s = base()
	s.hardwareError = errorStub{}
	tt = ui.NewTester(s.view, 800, 600)
	if tt.HasText("硬编") || tt.HasText("软编") {
		t.Fatal("no badge while the encoder check failed")
	}
}

type errorStub struct{}

func (errorStub) Error() string { return "probe failed" }

func TestRecordingDragMovesSelection(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1200, 900))
	s := &selection{record: true, desktop: img.Bounds(), snapshot: img, bitmap: ui.NewBitmap(img), scale: 1,
		options: recording.DefaultOptions(), fpsIndex: 1, hardwareChecked: true, hardwareNames: []string{"测试硬件编码器"},
		selected: true, rect: ui.Rect{X: 100, Y: 100, W: 400, H: 300}}
	tt := ui.NewTester(s.view, 1200, 900)
	tt.Press(200, 200)
	tt.Move(260, 240)
	tt.Release(260, 240)
	if s.rect.X != 160 || s.rect.Y != 140 || s.rect.W != 400 || s.rect.H != 300 {
		t.Fatalf("selection did not move as a whole: %+v", s.rect)
	}
	if s.moving {
		t.Fatal("moving must stop on release")
	}
	// Dragging past the screen edge keeps the region inside it.
	tt.Press(200, 200)
	tt.Move(0, 0)
	tt.Release(0, 0)
	if s.rect.X != 0 || s.rect.Y != 0 || s.rect.W != 400 || s.rect.H != 300 {
		t.Fatalf("move must stay on screen: %+v", s.rect)
	}
}

func TestScreenshotDragStillRedraws(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	s := &selection{desktop: img.Bounds(), snapshot: img, bitmap: ui.NewBitmap(img), scale: 1,
		selected: true, rect: ui.Rect{X: 100, Y: 100, W: 300, H: 200}}
	tt := ui.NewTester(s.view, 800, 600)
	tt.Press(200, 200)
	tt.Move(300, 260)
	tt.Release(300, 260)
	if s.rect.X != 200 || s.rect.Y != 200 || s.rect.W != 100 || s.rect.H != 60 {
		t.Fatalf("screenshot drag must redraw, not move: %+v", s.rect)
	}
}
