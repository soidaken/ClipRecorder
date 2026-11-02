package main

import (
	"image"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"cliprecorder/internal/recording"

	"cliprecorder/internal/i18n"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestScreenshotToolbarFollowsLanguage(t *testing.T) {
	previous := i18n.Current()
	t.Cleanup(func() { i18n.Use(previous) })

	img := image.NewRGBA(image.Rect(0, 0, 900, 600))
	s := &selection{desktop: img.Bounds(), snapshot: img, bitmap: ui.NewBitmap(img), scale: 1,
		selected: true, rect: ui.Rect{X: 60, Y: 40, W: 700, H: 400}, quality: qualityMedium, card: true}

	i18n.Use(i18n.EN)
	tt := ui.NewTester(s.view, 900, 600)
	for _, text := range []string{"Done", "Copy path", "Low", "Med", "High"} {
		if !tt.HasText(text) {
			t.Fatalf("English toolbar is missing %q", text)
		}
	}
	if tt.HasText("确认") || tt.HasText("复制路径") {
		t.Fatal("Chinese labels survived the English switch")
	}

	i18n.Use(i18n.JA)
	tt = ui.NewTester(s.view, 900, 600)
	if !tt.HasText("確認") || !tt.HasText("パスをコピー") {
		t.Fatal("Japanese toolbar labels missing")
	}

	i18n.Use(i18n.ZHHant)
	tt = ui.NewTester(s.view, 900, 600)
	if !tt.HasText("確認") || !tt.HasText("複製路徑") {
		t.Fatal("Traditional Chinese toolbar labels missing")
	}
}

func TestRecordingPanelFollowsLanguage(t *testing.T) {
	previous := i18n.Current()
	t.Cleanup(func() { i18n.Use(previous) })
	i18n.Use(i18n.EN)
	img := image.NewRGBA(image.Rect(0, 0, 900, 700))
	s := &selection{record: true, desktop: img.Bounds(), snapshot: img, bitmap: ui.NewBitmap(img), scale: 1,
		selected: true, rect: ui.Rect{X: 100, Y: 100, W: 400, H: 300}, quality: qualityMedium, hardwareChecked: true}
	tt := ui.NewTester(s.view, 900, 700)
	for _, text := range []string{"Screen recording", "System audio", "Microphone", "Frame rate", "Quality", "Encoder", "Start recording"} {
		if !tt.HasText(text) {
			t.Fatalf("English recording panel is missing %q", text)
		}
	}
}

func TestTrayItemLabelsFollowLanguage(t *testing.T) {
	previous := i18n.Current()
	t.Cleanup(func() { i18n.Use(previous) })

	a := &app{shotSaveFile: true}
	i18n.Use(i18n.JA)
	items := []*mygo.MenuItem{a.autostartMenuItem(), saveShotItem(a), imeIdleItem()}
	want := []string{"スタートアップ", "撮影画像を保存", "6 秒入力が無ければ英語入力"}
	for i, item := range items {
		if item.Label != want[i] {
			t.Fatalf("item %d label = %q, want %q", i, item.Label, want[i])
		}
	}

	i18n.Use(i18n.EN)
	a2 := &app{shotSaveFile: true}
	if got := saveShotItem(a2).Label; got != "Save shots to disk" {
		t.Fatalf("english label: %q", got)
	}
	if got := imeIdleItem().Label; got != "English after 6 s without typing" {
		t.Fatalf("english ime label: %q", got)
	}
}

func TestRecordingPanelTranslatesRatioAndStatus(t *testing.T) {
	previous := i18n.Current()
	t.Cleanup(func() { i18n.Use(previous) })
	i18n.Use(i18n.JA)
	img := image.NewRGBA(image.Rect(0, 0, 900, 700))
	s := &selection{record: true, desktop: img.Bounds(), snapshot: img, bitmap: ui.NewBitmap(img), scale: 1,
		selected: true, rect: ui.Rect{X: 100, Y: 100, W: 400, H: 300}, quality: qualityMedium, hardwareChecked: true}
	tt := ui.NewTester(s.view, 900, 700)
	for _, text := range []string{"アスペクト比", "録画開始", "フレームレート", "画質", "エンコード"} {
		if !tt.HasText(text) {
			t.Fatalf("Japanese recording panel is missing %q", text)
		}
	}
	if tt.HasText("比例") || tt.HasText("开始录制") {
		t.Fatal("Chinese labels survived the Japanese switch")
	}
}

func TestRecordingBarFollowsLanguage(t *testing.T) {
	previous := i18n.Current()
	t.Cleanup(func() { i18n.Use(previous) })
	i18n.Use(i18n.JA)

	a := &app{}
	job := &recordJob{start: time.Now().Add(-65 * time.Second), options: recording.DefaultOptions()}
	tt := ui.NewTester(func(c *ui.Context) { a.recordView(c, job) }, 420, 120)
	if !tt.HasText("ソフトウェアエンコード") {
		t.Fatalf("encoder label not translated: %q", tt.Texts())
	}
	if !tt.HasText("高画質") || !tt.HasText("システム音声") {
		t.Fatalf("quality/audio labels not translated: %q", tt.Texts())
	}
	if !tt.HasText("停止") {
		t.Fatalf("stop button not translated: %q", tt.Texts())
	}
	if tt.HasText("录制中") || tt.HasText("高清") {
		t.Fatal("Chinese labels survived the Japanese switch")
	}
}

// TestNoHardcodedChineseLabels keeps accessible labels and tooltips in the
// translation catalog: a literal here would stay Chinese in every language.
func TestNoHardcodedChineseLabels(t *testing.T) {
	hasCJK := func(s string) bool {
		for _, r := range s {
			if r >= 0x4e00 && r <= 0x9fff {
				return true
			}
		}
		return false
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\.Label\("([^"]*)"\)`),
		regexp.MustCompile(`\.Tooltip\("([^"]*)"\)`),
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, pattern := range patterns {
			for _, match := range pattern.FindAllStringSubmatch(string(source), -1) {
				if hasCJK(match[1]) {
					t.Fatalf("%s hardcodes a Chinese label: %s", name, match[0])
				}
			}
		}
	}
}

func TestPairingQRCodeIsGenerated(t *testing.T) {
	previous := syncClip.qr
	previousLine := syncClip.qrLine
	t.Cleanup(func() { syncClip.qr, syncClip.qrLine = previous, previousLine })

	first := syncQRCode()
	if first == nil {
		t.Fatal("no QR code for the pairing line")
	}
	w, h := first.Size()
	if w < 100 || h < 100 {
		t.Fatalf("QR code too small: %dx%d", w, h)
	}
	if second := syncQRCode(); second != first {
		t.Fatal("the QR code should be cached until the pairing line changes")
	}
}
