package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
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

func TestShotClipboardIsImageOnlyByDefault(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
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
	payload := shotClipboard(data, "")
	formats := payload.Formats()
	for _, f := range formats {
		if f == transfer.Text || f == transfer.FileList || f == transfer.URIList {
			t.Fatalf("image-only payload must not carry %s: %v", f, formats)
		}
	}
	if _, err := payload.Read(transfer.PNG); err != nil {
		t.Fatalf("image missing: %v", err)
	}
}

func TestShotClipboardCarriesImageFileAndPath(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	dir := t.TempDir()
	path := filepath.Join(dir, "shot one.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	payload := shotClipboard(data, path)
	formats := payload.Formats()
	for _, want := range []transfer.Format{transfer.PNG, transfer.Text, transfer.FileList} {
		found := false
		for _, f := range formats {
			if f == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("clipboard misses %s: %v", want, formats)
		}
	}
	text, err := payload.Read(transfer.Text)
	if err != nil || string(text) != path {
		t.Fatalf("path text: %q %v", text, err)
	}
	files, err := payload.Files()
	if err != nil || len(files) != 1 || files[0] != path {
		t.Fatalf("file representation: %v %v", files, err)
	}
	if !strings.Contains(fileURI(path), "file:///") || !strings.Contains(fileURI(path), "%20") {
		t.Fatalf("file URI not encoded: %s", fileURI(path))
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
