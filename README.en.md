# ClipRecorder

[![build](https://github.com/soidaken/ClipRecorder/actions/workflows/build.yml/badge.svg)](https://github.com/soidaken/ClipRecorder/actions/workflows/build.yml)

[简体中文](README.md) · **English** · [日本語](README.ja.md)

A native Windows screenshot, screen recorder and region translator that lives in the system tray. Written in Go, the UI is drawn directly by [MyGo](https://github.com/egoist/mygo) (GPU-backed native UI) — no WebView, HTML or JavaScript, and no FFmpeg dependency.

## What it does

- **Screenshot**: drag a region and copy it to the clipboard.
- **Record**: record a region to H.264 / MP4, with system audio and microphone, aspect-ratio presets, frame rate, quality and hardware-encoder options. A "硬编 / 软编" badge on the bitrate line shows which encoder the current parameters will actually use.
- **Region translate**: drag over English text and it turns into Chinese in place: cards, buttons, backgrounds and icons stay, only the English is erased and the Chinese is set where it was, in its colour, size, weight and alignment, with paragraphs reflowed to their width. A scan band passes over the selection while it is translated; hold the mouse button down on the result to see the source. The overlay has a 2 px frame and closes with `Esc` (even when another window has the focus) or a right click.

## Basic usage

| Shortcut | Action |
| --- | --- |
| `Ctrl+I` | Select a region → click the green "确认" button or press `Enter` |
| `Ctrl+R` | Select a region → set options → click "Start recording" or press `Enter`; drag inside the region to move it as a whole |
| `Ctrl+T` | Select English text → release to recognize and translate in place |
| `Ctrl+Shift+T` / "Stop" on the floating bar | Stop and save the recording |
| `Esc` / right click | Cancel the selection |

The tray menu offers the same actions and toggles start-at-login.

## Annotating a shot

After selecting a region the toolbar offers three annotation tools, all off by default; click one again to leave it:

- **Pen**: draw freely inside the selection, 5 px crimson.
- **Blur**: drag a rectangle inside the selection; on release that area is blurred (strength 75).
- **Number**: click inside the selection to drop 1, 2, 3… in red circles (radius 12 px) with bold white digits.
- **Region mark**: drag a 6 px red dashed rounded box inside the selection; any number of them.

A confirmed shot looks like a macOS window capture: continuous rounded corners with smooth antialiased edges, a hairline outline and a soft shadow cast downwards. The padding around it stays fully transparent (PNG keeps its alpha) and the shadow scales with the shot, so it floats on any page without a coloured plate. Turn it off with the square icon in the toolbar to copy the raw 1:1 shot.

Confirming also writes a PNG copy to `%USERPROFILE%\Pictures\ClipRecorder\`. "复制路径" in the toolbar is ticked by default: the clipboard then carries the image (PNG + DIB), the file and its absolute path as text, so chat and office apps take the picture or the file and text-only inputs (a terminal, say) get the path. A few apps, such as WPS/Excel cells, paste the path whenever text is present; untick it there to copy only the image. The clipboard is handed over to the system in full right after confirming, so pasting keeps working even if ClipRecorder is busy or has exited.

Annotations are included in the copied image. The bottom-left hint cancels with `Esc`. Once a region is chosen, clicking outside it does nothing; press `Esc` and select again to change it. Dragging from inside the region redraws it and clears the annotations.

The 低 / 中 / 高 (low / medium / high) segments set the copied resolution: low caps the longest side at 1280 px, medium (default) and high copy at the screen's physical pixels, 1:1. The clipboard only carries lossless PNG, so medium and high are the sharpest possible.

Recordings are saved to `%USERPROFILE%\Videos\ClipRecorder\`; when a recording finishes, its absolute path is copied to the clipboard.

The first translation asks for a DeepSeek API key, which is then encrypted with Windows DPAPI and stored locally (`%APPDATA%\ClipRecorder\deepseek-key.dat`). The selected screenshot is sent to DeepSeek as JPEG for recognition and translation.

Every selection saves two images to the Downloads folder: `translate_<time>.jpg` is the crop sent to the model, and `translate_<time>_zh.png` is the rendered Chinese result.

Two things run at once: Windows OCR finds every word's exact position locally, and DeepSeek's vision model reads and translates the text paragraph by paragraph. The two are matched on the source text, and size, baseline, text colour, background and alignment are measured on the screenshot's pixels, so the translation lands exactly where the source was. Numbers, dates and addresses that need no translation keep their original pixels. Without an OCR language, or when nothing can be placed, the translation is listed on a plain sheet instead. Before upload the crop is scaled to the resolution the model works at (about 1.7 megapixels) to keep the request small, and any block that still comes back in English is retranslated once in a small text-only request. Installing or upgrading closes a running ClipRecorder, letting an active recording finish saving first.

## Tech stack

| Part | Implementation |
| --- | --- |
| Language / UI | Go + MyGo v0.5.5: native window, GPU glass panels, tray, global hotkeys, clipboard |
| Capture | Win32 GDI `BitBlt` called directly from Go, sharing the recording frame buffer |
| Video | Windows Media Foundation, H.264 / MP4, hardware encoder auto-detected (e.g. NVENC) with software fallback |
| Audio | WASAPI via [go-wca](https://github.com/moutend/go-wca): system loopback + microphone mixed into AAC 48 kHz |
| OCR | DeepSeek's vision model reads paragraphs; built-in Windows OCR locates the words locally |
| Translation | DeepSeek Chat API |

## Download

Grab `ClipRecorder-Setup.exe` from [Releases](https://github.com/soidaken/ClipRecorder/releases): it installs to `%LOCALAPPDATA%\Programs\ClipRecorder` and enables start-at-login by default (toggle it from the tray).

For a portable build, run `build.ps1` and launch `dist/ClipRecorder.exe`.

Requires Windows 10 / 11, 64-bit.

## Build from source

Go 1.27+ is required; the installer also needs the Inno Setup 6 compiler (or set `ISCC` to its path).

```powershell
go run .
.\build.ps1            # dist/ClipRecorder.exe
.\build-installer.ps1  # dist/ClipRecorder-Setup.exe
```

`build.ps1` stops any ClipRecorder running from this project directory before overwriting `dist/ClipRecorder.exe`. GitHub Actions builds the exe and installer on every push and publishes a release for `v*` tags; documentation-only changes (`*.md`, `LICENSE`, …) skip the build.

## Limitations

- The mouse pointer is not recorded, and audio devices cannot be switched while recording.
- No annotations, window snapping or edge dragging in the selection.
- DRM-protected content may record as a black frame.
- Complex multi-column, decorative or rotated text is not guaranteed to match the original layout after translation.

## License

[MIT](LICENSE)
