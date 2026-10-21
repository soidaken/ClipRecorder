# ClipRecorder

**简体中文** · [English](README.en.md) · [日本語](README.ja.md)

Windows 原生截图 / 录屏 / 框选翻译工具，常驻系统托盘。用 Go 编写，界面由 [MyGo](https://github.com/egoist/mygo) 直接绘制（GPU 原生 UI），没有 WebView、HTML 或 JavaScript，也不依赖 FFmpeg。

## 能做什么

- **截图**：框选任意区域，复制到剪贴板。
- **录屏**：框选区域录制为 H.264 / MP4，可同时录系统声音和麦克风，支持分辨率比例、帧率、画质与硬件编码选项。
- **框选翻译**：框选英文区域，松开后由本机 OCR 识别、DeepSeek 翻译，并在原位置用白底中文覆盖显示，尽量保留原有排版和文字颜色。

## 基本操作

| 快捷键 | 功能 |
| --- | --- |
| `Ctrl+I` | 框选截图 → 点击「复制截图」或按 `Enter` |
| `Ctrl+R` | 框选录屏 → 设置参数 → 点击「开始录制」或按 `Enter` |
| `Ctrl+T` | 框选英文区域 → 松开即识别并翻译覆盖 |
| `Ctrl+Shift+T` / 悬浮条「停止」 | 停止录制并保存 |
| `Esc` / 右键 | 取消框选 |

托盘图标右键可执行同样的操作，并控制开机启动。

录屏文件：`%USERPROFILE%\Videos\ClipRecorder\`。

翻译首次使用需填入 DeepSeek API Key，之后用 Windows DPAPI 加密保存在本机（`%APPDATA%\ClipRecorder\deepseek-key.dat`），截图不会上传。

## 技术栈

| 环节 | 实现 |
| --- | --- |
| 语言 / UI | Go + MyGo v0.5.5：原生窗口、GPU 玻璃面板、托盘、全局快捷键、剪贴板 |
| 屏幕捕获 | Win32 GDI `BitBlt`（Go 直接调用，复用录制帧缓冲） |
| 视频编码 | Windows Media Foundation，H.264 / MP4，自动检测并使用硬件编码器（如 NVENC），不可用时回退软件 |
| 音频 | [go-wca](https://github.com/moutend/go-wca) 的 WASAPI 绑定，系统回环 + 麦克风，混音后 AAC 48kHz |
| 文字识别 | Windows 自带 OCR（本机运行，不上传图片） |
| 翻译 | DeepSeek Chat API，仅发送识别出的文本 |

## 下载

到 [Releases](https://github.com/soidaken/ClipRecorder/releases) 下载：

- `ClipRecorder.exe`：免安装版，双击运行。
- `ClipRecorder-Setup.exe`：安装版，安装到 `%LOCALAPPDATA%\Programs\ClipRecorder`，默认开机启动，可在托盘里取消。

系统要求：Windows 10 / 11，64 位；OCR 翻译需要系统已安装对应语言包。

## 从源码构建

需要 Go 1.27+；安装版还需要 Inno Setup 6 编译器（或用 `ISCC` 环境变量指定路径）。

```powershell
go run .
.\build.ps1            # dist/ClipRecorder.exe
.\build-installer.ps1  # dist/ClipRecorder-Setup.exe
```

`build.ps1` 会先停止本项目目录里正在运行的 ClipRecorder，再构建并覆盖 `dist/ClipRecorder.exe`。每次 push 和 `v*` tag 会由 GitHub Actions 构建 exe、安装包并发布 Release。

## 限制

- 不录制鼠标指针；录制中不能切换声音设备。
- 选区不支持标注、窗口自动吸附、拖动边缘调整。
- DRM 保护内容可能录成黑屏。
- 复杂多栏、艺术字体、旋转文字的翻译排版无法保证与原图一致。

## 许可证

[MIT](LICENSE)
