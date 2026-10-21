package translation

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestKeyProtection(t *testing.T) {
	key := []byte("sk-test-do-not-store-real-secrets")
	encrypted, err := protect(key, false)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, key) {
		t.Fatal("key stored as plaintext")
	}
	plain, err := protect(encrypted, true)
	if err != nil || !bytes.Equal(plain, key) {
		t.Fatalf("key roundtrip: %v", err)
	}
}

func TestSavedKeyIsEncryptedAndReplaceable(t *testing.T) {
	t.Setenv("AppData", t.TempDir())
	for _, key := range []string{"sk-test-first-local-key", "sk-test-replaced-local-key"} {
		if err := SaveKey(key); err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadKey()
		if err != nil || loaded != key {
			t.Fatalf("saved key roundtrip: %v", err)
		}
		path, err := keyPath()
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(key)) {
			t.Fatal("plaintext key on disk")
		}
	}
}

func TestWindowsOCR(t *testing.T) {
	if os.Getenv("CLIPRECORDER_OCR_TEST") != "1" {
		t.Skip("opt-in Windows OCR integration")
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(24).Background(ui.RGB(255, 255, 255)).Children(func() {
			ui.Text(c, "Hello world").Font("Segoe UI").FontSize(36).TextColor(ui.RGB(0, 0, 0))
			ui.Text(c, "Translate this text").Font("Segoe UI").FontSize(28).TextColor(ui.RGB(25, 60, 160))
		})
	}, 700, 220)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	lines, err := Recognize(ctx, tt.Image())
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for _, line := range lines {
		text.WriteString(line.Text)
		if line.X < 0 || line.Y < 0 || line.W <= 0 || line.H <= 0 {
			t.Fatalf("invalid OCR bounds: %+v", line)
		}
	}
	if !strings.Contains(strings.ReplaceAll(strings.ToLower(text.String()), " ", ""), "helloworld") {
		t.Fatalf("OCR did not recognize English: %+v", lines)
	}
	t.Logf("OCR lines: %+v", lines)
}
