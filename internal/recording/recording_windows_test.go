package recording

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cliprecorder/internal/capture"
)

func TestInvalidRegion(t *testing.T) {
	if err := Record(context.Background(), image.Rectangle{}, filepath.Join(t.TempDir(), "empty.mp4"), Options{FPS: 30}, nil); err == nil {
		t.Fatal("empty region accepted")
	}
}

func TestEncoderModes(t *testing.T) {
	if os.Getenv("CLIPRECORDER_INTEGRATION") != "1" {
		t.Skip("requires Windows encoders")
	}
	for _, mode := range []EncodingMode{EncodingAuto, EncodingHardware, EncodingSoftware} {
		t.Run(fmt.Sprintf("mode%d", mode), func(t *testing.T) {
			opts := DefaultOptions()
			opts.Encoding = mode
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			dir := os.Getenv("CLIPRECORDER_TEST_DIR")
			if dir == "" {
				dir = t.TempDir()
			}
			path := filepath.Join(dir, fmt.Sprintf("encoder-mode%d.mp4", mode))
			var info EncoderInfo
			started := false
			err := Record(ctx, image.Rect(0, 0, 718, 338), path, opts, func(actual EncoderInfo) {
				started, info = true, actual
				time.AfterFunc(time.Second, cancel)
			})
			if err != nil {
				t.Fatal(err)
			}
			if !started {
				t.Fatal("recording did not start")
			}
			if mode == EncodingHardware && !info.Hardware {
				t.Fatal("hardware selection used software")
			}
			if mode == EncodingSoftware && info.Hardware {
				t.Fatal("software selection used hardware")
			}
			t.Logf("actual encoder: %+v", info)
		})
	}
}

func TestEncoderProbeParameters(t *testing.T) {
	if os.Getenv("CLIPRECORDER_INTEGRATION") != "1" {
		t.Skip("requires Windows encoders")
	}
	for _, size := range []image.Point{image.Pt(718, 338), image.Pt(1920, 1080), image.Pt(128, 96)} {
		for _, fps := range []int{30, 60} {
			opts := DefaultOptions()
			opts.FPS, opts.Quality = fps, QualityUltra
			info, err := ProbeEncoder(size.X, size.Y, opts)
			if err != nil {
				t.Fatalf("probe %v %dfps: %v", size, fps, err)
			}
			t.Logf("%v %dfps: %+v", size, fps, info)
		}
	}
}

func TestAudioAndFrameRateIntegration(t *testing.T) {
	if os.Getenv("CLIPRECORDER_INTEGRATION") != "1" {
		t.Skip("requires Windows audio devices")
	}
	for _, opts := range []Options{
		{FPS: 30}, {FPS: 60}, {FPS: 30, SystemAudio: true}, {FPS: 60, SystemAudio: true},
		{FPS: 60, Microphone: true}, {FPS: 60, SystemAudio: true, Microphone: true},
	} {
		name := fmt.Sprintf("%dfps_system%t_mic%t", opts.FPS, opts.SystemAudio, opts.Microphone)
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name+".mp4")
			duration := time.Second
			if opts.SystemAudio && opts.Microphone && os.Getenv("CLIPRECORDER_LONG_TEST") == "1" {
				duration = 30 * time.Second
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			deadline := time.AfterFunc(duration+15*time.Second, cancel)
			defer deadline.Stop()
			rect := image.Rect(0, 0, 256, 144)
			if os.Getenv("CLIPRECORDER_FULLHD_TEST") == "1" {
				opts.Quality = QualityHigh
				desktop := capture.Desktop()
				rect = image.Rectangle{Min: desktop.Min, Max: desktop.Min.Add(image.Pt(min(1920, desktop.Dx()), min(1080, desktop.Dy())))}
				duration = 3 * time.Second
			}
			err := Record(ctx, rect, path, opts, func(info EncoderInfo) { t.Logf("actual encoder: %+v", info); time.AfterFunc(duration, cancel) })
			if err != nil {
				t.Fatal(err)
			}
			if dir := os.Getenv("CLIPRECORDER_TEST_DIR"); dir != "" {
				if err = os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				data, e := os.ReadFile(path)
				if e != nil {
					t.Fatal(e)
				}
				if err = os.WriteFile(filepath.Join(dir, name+".mp4"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// Exercises the actual GDI + COM + H.264 pipeline, opt-in for interactive Windows.
func TestHardwareDetection(t *testing.T) {
	if os.Getenv("CLIPRECORDER_INTEGRATION") != "1" {
		t.Skip("requires Windows Media Foundation")
	}
	names, err := DetectHardwareEncoders()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("runtime hardware H.264 encoders: %v", names)
	for _, name := range names {
		if name == "" {
			t.Fatal("hardware encoder must have a display name")
		}
	}
}

func TestDesktopRecording(t *testing.T) {
	if os.Getenv("CLIPRECORDER_INTEGRATION") != "1" {
		t.Skip("requires an interactive Windows desktop")
	}
	r := capture.Desktop()
	r.Max = r.Min.Add(image.Pt(128, 96))
	img, err := capture.Snapshot(r)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Size() != r.Size() || img.Pix[3] != 255 {
		t.Fatal("invalid captured pixels")
	}
	for _, size := range []image.Point{image.Pt(128, 96), image.Pt(127, 95)} {
		t.Run(size.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "smoke.mp4")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ready := false
			err := Record(ctx, image.Rectangle{Min: r.Min, Max: r.Min.Add(size)}, path, Options{FPS: 30}, func(EncoderInfo) { ready = true; time.AfterFunc(400*time.Millisecond, cancel) })
			if err != nil {
				t.Fatal(err)
			}
			if !ready {
				t.Fatal("encoder never became ready")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(data) < 100 || string(data[4:8]) != "ftyp" {
				t.Fatal("invalid MP4 container")
			}
			// Finalize must produce the movie metadata, not only an incomplete media payload.
			found := false
			for i := 0; i+4 <= len(data); i++ {
				if string(data[i:i+4]) == "moov" {
					found = true
					break
				}
			}
			if !found {
				t.Fatal("MP4 metadata not finalized")
			}
			if os.Getenv("CLIPRECORDER_TEST_MP4") != "" {
				_ = os.WriteFile(os.Getenv("CLIPRECORDER_TEST_MP4"), data, 0600)
			}
		})
	}
}
