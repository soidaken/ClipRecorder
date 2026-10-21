package main

import (
	"log"
	"os"

	"cliprecorder/internal/autostart"
	"github.com/egoist/mygo"
)

func (a *app) autostartMenuItem() *mygo.MenuItem {
	settings := autostart.New()
	enabled, err := settings.Enabled()
	if err != nil {
		log.Printf("读取开机启动设置失败：%v", err)
	}
	return &mygo.MenuItem{ID: "autostart", Label: "开机启动", Type: mygo.MenuItemCheckbox, Checked: enabled,
		Click: func(item *mygo.MenuItem, _ *mygo.Window) {
			// MyGo toggles checkbox items before invoking Click.
			path, err := os.Executable()
			if err == nil {
				err = settings.SetEnabled(path, item.IsChecked())
			}
			if err != nil {
				actual, readErr := settings.Enabled()
				if readErr == nil {
					item.SetChecked(actual)
				} else {
					item.SetChecked(!item.IsChecked())
				}
				a.notify("开机启动设置失败", err.Error())
			}
		},
	}
}
