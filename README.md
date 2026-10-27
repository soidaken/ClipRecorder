# ClipRecorder

[![build](https://github.com/soidaken/ClipRecorder/actions/workflows/build.yml/badge.svg)](https://github.com/soidaken/ClipRecorder/actions/workflows/build.yml)

**简体中文** · [English](README.en.md) · [日本語](README.ja.md)

Windows 原生截图 / 录屏 / 框选翻译工具，常驻系统托盘。用 Go 编写，界面由 [MyGo](https://github.com/egoist/mygo) 直接绘制（GPU 原生 UI），没有 WebView、HTML 或 JavaScript，也不依赖 FFmpeg。

## 能做什么

- **截图**：框选任意区域，复制到剪贴板。
- **录屏**：框选区域录制为 H.264 / MP4，可同时录系统声音和麦克风，支持分辨率比例、帧率、画质与硬件编码选项；参数面板的码率行右侧用「硬编 / 软编」标出按当前参数实际会使用的编码器。
- **框选翻译**：框选英文区域，松开后原地换成中文：卡片、按钮、底色、图标都保留，只把英文擦掉、在原位置用原来的颜色、字号、粗细和对齐方式写上中文，段落按原宽度重新排版。翻译时选区上有一道扫描光带，结果出来后按住鼠标可查看原文。覆盖层带 2px 细边框，`Esc`（即使焦点在别的窗口也有效）或右键关闭。

## 基本操作

| 快捷键 | 功能 |
| --- | --- |
| `Ctrl+I` | 框选截图 → 点击绿色「确认」按钮或按 `Enter` |
| `Ctrl+R` | 框选录屏 → 设置参数 → 点击「开始录制」或按 `Enter`；区域选定后可在选区内按住左键整体拖动范围 |
| `Ctrl+T` | 框选英文区域 → 松开即识别并翻译覆盖 |
| `Ctrl+Shift+T` / 悬浮条「停止」 | 停止录制并保存 |
| `Esc` / 右键 | 取消框选 |

托盘图标右键可执行同样的操作，并控制开机启动。

## 截图标注

框选完成后，工具条上有三个标注工具，默认都不选中，再点一次即退出：

- **画笔**：在选区内自由涂画，5px 大红色。
- **模糊**：在选区内拖出矩形，松手后该区域高斯模糊（强度 75）。
- **序号**：在选区内单击，依次落下 1、2、3… 的红色圆形序号（半径 12px），数字白色加粗，字号按数字位数自适应。
- **标记区域**：在选区内拖出红色圆角虚线框（6px 粗），可画多个，用来圈出重点区域。

点「确认」后输出的图片会做成 macOS 窗口截图的样子：连续曲率圆角、平滑抗锯齿边缘、一圈细描边和向下的柔和阴影。截图四周留白完全透明（PNG 保留 alpha），阴影大小随截图尺寸缩放，所以粘贴到任何页面都像一张悬浮的卡片，没有底色块；工具条上的方形图标可关掉，回到原始 1:1 截图。

点「确认」后会把 PNG 存一份到 `%USERPROFILE%\Pictures\ClipRecorder\`。工具条上的「复制路径」默认勾选：剪贴板同时放图片（PNG + DIB）、文件和绝对路径文本，聊天/办公软件按需取图片或文件，只收文字的输入框（如终端）拿到路径。个别应用（如 WPS/Excel 单元格）看到文本会优先粘路径，这时取消勾选即可只放图片。剪贴板内容在确认后立即完整交给系统，之后程序忙碌或退出都不影响粘贴。

标注会随截图一起复制到剪贴板；左下角提示 `Esc` 取消。框选完成后点击选区外不会重新开始框选，想换范围按 `Esc` 退出后重新框选，在选区内部拖动则会重画并清空已有标注。

工具条上的「低 / 中 / 高」控制复制出来的图片：低档把长边压到 1280px 换体积，中档（默认）与高档都按屏幕物理像素 1:1 输出。剪贴板只承载无损 PNG，所以清晰度上限就是中/高这一档。

录屏文件：`%USERPROFILE%\Videos\ClipRecorder\`。录制结束、文件保存完成后，该文件的绝对路径会自动复制到剪贴板。

翻译首次使用需填入 DeepSeek API Key，之后用 Windows DPAPI 加密保存在本机（`%APPDATA%\ClipRecorder\deepseek-key.dat`）。框选的截图会以 JPEG 发送给 DeepSeek 识别与翻译。

每次框选都会在下载目录保存两份图片：`translate_时间.jpg` 是发给模型的原始框选截图，`translate_时间_zh.png` 是渲染后的中文结果。

翻译分两路同时进行：Windows 自带 OCR 在本机找出每个单词的精确位置，DeepSeek 视觉模型按段落识别并翻译；两边按原文对齐后，字号、基线、文字颜色、背景色和对齐方式都从截图像素里测量，所以译文能严丝合缝地落在原处。数字、日期、IP 等不需要翻译的内容保留原像素不动。系统没有 OCR 语言包或没能对上位置时，改为在白底上按顺序列出译文。发送前截图会缩到模型自身处理的分辨率（约 170 万像素）以加快上传；若返回的某段仍是英文，会自动补发一次纯文本请求重译。安装或覆盖安装会自动关闭正在运行的 ClipRecorder，正在录制的文件会先完成保存。

## 技术栈

| 环节 | 实现 |
| --- | --- |
| 语言 / UI | Go + MyGo v0.5.5：原生窗口、GPU 玻璃面板、托盘、全局快捷键、剪贴板 |
| 屏幕捕获 | Win32 GDI `BitBlt`（Go 直接调用，复用录制帧缓冲） |
| 视频编码 | Windows Media Foundation，H.264 / MP4，自动检测并使用硬件编码器（如 NVENC），不可用时回退软件 |
| 音频 | [go-wca](https://github.com/moutend/go-wca) 的 WASAPI 绑定，系统回环 + 麦克风，混音后 AAC 48kHz |
| 文字识别 | DeepSeek 视觉模型按段落识别；Windows 自带 OCR 在本机提供单词位置 |
| 翻译 | DeepSeek Chat API |

## 下载

到 [Releases](https://github.com/soidaken/ClipRecorder/releases) 下载 `ClipRecorder-Setup.exe`：安装到 `%LOCALAPPDATA%\Programs\ClipRecorder`，默认开机启动，可在托盘里取消。

需要免安装版时，自行构建 `dist/ClipRecorder.exe`，双击即可运行。

系统要求：Windows 10 / 11，64 位。

## 从源码构建

需要 Go 1.27+；安装版还需要 Inno Setup 6 编译器（或用 `ISCC` 环境变量指定路径）。

```powershell
go run .
.\build.ps1            # dist/ClipRecorder.exe
.\build-installer.ps1  # dist/ClipRecorder-Setup.exe
```

`build.ps1` 会先停止本项目目录里正在运行的 ClipRecorder，再构建并覆盖 `dist/ClipRecorder.exe`。每次 push 和 `v*` tag 会由 GitHub Actions 构建 exe、安装包并发布 Release；仅改动文档（`*.md`、`LICENSE` 等）不会触发构建。

## 限制

- 不录制鼠标指针；录制中不能切换声音设备。
- 选区不支持标注、窗口自动吸附、拖动边缘调整。
- DRM 保护内容可能录成黑屏。
- 复杂多栏、艺术字体、旋转文字的翻译排版无法保证与原图一致。

## 许可证

[MIT](LICENSE)
