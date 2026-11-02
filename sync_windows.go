package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"cliprecorder/internal/clipsync"
	"cliprecorder/internal/i18n"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Clipboard sync for the local network: the PC serves its clipboard and a
// paired phone long polls it. Off until the user turns it on.

const (
	syncTokenValue = "ClipSyncToken"
	syncOnValue    = "ClipSync"
)

type syncState struct {
	mu      sync.Mutex
	server  *clipsync.Server
	store   *clipsync.Store
	token   string
	addr    string
	running bool
	stop    chan struct{}
	window  *mygo.Window
	copied  bool
}

var syncClip = &syncState{}

// syncToken reads the saved pairing token, creating one on first use.
func syncToken() string {
	if syncClip.token != "" {
		return syncClip.token
	}
	if saved := loadStringSetting(syncTokenValue); saved != "" {
		syncClip.token = saved
		return saved
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		log.Print("生成配对令牌失败: ", err)
	}
	syncClip.token = hex.EncodeToString(raw)
	saveStringSetting(syncTokenValue, syncClip.token)
	return syncClip.token
}

// lanAddress is the IPv4 address a phone on the same Wi-Fi can reach.
func lanAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	var fallback string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			if ip.IsPrivate() {
				return ip.String()
			}
			if fallback == "" {
				fallback = ip.String()
			}
		}
	}
	if fallback != "" {
		return fallback
	}
	return "127.0.0.1"
}

// pairingLine is what the phone needs, copied from the pairing window.
func pairingLine() string {
	port := "47820"
	if syncClip.addr != "" {
		if _, p, err := net.SplitHostPort(syncClip.addr); err == nil && p != "" {
			port = p
		}
	}
	return clipsync.Pairing(net.JoinHostPort(lanAddress(), port), syncToken())
}

// startClipboardSync opens the listener and starts watching the clipboard.
func startClipboardSync() error {
	syncClip.mu.Lock()
	if syncClip.running {
		syncClip.mu.Unlock()
		return nil
	}
	store := clipsync.NewStore()
	server := clipsync.NewServer(syncToken(), store, func(text string) {
		if err := copyTextToClipboard(text); err != nil {
			log.Print("写入剪贴板失败: ", err)
		}
	})
	addr, err := server.Start()
	if err != nil {
		syncClip.mu.Unlock()
		syncDebug("listen failed: " + err.Error())
		return err
	}
	// Start from whatever is on the clipboard right now.
	if current, readErr := clipsync.ReadClipboardText(); readErr == nil && current != "" {
		store.Set(current)
	}
	stop := make(chan struct{})
	syncClip.server, syncClip.store, syncClip.addr, syncClip.running, syncClip.stop = server, store, addr, true, stop
	syncClip.mu.Unlock()

	go watchClipboard(store, stop)
	syncDebug("started on " + addr)
	log.Print("剪贴板同步已开启: ", addr)
	return nil
}

// watchClipboard polls the clipboard sequence number and publishes changes.
func watchClipboard(store *clipsync.Store, stop chan struct{}) {
	lastSequence := clipsync.ClipboardSequence()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			sequence := clipsync.ClipboardSequence()
			if sequence == lastSequence {
				continue
			}
			lastSequence = sequence
			text, err := clipsync.ReadClipboardText()
			if err != nil || text == "" {
				continue
			}
			if clipsync.HashFor(text) == store.Hash() {
				continue // the phone just wrote this text
			}
			store.Set(text)
		}
	}
}

// stopClipboardSync closes the listener and stops the watcher.
func stopClipboardSync() {
	syncClip.mu.Lock()
	server, stop := syncClip.server, syncClip.stop
	syncClip.server, syncClip.store, syncClip.running, syncClip.stop = nil, nil, false, nil
	syncClip.mu.Unlock()
	if stop != nil {
		close(stop)
	}
	if server != nil {
		_ = server.Close()
	}
}

// showPairingWindow shows the line the phone needs.
func (a *app) showPairingWindow() {
	if syncClip.window != nil {
		syncClip.window.Focus()
		return
	}
	syncClip.copied = false
	window := mygo.NewWindow(mygo.WindowOptions{Title: i18n.T("剪贴板同步"), Width: 460, Height: 300, Hidden: true, AlwaysOnTop: true, DisableResize: true,
		Content: ui.View(func(c *ui.Context) {
			toolTheme(c)
			ui.Column(c).Fill().Padding(24).Gap(12).Background(ui.RGB(25, 31, 38)).Children(func() {
				ui.Text(c, i18n.T("剪贴板同步")).FontSize(18).FontWeight(600)
				if !syncClip.running {
					ui.Text(c, i18n.T("剪贴板同步未开启，请先在托盘菜单里勾选。")).FontSize(12).TextColor(muted)
				}
				ui.Text(c, i18n.T("手机和电脑连同一个 Wi-Fi，把下面这行粘贴到 ClipSync App：")).FontSize(12).TextColor(muted)
				ui.Text(c, pairingLine()).Font("Consolas").FontSize(13).TextColor(ink)
				ui.Text(c, i18n.T("首次连接若被 Windows 防火墙拦截，请允许 ClipRecorder 访问网络。")).FontSize(11).TextColor(muted)
				ui.Row(c).Gap(10).Children(func() {
					label := i18n.T("复制配对信息")
					if syncClip.copied {
						label = i18n.T("已复制")
					}
					if toolButton(c, label, true, syncClip.running).Clicked() {
						if err := copyTextToClipboard(pairingLine()); err == nil {
							syncClip.copied = true
							syncClip.window.Invalidate()
						}
					}
					if toolButton(c, i18n.T("关闭"), false, true).Clicked() {
						syncClip.window.Close()
					}
				})
			})
		}),
	})
	syncClip.window = window
	window.OnClosed(func() { syncClip.window = nil })
	window.Show()
	window.Focus()
}

// syncMenuItem toggles the LAN clipboard sync.
func syncMenuItem(a *app) *mygo.MenuItem {
	item := &mygo.MenuItem{ID: "clip-sync", Label: i18n.T("剪贴板同步"), Type: mygo.MenuItemCheckbox, Checked: syncClip.running}
	item.Click = func(item *mygo.MenuItem, _ *mygo.Window) {
		if item.IsChecked() {
			if err := startClipboardSync(); err != nil {
				item.SetChecked(false)
				a.notify(i18n.T("剪贴板同步开启失败"), i18n.Tf("%v，请检查端口 47820 是否被占用", err))
				return
			}
		} else {
			stopClipboardSync()
		}
		saveBoolSetting(syncOnValue, item.IsChecked())
		if syncClip.window != nil {
			syncClip.window.Invalidate()
		}
	}
	return item
}

// syncPairingMenuItem opens the pairing window.
func syncPairingMenuItem(a *app) *mygo.MenuItem {
	return &mygo.MenuItem{ID: "clip-sync-pairing", Label: i18n.T("显示配对信息…"), Click: func(*mygo.MenuItem, *mygo.Window) {
		a.showPairingWindow()
	}}
}

// syncEnabledAtStart reports whether the sync was on when the app last quit.
func syncEnabledAtStart() bool { return loadBoolSetting(syncOnValue) }

func syncDebug(msg string) {
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "syncdebug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, time.Now().Format("15:04:05.000"), msg)
}
