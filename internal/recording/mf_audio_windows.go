package recording

var (
	audioMajor    = g(0x73647561, 0, 0x10, 0x800000aa00389b71)
	pcm           = g(1, 0, 0x10, 0x800000aa00389b71)
	aac           = g(0x1610, 0, 0x10, 0x800000aa00389b71)
	audioBits     = g(0xf2deb57f, 0x40fa, 0x4764, 0xaa33ed4f2d1ff669)
	audioRate     = g(0x5faeeae7, 0x0290, 0x4c31, 0x9e8ac534f68d9dba)
	audioChannels = g(0x37e48bf5, 0x645e, 0x4c5b, 0x89deada9e29b696a)
	audioBytes    = g(0x1aab75c8, 0xcfef, 0x451c, 0xab95ac034b8e1731)
	audioBlock    = g(0x322de230, 0x9eeb, 0x43bd, 0xab7aff412251541d)
)

func audioType(input bool) (*com, error) {
	var mt *com
	if err := invoke(createType, ptr(&mt)); err != nil {
		return nil, err
	}
	typ := aac
	bytesPerSec, block := 24000, 1
	if input {
		typ = pcm
		bytesPerSec = sampleRate * 4
		block = 4
	}
	if err := mt.call(24, ptr(&major), ptr(&audioMajor)); err != nil {
		mt.release()
		return nil, err
	}
	if err := mt.call(24, ptr(&subtype), ptr(&typ)); err != nil {
		mt.release()
		return nil, err
	}
	for _, a := range []struct {
		key   *guid
		value int
	}{
		{&audioBits, 16}, {&audioRate, sampleRate}, {&audioChannels, 2},
		{&audioBytes, bytesPerSec}, {&audioBlock, block},
	} {
		if err := mt.call(21, ptr(a.key), a.value); err != nil {
			mt.release()
			return nil, err
		}
	}
	return mt, nil
}
func (e *encoder) addAudio() error {
	out, err := audioType(false)
	if err != nil {
		return err
	}
	defer out.release()
	in, err := audioType(true)
	if err != nil {
		return err
	}
	defer in.release()
	if err = e.writer.call(3, ptr(out), ptr(&e.audioStream)); err != nil {
		return err
	}
	return e.writer.call(4, uintptr(e.audioStream), ptr(in), 0)
}
func (e *encoder) writeAudio(packet audioPacket) error {
	return e.writeSample(e.audioStream, packet.PCM, packet.Start*10000000/sampleRate, packet.Frames*10000000/sampleRate)
}
