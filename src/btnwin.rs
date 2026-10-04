use crate::app;
use std::boxed::Box;
use std::time::Instant;
use windows::core::*;
use windows::Win32::Foundation::*;
use windows::Win32::Graphics::Gdi::*;
use windows::Win32::System::LibraryLoader::GetModuleHandleW;

use windows::Win32::UI::Controls::WM_MOUSELEAVE;
use windows::Win32::UI::Input::KeyboardAndMouse::{TrackMouseEvent, TME_LEAVE, TRACKMOUSEEVENT};
use windows::Win32::UI::WindowsAndMessaging::*;

pub const BTN_OK: usize = 1;
pub const BTN_CANCEL: usize = 2;
pub const BTN_STOP: usize = 3;

const BG: u32 = 0x1E1C1C;
const BTN_BG: u32 = 0x444040;
const BTN_HOVER: u32 = 0x5E5858;
const ACCENT: u32 = 0xFFC24C;
const ACCENT_TEXT: u32 = 0x000000;

pub enum Kind {
    Confirm,
    Indicator,
}

pub struct BtnWnd {
    pub parent: HWND,
    pub kind: Kind,
    pub buttons: Vec<(usize, String)>,
    pub label: String,
    pub start: Instant,
    pub hover: usize,
    pub font: HFONT,
}

pub fn create_confirm(parent: HWND, sel: &RECT, virt: &RECT) -> HWND {
    let buttons = vec![
        (BTN_OK, String::from("\u{2713} 确认")),
        (BTN_CANCEL, String::from("取消")),
    ];
    // place at bottom-right inside selection, flip above if near bottom
    let pad = 8;
    let (ww, wh) = layout_size(&buttons, true);
    let bx = (sel.right + virt.left - ww - pad).max(virt.left + pad);
    let by = sel.bottom + virt.top + pad;
    let by = if by + wh > virt.bottom - pad {
        sel.top + virt.top - wh - pad
    } else {
        by
    };
    let by = by.max(virt.top + pad);
    create_btnwnd(parent, bx, by, ww, wh, Kind::Confirm, buttons, String::new())
}

pub fn create_indicator(parent: HWND) -> HWND {
    // place near top-left of primary screen
    let mut pt = POINT::default();
    unsafe {
        let _ = GetCursorPos(&mut pt);
    }
    let buttons = vec![(BTN_STOP, String::from("\u{25a0} 停止"))];
    let (ww, wh) = layout_size(&buttons, true);
    let bx = (pt.x - ww / 2).max(8);
    let by = (pt.y - 80).max(8);
    create_btnwnd(parent, bx, by, ww, wh, Kind::Indicator, buttons, String::from("00:00"))
}

fn layout_size(buttons: &[(usize, String)], _with_label: bool) -> (i32, i32) {
    let bh = 36;
    let mut w = 12;
    for (_, label) in buttons {
        w += label.chars().count() as i32 * 9 + 16 + 24;
    }
    (w + 4, bh + 16)
}

fn create_btnwnd(
    parent: HWND,
    x: i32,
    y: i32,
    w: i32,
    h: i32,
    kind: Kind,
    buttons: Vec<(usize, String)>,
    label: String,
) -> HWND {
    unsafe {
        let hinstance = HINSTANCE(GetModuleHandleW(None).unwrap().0);
        let hwnd = CreateWindowExW(
            WS_EX_TOPMOST | WS_EX_TOOLWINDOW,
            w!("CR_Btn"),
            w!(""),
            WS_POPUP,
            x,
            y,
            w,
            h,
            None,
            None,
            Some(hinstance),
            None,
        )
        .expect("create btn window");
        let font = CreateFontW(
            -15,
            0,
            0,
            0,
            FW_SEMIBOLD.0 as i32,
            0,
            0,
            0,
            DEFAULT_CHARSET,
            OUT_DEFAULT_PRECIS,
            CLIP_DEFAULT_PRECIS,
            CLEARTYPE_QUALITY,
            (DEFAULT_PITCH.0 | FF_DONTCARE.0) as u32,
            w!("Segoe UI"),
        );
        let is_indicator = matches!(kind, Kind::Indicator);
        let data = Box::new(BtnWnd {
            parent,
            kind,
            buttons,
            label,
            start: Instant::now(),
            hover: 0,
            font,
        });
        SetWindowLongPtrW(hwnd, GWLP_USERDATA, Box::into_raw(data) as isize);
        // round corners
        let rgn = CreateRoundRectRgn(0, 0, w + 1, h + 1, 16, 16);
        let _ = SetWindowRgn(hwnd, Some(rgn), true);
        let _ = ShowWindow(hwnd, SW_SHOWNOACTIVATE);
        let _ = SetWindowPos(
            hwnd,
            Some(HWND_TOPMOST),
            0,
            0,
            0,
            0,
            SWP_NOMOVE | SWP_NOSIZE | SWP_NOACTIVATE,
        );
        if is_indicator {
            SetTimer(Some(hwnd), 1, 500, None);
        }
        hwnd
    }
}

