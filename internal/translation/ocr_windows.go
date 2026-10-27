package translation

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unicode/utf16"

	"golang.org/x/image/draw"
)

//go:embed ocr.ps1
var ocrScript string

// Word is one word Windows OCR found, in pixels of the recognized image.
type Word struct {
	Text       string
	X, Y, W, H float64
}

// OCRLine is a line of words in reading order.
type OCRLine struct{ Words []Word }

// ocrUpscaleLimit is the longest side up to which a selection is enlarged
// twice before recognition: Windows OCR misreads and skips UI-sized text at
// its native size, and reads it reliably at double.
const ocrUpscaleLimit = 2400

// Recognize runs Windows OCR on img and returns the words with boxes in img's
// pixels. The text only helps to place the model's blocks; the boxes are
// what the overlay is laid out by.
func Recognize(ctx context.Context, img *image.RGBA) ([]OCRLine, error) {
	b := img.Bounds()
	scale := 1.0
	src := image.Image(img)
	if max(b.Dx(), b.Dy()) <= ocrUpscaleLimit {
		scale = 2
		big := image.NewRGBA(image.Rect(0, 0, b.Dx()*2, b.Dy()*2))
		draw.CatmullRom.Scale(big, big.Bounds(), img, b, draw.Src, nil)
		src = big
	}
	f, err := os.CreateTemp("", "cliprecorder-ocr-*.png")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	err = png.Encode(f, src)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	// Windows PowerShell carries the WinRT projection the script needs.
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodeCommand(ocrScript))
	cmd.Env = append(os.Environ(), "CLIPRECORDER_OCR_IMAGE="+f.Name())
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err = cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("本机文字识别失败：%s", msg)
		}
		return nil, fmt.Errorf("本机文字识别失败：%w", err)
	}
	return parseOCR(stdout.Bytes(), scale)
}

func parseOCR(out []byte, scale float64) ([]OCRLine, error) {
	var result struct {
		Lines []struct {
			Words [][]any `json:"words"`
		} `json:"lines"`
	}
	if err := json.Unmarshal(bytes.TrimPrefix(bytes.TrimSpace(out), []byte{0xef, 0xbb, 0xbf}), &result); err != nil {
		return nil, fmt.Errorf("本机文字识别结果无法解析：%w", err)
	}
	var lines []OCRLine
	for _, l := range result.Lines {
		var line OCRLine
		for _, w := range l.Words {
			if len(w) != 5 {
				continue
			}
			text, _ := w[0].(string)
			var box [4]float64
			ok := text != ""
			for i := range box {
				v, isNumber := w[i+1].(float64)
				ok = ok && isNumber
				box[i] = v / scale
			}
			if ok && box[2] > 0 && box[3] > 0 {
				line.Words = append(line.Words, Word{Text: text, X: box[0], Y: box[1], W: box[2], H: box[3]})
			}
		}
		if len(line.Words) > 0 {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

// encodeCommand is PowerShell's -EncodedCommand form: base64 of UTF-16LE.
func encodeCommand(script string) string {
	units := utf16.Encode([]rune(script))
	b := make([]byte, len(units)*2)
	for i, u := range units {
		b[2*i], b[2*i+1] = byte(u), byte(u>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}
