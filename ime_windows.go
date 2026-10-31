package main

import (
	"encoding/binary"
	"log"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// After a few seconds without keyboard input the input method is switched to
// English the same way Ctrl+Space does, so typing after a pause does not start
// with half a Chinese sentence. A low level keyboard hook only records when the
// last key was pressed; it never swallows anything.

const (
	imeIdleTimeout = 6 * time.Second
	whKeyboardLL   = 13
	wmKeyDown      = 0x0100
	wmSysKeyDown   = 0x0104
	imeCmodeNative = 0x0001
	vkControl      = 0x11
	vkSpace        = 0x20
	inputKeyboard  = 1
	keyEventKeyUp  = 0x0002
)

var (
	user32IME         = syscall.NewLazyDLL("user32.dll")
	imm32             = syscall.NewLazyDLL("imm32.dll")
	setWindowsHookEx  = user32IME.NewProc("SetWindowsHookExW")
	callNextHookEx    = user32IME.NewProc("CallNextHookEx")
	getForeground     = user32IME.NewProc("GetForegroundWindow")
	sendInput         = user32IME.NewProc("SendInput")
	immGetContext     = imm32.NewProc("ImmGetContext")
	immReleaseContext = imm32.NewProc("ImmReleaseContext")
	immGetConvStatus  = imm32.NewProc("ImmGetConversionStatus")
	immSetConvStatus  = imm32.NewProc("ImmSetConversionStatus")
)

type imeWatcher struct {
	enabled  atomic.Bool
	lastKey  atomic.Int64 // unix nanos of the last key press
	done     atomic.Bool  // already switched during this idle period
	injected atomic.Bool  // our own Ctrl+Space is not user input
	hook     uintptr
	// switchIME and timeout are swapped in tests; production uses the real
	// input method switch and the six second timeout.
	switchIME func() bool
	timeout   time.Duration
}

var imeWatch = &imeWatcher{}

func (w *imeWatcher) install() {
	if w.hook != 0 {
		return
	}
	if w.timeout <= 0 {
		w.timeout = imeIdleTimeout
	}
	w.lastKey.Store(time.Now().UnixNano())
	callback := syscall.NewCallback(w.keyboardProc)
	hook, _, err := setWindowsHookEx.Call(whKeyboardLL, callback, 0, 0)
	if hook == 0 {
		log.Print("键盘空闲检测不可用: ", err)
		return
	}
	w.hook = hook
	go w.loop()
}

func (w *imeWatcher) keyboardProc(code, msg, data uintptr) uintptr {
	if code >= 0 && (msg == wmKeyDown || msg == wmSysKeyDown) && !w.injected.Load() {
		w.lastKey.Store(time.Now().UnixNano())
		w.done.Store(false)
	}
	next, _, _ := callNextHookEx.Call(w.hook, code, msg, data)
	return next
}

func (w *imeWatcher) loop() {
	// Sample often enough to fire close to the timeout, but never more than
	// twice a second.
	interval := w.timeout / 10
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	if interval > 500*time.Millisecond {
		interval = 500 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		if w.switchIME == nil {
			w.switchIME = switchIMEToEnglish
		}
		if !w.enabled.Load() || w.done.Load() {
			continue
		}
		timeout := w.timeout
		if timeout <= 0 {
			timeout = imeIdleTimeout
		}
		if time.Since(time.Unix(0, w.lastKey.Load())) < timeout {
			continue
		}
		// One attempt per idle period: a failing switch must not hammer
		// Ctrl+Space on every tick.
		w.done.Store(true)
		if !w.switchIME() {
			log.Print("切换到英文输入法失败")
		}
	}
}

// switchIMEToEnglish puts the focused window's input method into English mode
// by sending Ctrl+Space, the IME's own Chinese/English switch, so third-party
// IMEs keep working instead of switching the user to the system ENG layout.
func switchIMEToEnglish() bool {
	hwnd, _, _ := getForeground.Call()
	if hwnd == 0 || hwnd == ^uintptr(0) {
		return false
	}
	// Only switch when the IME itself reports Chinese mode. Ctrl+Space toggles,
	// so sending it blind would turn English into Chinese for half of the
	// applications, and Text Services apps (Chrome, VS Code, Windows 11
	// Notepad) expose no mode we can read.
	conversion, known := imeConversion(hwnd)
	if !known {
		log.Print("输入法状态不可读，跳过切换")
		return false
	}
	if !imeNeedsEnglish(conversion, known) {
		return true // already alphanumeric
	}
	if sendCtrlSpace() {
		return true
	}
	// Some IMEs only accept the API.
	if himc, _, _ := immGetContext.Call(hwnd); himc != 0 {
		defer immReleaseContext.Call(hwnd, himc)
		var sentence uint32
		if res, _, _ := immSetConvStatus.Call(himc, uintptr(conversion&^imeCmodeNative), uintptr(sentence)); res != 0 {
			return true
		}
	}
	return false
}

// imeNeedsEnglish reports whether a known conversion mode is Chinese.
func imeNeedsEnglish(conversion uint32, known bool) bool {
	return known && conversion&imeCmodeNative != 0
}

// imeConversion reads the conversion mode of the window's input method.
func imeConversion(hwnd uintptr) (uint32, bool) {
	himc, _, _ := immGetContext.Call(hwnd)
	if himc == 0 {
		return 0, false
	}
	defer immReleaseContext.Call(hwnd, himc)
	var conversion, sentence uint32
	ok, _, _ := immGetConvStatus.Call(himc, uintptr(unsafe.Pointer(&conversion)), uintptr(unsafe.Pointer(&sentence)))
	return conversion, ok != 0
}

// keyboardInputBytes builds a 40 byte INPUT for one keyboard event, the size
// Windows expects on amd64 (type, padding, then the 32 byte union).
func keyboardInputBytes(vk uint16, flags uint32) [40]byte {
	var b [40]byte
	binary.LittleEndian.PutUint32(b[0:], inputKeyboard)
	binary.LittleEndian.PutUint16(b[8:], vk)
	binary.LittleEndian.PutUint16(b[10:], vk)
	binary.LittleEndian.PutUint32(b[12:], flags)
	return b
}

func sendCtrlSpace() bool {
	imeWatch.injected.Store(true)
	defer imeWatch.injected.Store(false)
	inputs := []struct{ raw [40]byte }{
		{keyboardInputBytes(vkControl, 0)},
		{keyboardInputBytes(vkSpace, 0)},
		{keyboardInputBytes(vkSpace, keyEventKeyUp)},
		{keyboardInputBytes(vkControl, keyEventKeyUp)},
	}
	sent, _, _ := sendInput.Call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	return sent == uintptr(len(inputs))
}
