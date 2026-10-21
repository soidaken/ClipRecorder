// Package autostart manages the current user's Windows login startup entry.
package autostart

import (
	"errors"
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

type Settings struct {
	runKey, preferenceKey, valueName string
}

func New() Settings {
	return Settings{`Software\Microsoft\Windows\CurrentVersion\Run`, `Software\ClipRecorder`, "ClipRecorder"}
}

func (s Settings) Enabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, s.runKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer k.Close()
	command, _, err := k.GetStringValue(s.valueName)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	return command != "" && err == nil, err
}

func (s Settings) SetEnabled(executable string, enabled bool) error {
	path, err := filepath.Abs(executable)
	if err != nil {
		return err
	}
	run, _, err := registry.CreateKey(registry.CURRENT_USER, s.runKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer run.Close()
	previous, _, readErr := run.GetStringValue(s.valueName)
	if readErr != nil && !errors.Is(readErr, registry.ErrNotExist) {
		return readErr
	}
	if enabled {
		err = run.SetStringValue(s.valueName, fmt.Sprintf("\"%s\" --startup", path))
	} else {
		err = run.DeleteValue(s.valueName)
		if errors.Is(err, registry.ErrNotExist) {
			err = nil
		}
	}
	if err != nil {
		return err
	}
	// Persist an explicit preference so reinstalling/upgrading keeps opt-out.
	pref, _, err := registry.CreateKey(registry.CURRENT_USER, s.preferenceKey, registry.SET_VALUE)
	if err == nil {
		value := uint32(0)
		if enabled {
			value = 1
		}
		err = pref.SetDWordValue("AutoStart", value)
		pref.Close()
	}
	if err != nil {
		if readErr == nil {
			return errors.Join(err, run.SetStringValue(s.valueName, previous))
		}
		rollbackErr := run.DeleteValue(s.valueName)
		if errors.Is(rollbackErr, registry.ErrNotExist) {
			rollbackErr = nil
		}
		return errors.Join(err, rollbackErr)
	}
	return nil
}
