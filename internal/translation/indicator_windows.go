package translation

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/image/draw"
)

//go:embed indicator.ps1
var indicatorScript string

// RecognizeIndicator reads the text of a small screenshot of the input method
// indicator near the clock, with a Chinese capable OCR engine. The caller looks
// for 中 / 英 in the result to tell which mode the IME is in.
func RecognizeIndicator(ctx context.Context, img *image.RGBA) (string, error) {
	b := img.Bounds()
	if b.Empty() {
		return "", fmt.Errorf("指示器区域为空")
	}
	// Small glyphs: recognize an enlarged copy.
	big := image.NewRGBA(image.Rect(0, 0, b.Dx()*3, b.Dy()*3))
	draw.CatmullRom.Scale(big, big.Bounds(), img, b, draw.Src, nil)
	f, err := os.CreateTemp("", "cliprecorder-indicator-*.png")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	err = png.Encode(f, big)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodeCommand(indicatorScript))
	cmd.Env = append(os.Environ(), "CLIPRECORDER_OCR_IMAGE="+f.Name())
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("读取输入法指示器失败：%s", msg)
		}
		return "", fmt.Errorf("读取输入法指示器失败：%w", err)
	}
	return strings.TrimSpace(strings.TrimPrefix(stdout.String(), "\ufeff")), nil
}
