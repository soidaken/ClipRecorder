package recording

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/moutend/go-wca/pkg/wca"
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	queryCounter   = kernel32.NewProc("QueryPerformanceCounter")
	queryFrequency = kernel32.NewProc("QueryPerformanceFrequency")
)

// WASAPI reports QPC timestamps in 100ns units, independent of device clocks.
func qpcNow() int64 {
	var counter, freq int64
	queryCounter.Call(uintptr(unsafe.Pointer(&counter)))
	queryFrequency.Call(uintptr(unsafe.Pointer(&freq)))
	return counter/freq*10000000 + (counter%freq)*10000000/freq
}

type audioOrigin struct {
	time time.Time
	qpc  int64
}
type audioSession struct {
	packets chan audioPacket
	done    chan error
	begin   chan audioOrigin
	cancel  context.CancelFunc
}

func prepareAudio(opts Options) (*audioSession, error) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &audioSession{packets: make(chan audioPacket, 256), done: make(chan error, 1), begin: make(chan audioOrigin, 1), cancel: cancel}
	ready := make(chan error, 1)
	go func() {
		err := s.run(ctx, opts, ready)
		close(s.packets)
		s.done <- err
	}()
	if err := <-ready; err != nil {
		cancel()
		<-s.done
		return nil, err
	}
	return s, nil
}

func (s *audioSession) run(ctx context.Context, opts Options, ready chan<- error) (err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	announced := false
	defer func() {
		if !announced {
			ready <- err
		}
	}()
	if err = invoke(coInit, 0, 0); err != nil {
		return err
	}
	defer coUninit.Call()
	var devices []*audioDevice
	defer func() {
		for _, d := range devices {
			d.close()
		}
	}()
	for _, source := range []struct {
		enabled, loopback bool
		name              string
	}{
		{opts.SystemAudio, true, "系统声音"}, {opts.Microphone, false, "麦克风"},
	} {
		if !source.enabled {
			continue
		}
		d, e := openAudioDevice(source.loopback)
		if e != nil {
			return fmt.Errorf("%s不可用：%w；请检查默认声音设备和权限，或取消勾选后重试", source.name, e)
		}
		d.name = source.name
		devices = append(devices, d)
	}
	for _, d := range devices {
		if err = d.client.Start(); err != nil {
			return fmt.Errorf("%s启动失败：%w", d.name, err)
		}
	}
	ready <- nil
	announced = true
	var origin audioOrigin
	select {
	case origin = <-s.begin:
	case <-ctx.Done():
		return nil
	}
	mixer := newMixer()
	if opts.SystemAudio && opts.Microphone {
		// Two full-scale sources must not be summed into a clipped PCM16
		// waveform. Reserve 6dB per source while preserving single-source gain.
		mixer.divisor = 2
	}
	poll := func() error {
		for _, d := range devices {
			if e := d.read(mixer, origin.qpc); e != nil {
				return fmt.Errorf("%s采集中断：%w", d.name, e)
			}
		}
		return nil
	}
	emit := func(end int64) error {
		for mixer.position < end {
			packet := mixer.take(min(int64(blockFrames), end-mixer.position))
			select {
			case s.packets <- packet:
			default:
				return fmt.Errorf("音频编码未及时处理，已停止录制以避免音画不同步")
			}
		}
		return nil
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			end := time.Since(origin.time).Nanoseconds() * sampleRate / int64(time.Second)
			if err = poll(); err != nil {
				return err
			}
			return emit(max(int64(0), end))
		case <-ticker.C:
			if err = poll(); err != nil {
				return err
			}
			// Allow both endpoints time to deliver packets before filling missing
			// positions with silence. Flush this tail when the user stops.
			end := (time.Since(origin.time) - 80*time.Millisecond).Nanoseconds() * sampleRate / int64(time.Second)
			end = end / blockFrames * blockFrames
			if err = emit(max(int64(0), end)); err != nil {
				return err
			}
		}
	}
}

type audioDevice struct {
	client  *wca.IAudioClient
	capture *wca.IAudioCaptureClient
	name    string
	clock   packetClock
}

func openAudioDevice(loopback bool) (d *audioDevice, err error) {
	var enumerator *wca.IMMDeviceEnumerator
	if err = wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, 23, wca.IID_IMMDeviceEnumerator, &enumerator); err != nil {
		return nil, err
	}
	defer enumerator.Release()
	var endpoint *wca.IMMDevice
	flow := uint32(1)
	if loopback {
		flow = 0
	}
	if err = enumerator.GetDefaultAudioEndpoint(flow, wca.EConsole, &endpoint); err != nil {
		return nil, err
	}
	defer endpoint.Release()
	d = &audioDevice{}
	owned := d
	good := false
	defer func() {
		if !good {
			owned.close()
		}
	}()
	if err = endpoint.Activate(wca.IID_IAudioClient, 23, nil, &d.client); err != nil {
		return nil, err
	}
	format := wca.WAVEFORMATEX{WFormatTag: 1, NChannels: 2, NSamplesPerSec: sampleRate, NAvgBytesPerSec: sampleRate * 4, NBlockAlign: 4, WBitsPerSample: 16}
	flags := uint32(wca.AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM | wca.AUDCLNT_STREAMFLAGS_SRC_DEFAULT_QUALITY)
	if loopback {
		flags |= wca.AUDCLNT_STREAMFLAGS_LOOPBACK
	}
	if err = d.client.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, flags, 1000000, 0, &format, nil); err != nil {
		return nil, err
	}
	if err = d.client.GetService(wca.IID_IAudioCaptureClient, &d.capture); err != nil {
		return nil, err
	}
	good = true
	return d, nil
}

func (d *audioDevice) close() {
	if d.client != nil {
		_ = d.client.Stop()
	}
	if d.capture != nil {
		d.capture.Release()
	}
	if d.client != nil {
		d.client.Release()
	}
}
func (d *audioDevice) read(mixer *audioMixer, originQPC int64) error {
	for {
		var size uint32
		if err := d.capture.GetNextPacketSize(&size); err != nil {
			return err
		}
		if size == 0 {
			return nil
		}
		var data *byte
		var frames, flags uint32
		var position, qpc uint64
		if err := d.capture.GetBuffer(&data, &frames, &flags, &position, &qpc); err != nil {
			return err
		}
		valid := flags&wca.AUDCLNT_BUFFERFLAGS_TIMESTAMP_ERROR == 0
		observed := (int64(qpc) - originQPC) * sampleRate / 10000000
		if !valid {
			observed = (qpcNow()-originQPC)*sampleRate/10000000 - int64(frames)
		}
		start := d.clock.place(position, observed, frames, valid)
		var mixErr error
		if flags&wca.AUDCLNT_BUFFERFLAGS_SILENT == 0 && data != nil {
			mixErr = mixer.add(start, unsafe.Slice((*int16)(unsafe.Pointer(data)), int(frames)*2))
		}
		releaseErr := d.capture.ReleaseBuffer(frames)
		if mixErr != nil || releaseErr != nil {
			return errors.Join(mixErr, releaseErr)
		}
	}
}
