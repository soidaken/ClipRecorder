package main

import (
	"context"
	"encoding/binary"
	"sync/atomic"
	"testing"
	"time"
)

func TestIMEIdleSwitchesOnceAfterTimeout(t *testing.T) {
	var switches atomic.Int32
	w := &imeWatcher{timeout: 60 * time.Millisecond}
	w.switchIME = func(context.Context) bool {
		switches.Add(1)
		return true
	}
	w.enabled.Store(true)
	w.lastKey.Store(time.Now().UnixNano())
	go w.loop()

	time.Sleep(30 * time.Millisecond)
	if got := switches.Load(); got != 0 {
		t.Fatalf("switched before the timeout: %d", got)
	}
	time.Sleep(120 * time.Millisecond)
	if got := switches.Load(); got != 1 {
		t.Fatalf("expected one switch, got %d", got)
	}
	// Remaining idle must not switch again.
	time.Sleep(120 * time.Millisecond)
	if got := switches.Load(); got != 1 {
		t.Fatalf("switch repeated while still idle: %d", got)
	}
}

func TestIMEIdleResetsOnKeyboardInput(t *testing.T) {
	var switches atomic.Int32
	w := &imeWatcher{timeout: 80 * time.Millisecond}
	w.switchIME = func(context.Context) bool {
		switches.Add(1)
		return true
	}
	w.enabled.Store(true)
	w.lastKey.Store(time.Now().UnixNano())
	w.done.Store(true) // pretend a switch already happened
	// A key press (as the hook records it) resets both the timer and the flag.
	w.lastKey.Store(time.Now().UnixNano())
	w.done.Store(false)
	go w.loop()

	time.Sleep(40 * time.Millisecond)
	if got := switches.Load(); got != 0 {
		t.Fatalf("keyboard input must reset the idle timer: %d", got)
	}
	time.Sleep(120 * time.Millisecond)
	if got := switches.Load(); got != 1 {
		t.Fatalf("switch after the reset timeout expected, got %d", got)
	}
	w.enabled.Store(false)
}

func TestIMEIdleDoesNotSwitchWhenDisabled(t *testing.T) {
	var switches atomic.Int32
	w := &imeWatcher{timeout: 40 * time.Millisecond}
	w.switchIME = func(context.Context) bool {
		switches.Add(1)
		return true
	}
	w.lastKey.Store(time.Now().UnixNano())
	go w.loop()
	time.Sleep(150 * time.Millisecond)
	if got := switches.Load(); got != 0 {
		t.Fatalf("disabled watcher switched: %d", got)
	}
}

func TestIMEModeFromIndicator(t *testing.T) {
	cases := []struct {
		text    string
		chinese bool
		known   bool
	}{
		{"老 中 363 KB/s", true, true},
		{"英 17:40", false, true},
		{"17:40 2026", false, false},
		{"", false, false},
	}
	for _, c := range cases {
		chinese, known := imeModeFromIndicator(c.text)
		if chinese != c.chinese || known != c.known {
			t.Fatalf("imeModeFromIndicator(%q) = %v,%v want %v,%v", c.text, chinese, known, c.chinese, c.known)
		}
	}
}

func TestIMENeedsEnglishOnlyWhenKnownAndChinese(t *testing.T) {
	cases := []struct {
		conversion uint32
		known      bool
		want       bool
	}{
		{1, true, true},   // native (Chinese) reported
		{0, true, false},  // alphanumeric
		{1, false, false}, // unknown state: never guess
		{0, false, false},
	}
	for _, c := range cases {
		if got := imeNeedsEnglish(c.conversion, c.known); got != c.want {
			t.Fatalf("imeNeedsEnglish(%#x, %v) = %v, want %v", c.conversion, c.known, got, c.want)
		}
	}
}

func TestInjectedKeysUseRealScanCodes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		vk     uint16
		expect uint16
	}{
		{"control", vkControl, 0x1d},
		{"space", vkSpace, 0x39},
	} {
		raw := keyboardInputBytes(tc.vk, 0)
		vk := binary.LittleEndian.Uint16(raw[8:])
		scan := binary.LittleEndian.Uint16(raw[10:])
		if vk != tc.vk {
			t.Fatalf("%s: virtual key %#x", tc.name, vk)
		}
		if scan == tc.vk {
			t.Fatalf("%s: scan code repeats the virtual key (%#x), which types letters", tc.name, scan)
		}
		if scan != tc.expect {
			t.Fatalf("%s: scan code %#x, want %#x", tc.name, scan, tc.expect)
		}
	}
}
