# ClipRecorder

[![build](https://github.com/soidaken/ClipRecorder/actions/workflows/build.yml/badge.svg)](https://github.com/soidaken/ClipRecorder/actions/workflows/build.yml)

[简体中文](README.md) · **English** · [日本語](README.ja.md)

A native Windows screenshot, screen recorder and region translator that lives in the system tray. Written in Go, the UI is drawn directly by [MyGo](https://github.com/egoist/mygo) (GPU-backed native UI) — no WebView, HTML or JavaScript, and no FFmpeg dependency.

## What it does

- **Screenshot**: drag a region and copy it to the clipboard.
- **Record**: record a region to H.264 / MP4, with system audio and microphone, aspect-ratio presets, frame rate, quality and hardware-encoder options.
- **Region translate**: drag over English text; the screenshot itself is sent to the model, which reads and translates it, and the Chinese result is drawn over the same region on a white background, keeping the original layout and text color where possible.

## Basic usage

| Shortcut | Action |
| --- | --- |
| `Ctrl+I` | Select a region → click the green "确认" button or press `Enter` |
| `Ctrl+R` | Select a region → set options → click "Start recording" or press `Enter` |
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

Confirming also writes a PNG copy to `%USERPROFILE%\Pictures\ClipRecorder\` and puts the file and its absolute path on the clipboard alongside the image, so apps can take the bitmap, the file or the text. Turn it off with "截图同时保存文件并复制路径" in the tray menu.

Annotations are included in the copied image. The bottom-left hint cancels with `Esc`. Once a region is chosen, clicking outside it does nothing; press `Esc` and select again to change it. Dragging from inside the region redraws it and clears the annotations.

The 低 / 中 / 高 (low / medium / high) segments set the copied resolution: low caps the longest side at 1280 px, medium (default) and high copy at the screen's physical pixels, 1:1. The clipboard only carries lossless PNG, so medium and high are the sharpest possible.

Recordings are saved to `%USERPROFILE%\Videos\ClipRecorder\`.

The first translation asks for a DeepSeek API key, which is then encrypted with Windows DPAPI and stored locally (`%APPDATA%\ClipRecorder\deepseek-key.dat`). The selected screenshot is sent to DeepSeek as JPEG for recognition and translation.

Every selection saves two images to the Downloads folder: `translate_<time>.jpg` is the crop sent to the model, and `translate_<time>_zh.png` is the rendered Chinese result.

Original colors that are too light (for example white text on a dark background) are darkened until they are readable on white, keeping the hue where possible. Before upload the crop is scaled to the resolution the model works at (about 1.7 megapixels) to keep the request small; lines of one block share a font size, centered titles stay centered, and any line that still comes back in English is retranslated once in a small text-only request. Installing or upgrading closes a running ClipRecorder, letting an active recording finish saving first.

## Tech stack

| Part | Implementation |
| --- | --- |
| Language / UI | Go + MyGo v0.5.5: native window, GPU glass panels, tray, global hotkeys, clipboard |
| Capture | Win32 GDI `BitBlt` called directly from Go, sharing the recording frame buffer |
| Video | Windows Media Foundation, H.264 / MP4, hardware encoder auto-detected (e.g. NVENC) with software fallback |
| Audio | WASAPI via [go-wca](https://github.com/moutend/go-wca): system loopback + microphone mixed into AAC 48 kHz |
| OCR | No local OCR: DeepSeek's vision model reads the screenshot directly |
| Translation | DeepSeek Chat API, receiving only the recognized text |

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
