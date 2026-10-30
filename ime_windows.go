package main

import (
	"log"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// After a few seconds without keyboard input the input method is switched to
// English, the same effect as pressing Ctrl+Space, so typing after a pause does
// not start with half a Chinese sentence. A low level keyboard hook only
// records the time of the last key; it never swallows anything.

const (
	imeIdleTimeout    = 6 * time.Second
	whKeyboardLL      = 13
	wmKeyDown         = 0x0100
	wmSysKeyDown      = 0x0104
	imeCmodeNative    = 0x0001
	vkControl         = 0x11
	vkSpace           = 0x20
	langEnglish       = 0x0409
	wmInputLangChange = 0x0050
	inputKeyboard     = 1
	keyEventKeyUp     = 0x0002
)

var (
	user32IME          = syscall.NewLazyDLL("user32.dll")
	imm32              = syscall.NewLazyDLL("imm32.dll")
	setWindowsHookEx   = user32IME.NewProc("SetWindowsHookExW")
	callNextHookEx     = user32IME.NewProc("CallNextHookEx")
	getForeground      = user32IME.NewProc("GetForegroundWindow")
	sendInput          = user32IME.NewProc("SendInput")
	immGetContext      = imm32.NewProc("ImmGetContext")
	immReleaseContext  = imm32.NewProc("ImmReleaseContext")
	immGetConvStatus   = imm32.NewProc("ImmGetConversionStatus")
	immSetConvStatus   = imm32.NewProc("ImmSetConversionStatus")
	getWindowThreadID  = user32IME.NewProc("GetWindowThreadProcessId")
	getKeyboardLayout  = user32IME.NewProc("GetKeyboardLayout")
	postMessage        = user32IME.NewProc("PostMessageW")
	loadKeyboardLayout = user32IME.NewProc("LoadKeyboardLayoutW")
)

type keyboardInput struct {
	Type  uint32
	_     uint32
	Vk    uint16
	Scan  uint16
	Flags uint32
	Time  uint32
	_     uint32
	Extra uintptr
}

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
		idle := time.Since(time.Unix(0, w.lastKey.Load()))
		timeout := w.timeout
		if timeout <= 0 {
			timeout = imeIdleTimeout
		}
		if idle < timeout {
			continue
		}
		if w.switchIME() {
			w.done.Store(true)
		}
	}
}

// switchIMEToEnglish puts the focused window's input method into alphanumeric
// mode. Standard IMEs accept ImmSetConversionStatus; others only react to the
// Ctrl+Space shortcut, which is sent as a fallback.
func switchIMEToEnglish() bool {
	hwnd, _, _ := getForeground.Call()
	if hwnd == 0 || hwnd == ^uintptr(0) {
		return false
	}
	himc, _, _ := immGetContext.Call(hwnd)
	if himc == 0 {
		// Windows 11's Notepad, Chrome and other Text Services apps have no IMM
		// context; switching their keyboard layout to English is the reliable
		// equivalent there.
		return switchLayoutToEnglish(hwnd)
	}
	defer immReleaseContext.Call(hwnd, himc)

	var conversion, sentence uint32
	ok, _, _ := immGetConvStatus.Call(himc, uintptr(unsafe.Pointer(&conversion)), uintptr(unsafe.Pointer(&sentence)))
	if ok == 0 || conversion&imeCmodeNative == 0 {
		return conversion&imeCmodeNative == 0 && ok != 0
	}
	if res, _, _ := immSetConvStatus.Call(himc, uintptr(conversion&^imeCmodeNative), uintptr(sentence)); res != 0 {
		var now uint32
		if ok, _, _ := immGetConvStatus.Call(himc, uintptr(unsafe.Pointer(&now)), uintptr(unsafe.Pointer(&sentence))); ok != 0 && now&imeCmodeNative == 0 {
			return true
		}
	}
	return sendCtrlSpace()
}

// switchLayoutToEnglish posts an input language change to the window, using the
// installed English layout when there is one.
func switchLayoutToEnglish(hwnd uintptr) bool {
	tid, _, _ := getWindowThreadID.Call(hwnd, 0)
	layout, _, _ := getKeyboardLayout.Call(tid)
	if layout&0xffff == langEnglish {
		return true // already typing English
	}
	english, _, _ := loadKeyboardLayout.Call(uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("00000409"))), 1)
	if english == 0 {
		return false
	}
	postMessage.Call(hwnd, wmInputLangChange, 0, english)
	return true
}

func sendCtrlSpace() bool {
	imeWatch.injected.Store(true)
	defer imeWatch.injected.Store(false)
	inputs := []keyboardInput{
		{Type: inputKeyboard, Vk: vkControl},
		{Type: inputKeyboard, Vk: vkSpace},
		{Type: inputKeyboard, Vk: vkSpace, Flags: keyEventKeyUp},
		{Type: inputKeyboard, Vk: vkControl, Flags: keyEventKeyUp},
	}
	sent, _, _ := sendInput.Call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	return sent == uintptr(len(inputs))
}
