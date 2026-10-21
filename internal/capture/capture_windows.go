// Package capture captures physical desktop pixels using Windows GDI.
package capture

import (
	"fmt"
	"image"
	"syscall"
	"unsafe"
)

var (
	user32       = syscall.NewLazyDLL("user32.dll")
	gdi32        = syscall.NewLazyDLL("gdi32.dll")
	getDC        = user32.NewProc("GetDC")
	releaseDC    = user32.NewProc("ReleaseDC")
	metrics      = user32.NewProc("GetSystemMetrics")
	createDC     = gdi32.NewProc("CreateCompatibleDC")
	createDIB    = gdi32.NewProc("CreateDIBSection")
	selectObject = gdi32.NewProc("SelectObject")
	deleteObject = gdi32.NewProc("DeleteObject")
	deleteDC     = gdi32.NewProc("DeleteDC")
	bitBlt       = gdi32.NewProc("BitBlt")
	flush        = gdi32.NewProc("GdiFlush")
)

type bitmapInfo struct {
	Size                   uint32
	Width, Height          int32
	Planes, BitCount       uint16
	Compression, SizeImage uint32
	XPels, YPels           int32
	ClrUsed, ClrImportant  uint32
	Colors                 [1]uint32
}

// Surface owns a reusable GDI buffer. Close it on every exit path.
type Surface struct {
	Rect                    image.Rectangle
	screen, dc, bitmap, old uintptr
	pixels                  unsafe.Pointer
}

func Desktop() image.Rectangle {
	metric := func(i uintptr) int { v, _, _ := metrics.Call(i); return int(int32(v)) }
	x, y := metric(76), metric(77)
	return image.Rect(x, y, x+metric(78), y+metric(79))
}

func New(r image.Rectangle) (*Surface, error) {
	if r.Empty() || r.Dx() > 32768 || r.Dy() > 32768 {
		return nil, fmt.Errorf("无效截图区域: %v", r)
	}
	s := &Surface{Rect: r}
	s.screen, _, _ = getDC.Call(0)
	if s.screen == 0 {
		return nil, fmt.Errorf("GetDC 失败")
	}
	s.dc, _, _ = createDC.Call(s.screen)
	if s.dc == 0 {
		s.Close()
		return nil, fmt.Errorf("CreateCompatibleDC 失败")
	}
	info := bitmapInfo{Size: 40, Width: int32(r.Dx()), Height: -int32(r.Dy()), Planes: 1, BitCount: 32}
	s.bitmap, _, _ = createDIB.Call(s.screen, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&s.pixels)), 0, 0)
	if s.bitmap == 0 || s.pixels == nil {
		s.Close()
		return nil, fmt.Errorf("CreateDIBSection 失败")
	}
	s.old, _, _ = selectObject.Call(s.dc, s.bitmap)
	if s.old == 0 || s.old == ^uintptr(0) {
		s.Close()
		return nil, fmt.Errorf("SelectObject 失败")
	}
	return s, nil
}

// Grab returns borrowed BGRA pixels, valid until the next Grab or Close.
func (s *Surface) Grab() ([]byte, error) {
	r := s.Rect
	ok, _, err := bitBlt.Call(s.dc, 0, 0, uintptr(r.Dx()), uintptr(r.Dy()), s.screen, uintptr(r.Min.X), uintptr(r.Min.Y), 0x40cc0020)
	if ok == 0 {
		return nil, fmt.Errorf("BitBlt: %w", err)
	}
	flush.Call()
	return unsafe.Slice((*byte)(s.pixels), r.Dx()*r.Dy()*4), nil
}

func (s *Surface) Close() {
	if s.old != 0 && s.old != ^uintptr(0) {
		selectObject.Call(s.dc, s.old)
		s.old = 0
	}
	if s.bitmap != 0 {
		deleteObject.Call(s.bitmap)
		s.bitmap = 0
	}
	if s.dc != 0 {
		deleteDC.Call(s.dc)
		s.dc = 0
	}
	if s.screen != 0 {
		releaseDC.Call(0, s.screen)
		s.screen = 0
	}
}

func Snapshot(r image.Rectangle) (*image.RGBA, error) {
	s, err := New(r)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	pixels, err := s.Grab()
	if err != nil {
		return nil, err
	}
	img := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for i := 0; i < len(pixels); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = pixels[i+2], pixels[i+1], pixels[i], 255
	}
	return img, nil
}
