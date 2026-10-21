package recording

import (
	"context"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

func TestSystemToneRecording(t *testing.T) {
	if os.Getenv("CLIPRECORDER_TONE_TEST") != "1" {
		t.Skip("plays a quiet test tone through the default output")
	}
	player, err := exec.LookPath("ffplay")
	if err != nil {
		t.Fatal(err)
	}
	dir := os.Getenv("CLIPRECORDER_TEST_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	deadline := time.AfterFunc(15*time.Second, cancel)
	defer deadline.Stop()
	tone := exec.CommandContext(ctx, player, "-nodisp", "-autoexit", "-volume", "10", "-f", "lavfi", "-i", "sine=frequency=997:sample_rate=48000:duration=3")
	tone.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var playErr error
	opts := DefaultOptions()
	err = Record(ctx, image.Rect(0, 0, 718, 338), filepath.Join(dir, "system-tone.mp4"), opts, func(EncoderInfo) {
		playErr = tone.Start()
		time.AfterFunc(4*time.Second, cancel)
	})
	if playErr != nil {
		t.Fatal(playErr)
	}
	if tone.Process != nil {
		_ = tone.Wait()
	}
	if err != nil {
		t.Fatal(err)
	}
}

// Compare native frame positions with QPC packet placement on the real device.
func TestAudioDeviceClock(t *testing.T) {
	if os.Getenv("CLIPRECORDER_INTEGRATION") != "1" {
		t.Skip("requires audio devices")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := invoke(coInit, 0, 0); err != nil {
		t.Fatal(err)
	}
	defer coUninit.Call()
	for _, loopback := range []bool{true, false} {
		d, err := openAudioDevice(loopback)
		if err != nil {
			t.Fatal(err)
		}
		if err = d.client.Start(); err != nil {
			d.close()
			t.Fatal(err)
		}
		var packets, qpcJumps, positionJumps int
		var nextQPC int64
		var nextPosition uint64
		var minJump, maxJump int64
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			var size uint32
			if err = d.capture.GetNextPacketSize(&size); err != nil {
				break
			}
			if size == 0 {
				time.Sleep(time.Millisecond)
				continue
			}
			var data *byte
			var frames, flags uint32
			var position, qpc uint64
			if err = d.capture.GetBuffer(&data, &frames, &flags, &position, &qpc); err != nil {
				break
			}
			start := int64(qpc) * sampleRate / 10000000
			if packets > 0 {
				jump := start - nextQPC
				if jump != 0 {
					qpcJumps++
				}
				minJump, maxJump = min(minJump, jump), max(maxJump, jump)
				if position != nextPosition {
					positionJumps++
				}
			}
			packets++
			nextQPC, nextPosition = start+int64(frames), position+uint64(frames)
			if err = d.capture.ReleaseBuffer(frames); err != nil {
				break
			}
		}
		d.close()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("loopback=%t packets=%d QPC jumps=%d range=[%d,%d] frames; device-position jumps=%d", loopback, packets, qpcJumps, minJump, maxJump, positionJumps)
	}
}
