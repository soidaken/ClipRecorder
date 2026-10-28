package main

import (
	"image"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	setWindowPos    = user32.NewProc("SetWindowPos")
	getDpi          = user32.NewProc("GetDpiForWindow")
	monitorFromRect = user32.NewProc("MonitorFromRect")
	getMonitorInfo  = user32.NewProc("GetMonitorInfoW")
)

// MyGo uses DIPs. The selection overlay must cover the physical virtual
// desktop exactly, including monitors left of or above the primary monitor.
func placePhysical(w *mygo.Window, r image.Rectangle) {
	mygo.RunOnMain(func() {
		setWindowPos.Call(w.NativeHandle(), ^uintptr(0), uintptr(r.Min.X), uintptr(r.Min.Y), uintptr(r.Dx()), uintptr(r.Dy()), 0x10)
	})
}

// Disable DWM's show/restore transition for capture and translation overlays.
// This is per-window and does not change the user's system animation setting.
func disableWindowAnimation(w *mygo.Window) {
	disabled := int32(1)
	setAttribute := syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	setAttribute.Call(w.NativeHandle(), 3, uintptr(unsafe.Pointer(&disabled)), 4)
	// A rectangular overlay must cover the selected pixels, including corners.
	setAttribute.Call(w.NativeHandle(), 33, uintptr(unsafe.Pointer(&disabled)), 4)
}
func windowScale(w *mygo.Window) float64 {
	dpi, _, _ := getDpi.Call(w.NativeHandle())
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96
}

type nativeRect struct{ Left, Top, Right, Bottom int32 }

const recordingBarWidth = 372
const recordingBarHeight = 92

// placeRecordingBar puts the bar in a corner of the monitor's work area that
// the recording does not cover, and returns where it ends up in physical
// pixels. Staying outside the captured region matters beyond looks: it lets
// the bar skip content protection, so it also shows up in remote desktop and
// screen sharing views.
func placeRecordingBar(w *mygo.Window, selection image.Rectangle) image.Rectangle {
	r := nativeRect{int32(selection.Min.X), int32(selection.Min.Y), int32(selection.Max.X), int32(selection.Max.Y)}
	monitor, _, _ := monitorFromRect.Call(uintptr(unsafe.Pointer(&r)), 2)
	var info struct {
		Size          uint32
		Monitor, Work nativeRect
		Flags         uint32
	}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, _ := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		w.SetBounds(mygo.Rectangle{X: 40, Y: 40, Width: recordingBarWidth, Height: recordingBarHeight})
		return image.Rect(40, 40, 40+recordingBarWidth, 40+recordingBarHeight)
	}
	work := image.Rect(int(info.Work.Left), int(info.Work.Top), int(info.Work.Right), int(info.Work.Bottom))
	// The window is still hidden. Move it to the destination monitor first,
	// then read that monitor's effective DPI before setting its final bounds.
	var placed image.Rectangle
	for i := 0; i < 2; i++ {
		scale := windowScale(w)
		width, height, margin := int(recordingBarWidth*scale), int(recordingBarHeight*scale), int(16*scale)
		placed = recordingBarPlacement(work, width, height, margin, selection)
		placePhysical(w, placed)
	}
	return placed
}

// recordingBarPlacement returns the first corner of work, starting at the
// bottom right, whose bar does not overlap avoid.
func recordingBarPlacement(work image.Rectangle, width, height, margin int, avoid image.Rectangle) image.Rectangle {
	corners := []image.Point{
		{X: work.Max.X - width - margin, Y: work.Max.Y - height - margin},
		{X: work.Max.X - width - margin, Y: work.Min.Y + margin},
		{X: work.Min.X + margin, Y: work.Max.Y - height - margin},
		{X: work.Min.X + margin, Y: work.Min.Y + margin},
	}
	var first image.Rectangle
	for i, c := range corners {
		c.X = max(work.Min.X, c.X)
		c.Y = max(work.Min.Y, c.Y)
		bar := image.Rect(c.X, c.Y, c.X+width, c.Y+height)
		if i == 0 {
			first = bar
		}
		if !bar.Overlaps(avoid) {
			return bar
		}
	}
	// A region covering the whole work area leaves no free corner.
	return first
}
