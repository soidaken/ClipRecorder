use crate::app;
use crate::capture;
use crate::btnwin;
use std::boxed::Box;
use windows::core::*;
use windows::Win32::Foundation::*;
use windows::Win32::Graphics::Gdi::*;
use windows::Win32::System::LibraryLoader::GetModuleHandleW;
use windows::Win32::UI::Input::KeyboardAndMouse::{SetCapture, SetFocus, ReleaseCapture, VK_ESCAPE};
use windows::Win32::UI::WindowsAndMessaging::*;

const ACCENT: u32 = 0xFFC24C; // COLORREF 0x00BBGGRR -> #4CC2FF
const DIM_ALPHA: u8 = 110;

#[derive(Clone, Copy, PartialEq)]
pub enum Mode {
    Shot,
    Rec,
}

pub struct Overlay {
    pub hwnd: HWND,
    pub virt: RECT,
    pub sel: RECT,
    pub has_sel: bool,
    pub dragging: bool,
    pub anchor: POINT,
    pub mode: Mode,
    pub dark_dc: HDC,
    pub dark_bmp: HBITMAP,
    pub orig_dc: HDC,
    pub orig_bmp: HBITMAP,
    pub btns: Option<HWND>,
}

pub fn begin(mode: Mode) {
    unsafe {
        // virtual screen bounds
        let vx = GetSystemMetrics(SM_XVIRTUALSCREEN);
        let vy = GetSystemMetrics(SM_YVIRTUALSCREEN);
        let vw = GetSystemMetrics(SM_CXVIRTUALSCREEN);
        let vh = GetSystemMetrics(SM_CYVIRTUALSCREEN);
        let virt = RECT { left: vx, top: vy, right: vx + vw, bottom: vy + vh };

        // snapshot before showing overlay (so we don't capture ourselves)
        let sdc = GetDC(None);
        if sdc.is_invalid() {
            return;
        }
        let (orig_dc, orig_bmp, _) = match capture::create_topdown_dib(sdc, vw, vh) {
            Ok(v) => v,
            Err(_) => {
                ReleaseDC(None, sdc);
                return;
            }
        };
        BitBlt(orig_dc, 0, 0, vw, vh, Some(sdc), vx, vy, SRCCOPY | CAPTUREBLT).ok();
        // darkened copy
        let (dark_dc, dark_bmp, _) = match capture::create_topdown_dib(sdc, vw, vh) {
            Ok(v) => v,
            Err(_) => {
                let _ = DeleteObject(HGDIOBJ(orig_bmp.0));
                let _ = DeleteDC(orig_dc);
                ReleaseDC(None, sdc);
                return;
            }
        };
        BitBlt(dark_dc, 0, 0, vw, vh, Some(orig_dc), 0, 0, SRCCOPY).ok();
        // 1x1 black source for constant-alpha dim
        let (bdc, bbmp, _) = match capture::create_topdown_dib(sdc, 1, 1) {
            Ok(v) => v,
            Err(_) => {
                let _ = DeleteObject(HGDIOBJ(orig_bmp.0));
                let _ = DeleteDC(orig_dc);
                let _ = DeleteObject(HGDIOBJ(dark_bmp.0));
                let _ = DeleteDC(dark_dc);
                ReleaseDC(None, sdc);
                return;
            }
        };
        // fill black
        let mut bmi = BITMAPINFO::default();
        bmi.bmiHeader.biSize = std::mem::size_of::<BITMAPINFOHEADER>() as u32;
        bmi.bmiHeader.biWidth = 1;
        bmi.bmiHeader.biHeight = -1;
        bmi.bmiHeader.biPlanes = 1;
        bmi.bmiHeader.biBitCount = 32;
        bmi.bmiHeader.biCompression = BI_RGB.0;
        let brush = CreateSolidBrush(COLORREF(0));
        let oldb = SelectObject(bdc, HGDIOBJ(brush.0));
        let _ = Rectangle(bdc, -1, -1, 2, 2);
        SelectObject(bdc, oldb);
        let _ = DeleteObject(HGDIOBJ(brush.0));
        let bf = BLENDFUNCTION {
            BlendOp: AC_SRC_OVER as u8,
            BlendFlags: 0,
            SourceConstantAlpha: DIM_ALPHA,
            AlphaFormat: 0,
        };
        let _ = AlphaBlend(dark_dc, 0, 0, vw, vh, bdc, 0, 0, 1, 1, bf);
        let _ = DeleteObject(HGDIOBJ(bbmp.0));
        let _ = DeleteDC(bdc);
        ReleaseDC(None, sdc);

        let hinstance = HINSTANCE(GetModuleHandleW(None).unwrap().0);
        let hwnd = CreateWindowExW(
            WS_EX_TOPMOST | WS_EX_TOOLWINDOW,
            w!("CR_Overlay"),
            w!(""),
            WS_POPUP,
            vx,
            vy,
            vw,
            vh,
            None,
            None,
            Some(hinstance),
            None,
        )
        .unwrap_or_else(|e| {
            eprintln!("overlay create: {e}");
            HWND::default()
        });
        if hwnd.is_invalid() {
            let _ = DeleteObject(HGDIOBJ(orig_bmp.0));
            let _ = DeleteDC(orig_dc);
            let _ = DeleteObject(HGDIOBJ(dark_bmp.0));
            let _ = DeleteDC(dark_dc);
            return;
        }
        let ov = Box::new(Overlay {
            hwnd,
            virt,
            sel: RECT::default(),
            has_sel: false,
            dragging: false,
            anchor: POINT::default(),
            mode,
            dark_dc,
            dark_bmp,
            orig_dc,
            orig_bmp,
            btns: None,
        });
        SetWindowLongPtrW(hwnd, GWLP_USERDATA, Box::into_raw(ov) as isize);
        let _ = ShowWindow(hwnd, SW_SHOW);
        let _ = SetForegroundWindow(hwnd);
        let _ = SetFocus(Some(hwnd));
        let _ = UpdateWindow(hwnd);
    }
}

