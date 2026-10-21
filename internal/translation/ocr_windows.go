package translation

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"

	"golang.org/x/image/draw"
)

//go:embed ocr.ps1
var ocrScript string

type Line struct {
	Text string  `json:"text"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
}

func Recognize(ctx context.Context, img image.Image) ([]Line, error) {
	dir, err := os.MkdirTemp("", "ClipRecorder-ocr-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	b := img.Bounds()
	scale := min(1.0, 3500.0/float64(max(b.Dx(), b.Dy())))
	if scale < 1 {
		resized := image.NewRGBA(image.Rect(0, 0, int(float64(b.Dx())*scale), int(float64(b.Dy())*scale)))
		draw.CatmullRom.Scale(resized, resized.Bounds(), img, b, draw.Src, nil)
		img = resized
	}
	f, err := os.Create(filepath.Join(dir, "selection.png"))
	if err != nil {
		return nil, err
	}
	err = png.Encode(f, img)
	closeErr := f.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	chars := utf16.Encode([]rune(ocrScript))
	encoded := make([]byte, len(chars)*2)
	for i, c := range chars {
		binary.LittleEndian.PutUint16(encoded[i*2:], c)
	}
	cmd := exec.CommandContext(ctx, filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Env = append(os.Environ(), "CLIPRECORDER_OCR_IMAGE="+f.Name())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("Windows 文字识别失败：%s", strings.TrimSpace(stderr.String()))
	}
	var result struct {
		Lines []Line `json:"lines"`
	}
	if err = json.Unmarshal(bytes.TrimPrefix(out, []byte{0xef, 0xbb, 0xbf}), &result); err != nil {
		return nil, fmt.Errorf("无法读取文字识别结果：%w", err)
	}
	for i := range result.Lines {
		l := &result.Lines[i]
		l.X /= scale
		l.Y /= scale
		l.W /= scale
		l.H /= scale
	}
	return result.Lines, nil
}
