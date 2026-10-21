package autostart

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func TestStartupTogglePersistsPreference(t *testing.T) {
	// Use isolated registry keys; tests never register a real login startup task.
	base := fmt.Sprintf(`Software\ClipRecorderTest-%d-%d`, os.Getpid(), time.Now().UnixNano())
	s := Settings{base + `\Run`, base + `\Settings`, "ClipRecorder"}
	t.Cleanup(func() {
		for _, key := range []string{s.runKey, s.preferenceKey, base} {
			if err := registry.DeleteKey(registry.CURRENT_USER, key); err != nil && err != registry.ErrNotExist {
				t.Errorf("clean registry %s: %v", key, err)
			}
		}
	})
	if on, err := s.Enabled(); err != nil || on {
		t.Fatalf("initial state: enabled=%v err=%v", on, err)
	}
	path := filepath.Join(t.TempDir(), "中文 路径", "ClipRecorder.exe")
	for _, enable := range []bool{true, false, false, true} {
		if err := s.SetEnabled(path, enable); err != nil {
			t.Fatal(err)
		}
		if on, err := s.Enabled(); err != nil || on != enable {
			t.Fatalf("toggle %v: enabled=%v err=%v", enable, on, err)
		}
		pref, err := registry.OpenKey(registry.CURRENT_USER, s.preferenceKey, registry.QUERY_VALUE)
		if err != nil {
			t.Fatal(err)
		}
		value, _, err := pref.GetIntegerValue("AutoStart")
		pref.Close()
		if err != nil || (value == 1) != enable {
			t.Fatalf("saved preference: %d %v", value, err)
		}
		if enable {
			run, err := registry.OpenKey(registry.CURRENT_USER, s.runKey, registry.QUERY_VALUE)
			if err != nil {
				t.Fatal(err)
			}
			command, _, err := run.GetStringValue(s.valueName)
			run.Close()
			if err != nil || command != `"`+path+`" --startup` {
				t.Fatalf("startup command not quoted: %q %v", command, err)
			}
		}
	}
}
