package main

import (
	"image"
	"testing"
)

func TestRecordingBarAvoidsRecordedRegion(t *testing.T) {
	work := image.Rect(0, 0, 1920, 1080)
	const w, h, margin = 400, 92, 16
	bottomRight := image.Rect(1920-w-margin, 1080-h-margin, 1920-margin, 1080-margin)

	if got := recordingBarPlacement(work, w, h, margin, image.Rect(0, 0, 800, 600)); got != bottomRight {
		t.Fatalf("free region must keep the bar bottom right: %v", got)
	}
	// Recording the bottom right corner moves the bar to the top right.
	topRight := image.Rect(1920-w-margin, margin, 1920-margin, margin+h)
	if got := recordingBarPlacement(work, w, h, margin, bottomRight); got != topRight {
		t.Fatalf("bar should move to the top right: %v", got)
	}
	// A full work area leaves no free corner: fall back to bottom right.
	if got := recordingBarPlacement(work, w, h, margin, work); got != bottomRight {
		t.Fatalf("full region should fall back to bottom right: %v", got)
	}
	// Never leave the work area, even on a tiny one.
	tiny := image.Rect(0, 0, 300, 200)
	got := recordingBarPlacement(tiny, 400, 92, 16, image.Rect(0, 0, 10, 10))
	if got.Min.X < 0 || got.Min.Y < 0 {
		t.Fatalf("bar left the work area: %v", got)
	}
}
