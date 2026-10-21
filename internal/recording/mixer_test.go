package recording

import (
	"encoding/binary"
	"testing"
)

func TestMixerPreservesTimeAndMixesSources(t *testing.T) {
	m := newMixer()
	// Pre-roll is discarded; one source starts before video, the other later.
	if err := m.add(-1, []int16{10, 20, 100, 200, 30000, -30000}); err != nil {
		t.Fatal(err)
	}
	if err := m.add(1, []int16{10000, -10000}); err != nil {
		t.Fatal(err)
	}
	p := m.take(3)
	want := []int16{100, 200, 32767, -32768, 0, 0}
	for i, v := range want {
		if got := int16(binary.LittleEndian.Uint16(p.PCM[i*2:])); got != v {
			t.Fatalf("sample %d: %d, want %d", i, got, v)
		}
	}
	if p.Start != 0 || p.Frames != 3 {
		t.Fatal("incorrect timeline")
	}
	p = m.take(2)
	if p.Start != 3 || len(m.blocks) > 1 {
		t.Fatal("timeline or buffering incorrect")
	}
}

func TestMixerSilentGapAndLongTimeline(t *testing.T) {
	m := newMixer()
	for i := 0; i < 10000; i++ {
		if err := m.add(m.position+2, []int16{100, -100}); err != nil {
			t.Fatal(err)
		}
		p := m.take(blockFrames)
		if binary.LittleEndian.Uint16(p.PCM[0:]) != 0 || binary.LittleEndian.Uint16(p.PCM[8:]) != 100 {
			t.Fatal("audio gap compressed")
		}
		if len(m.blocks) != 0 {
			t.Fatal("consumed audio retained")
		}
	}
	if m.position != 4800000 {
		t.Fatal("audio clock drift")
	}
}

func TestOptions(t *testing.T) {
	o := DefaultOptions()
	if o.FPS != 60 || !o.SystemAudio || o.Microphone || o.Quality != QualityHigh {
		t.Fatal("incorrect defaults")
	}
	if err := (Options{FPS: 24}).Validate(); err == nil {
		t.Fatal("unsupported frame rate accepted")
	}
	if err := (Options{FPS: 60, Encoding: 99}).Validate(); err == nil {
		t.Fatal("invalid encoder mode accepted")
	}
}

func TestQualityBitrates(t *testing.T) {
	want := []int{12441600, 24883200, 43545600}
	for q := QualityStandard; q <= QualityUltra; q++ {
		o := Options{FPS: 60, Quality: q}
		if got := o.VideoBitrate(1920, 1080); got != want[q] {
			t.Fatalf("%s: %d", q.Label(), got)
		}
		o.FPS = 30
		if got := o.VideoBitrate(1920, 1080); got != want[q]/2 {
			t.Fatalf("30fps %s: %d", q.Label(), got)
		}
	}
	if err := (Options{FPS: 60, Quality: 99}).Validate(); err == nil {
		t.Fatal("invalid quality accepted")
	}
}
