use std::error::Error;
use windows::Win32::Graphics::Gdi::*;

pub struct ScreenBits {
    pub w: i32,
    pub h: i32,
    /// Top-down 32bpp BGRA, stride = w*4
    pub bgra: Vec<u8>,
}

/// Grab a screen region (global physical pixel coords) into a BGRA buffer.
pub fn grab_screen(x: i32, y: i32, w: i32, h: i32) -> Result<ScreenBits, Box<dyn Error>> {
    unsafe {
        let sdc = GetDC(None);
        if sdc.is_invalid() {
            return Err("GetDC failed".into());
        }
        let bmi = make_bmi(w, h);
        let mut bits: *mut u8 = std::ptr::null_mut();
        let hbmp = CreateDIBSection(
            Some(sdc),
            &bmi,
            DIB_RGB_COLORS,
            &mut bits as *mut _ as *mut *mut core::ffi::c_void,
            None,
            0,
        )?;
        if hbmp.is_invalid() || bits.is_null() {
            ReleaseDC(None, sdc);
            return Err("CreateDIBSection failed".into());
        }
        let mdc = CreateCompatibleDC(Some(sdc));
        let old = SelectObject(mdc, HGDIOBJ(hbmp.0));
        BitBlt(mdc, 0, 0, w, h, Some(sdc), x, y, SRCCOPY | CAPTUREBLT)?;
        let _ = GdiFlush();
        SelectObject(mdc, old);
        let data = std::slice::from_raw_parts(bits, (w * h * 4) as usize).to_vec();
        let _ = DeleteObject(HGDIOBJ(hbmp.0));
        let _ = DeleteDC(mdc);
        ReleaseDC(None, sdc);
        Ok(ScreenBits { w, h, bgra: data })
    }
}

/// Copy a BGRA ScreenBits to the clipboard as an image.
pub fn copy_to_clipboard(b: &ScreenBits) -> Result<(), Box<dyn Error>> {
    let mut rgba = b.bgra.clone();
    for px in rgba.chunks_exact_mut(4) {
        px.swap(0, 2);
    }
    let mut cb = arboard::Clipboard::new()?;
    cb.set_image(arboard::ImageData {
        width: b.w as usize,
        height: b.h as usize,
        bytes: rgba.into(),
    })?;
    Ok(())
}

/// Create a top-down 32bpp DIB section, returns (hdc, hbmp, bits ptr).
pub unsafe fn create_topdown_dib(
    compatible_with: HDC,
    w: i32,
    h: i32,
) -> Result<(HDC, HBITMAP, *mut u8), Box<dyn Error>> {
    let bmi = make_bmi(w, h);
    let mut bits: *mut u8 = std::ptr::null_mut();
    let hbmp = CreateDIBSection(
        Some(compatible_with),
        &bmi,
        DIB_RGB_COLORS,
        &mut bits as *mut _ as *mut *mut core::ffi::c_void,
        None,
        0,
    )?;
    if hbmp.is_invalid() || bits.is_null() {
        return Err("CreateDIBSection failed".into());
    }
    let mdc = CreateCompatibleDC(Some(compatible_with));
    SelectObject(mdc, HGDIOBJ(hbmp.0));
    Ok((mdc, hbmp, bits))
}

unsafe fn make_bmi(w: i32, h: i32) -> BITMAPINFO {
    let mut bmi = BITMAPINFO::default();
    bmi.bmiHeader.biSize = std::mem::size_of::<BITMAPINFOHEADER>() as u32;
    bmi.bmiHeader.biWidth = w;
    bmi.bmiHeader.biHeight = -h; // top-down
    bmi.bmiHeader.biPlanes = 1;
    bmi.bmiHeader.biBitCount = 32;
    bmi.bmiHeader.biCompression = BI_RGB.0;
    bmi
}
