# Platform Support — สถานะจริง ณ 2026-07-25

> **จุดยืน (owner, 2026-07-25):** ยังไม่ข้ามไป Linux/macOS ตอนนี้ **เอา Windows ให้เสถียรก่อน**
> เอกสารนี้มีไว้บันทึกว่า "ถ้าจะไป ต้องแก้อะไรบ้าง" เท่านั้น ไม่ใช่แผนที่กำลังทำ
> Decision section: [ARCHITECTURE.md §29](ARCHITECTURE.md)

## สรุปสามบรรทัด

- **Windows** = แพลตฟอร์มจริงเพียงตัวเดียว มี CI รัน vet + `go test ./...` + vitest ทุก push
- **CLI + engine** (`cmd/`, `internal/`) **cross-compile ผ่าน** linux/darwin แล้ว และ CI เช็คให้ทุกครั้ง — แต่ **ไม่เคยถูกรันจริงบนเครื่อง Unix เลยสักครั้ง**
- **Desktop GUI** (`desktop/`) **ยังไปไม่ได้** ติดที่ dependency ระดับ Windows API ไม่ใช่เรื่องเล็ก

## ตารางสถานะ

| ส่วน | Windows | Linux / macOS | ติดอะไร |
|:---|:---|:---|:---|
| `internal/` (engine, tools, providers) | ✅ build + test + ใช้จริง | ⚠️ compile ผ่าน ไม่เคยรัน | ต้องมีเครื่อง/CI job จริงถึงจะรู้ |
| `cmd/aetox` (CLI) | ✅ build + test + ใช้จริง | ⚠️ compile ผ่าน ไม่เคยรัน | เหมือนข้างบน |
| `desktop/` (Wails GUI) | ✅ build + test + ใช้จริง | ❌ compile ไม่ผ่าน | conpty, go-webview2, `browser.go` เรียก Win32 ตรงๆ ไม่มี build tag |
| binary ภายนอก (tesseract, ffmpeg, whisper.cpp) | ✅ ใช้จริง | ✅ ข้ามแพลตฟอร์มอยู่แล้ว | เรียกผ่าน `exec` ไม่มีโค้ดเฉพาะ OS — มีแต่ข้อความบอกวิธีติดตั้งที่แยกตาม OS |

## สิ่งที่ต้องแก้ถ้าจะไปจริง (เรียงจากถูกไปแพง)

1. **รัน test suite บน Linux จริง** — เพิ่ม job `ubuntu-latest` ใน CI สำหรับ `./cmd/... ./internal/...`
   นี่คือขั้นที่ถูกที่สุดและได้ข้อมูลมากที่สุด เพราะโค้ดฝั่ง `!windows` ทุกไฟล์ (`hide_other.go`,
   `job_other.go`, `shell_other.go`, `tree_other.go`) **compile ผ่านแต่ไม่เคยถูกรัน**
   โดยเฉพาะ `tree_other.go` ที่ใช้ `Setpgid` + `kill(-pgid)` ซึ่งเป็นโค้ดที่ต้องเจอ process จริงถึงจะรู้ว่าถูก
2. **`proc.KillTreeOnExit` เป็น no-op นอก Windows** — Job Object ไม่มีบน Unix ต้องใช้ process group
   หรือ `prctl(PR_SET_PDEATHSIG)` แทน ตอนนี้ปิดแอปบน Unix แล้ว MCP server จะค้าง
3. **Desktop GUI** — งานใหญ่สุด ต้องหา WebView ของแต่ละ OS (WebKitGTK / WKWebView) และเขียน
   `browser.go` ใหม่ทั้งไฟล์ มี blueprint อยู่แล้วใน
   [docs/architecture/native-browser-embedding-2026-07-24.md](docs/architecture/native-browser-embedding-2026-07-24.md)
4. **Terminal (ConPTY)** — ต้องเปลี่ยนไปใช้ pty ของ Unix
5. **Distribution** — ตอนนี้มีแต่ NSIS installer + portable zip + Scoop (ARCHITECTURE.md §23)

## ที่ทำไปแล้วโดยไม่ได้ตั้งใจจะพอร์ต

- `internal/model/context_windows.go` → `context_window.go` — ชื่อไฟล์ลงท้าย `_windows.go` เป็น
  build constraint ของ Go ทำให้ CLI **build บน Linux/macOS ไม่ได้เลย** ทั้งที่ไฟล์นี้พูดถึง
  context window ของโมเดล ไม่เกี่ยวกับ OS เลย (ARCHITECTURE.md §28.1)
- CI มี step cross-compile linux/darwin แล้ว — กัน regression แบบเดียวกันไม่ให้เกิดอีก โดยไม่ต้อง
  commit อะไรเพิ่มเพื่อรองรับ Unix จริง
