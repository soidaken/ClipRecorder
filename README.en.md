# ClipRecorder

[简体中文](README.md) · **English** · [日本語](README.ja.md)

A native Windows screenshot, screen recorder and region translator that lives in the system tray. Written in Go, the UI is drawn directly by [MyGo](https://github.com/egoist/mygo) (GPU-backed native UI) — no WebView, HTML or JavaScript, and no FFmpeg dependency.

## What it does

- **Screenshot**: drag a region and copy it to the clipboard.
- **Record**: record a region to H.264 / MP4, with system audio and microphone, aspect-ratio presets, frame rate, quality and hardware-encoder options.
- **Region translate**: drag over English text; local OCR reads it, DeepSeek translates it, and the Chinese result is drawn over the same region on a white background, keeping the original layout and text color where possible.

## Basic usage

| Shortcut | Action |
| --- | --- |
| `Ctrl+I` | Select a region → click "Copy screenshot" or press `Enter` |
| `Ctrl+R` | Select a region → set options → click "Start recording" or press `Enter` |
| `Ctrl+T` | Select English text → release to recognize and translate in place |
| `Ctrl+Shift+T` / "Stop" on the floating bar | Stop and save the recording |
| `Esc` / right click | Cancel the selection |

The tray menu offers the same actions and toggles start-at-login.

Recordings are saved to `%USERPROFILE%\Videos\ClipRecorder\`.

The first translation asks for a DeepSeek API key, which is then encrypted with Windows DPAPI and stored locally (`%APPDATA%\ClipRecorder\deepseek-key.dat`). Screenshots are never uploaded.

## Tech stack

| Part | Implementation |
| --- | --- |
| Language / UI | Go + MyGo v0.5.5: native window, GPU glass panels, tray, global hotkeys, clipboard |
| Capture | Win32 GDI `BitBlt` called directly from Go, sharing the recording frame buffer |
| Video | Windows Media Foundation, H.264 / MP4, hardware encoder auto-detected (e.g. NVENC) with software fallback |
| Audio | WASAPI via [go-wca](https://github.com/moutend/go-wca): system loopback + microphone mixed into AAC 48 kHz |
| OCR | Windows built-in OCR, running locally (images are never uploaded) |
| Translation | DeepSeek Chat API, receiving only the recognized text |

## Download

Grab a build from [Releases](https://github.com/soidaken/ClipRecorder/releases):

- `ClipRecorder.exe`: portable, just run it.
- `ClipRecorder-Setup.exe`: installer for `%LOCALAPPDATA%\Programs\ClipRecorder`, enables start-at-login by default (toggle it from the tray).

Requires Windows 10 / 11, 64-bit. Translation needs the matching OCR language pack installed.

## Build from source

Go 1.27+ is required; the installer also needs the Inno Setup 6 compiler (or set `ISCC` to its path).

```powershell
go run .
.\build.ps1            # dist/ClipRecorder.exe
.\build-installer.ps1  # dist/ClipRecorder-Setup.exe
```

`build.ps1` stops any ClipRecorder running from this project directory before overwriting `dist/ClipRecorder.exe`. GitHub Actions builds the exe and installer on every push and publishes a release for `v*` tags.

## Limitations

- The mouse pointer is not recorded, and audio devices cannot be switched while recording.
- No annotations, window snapping or edge dragging in the selection.
- DRM-protected content may record as a black frame.
- Complex multi-column, decorative or rotated text is not guaranteed to match the original layout after translation.

## License

[MIT](LICENSE)
