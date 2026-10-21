package main

import (
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

const uiFont = "Microsoft YaHei UI, Segoe UI, sans-serif"

const recordingPanelHeight float32 = 410

var (
	ink   = ui.RGB(242, 245, 248)
	muted = ui.RGB(168, 179, 190)
	ice   = ui.RGB(185, 221, 246)
	rim   = ui.RGBA(255, 255, 255, 0.16)
)

func toolTheme(c *ui.Context) {
	t := ui.DarkTheme()
	t.Font, t.FontSize, t.Radius = uiFont, 13, 8
	t.Background = ui.Transparent
	t.Text, t.TextMuted = ink, muted
	t.Accent, t.AccentText = ice, ui.RGB(24, 35, 45)
	t.Focus = ui.RGBA(185, 221, 246, 0.7)
	c.SetTheme(t)
}

func glassPanel(e ui.Element) ui.Element {
	return e.Radius(16).Material(glass.Glass{Tint: ui.RGBA(22, 28, 35, 0.62)}).
		Border(1, rim).Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.28))
}

// Keep focus, keyboard activation and pointer handling native to MyGo;
// only the compact tool appearance is shared here.
func toolButton(c *ui.Context, label string, primary, enabled bool) ui.Element {
	b := ui.ButtonBase(c).Height(36).Padding(0, 16).Gap(8).Radius(8).
		Disabled(!enabled).Cursor(ui.CursorPointer)
	fill, edge, color := ui.RGBA(255, 255, 255, 0.06), ui.RGBA(255, 255, 255, 0.1), ink
	if primary {
		fill, edge, color = ice, ui.RGBA(255, 255, 255, 0.35), ui.RGB(24, 35, 45)
	}
	if !enabled {
		fill, edge, color = ui.RGBA(255, 255, 255, 0.045), ui.RGBA(255, 255, 255, 0.06), muted
	} else if b.Pressed() {
		fill = fill.Mix(ui.RGB(0, 0, 0), 0.12)
	} else if b.Hovered() {
		fill = fill.Mix(ui.RGB(255, 255, 255), 0.12)
	}
	b.Background(fill).Border(1, edge).InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.08))
	b.Children(func() { ui.Text(c, label).FontWeight(500).TextColor(color) })
	return b
}

// Apply the bound value before drawing so the check mark never lags a click.
func audioCheckbox(c *ui.Context, checked *bool, label string) {
	row := ui.CheckboxBase(c, checked).Height(38).Gap(8).Padding(0, 12).
		Radius(9).Cursor(ui.CursorPointer).FocusRing(false)
	on := *checked
	fill, edge := ui.RGBA(255, 255, 255, 0.035), ui.RGBA(255, 255, 255, 0.08)
	if on {
		fill, edge = ui.RGBA(185, 221, 246, 0.12), ui.RGBA(185, 221, 246, 0.3)
	} else if row.Hovered() {
		fill = ui.RGBA(255, 255, 255, 0.09)
	}
	row.Background(fill).Border(1, edge).Children(func() {
		box := ui.Box(c).Size(16, 16).Radius(4).Shrink(0)
		if on {
			box.Background(ice)
		} else {
			box.Background(ui.RGBA(0, 0, 0, 0.12)).Border(1, ui.RGBA(255, 255, 255, 0.3))
		}
		box.DrawOver(func(p *ui.Painter, r ui.Rect) {
			if on {
				var check ui.Path
				check.MoveTo(r.X+4, r.Y+8).LineTo(r.X+7, r.Y+11).LineTo(r.X+12, r.Y+5)
				p.StrokePath(&check, 1.6, ui.RGB(24, 35, 45))
			}
			if row.FocusVisible() {
				p.FocusRing(r, [4]float32{4, 4, 4, 4})
			}
		})
		ui.Text(c, label).TextColor(ink)
	})
}

func frameRateControl(c *ui.Context, index *int) {
	choiceControl(c, index, 88, "30 fps", "60 fps")
}

func choiceControl(c *ui.Context, index *int, width float32, labels ...string) {
	choiceControlEnabled(c, index, width, nil, labels...)
}

func choiceControlEnabled(c *ui.Context, index *int, width float32, enabled []bool, labels ...string) {
	ui.Row(c).Padding(3).Gap(3).Radius(9).Background(ui.RGBA(0, 0, 0, 0.18)).Children(func() {
		for i, label := range labels {
			active := enabled == nil || enabled[i]
			b := ui.ButtonBase(c.Key(label)).Size(width, 28).Radius(6).Disabled(!active).Cursor(ui.CursorPointer)
			if active && b.Clicked() {
				*index = i
			}
			color := muted
			if *index == i {
				b.Background(ui.RGBA(255, 255, 255, 0.13)).Border(1, ui.RGBA(255, 255, 255, 0.12))
				color = ink
			} else if active && b.Hovered() {
				b.Background(ui.RGBA(255, 255, 255, 0.05))
			}
			b.Role(ui.RoleRadio).Checked(*index == i).Children(func() {
				ui.Text(c, label).Font(uiFont).FontSize(13).TextColor(color)
			})
		}
	})
}

func recordingStopButton(c *ui.Context, enabled bool) ui.Element {
	b := ui.ButtonBase(c).Size(86, 54).Radius(10).Disabled(!enabled).Cursor(ui.CursorPointer)
	fill := ui.RGBA(255, 255, 255, 0.07)
	if enabled && b.Hovered() {
		fill = ui.RGBA(255, 255, 255, 0.14)
	}
	if enabled && b.Pressed() {
		fill = ui.RGBA(255, 255, 255, 0.04)
	}
	b.Background(fill).Border(1, ui.RGBA(255, 255, 255, 0.16)).
		InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.09)).Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Row(c).Gap(7).Children(func() {
				color := ui.RGB(255, 151, 161)
				if !enabled {
					color = muted
				}
				ui.Box(c).Size(8, 8).Radius(2).Background(color)
				ui.Text(c, "停止").TextColor(ink).FontWeight(500)
			})
			ui.Text(c, "Ctrl+Shift+T").Font("Segoe UI").FontSize(9).TextColor(muted).TextAlign(ui.Center)
		})
	})
	return b
}
