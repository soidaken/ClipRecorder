use crate::{btnwin, overlay, recorder};
use std::cell::RefCell;
use std::sync::atomic::AtomicBool;
use std::sync::Arc;
use windows::core::*;
use windows::Win32::Foundation::*;
use windows::Win32::System::LibraryLoader::GetModuleHandleW;
use windows::Win32::System::Threading::CreateMutexW;
use windows::Win32::UI::HiDpi::*;
use windows::Win32::UI::Input::KeyboardAndMouse::*;
use windows::Win32::UI::Shell::*;
use windows::Win32::UI::WindowsAndMessaging::*;

pub const WM_TRAY: u32 = WM_APP + 1;
pub const WM_BTN_CLICK: u32 = WM_APP + 2; // wParam = button id, lParam = sender hwnd
pub const WM_REC_DONE: u32 = WM_APP + 3; // lParam = Box<str>* (saved path)

// menu ids
const ID_TRAY_SHOT: usize = 1;
const ID_TRAY_REC: usize = 2;
const ID_TRAY_STOP: usize = 3;
const ID_TRAY_EXIT: usize = 9;

pub struct Rec {
    pub stop: Arc<AtomicBool>,
    pub indicator: HWND,
}

struct App {
    main_hwnd: HWND,
    tray: NOTIFYICONDATAW,
    overlay: Option<HWND>,
    recording: Option<Rec>,
}

thread_local! {
    static APP: RefCell<App> = RefCell::new(App {
        main_hwnd: HWND::default(),
        tray: NOTIFYICONDATAW::default(),
        overlay: None,
        recording: None,
    });
}

pub fn is_busy() -> bool {
    APP.with(|a| {
        let a = a.borrow();
        a.overlay.is_some() || a.recording.is_some()
    })
}

pub fn is_recording() -> bool {
    APP.with(|a| a.borrow().recording.is_some())
}

pub fn main_hwnd() -> HWND {
    APP.with(|a| a.borrow().main_hwnd)
}

pub fn set_overlay(hwnd: Option<HWND>) {
    APP.with(|a| a.borrow_mut().overlay = hwnd);
}

pub fn start_recording(rect: RECT) {
    // rect is in global physical coords
    let main = main_hwnd();
    let indicator = btnwin::create_indicator(main, &rect);
    let stop = recorder::start(rect, main);
    APP.with(|a| {
        a.borrow_mut().recording = Some(Rec { stop, indicator });
    });
    tray_info("ClipRecorder", "开始录屏，点击悬浮条上的停止按钮结束");
}

pub fn stop_recording() {
    APP.with(|a| {
        if let Some(r) = &a.borrow().recording {
            r.stop.store(true, std::sync::atomic::Ordering::SeqCst);
        }
    });
}

pub fn tray_info(title: &str, text: &str) {
    APP.with(|a| {
        let mut a = a.borrow_mut();
        unsafe { notify_info(&mut a.tray, title, text) };
    });
}

unsafe fn notify_info(nid: &mut NOTIFYICONDATAW, title: &str, text: &str) {
    nid.uFlags |= NIF_INFO;
    let t: Vec<u16> = text.encode_utf16().take(255).collect();
    let ti: Vec<u16> = title.encode_utf16().take(63).collect();
    for (i, c) in t.iter().chain(std::iter::once(&0)).enumerate() {
        nid.szInfo[i] = *c;
    }
    for (i, c) in ti.iter().chain(std::iter::once(&0)).enumerate() {
        nid.szInfoTitle[i] = *c;
    }
    nid.dwInfoFlags = NIIF_RESPECT_QUIET_TIME;
    let _ = Shell_NotifyIconW(NIM_MODIFY, nid);
}

pub fn run() {
    unsafe {
        let _ = SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2);
        // single instance
        let mutex = CreateMutexW(None, false, w!("ClipRecorder_SingleInstance_Mutex"));
        if mutex.is_ok() && GetLastError() == ERROR_ALREADY_EXISTS {
            return;
        }

        let hinstance = GetModuleHandleW(None).unwrap();
        let hinstance = HINSTANCE(hinstance.0);

        // main hidden window class
        let wc = WNDCLASSW {
            style: CS_HREDRAW | CS_VREDRAW,
            lpfnWndProc: Some(main_wndproc),
            hInstance: hinstance.into(),
            lpszClassName: w!("CR_Main"),
            ..Default::default()
        };
        RegisterClassW(&wc);

        let hwnd = CreateWindowExW(
            WINDOW_EX_STYLE(0),
            w!("CR_Main"),
            w!("ClipRecorder"),
            WS_OVERLAPPEDWINDOW,
            0,
            0,
            0,
            0,
            None,
            None,
            Some(hinstance),
            None,
        )
        .expect("create main window");

        overlay::register_class(hinstance);
        btnwin::register_class(hinstance);

        // global hotkeys: PrtSc = screenshot, Ctrl+PrtSc = record
        RegisterHotKey(Some(hwnd), 1, MOD_NOREPEAT, VK_SNAPSHOT.0 as u32)
            .unwrap_or_else(|e| eprintln!("hotkey1: {e}"));
        RegisterHotKey(Some(hwnd), 2, MOD_CONTROL | MOD_NOREPEAT, VK_SNAPSHOT.0 as u32)
            .unwrap_or_else(|e| eprintln!("hotkey2: {e}"));

        // tray icon
        let icon = LoadIconW(None, IDI_APPLICATION).unwrap_or_default();
        let mut tip = [0u16; 128];
        let tip_text = w!("ClipRecorder: PrtSc 截图 / Ctrl+PrtSc 录屏");
        for (i, c) in tip_text.as_wide().iter().enumerate().take(127) {
            tip[i] = *c;
        }
        let mut tray = NOTIFYICONDATAW {
            cbSize: std::mem::size_of::<NOTIFYICONDATAW>() as u32,
            hWnd: hwnd,
            uID: 1,
            uFlags: NIF_MESSAGE | NIF_ICON | NIF_TIP,
            uCallbackMessage: WM_TRAY,
            hIcon: icon,
            szTip: tip,
            ..Default::default()
        };
        Shell_NotifyIconW(NIM_ADD, &tray)
            .ok()
            .unwrap_or_else(|e| eprintln!("tray: {e}"));
        tray.uFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP; // keep

        APP.with(|a| {
            let mut a = a.borrow_mut();
            a.main_hwnd = hwnd;
            a.tray = tray;
        });

        // message loop
        let mut msg = MSG::default();
        while GetMessageW(&mut msg, None, 0, 0).as_bool() {
            let _ = TranslateMessage(&msg);
            DispatchMessageW(&msg);
        }

        // cleanup
        let _ = Shell_NotifyIconW(NIM_DELETE, &APP.with(|a| a.borrow().tray));
        let _ = UnregisterHotKey(Some(hwnd), 1);
        let _ = UnregisterHotKey(Some(hwnd), 2);
    }
}

