package main

import (
	"golang.org/x/sys/windows/registry"
)

// Small helpers for the handful of settings stored under one registry key.

const settingsKey = `Software\ClipRecorder`

func loadBoolSetting(name string) bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, settingsKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetIntegerValue(name)
	return err == nil && value != 0
}

func saveBoolSetting(name string, enabled bool) {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, settingsKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	value := uint32(0)
	if enabled {
		value = 1
	}
	_ = key.SetDWordValue(name, value)
}

func loadStringSetting(name string) string {
	key, err := registry.OpenKey(registry.CURRENT_USER, settingsKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	value, _, err := key.GetStringValue(name)
	if err != nil {
		return ""
	}
	return value
}

func saveStringSetting(name, value string) {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, settingsKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	_ = key.SetStringValue(name, value)
}
