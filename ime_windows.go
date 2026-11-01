package main

import (
	"context"
	"encoding/binary"
	"image"
	"log"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"cliprecorder/internal/capture"
	"cliprecorder/internal/translation"
	"golang.org/x/sys/windows/registry"
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
	getSystemMetrics  = user32IME.NewProc("GetSystemMetrics")
	mapVirtualKey     = user32IME.NewProc("MapVirtualKeyW")
	monitorFromPoint  = user32IME.NewProc("MonitorFromPoint")
)

const (
	smCxScreen = 0
	smCyScreen = 1
)

type imeWatcher struct {
	enabled  atomic.Bool
	lastKey  atomic.Int64 // unix nanos of the last key press
	done     atomic.Bool  // already switched during this idle period
	injected atomic.Bool  // our own Ctrl+Space is not user input
	hook     uintptr
	// switchIME and timeout are swapped in tests; production uses the real
	// input method switch and the six second timeout.
	switchIME func(context.Context) bool
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ok := w.switchIME(ctx)
		cancel()
		if !ok {
			log.Print("切换到英文输入法失败")
		}
	}
}

// switchIMEToEnglish puts the focused window's input method into English mode
// by sending Ctrl+Space, the IME's own Chinese/English switch, so third-party
// IMEs keep working instead of switching the user to the system ENG layout.
func switchIMEToEnglish(ctx context.Context) bool {
	hwnd, _, _ := getForeground.Call()
	if hwnd == 0 || hwnd == ^uintptr(0) {
		return false
	}
	chinese, known := imeIsChinese(ctx, hwnd)
	if !known {
		// Ctrl+Space toggles: without knowing the current mode it could turn
		// English back into Chinese.
		log.Print("输入法状态不可读，跳过切换")
		return false
	}
	if !chinese {
		return true
	}
	return sendCtrlSpace()
}

// imeIsChinese reports whether the input method is in Chinese mode, and
// whether that could be determined at all. The tray indicator near the clock
// shows 中 or 英 and works for third-party IMEs and Text Services apps alike;
// the IMM conversion status is the fallback for setups without the indicator.
func imeIsChinese(ctx context.Context, hwnd uintptr) (bool, bool) {
	if text, err := indicatorText(ctx); err == nil {
		if chinese, known := imeModeFromIndicator(text); known {
			return chinese, true
		}
	}
	if conversion, ok := imeConversion(hwnd); ok {
		return conversion&imeCmodeNative != 0, true
	}
	return false, false
}

// imeModeFromIndicator reads the 中 / 英 marker out of the recognized text of
// the tray area.
func imeModeFromIndicator(text string) (bool, bool) {
	if strings.Contains(text, "中") {
		return true, true
	}
	if strings.Contains(text, "英") {
		return false, true
	}
	return false, false
}

// indicatorText captures the bottom right corner of the primary monitor, where
// the input method draws its 中 / 英 indicator, and recognizes the text. Extra
// monitors never matter: the taskbar indicator lives on the primary one.
func indicatorText(ctx context.Context) (string, error) {
	const size = 320
	area, ok := primaryMonitorRect()
	if !ok {
		cx, _, _ := getSystemMetrics.Call(smCxScreen)
		cy, _, _ := getSystemMetrics.Call(smCyScreen)
		area = image.Rect(0, 0, int(cx), int(cy))
	}
	rect := rectWithin(area, size)
	shot, err := capture.Snapshot(rect)
	if err != nil {
		return "", err
	}
	return translation.RecognizeIndicator(ctx, shot)
}

// rectWithin is the bottom right square of size inside area.
func rectWithin(area image.Rectangle, size int) image.Rectangle {
	return image.Rect(area.Max.X-size, area.Max.Y-size, area.Max.X, area.Max.Y)
}

// primaryMonitorRect is the full rectangle of the primary monitor in physical
// pixels, including the taskbar.
func primaryMonitorRect() (image.Rectangle, bool) {
	origin := struct{ X, Y int32 }{}
	monitor, _, _ := monitorFromPoint.Call(uintptr(unsafe.Pointer(&origin)), 1) // MONITOR_DEFAULTTOPRIMARY
	if monitor == 0 {
		return image.Rectangle{}, false
	}
	var info struct {
		Size          uint32
		Monitor, Work nativeRect
		Flags         uint32
	}
	info.Size = uint32(unsafe.Sizeof(info))
	if ok, _, _ := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return image.Rectangle{}, false
	}
	return image.Rect(int(info.Monitor.Left), int(info.Monitor.Top), int(info.Monitor.Right), int(info.Monitor.Bottom)), true
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
//
// The scan code must be the real one: writing the virtual key there made
// receivers translate it again, so Ctrl+Space arrived as the letters "wd"
// (scan codes 0x11 and 0x20) in some input fields.
func keyboardInputBytes(vk uint16, flags uint32) [40]byte {
	var b [40]byte
	binary.LittleEndian.PutUint32(b[0:], inputKeyboard)
	binary.LittleEndian.PutUint16(b[8:], vk)
	scan, _, _ := mapVirtualKey.Call(uintptr(vk), 0) // MAPVK_VK_TO_VSC
	binary.LittleEndian.PutUint16(b[10:], uint16(scan))
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

// The idle switch is off until the user turns it on in the tray menu; the
// choice survives restarts.
const imePrefValue = "ImeIdleSwitch"

func loadIMEIdlePref() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\ClipRecorder`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetIntegerValue(imePrefValue)
	return err == nil && value != 0
}

// saveIMEIdlePrefFunc is swapped in tests so they never touch the real
// setting.
var saveIMEIdlePrefFunc = saveIMEIdlePref

func saveIMEIdlePref(enabled bool) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\ClipRecorder`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	value := uint32(0)
	if enabled {
		value = 1
	}
	return key.SetDWordValue(imePrefValue, value)
}
