package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSaveShotWritesFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var buf strings.Builder
	_ = buf
	f, err := os.CreateTemp(t.TempDir(), "shot-*.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	path, err := saveShot(data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, filepath.Join(home, "Pictures", "ClipRecorder")) {
		t.Fatalf("unexpected location: %s", path)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("file not written: %v", err)
	}
	saved, err := os.ReadFile(path)
	if err != nil || len(saved) != len(data) {
		t.Fatalf("content mismatch: %v", err)
	}
}

func TestImageToDIBLayout(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.SetRGBA(0, 0, color.RGBA{255, 0, 0, 255})
	img.SetRGBA(1, 0, color.RGBA{0, 255, 0, 255})
	img.SetRGBA(0, 1, color.RGBA{0, 0, 255, 255})
	img.SetRGBA(1, 1, color.RGBA{255, 255, 255, 128})
	dib := imageToDIB(img)
	if len(dib) != 40+2*2*4 {
		t.Fatalf("dib size: %d", len(dib))
	}
	width := int(uint32(dib[4]) | uint32(dib[5])<<8 | uint32(dib[6])<<16 | uint32(dib[7])<<24)
	height := int(uint32(dib[8]) | uint32(dib[9])<<8 | uint32(dib[10])<<16 | uint32(dib[11])<<24)
	if width != 2 || height != 2 || dib[14] != 32 {
		t.Fatalf("header: %dx%d %d bpp", width, height, dib[14])
	}
	// Bottom-up: the tail of the buffer holds the image's top row, in BGRA.
	top := dib[40+2*4:]
	if top[0] != 0 || top[1] != 0 || top[2] != 255 {
		t.Fatalf("last row should hold the top-left red pixel in BGRA: %v", top[:4])
	}
	bottom := dib[40:]
	if bottom[0] != 255 || bottom[1] != 0 || bottom[2] != 0 {
		t.Fatalf("first row should hold the bottom-left blue pixel: %v", bottom[:4])
	}
	if dib[40+7] != 128 {
		t.Fatalf("alpha not carried: %v", dib[40:48])
	}
}

func TestCopyPathCheckboxUnticks(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 900, 600))
	// beginSelection starts every shot with the path ticked.
	s := &selection{desktop: img.Bounds(), snapshot: img, bitmap: ui.NewBitmap(img), scale: 1,
		selected: true, rect: ui.Rect{X: 60, Y: 40, W: 700, H: 400}, quality: qualityMedium, card: true, copyPath: true}
	tt := ui.NewTester(s.view, 900, 600)
	if !tt.HasText("复制路径") {
		t.Fatal("path checkbox missing from the toolbar")
	}
	if err := tt.Click("复制路径"); err != nil {
		t.Fatal(err)
	}
	if s.copyPath {
		t.Fatalf("checkbox did not untick, copyPath=%v", s.copyPath)
	}
}

// TestCopyShotToClipboard replaces the actual clipboard, so it only runs when
// asked: CLIPRECORDER_CLIPBOARD_TEST=1 go test -run TestCopyShotToClipboard
func TestCopyShotToClipboard(t *testing.T) {
	if os.Getenv("CLIPRECORDER_CLIPBOARD_TEST") == "" {
		t.Skip("set CLIPRECORDER_CLIPBOARD_TEST=1 to exercise the system clipboard")
	}
	img := image.NewRGBA(image.Rect(0, 0, 60, 40))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 40, 160, 90, 255
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := copyShotToClipboard(img, buf.Bytes(), `C:\temp\shot.png`); err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, format := range []uintptr{cfDIB, cfUnicodeText} {
		if ok, _, _ := isFormatAvailable.Call(format); ok == 0 {
			t.Fatalf("format %d missing after write", format)
		}
	}
	if err := copyShotToClipboard(img, buf.Bytes(), ""); err != nil {
		t.Fatalf("write image only: %v", err)
	}
	if ok, _, _ := isFormatAvailable.Call(cfUnicodeText); ok != 0 {
		t.Fatal("image-only write must not leave text on the clipboard")
	}
}

// The bitmap is built straight from the card's pixels; it must match the
// generic conversion, premultiplied edges included.
func TestImageToDIBFastPathsMatchGeneric(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 7, 5))
	for i := 0; i < len(src.Pix); i += 4 {
		a := uint8(i * 11)
		src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = a/2, a/3, a, a
	}
	for name, img := range map[string]image.Image{"RGBA": src, "NRGBA": toNRGBA(src), "sub-image": src.SubImage(image.Rect(2, 1, 6, 4))} {
		got, want := imageToDIB(img), imageToDIB(struct{ image.Image }{img})
		if len(got) != len(want) {
			t.Fatalf("%s: size %d, want %d", name, len(got), len(want))
		}
		for i := range got {
			if d := int(got[i]) - int(want[i]); d < -1 || d > 1 {
				t.Fatalf("%s: byte %d is %d, want %d", name, i, got[i], want[i])
			}
		}
	}
}

func toNRGBA(src *image.RGBA) *image.NRGBA {
	out := image.NewNRGBA(src.Bounds())
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			out.Set(x, y, src.At(x, y))
		}
	}
	return out
}
