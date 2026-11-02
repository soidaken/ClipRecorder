package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// A tiny startup log, so a machine where the app will not start leaves
// something to read. It lives next to the settings and is truncated on every
// launch.
var (
	startupLogMu   sync.Mutex
	startupLogFile *os.File
)

func startupLogPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "cliprecorder-startup.log")
	}
	return filepath.Join(dir, "ClipRecorder", "startup.log")
}

func startupLog(format string, args ...any) {
	startupLogMu.Lock()
	defer startupLogMu.Unlock()
	if startupLogFile == nil {
		path := startupLogPath()
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return
		}
		startupLogFile = file
	}
	fmt.Fprintf(startupLogFile, "%s %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
	_ = startupLogFile.Sync()
}
