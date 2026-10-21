package recording

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestPacketClockDoesNotDamageContinuousWaveform(t *testing.T) {
	var clock packetClock
	m := newMixer()
	const packets = 100
	for packet := 0; packet < packets; packet++ {
		position := packet * blockFrames
		jitter := int64(packet%11 - 5)
		if packet == 0 {
			jitter = 0
		}
		start := clock.place(uint64(position), int64(position)+jitter, blockFrames, true)
		samples := make([]int16, blockFrames*2)
		for i := 0; i < blockFrames; i++ {
			s := int16(24000 * math.Sin(2*math.Pi*997*float64(position+i)/sampleRate))
			samples[i*2], samples[i*2+1] = s, s
		}
		if err := m.add(start, samples); err != nil {
			t.Fatal(err)
		}
		p := m.take(blockFrames)
		for i, v := range samples {
			if got := int16(binary.LittleEndian.Uint16(p.PCM[i*2:])); got != v {
				t.Fatalf("packet %d sample %d changed: %d != %d", packet, i, got, v)
			}
		}
	}
}

func TestPacketClockPreservesRealGaps(t *testing.T) {
	var c packetClock
	if got := c.place(1000, -20, 480, true); got != -20 {
		t.Fatal(got)
	}
	if got := c.place(1480, 465, 480, true); got != 460 {
		t.Fatal(got)
	}
	if got := c.place(2440, 1420, 480, true); got != 1420 {
		t.Fatal("lost audio must remain a gap", got)
	}
	if got := c.place(0, 99999, 480, false); got != 1900 {
		t.Fatal("invalid timestamp must continue", got)
	}
	if got := c.place(0, 2500, 480, true); got != 2500 {
		t.Fatal("device reset must reanchor", got)
	}
}

func TestTwoSourceMixHasHeadroom(t *testing.T) {
	m := newMixer()
	m.divisor = 2
	for i := 0; i < 2; i++ {
		if err := m.add(0, []int16{30000, -30000}); err != nil {
			t.Fatal(err)
		}
	}
	p := m.take(1)
	if got := int16(binary.LittleEndian.Uint16(p.PCM)); got != 30000 {
		t.Fatal("positive peak clipped", got)
	}
	if got := int16(binary.LittleEndian.Uint16(p.PCM[2:])); got != -30000 {
		t.Fatal("negative peak clipped", got)
	}
}