unsafe fn show_tray_menu(hwnd: HWND) {
    let menu = CreatePopupMenu().unwrap();
    let _ = AppendMenuW(menu, MF_STRING, ID_TRAY_SHOT, w!("截图  (PrtSc)"));
    let _ = AppendMenuW(menu, MF_STRING, ID_TRAY_REC, w!("录屏  (Ctrl+PrtSc)"));
    if is_recording() {
        let _ = AppendMenuW(menu, MF_STRING, ID_TRAY_STOP, w!("停止录制"));
    }
    let _ = AppendMenuW(menu, MF_SEPARATOR, 0, PCWSTR::null());
    let _ = AppendMenuW(menu, MF_STRING, ID_TRAY_EXIT, w!("退出"));
    let mut pt = POINT::default();
    let _ = GetCursorPos(&mut pt);
    let _ = SetForegroundWindow(hwnd);
    let _ = TrackPopupMenu(menu, TPM_RIGHTBUTTON, pt.x, pt.y, Some(0), hwnd, None);
    let _ = DestroyMenu(menu);
}

unsafe extern "system" fn main_wndproc(
    hwnd: HWND,
    msg: u32,
    wparam: WPARAM,
    lparam: LPARAM,
) -> LRESULT {
    match msg {
        WM_HOTKEY => {
            match wparam.0 {
                1 => {
                    if !is_busy() {
                        overlay::begin(crate::overlay::Mode::Shot);
                    }
                }
                2 => {
                    if is_recording() {
                        stop_recording();
                    } else if !is_busy() {
                        overlay::begin(crate::overlay::Mode::Rec);
                    }
                }
                _ => {}
            }
            LRESULT(0)
        }
        WM_TRAY => {
            let ev = lparam.0 as u32;
            if ev == WM_LBUTTONUP || ev == WM_RBUTTONUP {
                show_tray_menu(hwnd);
            }
            LRESULT(0)
        }
        WM_COMMAND => {
            let id = (wparam.0 & 0xffff) as usize;
            match id {
                ID_TRAY_SHOT => {
                    if !is_busy() {
                        overlay::begin(crate::overlay::Mode::Shot);
                    }
                }
                ID_TRAY_REC => {
                    if is_recording() {
                        stop_recording();
                    } else if !is_busy() {
                        overlay::begin(crate::overlay::Mode::Rec);
                    }
                }
                ID_TRAY_STOP => stop_recording(),
                ID_TRAY_EXIT => {
                    let _ = DestroyWindow(hwnd);
                }
                _ => {}
            }
            LRESULT(0)
        }
        WM_BTN_CLICK => {
            // indicator stop button
            if wparam.0 == btnwin::BTN_STOP {
                stop_recording();
            }
            LRESULT(0)
        }
        WM_REC_DONE => {
            let path_ptr = lparam.0 as *mut String;
            let text = if path_ptr.is_null() {
                String::from("录屏已保存")
            } else {
                let s = Box::from_raw(path_ptr);
                format!("录屏已保存: {}", s)
            };
            APP.with(|a| {
                let mut a = a.borrow_mut();
                if let Some(r) = a.recording.take() {
                    let _ = DestroyWindow(r.indicator);
                }
            });
            tray_info("ClipRecorder", &text);
            LRESULT(0)
        }
        WM_DESTROY => {
            APP.with(|a| {
                let mut a = a.borrow_mut();
                if let Some(r) = a.recording.take() {
                    r.stop.store(true, std::sync::atomic::Ordering::SeqCst);
                    let _ = DestroyWindow(r.indicator);
                }
            });
            PostQuitMessage(0);
            LRESULT(0)
        }
        _ => DefWindowProcW(hwnd, msg, wparam, lparam),
    }
}
