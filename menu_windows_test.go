package main

import (
	"testing"

	"github.com/egoist/mygo"
	"golang.org/x/sys/windows/registry"
)

func TestTrayItemsAreRealCheckboxes(t *testing.T) {
	a := &app{shotSaveFile: true}
	save := saveShotItem(a)
	ime := imeIdleItem()
	for _, item := range []*mygo.MenuItem{save, ime} {
		if item.Type != mygo.MenuItemCheckbox {
			t.Fatalf("%q is not a checkbox item: %q", item.Label, item.Type)
		}
	}
	// MyGo flips the state before invoking Click; the handler must follow it
	// instead of flipping again, or the menu and the setting drift apart.
	save.SetChecked(false)
	save.Click(save, nil)
	if a.shotSaveFile {
		t.Fatal("save-shot checkbox did not turn the saving off")
	}
	save.SetChecked(true)
	save.Click(save, nil)
	if !a.shotSaveFile {
		t.Fatal("save-shot checkbox did not turn the saving back on")
	}

	// The click handler stores the preference; keep the real setting untouched.
	saved := []bool{}
	saveIMEIdlePrefFunc = func(enabled bool) error { saved = append(saved, enabled); return nil }
	t.Cleanup(func() { saveIMEIdlePrefFunc = saveIMEIdlePref })
	imeWatch.enabled.Store(true)
	ime.SetChecked(false)
	ime.Click(ime, nil)
	if imeWatch.enabled.Load() {
		t.Fatal("ime checkbox did not turn the switch off")
	}
	ime.SetChecked(true)
	ime.Click(ime, nil)
	if !imeWatch.enabled.Load() {
		t.Fatal("ime checkbox did not turn the switch back on")
	}
	if len(saved) != 2 || saved[0] || !saved[1] {
		t.Fatalf("preference saves: %v", saved)
	}
}

func TestIMEIdlePreferenceRoundTrips(t *testing.T) {
	previous := loadIMEIdlePref()
	t.Cleanup(func() { _ = saveIMEIdlePref(previous) })

	if err := saveIMEIdlePref(true); err != nil {
		t.Fatal(err)
	}
	if !loadIMEIdlePref() {
		t.Fatal("enabled preference did not persist")
	}
	if err := saveIMEIdlePref(false); err != nil {
		t.Fatal(err)
	}
	if loadIMEIdlePref() {
		t.Fatal("disabled preference did not persist")
	}
}

func TestIMEIdleIsOffUntilChosen(t *testing.T) {
	previous := loadIMEIdlePref()
	t.Cleanup(func() { _ = saveIMEIdlePref(previous) })

	// A fresh install has no value at all: the switch must stay off.
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\ClipRecorder`, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	_ = key.DeleteValue(imePrefValue)
	key.Close()
	if loadIMEIdlePref() {
		t.Fatal("the IME switch must start disabled")
	}
}