unsafe fn state(hwnd: HWND) -> Option<&'static mut BtnWnd> {
    let p = GetWindowLongPtrW(hwnd, GWLP_USERDATA);
    if p == 0 {
        None
    } else {
        Some(&mut *(p as *mut BtnWnd))
    }
}

unsafe fn layout(hwnd: HWND, data: &BtnWnd) -> (Vec<(RECT, usize)>, RECT, i32, i32) {
    let mut wrc = RECT::default();
    let _ = GetWindowRect(hwnd, &mut wrc);
    let ww = wrc.right - wrc.left;
    let wh = wrc.bottom - wrc.top;
    let bh = 36;
    let mut btns = Vec::new();
    // right aligned
    let mut x = ww - 8;
    for (id, label) in data.buttons.iter().rev() {
        let mut wide: Vec<u16> = label.encode_utf16().collect();
        let bw = (wide.len() as i32 * 9).max(40) + 24;
        x -= bw;
        btns.push((
            RECT { left: x, top: (wh - bh) / 2, right: x + bw, bottom: (wh - bh) / 2 + bh },
            *id,
        ));
        x -= 8;
        let _ = &mut wide;
    }
    let label_rc = RECT { left: 14, top: 0, right: x, bottom: wh };
    (btns, label_rc, ww, wh)
}

