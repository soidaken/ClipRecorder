// Package recording writes H.264 MP4 through the system Media Foundation encoder.
package recording

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	mf               = syscall.NewLazyDLL("mfplat.dll")
	mfreadwrite      = syscall.NewLazyDLL("mfreadwrite.dll")
	ole32            = syscall.NewLazyDLL("ole32.dll")
	startup          = mf.NewProc("MFStartup")
	shutdown         = mf.NewProc("MFShutdown")
	createType       = mf.NewProc("MFCreateMediaType")
	createAttributes = mf.NewProc("MFCreateAttributes")
	createBuffer     = mf.NewProc("MFCreateMemoryBuffer")
	createSample     = mf.NewProc("MFCreateSample")
	createWriter     = mfreadwrite.NewProc("MFCreateSinkWriterFromURL")
	coInit           = ole32.NewProc("CoInitializeEx")
	coUninit         = ole32.NewProc("CoUninitialize")
)

type guid struct {
	A    uint32
	B, C uint16
	D    [8]byte
}

func g(a uint32, b, c uint16, d uint64) guid {
	v := guid{A: a, B: b, C: c}
	for i := 7; i >= 0; i-- {
		v.D[i] = byte(d)
		d >>= 8
	}
	return v
}

var (
	major             = g(0x48eba18e, 0xf8c9, 0x4687, 0xbf110a74c9f96a8f)
	subtype           = g(0xf7e34c9a, 0x42e8, 0x4714, 0xb74bcb29d72c35e5)
	frameSize         = g(0x1652c33d, 0xd6b2, 0x4012, 0xb83472030849a37d)
	frameRate         = g(0xc459a2e8, 0x3d2c, 0x4e44, 0xb132fee5156c7bb0)
	pixelAspect       = g(0xc6376a1e, 0x8d0a, 0x4027, 0xbe456d9a0ad39bb6)
	interlace         = g(0xe2724bb8, 0xe676, 0x4806, 0xb4b2a8d6efb44ccd)
	bitrate           = g(0x20332624, 0xfb0d, 0x4d9e, 0xbd0dcbf6786c102e)
	stride            = g(0x644b4e48, 0x1e02, 0x4516, 0xb0ebc01ca9d49ac6)
	video             = g(0x73646976, 0, 0x10, 0x800000aa00389b71)
	h264              = g(0x34363248, 0, 0x10, 0x800000aa00389b71)
	rgb32             = g(0x16, 0, 0x10, 0x800000aa00389b71)
	disableThrottling = g(0x08b845d8, 0x2b74, 0x4afe, 0x9d53be16d2d5ae4f)
	videoProfile      = g(0xad76a80b, 0x2d5c, 0x4e0b, 0xb37564e520137036)
)

// COM interfaces use their documented vtable slots. All calls stay on one OS thread.
type com struct{ vtable *[64]uintptr }

func (o *com) call(slot int, args ...any) error {
	return nativeCall(o.vtable[slot], append([]any{unsafe.Pointer(o)}, args...)...)
}
func (o *com) release() {
	if o != nil {
		_ = o.call(2)
	}
}
func hresult(hr uintptr) error {
	if int32(hr) < 0 {
		return fmt.Errorf("Media Foundation HRESULT 0x%08X", uint32(hr))
	}
	return nil
}
func invoke(p *syscall.LazyProc, args ...any) error {
	return nativeCall(p.Addr(), args...)
}
func ptr[T any](v *T) unsafe.Pointer { return unsafe.Pointer(v) }

// Keep Go argument storage pinned until native code has returned, including
// output pointers and GUIDs. Converting pointers in a helper to uintptr would
// otherwise hide their lifetimes from the compiler and stack copier.
func nativeCall(addr uintptr, args ...any) error {
	var pins runtime.Pinner
	defer pins.Unpin()
	values := make([]uintptr, len(args))
	for i, arg := range args {
		switch v := arg.(type) {
		case unsafe.Pointer:
			pins.Pin(v)
			values[i] = uintptr(v)
		case uintptr:
			values[i] = v
		case int:
			values[i] = uintptr(v)
		default:
			panic("unsupported native argument")
		}
	}
	hr, _, _ := syscall.SyscallN(addr, values...)
	runtime.KeepAlive(args)
	return hresult(hr)
}