pub fn end(ov: &mut Overlay) {
    unsafe {
        if let Some(b) = ov.btns.take() {
            let _ = DestroyWindow(b);
        }
        let hwnd = ov.hwnd;
        let _ = DestroyWindow(hwnd); // WM_NCDESTROY drops the Box, which frees GDI objects
    }
}

pub fn confirm(ov: &mut Overlay) {
    let global = RECT {
        left: ov.sel.left + ov.virt.left,
        top: ov.sel.top + ov.virt.top,
        right: ov.sel.right + ov.virt.left,
        bottom: ov.sel.bottom + ov.virt.top,
    };
    let mode = ov.mode;
    end(ov);
    app::set_overlay(None);
    match mode {
        Mode::Shot => {
            let shot = capture::grab_screen(global.left, global.top, global.right - global.left, global.bottom - global.top);
            match shot.and_then(|b| capture::copy_to_clipboard(&b)) {
                Ok(()) => app::tray_info("ClipRecorder", "截图已复制到剪贴板"),
                Err(e) => app::tray_info("ClipRecorder", &format!("截图失败: {e}")),
            }
        }
        Mode::Rec => {
            app::start_recording(global);
        }
    }
}

impl Drop for Overlay {
    fn drop(&mut self) {
        unsafe {
            let _ = DeleteDC(self.dark_dc);
            let _ = DeleteObject(HGDIOBJ(self.dark_bmp.0));
            let _ = DeleteDC(self.orig_dc);
            let _ = DeleteObject(HGDIOBJ(self.orig_bmp.0));
        }
    }
}

unsafe fn state(hwnd: HWND) -> Option<&'static mut Overlay> {
    let p = GetWindowLongPtrW(hwnd, GWLP_USERDATA);
    if p == 0 {
        None
    } else {
        Some(&mut *(p as *mut Overlay))
    }
}

