package recording

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"cliprecorder/internal/capture"
)

// Record blocks until cancellation, finalizing the MP4 before it returns.
// ready is called only after the encoder and capture buffer are ready.
func Record(ctx context.Context, rect image.Rectangle, path string, opts Options, ready func(EncoderInfo)) (err error) {
	if err = opts.Validate(); err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if rect.Empty() {
		return fmt.Errorf("录制区域为空")
	}
	// H.264 requires even dimensions. Keep the top-left corner fixed.
	rect.Max.X -= rect.Dx() % 2
	rect.Max.Y -= rect.Dy() % 2
	if rect.Empty() {
		return fmt.Errorf("录制区域太小")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err = invoke(coInit, 0, 0); err != nil {
		return err
	}
	defer coUninit.Call()
	if err = invoke(startup, 0x20070, 1); err != nil {
		return err
	}
	defer shutdown.Call()
	surface, err := capture.New(rect)
	if err != nil {
		return err
	}
	defer surface.Close()
	enc, err := newEncoder(path, rect.Dx(), rect.Dy(), opts)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, enc.close()) }()
	var audio *audioSession
	var packets <-chan audioPacket
	audioFinished := false
	if opts.HasAudio() {
		audio, err = prepareAudio(opts)
		if err != nil {
			return err
		}
		packets = audio.packets
		defer func() {
			audio.cancel()
			for packet := range packets {
				err = errors.Join(err, enc.writeAudio(packet))
			}
			if !audioFinished {
				err = errors.Join(err, <-audio.done)
			}
		}()
	}
	t0 := time.Now()
	if audio != nil {
		audio.begin <- audioOrigin{time: t0, qpc: qpcNow()}
	}
	if ready != nil {
		ready(enc.info)
	}
	writeVideo := func() error {
		pixels, e := surface.Grab()
		if e != nil {
			return e
		}
		ts := time.Since(t0).Nanoseconds() / 100
		return enc.write(pixels, ts, 10000000/int64(opts.FPS))
	}
	if ctx.Err() != nil {
		return nil
	}
	if err = writeVideo(); err != nil {
		return err
	}
	drainAudio := func() error {
		// BitBlt may wait for a display refresh. Consume the whole audio
		// backlog before capturing another frame; handling only one 10ms
		// packet per 16ms frame would gradually fall behind at 60fps.
		for len(packets) > 0 {
			if e := enc.writeAudio(<-packets); e != nil {
				return e
			}
		}
		return nil
	}
	ticker := time.NewTicker(time.Second / time.Duration(opts.FPS))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case packet, ok := <-packets:
			if !ok {
				audioFinished = true
				return errors.Join(fmt.Errorf("声音采集意外停止"), <-audio.done)
			}
			if err = enc.writeAudio(packet); err != nil {
				return err
			}
			if err = drainAudio(); err != nil {
				return err
			}
		case <-ticker.C:
			if err = drainAudio(); err != nil {
				return err
			}
			if err = writeVideo(); err != nil {
				return err
			}
		}
	}
}

func OutputPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Videos", "ClipRecorder", "rec_"+time.Now().Format("20060102_150405.000")+".mp4"), nil
}