func mediaType(w, h int, opts Options, input bool) (*com, error) {
	var mt *com
	if err := invoke(createType, ptr(&mt)); err != nil {
		return nil, err
	}
	typ := h264
	if input {
		typ = rgb32
	}
	steps := []func() error{
		func() error { return mt.call(24, ptr(&major), ptr(&video)) }, // IMFAttributes::SetGUID
		func() error { return mt.call(24, ptr(&subtype), ptr(&typ)) },
		func() error { return mt.call(21, ptr(&interlace), 2) }, // SetUINT32
		func() error { return mt.call(22, ptr(&frameSize), uintptr(uint64(w)<<32|uint64(h))) },
		func() error { return mt.call(22, ptr(&frameRate), uintptr(uint64(opts.FPS)<<32|1)) },
		func() error { return mt.call(22, ptr(&pixelAspect), uintptr(uint64(1)<<32|1)) },
	}
	if input {
		steps = append(steps, func() error { return mt.call(21, ptr(&stride), uintptr(w*4)) })
	} else {
		steps = append(steps,
			func() error { return mt.call(21, ptr(&bitrate), uintptr(opts.VideoBitrate(w, h))) },
			func() error { return mt.call(21, ptr(&videoProfile), 100) }, // H.264 High
		)
	}
	for _, set := range steps {
		if err := set(); err != nil {
			mt.release()
			return nil, err
		}
	}
	return mt, nil
}

type encoder struct {
	writer      *com
	stream      uint32
	size        int
	audioStream uint32
	info        EncoderInfo
}

func createEncoderForMode(path string, w, h int, opts Options, allowHardware bool) (*encoder, error) {
	wide, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	e := &encoder{size: w * h * 4}
	var attrs *com
	if err = invoke(createAttributes, ptr(&attrs), 2); err != nil {
		return nil, err
	}
	defer attrs.release()
	// Capture is paced by our monotonic clock. Sink-writer throttling would
	// block the same thread that feeds the other stream during muxing.
	if err = attrs.call(21, ptr(&disableThrottling), 1); err != nil {
		return nil, err
	}
	useHardware := 0
	if allowHardware {
		useHardware = 1
	}
	if err = attrs.call(21, ptr(&enableHardware), useHardware); err != nil {
		return nil, err
	}
	if err = invoke(createWriter, ptr(wide), 0, ptr(attrs), ptr(&e.writer)); err != nil {
		return nil, err
	}
	good := false
	defer func() {
		if !good {
			e.writer.release()
		}
	}()
	out, err := mediaType(w, h, opts, false)
	if err != nil {
		return nil, err
	}
	defer out.release()
	if err = e.writer.call(3, ptr(out), ptr(&e.stream)); err != nil {
		return nil, err
	}
	in, err := mediaType(w, h, opts, true)
	if err != nil {
		return nil, err
	}
	defer in.release()
	if err = e.writer.call(4, uintptr(e.stream), ptr(in), 0); err != nil {
		return nil, err
	}
	if opts.HasAudio() {
		if err = e.addAudio(); err != nil {
			return nil, err
		}
	}
	if err = e.writer.call(5); err != nil {
		return nil, err
	}
	e.info, err = e.inspectEncoder()
	if err != nil {
		return nil, err
	}
	if opts.Encoding == EncodingHardware && !e.info.Hardware {
		return nil, fmt.Errorf("系统为该区域选择了软件编码器，未开始录制")
	}
	if !allowHardware && e.info.Hardware {
		return nil, fmt.Errorf("未能禁用硬件编码器")
	}
	good = true
	return e, nil
}

func (e *encoder) write(pixels []byte, ts, duration int64) error {
	if len(pixels) != e.size {
		return fmt.Errorf("视频帧尺寸不匹配")
	}
	return e.writeSample(e.stream, pixels, ts, duration)
}

func (e *encoder) writeSample(stream uint32, pixels []byte, ts, duration int64) error {
	var buf, sample *com
	if err := invoke(createBuffer, uintptr(len(pixels)), ptr(&buf)); err != nil {
		return err
	}
	defer buf.release()
	var dst unsafe.Pointer
	if err := buf.call(3, ptr(&dst), 0, 0); err != nil {
		return err
	}
	copy(unsafe.Slice((*byte)(dst), len(pixels)), pixels)
	if err := buf.call(4); err != nil {
		return err
	}
	if err := buf.call(6, uintptr(len(pixels))); err != nil {
		return err
	}
	if err := invoke(createSample, ptr(&sample)); err != nil {
		return err
	}
	defer sample.release()
	if err := sample.call(42, ptr(buf)); err != nil {
		return err
	} // IMFSample::AddBuffer
	if err := sample.call(36, uintptr(ts)); err != nil {
		return err
	}
	if err := sample.call(38, uintptr(duration)); err != nil {
		return err
	}
	return e.writer.call(6, uintptr(stream), ptr(sample))
}
func (e *encoder) close() error {
	defer e.writer.release()
	// Finalize drains queued samples; Flush would discard pending frames.
	return e.writer.call(11)
}
