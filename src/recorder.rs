use crate::app;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};
use windows::core::*;
use windows::Win32::Foundation::*;
use windows::Win32::Graphics::Gdi::*;
use windows::Win32::Media::MediaFoundation::*;
use windows::Win32::System::Com::*;
use windows::Win32::System::SystemInformation::GetLocalTime;
use windows::Win32::UI::WindowsAndMessaging::PostMessageW;

const FPS: u64 = 30;
const FRAME_100NS: i64 = (10_000_000 / FPS) as i64; // 333333

pub fn start(rect: RECT, done_hwnd: HWND) -> Arc<AtomicBool> {
    let stop = Arc::new(AtomicBool::new(false));
    let stop2 = stop.clone();
    let done_addr = done_hwnd.0 as isize;
    std::thread::spawn(move || unsafe {
        let done_hwnd = HWND(done_addr as *mut core::ffi::c_void);
        match record_thread(rect, done_hwnd, stop2) {
            Ok(()) => {}
            Err(e) => {
                let msg = Box::new(format!("录屏失败: {e}"));
                let ptr = Box::into_raw(msg);
                if !done_hwnd.is_invalid() {
                    let _ = PostMessageW(
                        Some(done_hwnd),
                        app::WM_REC_DONE,
                        WPARAM(1),
                        LPARAM(ptr as isize),
                    );
                } else {
                    drop(Box::from_raw(ptr));
                }
            }
        }
    });
    stop
}

unsafe fn build_video_type(w: i32, h: i32, subtype: &GUID, bitrate: u32) -> Result<IMFMediaType> {
    let mt = MFCreateMediaType()?;
    mt.SetGUID(&MF_MT_MAJOR_TYPE, &MFMediaType_Video)?;
    mt.SetGUID(&MF_MT_SUBTYPE, subtype)?;
    mt.SetUINT32(&MF_MT_INTERLACE_MODE, MFVideoInterlace_Progressive.0 as u32)?;
    mt.SetUINT32(&MF_MT_ALL_SAMPLES_INDEPENDENT, 1)?;
    mt.SetUINT64(&MF_MT_FRAME_SIZE, ((w as u64) << 32) | (h as u64))?;
    mt.SetUINT64(&MF_MT_FRAME_RATE, (FPS << 32) | 1)?;
    mt.SetUINT64(&MF_MT_PIXEL_ASPECT_RATIO, (1u64 << 32) | 1)?;
    if subtype == &MFVideoFormat_RGB32 {
        mt.SetUINT32(&MF_MT_DEFAULT_STRIDE, (w * 4) as u32)?;
    } else {
        mt.SetUINT32(&MF_MT_AVG_BITRATE, bitrate)?;
    }
    Ok(mt)
}