unsafe extern "system" fn btn_wndproc(
    hwnd: HWND,
    msg: u32,
    wparam: WPARAM,
    lparam: LPARAM,
) -> LRESULT {
    if msg == WM_NCDESTROY {
        let p = GetWindowLongPtrW(hwnd, GWLP_USERDATA);
        if p != 0 {
            let b = Box::from_raw(p as *mut BtnWnd);
            let _ = DeleteObject(HGDIOBJ(b.font.0));
            SetWindowLongPtrW(hwnd, GWLP_USERDATA, 0);
        }
        return LRESULT(0);
    }
    let data = match state(hwnd) {
        Some(d) => d,
        None => return DefWindowProcW(hwnd, msg, wparam, lparam),
    };
    match msg {
        WM_PAINT => {
            let mut ps = PAINTSTRUCT::default();
            let hdc = BeginPaint(hwnd, &mut ps);
            if !hdc.is_invalid() {
                let (btns, _label_rc, _ww, wh) = layout(hwnd, data);
                // background
                let bg = CreateSolidBrush(COLORREF(BG));
                let old = SelectObject(hdc, HGDIOBJ(bg.0));
                let _ = Rectangle(hdc, -2, -2, ps.rcPaint.right + 2, ps.rcPaint.bottom + 2);
                SelectObject(hdc, old);
                let _ = DeleteObject(HGDIOBJ(bg.0));
                // indicator label
                if matches!(data.kind, Kind::Indicator) {
                    let oldfont = SelectObject(hdc, HGDIOBJ(data.font.0));
                    // red dot
                    let dot = CreateSolidBrush(COLORREF(0x3030FF));
                    let od = SelectObject(hdc, HGDIOBJ(dot.0));
                    let _ = Ellipse(hdc, 14, wh / 2 - 6, 26, wh / 2 + 6);
                    SelectObject(hdc, od);
                    let _ = DeleteObject(HGDIOBJ(dot.0));
                    SetBkMode(hdc, TRANSPARENT);
                    SetTextColor(hdc, COLORREF(0xFFFFFF));
                    let mut wide: Vec<u16> = data.label.encode_utf16().collect();
                    let mut trc = RECT { left: 34, top: 0, right: 34 + 60, bottom: wh };
                    let _ = DrawTextW(hdc, &mut wide, &mut trc, DT_LEFT | DT_VCENTER | DT_SINGLELINE);
                    SelectObject(hdc, oldfont);
                }
                // buttons
                for (rc, id) in &btns {
                    let label = data.buttons.iter().find(|(i, _)| i == id).unwrap().1.clone();
                    let hovered = *id == data.hover;
                    let is_ok = *id == BTN_OK;
                    let color = if is_ok {
                        ACCENT
                    } else if hovered {
                        BTN_HOVER
                    } else {
                        BTN_BG
                    };
                    let brush = CreateSolidBrush(COLORREF(color));
                    let oldb = SelectObject(hdc, HGDIOBJ(brush.0));
                    let _ = RoundRect(hdc, rc.left, rc.top, rc.right, rc.bottom, 12, 12);
                    SelectObject(hdc, oldb);
                    let _ = DeleteObject(HGDIOBJ(brush.0));
                    let oldfont = SelectObject(hdc, HGDIOBJ(data.font.0));
                    SetBkMode(hdc, TRANSPARENT);
                    SetTextColor(hdc, COLORREF(if is_ok { ACCENT_TEXT } else { 0xFFFFFF }));
                    let mut wide: Vec<u16> = label.encode_utf16().collect();
                    let mut trc = *rc;
                    let _ = DrawTextW(hdc, &mut wide, &mut trc, DT_CENTER | DT_VCENTER | DT_SINGLELINE);
                    SelectObject(hdc, oldfont);
                }
                let _ = EndPaint(hwnd, &ps);
            }
            LRESULT(0)
        }
        WM_ERASEBKGND => LRESULT(1),
        WM_MOUSEMOVE => {
            let px = (lparam.0 & 0xffff) as i16 as i32;
            let py = ((lparam.0 >> 16) & 0xffff) as i16 as i32;
            let (btns, _, _, _) = layout(hwnd, data);
            let mut newhover = 0usize;
            for (rc, id) in &btns {
                if px >= rc.left && px < rc.right && py >= rc.top && py < rc.bottom {
                    newhover = *id;
                }
            }
            if newhover != data.hover {
                data.hover = newhover;
                let _ = InvalidateRect(Some(hwnd), None, false);
            }
            let mut tme = TRACKMOUSEEVENT {
                cbSize: std::mem::size_of::<TRACKMOUSEEVENT>() as u32,
                dwFlags: TME_LEAVE,
                hwndTrack: hwnd,
                dwHoverTime: 0,
            };
            let _ = TrackMouseEvent(&mut tme);
            LRESULT(0)
        }
        WM_MOUSELEAVE => {
            if data.hover != 0 {
                data.hover = 0;
                let _ = InvalidateRect(Some(hwnd), None, false);
            }
            LRESULT(0)
        }
        WM_LBUTTONDOWN => {
            let px = (lparam.0 & 0xffff) as i16 as i32;
            let py = ((lparam.0 >> 16) & 0xffff) as i16 as i32;
            let (btns, _, _, _) = layout(hwnd, data);
            for (rc, id) in &btns {
                if px >= rc.left && px < rc.right && py >= rc.top && py < rc.bottom {
                    let _ = PostMessageW(
                        Some(data.parent),
                        app::WM_BTN_CLICK,
                        WPARAM(*id),
                        LPARAM(hwnd.0 as isize),
                    );
                }
            }
            LRESULT(0)
        }
        WM_TIMER => {
            if matches!(data.kind, Kind::Indicator) {
                let el = data.start.elapsed();
                let secs = el.as_secs();
                let label = format!("{:02}:{:02}", secs / 60, secs % 60);
                if label != data.label {
                    data.label = label;
                    let _ = InvalidateRect(Some(hwnd), None, false);
                }
            }
            LRESULT(0)
        }
        _ => DefWindowProcW(hwnd, msg, wparam, lparam),
    }
}

pub fn register_class(hinstance: HINSTANCE) {
    unsafe {
        let wc = WNDCLASSW {
            style: CS_HREDRAW | CS_VREDRAW,
            lpfnWndProc: Some(btn_wndproc),
            hInstance: hinstance.into(),
            hCursor: LoadCursorW(None, IDC_ARROW).unwrap_or_default(),
            hbrBackground: HBRUSH::default(),
            lpszClassName: w!("CR_Btn"),
            ..Default::default()
        };
        RegisterClassW(&wc);
    }
}
