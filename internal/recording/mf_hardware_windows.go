package recording

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

// ProbeEncoder opens the same pipeline as recording with the selected media
// types, encodes one frame, and releases it before returning. No desktop or
// microphone is captured and the temporary file is always removed.
func ProbeEncoder(w, h int, opts Options) (EncoderInfo, error) {
	if err := opts.Validate(); err != nil {
		return EncoderInfo{}, err
	}
	w, h = w/2*2, h/2*2
	if w < 2 || h < 2 {
		return EncoderInfo{}, fmt.Errorf("录制区域太小")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := invoke(coInit, 0, 0); err != nil {
		return EncoderInfo{}, err
	}
	defer coUninit.Call()
	if err := invoke(startup, 0x20070, 1); err != nil {
		return EncoderInfo{}, err
	}
	defer shutdown.Call()
	dir, err := os.MkdirTemp("", "ClipRecorder-encoder-*")
	if err != nil {
		return EncoderInfo{}, err
	}
	defer os.RemoveAll(dir)
	for _, mode := range []EncodingMode{EncodingHardware, EncodingSoftware} {
		opts.Encoding = mode
		enc, openErr := newEncoder(filepath.Join(dir, fmt.Sprintf("probe-%d.mp4", mode)), w, h, opts)
		if openErr != nil {
			err = openErr
			continue
		}
		err = enc.write(make([]byte, w*h*4), 0, int64(10000000/opts.FPS))
		closeErr := enc.close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			return enc.info, nil
		}
	}
	return EncoderInfo{}, err
}

// DetectHardwareEncoders queries this computer's current H.264 registrations.
// Enumeration is advisory; recording startup validates the actual media types.
func DetectHardwareEncoders() ([]string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := invoke(coInit, 0, 0); err != nil {
		return nil, err
	}
	defer coUninit.Call()
	if err := invoke(startup, 0x20070, 1); err != nil {
		return nil, err
	}
	defer shutdown.Call()
	filter := struct{ Major, Subtype guid }{video, h264}
	var list **com
	var count uint32
	// MFT_ENUM_FLAG_HARDWARE | MFT_ENUM_FLAG_SORTANDFILTER.
	if err := invoke(mf.NewProc("MFTEnumEx"), ptr(&videoEncoderCategory), 0x44, 0, ptr(&filter), ptr(&list), ptr(&count)); err != nil {
		return nil, err
	}
	if list == nil {
		return nil, nil
	}
	defer ole32.NewProc("CoTaskMemFree").Call(uintptr(unsafe.Pointer(list)))
	names := make([]string, 0, count)
	for _, activation := range unsafe.Slice(list, int(count)) {
		name := activation.stringAttribute(&transformName)
		activation.release()
		if name == "" {
			name = "H.264 硬件编码器"
		}
		names = append(names, name)
	}
	return names, nil
}

var (
	enableHardware       = g(0xa634a91c, 0x822b, 0x41b9, 0xa4944de4643612b0)
	sinkWriterEx         = g(0x588d72ab, 0x5bc1, 0x496a, 0x8714b70617141b25)
	videoEncoderCategory = g(0xf79eac7d, 0xe545, 0x4387, 0xbdeed647d7bde42a)
	hardwareURL          = g(0x2fb866ac, 0xb078, 0x4942, 0xab6c003d05cda674)
	transformName        = g(0x314ffbae, 0x5b41, 0x4c95, 0x9c194e7d586face3)
)

func (a *com) stringAttribute(key *guid) string {
	var length uint32
	if err := a.call(11, ptr(key), ptr(&length)); err != nil || length > 32768 {
		return ""
	}
	buf := make([]uint16, int(length)+1)
	if err := a.call(12, ptr(key), ptr(&buf[0]), len(buf), ptr(&length)); err != nil {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// Inspect the MFT actually selected for this stream. Enabling hardware MFTs
// alone is permission to use them, not proof that an encoder is hardware.
func (e *encoder) inspectEncoder() (EncoderInfo, error) {
	var ex *com
	if err := e.writer.call(0, ptr(&sinkWriterEx), ptr(&ex)); err != nil {
		return EncoderInfo{}, err
	}
	defer ex.release()
	for index := 0; index < 16; index++ {
		var category guid
		var transform *com
		if err := ex.call(14, int(e.stream), index, ptr(&category), ptr(&transform)); err != nil {
			break
		}
		if category != videoEncoderCategory {
			transform.release()
			continue
		}
		var attrs *com
		err := transform.call(8, ptr(&attrs))
		transform.release()
		if err != nil {
			return EncoderInfo{}, fmt.Errorf("无法读取视频编码器属性：%w", err)
		}
		defer attrs.release()
		info := EncoderInfo{Hardware: attrs.stringAttribute(&hardwareURL) != "", Name: attrs.stringAttribute(&transformName)}
		if info.Name == "" {
			info.Name = info.Label() + " · H.264"
		}
		return info, nil
	}
	return EncoderInfo{}, fmt.Errorf("无法确认实际使用的视频编码器")
}

func newEncoder(path string, w, h int, opts Options) (*encoder, error) {
	allowHardware := opts.Encoding != EncodingSoftware
	e, err := createEncoderForMode(path, w, h, opts, allowHardware)
	if err != nil && opts.Encoding == EncodingAuto {
		// Recreate only before capture begins; never switch encoders mid-file.
		return createEncoderForMode(path, w, h, opts, false)
	}
	if err != nil && opts.Encoding == EncodingHardware {
		return nil, fmt.Errorf("当前区域或参数无法使用硬件 H.264 编码，请选择自动或软件编码：%w", err)
	}
	return e, err
}