unsafe fn record_thread(rect: RECT, done_hwnd: HWND, stop: Arc<AtomicBool>) -> Result<()> {
    let w = rect.right - rect.left;
    let h = rect.bottom - rect.top;
    if w <= 0 || h <= 0 {
        return Err(Error::new(E_INVALIDARG, "invalid region"));
    }

    let bitrate = (w as u32 * h as u32 * 3).clamp(1_500_000, 25_000_000);

    // output path
    let dir = std::env::var("USERPROFILE")
        .map(|p| std::path::Path::new(&p).join("Videos").join("ClipRecorder"))
        .unwrap_or_else(|_| std::path::PathBuf::from("Videos\\ClipRecorder"));
    let _ = std::fs::create_dir_all(&dir);
    let st = GetLocalTime();
    let name = format!(
        "rec_{:04}{:02}{:02}_{:02}{:02}{:02}.mp4",
        st.wYear, st.wMonth, st.wDay, st.wHour, st.wMinute, st.wSecond
    );
    let path = dir.join(name);
    let wide: Vec<u16> = path
        .as_os_str()
        .to_string_lossy()
        .encode_utf16()
        .chain([0])
        .collect();

    let _ = CoInitializeEx(None, COINIT_MULTITHREADED);
    MFStartup(MF_VERSION, MFSTARTUP_LITE)?;

    // sink writer
    let writer = MFCreateSinkWriterFromURL(PCWSTR(wide.as_ptr()), None, None)?;

    let out_type = build_video_type(w, h, &MFVideoFormat_H264, bitrate)?;
    let stream = writer.AddStream(&out_type)?;
    let in_type = build_video_type(w, h, &MFVideoFormat_RGB32, 0)?;
    writer.SetInputMediaType(stream, &in_type, None)?;
    writer.BeginWriting()?;

    // GDI capture setup
    let sdc = GetDC(None);
    let bmi_in = create_bmi(w, h);
    let mut bits: *mut u8 = std::ptr::null_mut();
    let hbmp = CreateDIBSection(
        Some(sdc),
        &bmi_in,
        DIB_RGB_COLORS,
        &mut bits as *mut _ as *mut *mut core::ffi::c_void,
        None,
        0,
    )?;
    let mdc = CreateCompatibleDC(Some(sdc));
    let old = SelectObject(mdc, HGDIOBJ(hbmp.0));

    let frame_len = (w * h * 4) as usize;
    let frame_dur = Duration::from_nanos(1_000_000_000 / FPS);
    let t0 = Instant::now();
    let mut next_frame: u64 = 0;
    let mut written: u64 = 0;

    loop {
        if stop.load(Ordering::SeqCst) {
            break;
        }
        // pace to next frame slot
        let target = t0 + frame_dur * next_frame as u32;
        let now = Instant::now();
        if now < target {
            std::thread::sleep(target - now);
        }
        next_frame += 1;

        if BitBlt(mdc, 0, 0, w, h, Some(sdc), rect.left, rect.top, SRCCOPY | CAPTUREBLT).is_err() {
            // transient GDI failure (e.g. secure desktop): skip this frame
            continue;
        }
        let _ = GdiFlush();

        // write sample
        let buf = MFCreateMemoryBuffer(frame_len as u32)?;
        let mut dst: *mut u8 = std::ptr::null_mut();
        let mut max_len = 0u32;
        let mut cur_len = 0u32;
        buf.Lock(&mut dst, Some(&mut max_len as *mut u32), Some(&mut cur_len as *mut u32))?;
        std::ptr::copy_nonoverlapping(bits, dst, frame_len);
        buf.Unlock()?;
        buf.SetCurrentLength(frame_len as u32)?;

        let sample = MFCreateSample()?;
        sample.AddBuffer(&buf)?;
        let ts = (written as i64) * FRAME_100NS;
        sample.SetSampleTime(ts)?;
        sample.SetSampleDuration(FRAME_100NS)?;
        writer.WriteSample(stream, &sample)?;
        written += 1;
    }

    eprintln!("stopping, calling flush");
    if let Err(e) = writer.Flush(stream) { eprintln!("flush err: {e}"); }
    eprintln!("flush done, calling finalize");
    match writer.Finalize() {
        Ok(()) => eprintln!("finalize ok"),
        Err(e) => eprintln!("finalize err: {e}"),
    }
    SelectObject(mdc, old);
    let _ = DeleteObject(HGDIOBJ(hbmp.0));
    let _ = DeleteDC(mdc);
    ReleaseDC(None, sdc);
    let _ = MFShutdown();
    CoUninitialize();

    eprintln!("frames written: {}", written);
    eprintln!("file size now: {:?}", std::fs::metadata(&path).map(|m| m.len()));
    let saved = Box::new(path.display().to_string());
    let ptr = Box::into_raw(saved);
    let _ = PostMessageW(
        Some(done_hwnd),
        app::WM_REC_DONE,
        WPARAM(0),
        LPARAM(ptr as isize),
    );
    Ok(())
}

unsafe fn create_bmi(w: i32, h: i32) -> BITMAPINFO {
    let mut bmi = BITMAPINFO::default();
    bmi.bmiHeader.biSize = std::mem::size_of::<BITMAPINFOHEADER>() as u32;
    bmi.bmiHeader.biWidth = w;
    bmi.bmiHeader.biHeight = -h;
    bmi.bmiHeader.biPlanes = 1;
    bmi.bmiHeader.biBitCount = 32;
    bmi.bmiHeader.biCompression = BI_RGB.0;
    bmi
}
