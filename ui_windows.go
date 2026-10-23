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

// confirmButton is the primary action of the shot toolbar: WeChat green with
// bold white text, so it stands out from the dark glass around it.
func confirmButton(c *ui.Context, label string, enabled bool) ui.Element {
	b := ui.ButtonBase(c).Height(36).Padding(0, 18).Gap(8).Radius(9).
		Disabled(!enabled).Cursor(ui.CursorPointer)
	fill, edge, color := ui.RGB(7, 193, 96), ui.RGBA(255, 255, 255, 0.28), ui.RGB(255, 255, 255)
	if !enabled {
		fill, edge, color = ui.RGBA(7, 193, 96, 0.35), ui.RGBA(255, 255, 255, 0.12), ui.RGBA(255, 255, 255, 0.6)
	} else if b.Pressed() {
		fill = ui.RGB(6, 168, 84)
	} else if b.Hovered() {
		fill = ui.RGB(20, 208, 110)
	}
	b.Background(fill).Border(1, edge).InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.2))
	b.Children(func() {
		ui.Text(c, label).FontWeight(800).FontSize(15).TextColor(color)
	})
	return b
}

// encoderBadge marks the encoder the current parameters will use, next to the
// bitrate line: hardware in the toolbar accent, software in neutral grey.
func encoderBadge(c *ui.Context, hardware bool, detail string) ui.Element {
	label := "软编"
	fill, edge, color := ui.RGBA(255, 255, 255, 0.06), ui.RGBA(255, 255, 255, 0.12), muted
	if hardware {
		label, fill, edge, color = "硬编", ui.RGBA(185, 221, 246, 0.16), ui.RGBA(185, 221, 246, 0.4), ice
	}
	b := ui.Row(c).Height(18).Padding(0, 7).Radius(5).Shrink(0).AlignItems(ui.Center).
		Background(fill).Border(1, edge)
	b.Children(func() {
		ui.Text(c, label).FontSize(10).FontWeight(600).TextColor(color)
	})
	if detail != "" {
		b = b.Tooltip(detail)
	}
	return b
}

// cardButton toggles the rounded card look of the confirmed shot: a small
// square with a page-shaped outline, tinted while it is on.
func cardButton(c *ui.Context, on bool) ui.Element {
	b := ui.ButtonBase(c).Size(32, 32).Radius(9).Cursor(ui.CursorPointer).FocusRing(false)
	fill, edge := ui.Transparent, ui.Transparent
	color := muted
	if on {
		fill, edge, color = ui.RGBA(185, 221, 246, 0.16), ui.RGBA(185, 221, 246, 0.38), ice
	} else if b.Hovered() {
		fill = ui.RGBA(255, 255, 255, 0.08)
	}
	b.Background(fill).Border(1, edge)
	b.DrawOver(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		outer := ui.Rect{X: cx - 9, Y: cy - 7, W: 18, H: 14}
		p.Stroke(outer, color, 3, 1.4)
		p.Stroke(ui.Rect{X: cx - 5.5, Y: cy - 4, W: 11, H: 8}, color, 2, 1.2)
	})
	return b
}

// pathCheckbox is the small "复制路径" toggle of the shot toolbar: unchecked by
// default, because text on the clipboard makes apps like WPS paste the path
// instead of the picture.
func pathCheckbox(c *ui.Context, checked *bool) {
	row := ui.CheckboxBase(c, checked).Height(32).Gap(6).Padding(0, 9).
		Radius(8).Cursor(ui.CursorPointer).FocusRing(false).Shrink(0).Label("复制路径")
	on := *checked
	fill, edge := ui.RGBA(255, 255, 255, 0.03), ui.RGBA(255, 255, 255, 0.08)
	if on {
		fill, edge = ui.RGBA(185, 221, 246, 0.14), ui.RGBA(185, 221, 246, 0.34)
	} else if row.Hovered() {
		fill = ui.RGBA(255, 255, 255, 0.08)
	}
	row.Background(fill).Border(1, edge).Children(func() {
		box := ui.Box(c).Size(14, 14).Radius(4).Shrink(0)
		if on {
			box.Background(ice)
		} else {
			box.Background(ui.RGBA(0, 0, 0, 0.12)).Border(1, ui.RGBA(255, 255, 255, 0.28))
		}
		box.DrawOver(func(p *ui.Painter, r ui.Rect) {
			if on {
				var check ui.Path
				check.MoveTo(r.X+3, r.Y+7).LineTo(r.X+6, r.Y+10).LineTo(r.X+11, r.Y+4)
				p.StrokePath(&check, 1.5, ui.RGB(24, 35, 45))
			}
		})
		color := muted
		if on {
			color = ice
		}
		ui.Text(c, "复制路径").FontSize(11.5).TextColor(color).PassThrough()
	})
}
