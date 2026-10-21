# ClipRecorder

[简体中文](README.md) · [English](README.en.md) · **日本語**

Windows ネイティブのスクリーンショット・画面録画・範囲翻訳ツールです。常駐型で、Go で書かれ、UI は [MyGo](https://github.com/egoist/mygo) が直接描画します（GPU によるネイティブ UI）。WebView・HTML・JavaScript を使わず、FFmpeg にも依存しません。

## できること

- **スクリーンショット**：範囲を選択してクリップボードにコピー。
- **画面録画**：選択範囲を H.264 / MP4 で録画。システム音声とマイクの同時録音、アスペクト比、フレームレート、画質、ハードウェアエンコーダーの選択に対応。
- **範囲翻訳**：英語の範囲をドラッグして離すと、ローカル OCR が認識し、DeepSeek が翻訳し、同じ位置に白背景で中国語を上書き表示します。元のレイアウトと文字色をできるだけ保ちます。

## 基本操作

| ショートカット | 動作 |
| --- | --- |
| `Ctrl+I` | 範囲を選択 → 「Copy screenshot」または `Enter` でコピー |
| `Ctrl+R` | 範囲を選択 → 設定 → 「Start recording」または `Enter` で録画開始 |
| `Ctrl+T` | 英語の範囲を選択 → 離すと認識・翻訳してその場に表示 |
| `Ctrl+Shift+T` / フローティングバーの「停止」 | 録画を停止して保存 |
| `Esc` / 右クリック | 選択をキャンセル |

トレイメニューからも同じ操作ができ、スタートアップ起動も切り替えられます。

録画ファイルの保存先：`%USERPROFILE%\Videos\ClipRecorder\`

翻訳を初めて使うときに DeepSeek API キーの入力が必要です。入力したキーは Windows DPAPI で暗号化してローカルに保存されます（`%APPDATA%\ClipRecorder\deepseek-key.dat`）。スクリーンショットは送信されません。

## 技術スタック

| 部分 | 実装 |
| --- | --- |
| 言語 / UI | Go + MyGo v0.5.5：ネイティブウィンドウ、GPU ガラスパネル、トレイ、グローバルホットキー、クリップボード |
| 画面キャプチャ | Win32 GDI `BitBlt` を Go から直接呼び出し（録画のフレームバッファを共有） |
| 映像 | Windows Media Foundation、H.264 / MP4。ハードウェアエンコーダー（NVENC など）を自動検出し、使えなければソフトウェアにフォールバック |
| 音声 | [go-wca](https://github.com/moutend/go-wca) 経由の WASAPI。システム音声とマイクをミックスして AAC 48kHz |
| 文字認識 | Windows 標準の OCR（ローカル処理、画像は送信しません） |
| 翻訳 | DeepSeek Chat API（認識したテキストのみ送信） |

## ダウンロード

[Releases](https://github.com/soidaken/ClipRecorder/releases) から `ClipRecorder-Setup.exe` を取得してください。`%LOCALAPPDATA%\Programs\ClipRecorder` にインストールされ、既定でスタートアップに登録されます（トレイから解除可能）。

インストール不要版が必要な場合は `build.ps1` で `dist/ClipRecorder.exe` をビルドして実行してください。

Windows 10 / 11（64 ビット）が必要です。翻訳には該当する OCR 言語パックのインストールが必要です。

## ソースからビルド

Go 1.27+ が必要です。インストーラーを作る場合は Inno Setup 6 のコンパイラーも必要です（`ISCC` 環境変数でパスを指定可）。

```powershell
go run .
.\build.ps1            # dist/ClipRecorder.exe
.\build-installer.ps1  # dist/ClipRecorder-Setup.exe
```

`build.ps1` はこのプロジェクトの ClipRecorder が起動中なら先に終了してから、`dist/ClipRecorder.exe` を上書きします。GitHub Actions が push のたびに exe とインストーラーをビルドし、`v*` タグでは Release を公開します。ドキュメントのみの変更（`*.md`、`LICENSE` など）ではビルドを実行しません。

## 制限

- マウスカーソルは録画されません。録画中の音声デバイス切り替えはできません。
- 選択範囲に対する注釈、ウィンドウ吸着、端のドラッグ調整はありません。
- DRM 保護されたコンテンツは黒画面になる場合があります。
- 複雑な段組み、装飾フォント、回転したテキストは、翻訳後のレイアウトが元と一致する保証はありません。

## ライセンス

[MIT](LICENSE)
