package clipsync

import (
	"errors"
	"syscall"
	"unsafe"
)

var (
	user32Clip        = syscall.NewLazyDLL("user32.dll")
	kernel32Clip      = syscall.NewLazyDLL("kernel32.dll")
	openClipboard     = user32Clip.NewProc("OpenClipboard")
	closeClipboard    = user32Clip.NewProc("CloseClipboard")
	getClipboardData  = user32Clip.NewProc("GetClipboardData")
	isFormatAvailable = user32Clip.NewProc("IsClipboardFormatAvailable")
	clipboardSequence = user32Clip.NewProc("GetClipboardSequenceNumber")
	globalLock        = kernel32Clip.NewProc("GlobalLock")
	globalUnlock      = kernel32Clip.NewProc("GlobalUnlock")
	globalSize        = kernel32Clip.NewProc("GlobalSize")
	moveMemory        = kernel32Clip.NewProc("RtlMoveMemory")

	cfUnicodeText = uintptr(13)
)

// ClipboardSequence changes whenever any application replaces the clipboard, so
// polling it costs nothing compared with reading the text itself.
func ClipboardSequence() uint32 {
	value, _, _ := clipboardSequence.Call()
	return uint32(value)
}

// ReadClipboardText returns the current clipboard text, or "" when there is
// none.
func ReadClipboardText() (string, error) {
	if ok, _, _ := isFormatAvailable.Call(cfUnicodeText); ok == 0 {
		return "", nil
	}
	if ok, _, _ := openClipboard.Call(0); ok == 0 {
		return "", errors.New("剪贴板正被其它程序占用")
	}
	defer closeClipboard.Call()
	handle, _, _ := getClipboardData.Call(cfUnicodeText)
	if handle == 0 {
		return "", nil
	}
	ptr, _, _ := globalLock.Call(handle)
	if ptr == 0 {
		return "", errors.New("无法读取剪贴板内容")
	}
	defer globalUnlock.Call(handle)
	// Copy exactly what the clipboard holds: reading past the block was an
	// access violation, and aliasing it would race with other applications.
	size, _, _ := globalSize.Call(handle)
	if size < 2 {
		return "", nil
	}
	if size > maxText {
		size = maxText
	}
	units := make([]uint16, size/2)
	moveMemory.Call(uintptr(unsafe.Pointer(&units[0])), ptr, size)
	for i, unit := range units {
		if unit == 0 {
			return syscall.UTF16ToString(units[:i]), nil
		}
	}
	return syscall.UTF16ToString(units), nil
}
