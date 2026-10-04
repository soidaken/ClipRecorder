# ClipRecorder

轻量的 Windows 截图 / 录屏工具。常驻系统托盘，全局快捷键触发。

## 功能

| 快捷键 | 功能 |
| --- | --- |
| `PrtSc` | 截图：全屏暗色遮罩 → 拖拽选区 → 点"确认" → 截图自动复制到剪贴板 |
| `Ctrl+PrtSc` | 录屏：同样的选区流程 → 确认后开始录制选区为 MP4 (H.264, 30fps) → 点悬浮条"停止"结束（再次按 `Ctrl+PrtSc` 也可停止） |

- 选区内显示尺寸提示；`Esc` 或右键取消选区。
- 录屏文件保存在 `视频\ClipRecorder\rec_YYYYMMDD_HHMMSS.mp4`。
- 录制时不包含鼠标指针（GDI 屏幕抓取的固有限制）。
- 托盘图标右键菜单：截图 / 录屏 / 停止录制 / 退出。

## 技术要点

- 纯 Rust 原生 Win32 实现（`windows` crate），单文件 exe（约 360 KB），无常驻运行时，热键触发到选区遮罩出现 < 50ms。
- 视频编码使用系统自带的 Media Foundation H.264 编码器，直接封装 MP4，无第三方编码依赖。
- 剪贴板图片使用 `arboard`。
- 录制帧捕获为 GDI `BitBlt`（`SRCCOPY | CAPTUREBLT`）。

## 构建

```
cargo build --release
```

产物：`target/release/ClipRecorder.exe`

## 已知限制

- Win10/Win11；多显示器支持（遮罩覆盖整个虚拟桌面）。
- 受保护内容（如 DRM 视频窗口）抓取为黑屏，属系统限制。
