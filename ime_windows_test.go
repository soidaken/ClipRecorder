package main

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestIMEIdleSwitchesOnceAfterTimeout(t *testing.T) {
	var switches atomic.Int32
	w := &imeWatcher{timeout: 60 * time.Millisecond}
	w.switchIME = func() bool {
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
	w.switchIME = func() bool {
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
	w.switchIME = func() bool {
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
