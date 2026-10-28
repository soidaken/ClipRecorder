package main

import (
	"fmt"
	"image"
	"image/color"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Shots go to the clipboard through Win32 directly: the formats are fixed
// (PNG, CF_DIB, optionally the path as text) and owned by the system at once,
// instead of being rendered lazily by this process. Mixing formats through a
// delayed provider left the clipboard in states browsers could not paste as an
// image.
//
// Windows ties an open clipboard to the thread that opened it, so every
// session runs on one locked OS thread from OpenClipboard to CloseClipboard.
// A goroutine is otherwise free to move threads after a slow call such as
// copying a large bitmap; CloseClipboard then failed on the new thread and
// left the clipboard locked, and other programs pasted nothing.

var (
	user32Clip              = syscall.NewLazyDLL("user32.dll")
	openClipboard           = user32Clip.NewProc("OpenClipboard")
	closeClipboard          = user32Clip.NewProc("CloseClipboard")
	emptyClipboard          = user32Clip.NewProc("EmptyClipboard")
	setClipboardData        = user32Clip.NewProc("SetClipboardData")
	registerClipboardFormat = user32Clip.NewProc("RegisterClipboardFormatW")

	kernel32Clip      = syscall.NewLazyDLL("kernel32.dll")
	globalAlloc       = kernel32Clip.NewProc("GlobalAlloc")
	globalFree        = kernel32Clip.NewProc("GlobalFree")
	globalLock        = kernel32Clip.NewProc("GlobalLock")
	globalUnlock      = kernel32Clip.NewProc("GlobalUnlock")
	moveMemory        = kernel32Clip.NewProc("RtlMoveMemory")
	isFormatAvailable = user32Clip.NewProc("IsClipboardFormatAvailable")

	cfDIB         = uintptr(8)
	cfUnicodeText = uintptr(13)
	gmemMoveable  = uintptr(0x0002)
)

// copyShotToClipboard puts the shot on the clipboard as an image, and the saved
// path as text when there is one. Image-only is the default because apps that
// see text next to a bitmap paste the text (WPS cells, for one), while an
// image-only payload is pasted as a picture everywhere. img is the picture
// pngBytes encodes, so the bitmap is built without decoding it again.
//
// Nothing re-checks the clipboard afterwards: input methods and remote
// desktop tools (WeType, ToDesk) open it to read every new item, and treating
// that as a loss rewrote the clipboard, emptying it under a paste in flight.
func copyShotToClipboard(img image.Image, pngBytes []byte, path string) error {
	dib := imageToDIB(img)
	pngFormat, _, _ := registerClipboardFormat.Call(uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("PNG"))))
	var text []byte
	if path != "" {
		text = utf16Bytes(path)
	}
	return withClipboard(func() error {
		if err := setClipboardBytes(cfDIB, dib); err != nil {
			return err
		}
		if pngFormat != 0 {
			if err := setClipboardBytes(pngFormat, pngBytes); err != nil {
				return err
			}
		}
		if text != nil {
			return setClipboardBytes(cfUnicodeText, text)
		}
		return nil
	})
}

// copyTextToClipboard puts plain text on the clipboard, e.g. the path of a
// recording. Everything this app copies goes through these Win32 calls: mixing
// them with another clipboard layer in the same process left the clipboard
// offering formats that no image paste could use.
func copyTextToClipboard(text string) error {
	data := utf16Bytes(text)
	return withClipboard(func() error { return setClipboardBytes(cfUnicodeText, data) })
}

// withClipboard opens and empties the clipboard, runs fill and closes it, all
// on one locked OS thread. Everything slow (building the bitmap, encoding
// text) is done before, so other programs wait as little as possible.
func withClipboard(fill func() error) (err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err = openClipboardRetry(); err != nil {
		return err
	}
	defer func() {
		if ok, _, closeErr := closeClipboard.Call(); ok == 0 && err == nil {
			err = fmt.Errorf("CloseClipboard 失败：%w", closeErr)
		}
	}()
	if ok, _, _ := emptyClipboard.Call(); ok == 0 {
		return fmt.Errorf("无法清空剪贴板")
	}
	return fill()
}

// openClipboardRetry waits up to a second for another program, typically one
// reading the previous item, to close the clipboard.
func openClipboardRetry() error {
	for attempt := 0; attempt < 50; attempt++ {
		if ok, _, _ := openClipboard.Call(0); ok != 0 {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("剪贴板正被其它程序占用")
}

func setClipboardBytes(format uintptr, data []byte) error {
	handle, _, err := globalAlloc.Call(gmemMoveable, uintptr(len(data)))
	if handle == 0 {
		return fmt.Errorf("GlobalAlloc 失败：%w", err)
	}
	if ptr, _, _ := globalLock.Call(handle); ptr == 0 {
		globalFree.Call(handle)
		return fmt.Errorf("GlobalLock 失败")
	} else if len(data) > 0 {
		moveMemory.Call(ptr, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	}
	globalUnlock.Call(handle)
	if ok, _, setErr := setClipboardData.Call(format, handle); ok == 0 {
		globalFree.Call(handle)
		return fmt.Errorf("SetClipboardData 失败：%w", setErr)
	}
	return nil
}

// imageToDIB renders img as a bottom-up 32-bit device independent bitmap, the
// form Windows apps paste as a picture, with straight (not premultiplied)
// alpha in BGRA order.
func imageToDIB(img image.Image) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]byte, 40+w*h*4)
	put := func(offset int, v uint32) {
		out[offset] = byte(v)
		out[offset+1] = byte(v >> 8)
		out[offset+2] = byte(v >> 16)
		out[offset+3] = byte(v >> 24)
	}
	put(0, 40)        // biSize
	put(4, uint32(w)) // biWidth
	put(8, uint32(h)) // biHeight, positive: bottom-up
	put(12, 1)        // biPlanes
	put(14, 32)       // biBitCount
	for y := 0; y < h; y++ {
		row := out[40+(h-1-y)*w*4:]
		switch src := img.(type) {
		case *image.RGBA:
			// Premultiplied: undo it for every pixel that is not opaque.
			pix := src.Pix[src.PixOffset(b.Min.X, b.Min.Y+y):]
			for x := 0; x < w; x++ {
				r, g, bb, a := pix[x*4], pix[x*4+1], pix[x*4+2], pix[x*4+3]
				if a != 0xff && a != 0 {
					r = uint8(min(uint32(r)*0xff/uint32(a), 0xff))
					g = uint8(min(uint32(g)*0xff/uint32(a), 0xff))
					bb = uint8(min(uint32(bb)*0xff/uint32(a), 0xff))
				}
				row[x*4], row[x*4+1], row[x*4+2], row[x*4+3] = bb, g, r, a
			}
		case *image.NRGBA:
			pix := src.Pix[src.PixOffset(b.Min.X, b.Min.Y+y):]
			for x := 0; x < w; x++ {
				row[x*4], row[x*4+1], row[x*4+2], row[x*4+3] = pix[x*4+2], pix[x*4+1], pix[x*4], pix[x*4+3]
			}
		default:
			for x := 0; x < w; x++ {
				c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
				row[x*4], row[x*4+1], row[x*4+2], row[x*4+3] = c.B, c.G, c.R, c.A
			}
		}
	}
	return out
}

func utf16Bytes(s string) []byte {
	u := windows.StringToUTF16(s)
	b := make([]byte, len(u)*2)
	for i, v := range u {
		b[i*2] = byte(v)
		b[i*2+1] = byte(v >> 8)
	}
	return b
}
