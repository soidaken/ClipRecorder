package recording

import "fmt"

type Quality int

type EncodingMode int

const (
	EncodingAuto EncodingMode = iota
	EncodingHardware
	EncodingSoftware
)

type EncoderInfo struct {
	Hardware bool
	Name     string
}

func (e EncoderInfo) Label() string {
	if e.Hardware {
		return "硬件编码"
	}
	return "软件编码"
}

const (
	QualityStandard Quality = iota
	QualityHigh
	QualityUltra
)

func (q Quality) Label() string {
	switch q {
	case QualityStandard:
		return "标准"
	case QualityHigh:
		return "高清"
	case QualityUltra:
		return "极清"
	default:
		return "未知"
	}
}

type Options struct {
	FPS         int
	SystemAudio bool
	Microphone  bool
	Quality     Quality
	Encoding    EncodingMode
}

func DefaultOptions() Options { return Options{FPS: 60, SystemAudio: true, Quality: QualityHigh} }
func (o Options) Validate() error {
	if o.FPS != 30 && o.FPS != 60 {
		return fmt.Errorf("帧率仅支持 30 或 60 fps")
	}
	if o.Quality < QualityStandard || o.Quality > QualityUltra {
		return fmt.Errorf("画质选项无效")
	}
	if o.Encoding < EncodingAuto || o.Encoding > EncodingSoftware {
		return fmt.Errorf("编码方式无效")
	}
	return nil
}

// Bits per pixel per frame: keep spatial detail comparable at 30 and 60 fps.
// Floors protect small text-heavy selections; caps bound encoder cost at 4K.
func (o Options) VideoBitrate(w, h int) int {
	bpp, floor, ceiling := int64(100), int64(2000000), int64(60000000)
	switch o.Quality {
	case QualityHigh:
		bpp, floor, ceiling = 200, 4000000, 100000000
	case QualityUltra:
		bpp, floor, ceiling = 350, 6000000, 160000000
	}
	return int(max(floor, min(ceiling, int64(w)*int64(h)*int64(o.FPS)*bpp/1000)))
}
func (o Options) HasAudio() bool { return o.SystemAudio || o.Microphone }
func (o Options) AudioLabel() string {
	switch {
	case o.SystemAudio && o.Microphone:
		return "系统 + 麦克风"
	case o.SystemAudio:
		return "系统声音"
	case o.Microphone:
		return "麦克风"
	default:
		return "无声"
	}
}
