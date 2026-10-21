package recording

import (
	"encoding/binary"
	"fmt"
)

const sampleRate = 48000
const blockFrames = 480 // 10ms, stereo PCM16

type audioPacket struct {
	PCM           []byte
	Start, Frames int64
}

// A bounded timeline joins independent device clocks through WASAPI's QPC
// timestamps. Missing packets are silence, never a compressed timeline.
type audioMixer struct {
	blocks   map[int64][]int32
	position int64
	divisor  int32
}

func newMixer() *audioMixer { return &audioMixer{blocks: make(map[int64][]int32), divisor: 1} }
func (m *audioMixer) add(start int64, samples []int16) error {
	if start+int64(len(samples)/2) > m.position+sampleRate*2 {
		return fmt.Errorf("声音时间戳异常，已停止录制")
	}
	for i := 0; i < len(samples)/2; i++ {
		frame := start + int64(i)
		if frame < m.position {
			continue
		}
		key := frame / blockFrames
		block := m.blocks[key]
		if block == nil {
			block = make([]int32, blockFrames*2)
			m.blocks[key] = block
		}
		offset := (frame % blockFrames) * 2
		block[offset] += int32(samples[i*2])
		block[offset+1] += int32(samples[i*2+1])
	}
	return nil
}
func (m *audioMixer) take(frames int64) audioPacket {
	p := audioPacket{PCM: make([]byte, frames*4), Start: m.position, Frames: frames}
	for i := int64(0); i < frames; i++ {
		frame := m.position + i
		block := m.blocks[frame/blockFrames]
		if block == nil {
			continue
		}
		for ch := int64(0); ch < 2; ch++ {
			value := max(int32(-32768), min(int32(32767), block[(frame%blockFrames)*2+ch]/m.divisor))
			binary.LittleEndian.PutUint16(p.PCM[(i*2+ch)*2:], uint16(int16(value)))
		}
		if frame%blockFrames == blockFrames-1 {
			delete(m.blocks, frame/blockFrames)
		}
	}
	m.position += frames
	return p
}
