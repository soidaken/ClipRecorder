package main

import (
	"testing"

	"github.com/egoist/mygo"
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
}