unsafe extern "system" fn overlay_wndproc(
    hwnd: HWND,
    msg: u32,
    wparam: WPARAM,
    lparam: LPARAM,
) -> LRESULT {
    if msg == WM_NCDESTROY {
        let p = GetWindowLongPtrW(hwnd, GWLP_USERDATA);
        if p != 0 {
            drop(Box::from_raw(p as *mut Overlay));
            SetWindowLongPtrW(hwnd, GWLP_USERDATA, 0);
        }
        return LRESULT(0);
    }
    let ov = match state(hwnd) {
        Some(o) => o,
        None => return DefWindowProcW(hwnd, msg, wparam, lparam),
    };
    match msg {
        WM_PAINT => {
            let mut ps = PAINTSTRUCT::default();
            let hdc = BeginPaint(hwnd, &mut ps);
            if !hdc.is_invalid() {
                let rc = ps.rcPaint;
                let w = rc.right - rc.left;
                let h = rc.bottom - rc.top;
                if w > 0 && h > 0 {
                    let _ = BitBlt(hdc, rc.left, rc.top, w, h, Some(ov.dark_dc), rc.left, rc.top, SRCCOPY);
                }
                if ov.has_sel {
                    let s = ov.sel;
                    // brighten selected region
                    let _ = BitBlt(
                        hdc,
                        s.left,
                        s.top,
                        s.right - s.left,
                        s.bottom - s.top,
                        Some(ov.orig_dc),
                        s.left,
                        s.top,
                        SRCCOPY,
                    );
                    // accent border
                    let pen = CreatePen(PS_SOLID, 2, COLORREF(ACCENT));
                    let oldpen = SelectObject(hdc, HGDIOBJ(pen.0));
                    let oldbrush = SelectObject(hdc, GetStockObject(NULL_BRUSH));
                    let _ = Rectangle(hdc, s.left, s.top, s.right, s.bottom);
                    SelectObject(hdc, oldpen);
                    SelectObject(hdc, oldbrush);
                    let _ = DeleteObject(HGDIOBJ(pen.0));
                    // size chip
                    let text = format!("{} \u{d7} {}", s.right - s.left, s.bottom - s.top);
                    let mut wide: Vec<u16> = text.encode_utf16().collect();
                    let mut sz = SIZE::default();
                    let _ = GetTextExtentPoint32W(hdc, &wide, &mut sz);
                    let cx = s.left + 8;
                    let cy = if s.top - sz.cy - 12 >= 0 { s.top - sz.cy - 12 } else { s.top + 8 };
                    let mut trc = RECT {
                        left: cx,
                        top: cy,
                        right: cx + sz.cx + 12,
                        bottom: cy + sz.cy + 4,
                    };
                    let bgbrush = CreateSolidBrush(COLORREF(0x1E1C1C));
                    let ob = SelectObject(hdc, HGDIOBJ(bgbrush.0));
                    let _ = Rectangle(hdc, trc.left, trc.top, trc.right, trc.bottom);
                    SelectObject(hdc, ob);
                    let _ = DeleteObject(HGDIOBJ(bgbrush.0));
                    SetBkMode(hdc, TRANSPARENT);
                    SetTextColor(hdc, COLORREF(0xFFFFFF));
                    trc.top += 2;
                    let _ = DrawTextW(hdc, &mut wide, &mut trc, DT_CENTER | DT_VCENTER | DT_SINGLELINE);
                }
                let _ = EndPaint(hwnd, &ps);
            }
            LRESULT(0)
        }
        WM_ERASEBKGND => LRESULT(1),
        WM_LBUTTONDOWN => {
            ov.dragging = true;
            ov.has_sel = false;
            ov.anchor = POINT { x: (lparam.0 & 0xffff) as i16 as i32, y: ((lparam.0 >> 16) & 0xffff) as i16 as i32 };
            if let Some(b) = ov.btns.take() {
                let _ = DestroyWindow(b);
            }
            let _ = SetCapture(hwnd);
            let _ = InvalidateRect(Some(hwnd), None, false);
            LRESULT(0)
        }
        WM_MOUSEMOVE => {
            if ov.dragging {
                let cur = POINT { x: (lparam.0 & 0xffff) as i16 as i32, y: ((lparam.0 >> 16) & 0xffff) as i16 as i32 };
                let old = ov.sel;
                let (a, b) = (ov.anchor, cur);
                ov.sel = RECT {
                    left: a.x.min(b.x),
                    top: a.y.min(b.y),
                    right: a.x.max(b.x),
                    bottom: a.y.max(b.y),
                };
                ov.has_sel = true;
                let mut u = RECT {
                    left: old.left.min(ov.sel.left) - 8,
                    top: old.top.min(ov.sel.top) - 30,
                    right: old.right.max(ov.sel.right) + 8,
                    bottom: old.bottom.max(ov.sel.bottom) + 8,
                };
                // include chip area of both
                u.left = u.left.min(old.left - 8).min(ov.sel.left - 8);
                let _ = InvalidateRect(Some(hwnd), Some(&u), false);
            }
            LRESULT(0)
        }
        WM_LBUTTONUP => {
            if ov.dragging {
                ov.dragging = false;
                let _ = ReleaseCapture();
                let s = ov.sel;
                if s.right - s.left < 10 || s.bottom - s.top < 10 {
                    // too small: treat as cancel
                    end(ov);
                    app::set_overlay(None);
                } else {
                    let btns = btnwin::create_confirm(hwnd, &ov.sel, &ov.virt);
                    ov.btns = Some(btns);
                }
            }
            LRESULT(0)
        }
        WM_RBUTTONDOWN => {
            end(ov);
            app::set_overlay(None);
            LRESULT(0)
        }
        WM_KEYDOWN => {
            if wparam.0 as u32 == VK_ESCAPE.0 as u32 {
                end(ov);
                app::set_overlay(None);
            }
            LRESULT(0)
        }
        WM_KILLFOCUS => {
            // clicking outside (shouldn't happen) — keep overlay
            LRESULT(0)
        }
        app::WM_BTN_CLICK => {
            let id = wparam.0;
            match id {
                btnwin::BTN_OK => confirm(ov),
                _ => {
                    end(ov);
                    app::set_overlay(None);
                }
            }
            LRESULT(0)
        }
        WM_DESTROY => {
            // free GDI objects (Box already dropped only on NCDESTROY; handle both paths)
            LRESULT(0)
        }
        _ => DefWindowProcW(hwnd, msg, wparam, lparam),
    }
}

pub fn register_class(hinstance: HINSTANCE) {
    unsafe {
        let wc = WNDCLASSW {
            style: CS_HREDRAW | CS_VREDRAW,
            lpfnWndProc: Some(overlay_wndproc),
            hInstance: hinstance.into(),
            hCursor: LoadCursorW(None, IDC_CROSS).unwrap_or_default(),
            hbrBackground: HBRUSH::default(),
            lpszClassName: w!("CR_Overlay"),
            ..Default::default()
        };
        RegisterClassW(&wc);
    }
}
