# Platform Support — แผนพอร์ตที่กำลังทำอยู่ · อัปเดต 2026-07-27

> **จุดยืน (owner, 2026-07-27):** กลับข้าง — จาก "บันทึกไว้เฉยๆ" เป็น **"ทำจริง เดสก์ท็อปก่อน"** เป้าหมาย v0.7.0
> เอกสารนี้เคยเป็นบันทึกสถานะ ตอนนี้เป็นแผนที่เดินอยู่ · Decision section: [ARCHITECTURE.md §48](ARCHITECTURE.md)
> (จุดยืนเดิม "Windows first, ไม่ไล่ตาม" อยู่ที่ §29 — ถูก §48 แทนที่แล้ว)

## สรุปสามบรรทัด

- **engine + CLI พร้อมกว่าที่เคยคิด** — 2026-07-27 รันเทสต์ทั้งชุดบน Linux จริงครั้งแรก **ผ่านหมด 23 แพ็กเกจ** และ `-race` ก็สะอาด
- **`desktop/` ติดแค่ 2 ไฟล์** — `browser.go` กับ `terminal.go` ที่เหลืออีก 30 กว่าไฟล์พกไปได้เลย
- **macOS ทดสอบบนเครื่องนี้ไม่ได้เลย** — ไม่มี container ไม่มี VM ที่ถูกกฎ เหลือทางเดียวคือ `macos-latest` runner

## ตารางสถานะ

| ส่วน | Windows | Linux | macOS |
|:---|:---|:---|:---|
| `internal/` (engine, tools, providers) | ✅ ใช้จริง | ✅ **เทสต์ผ่านจริง + race สะอาด** | ⚠️ compile ผ่าน ยังไม่เคยรัน |
| `cmd/aetox` (CLI) | ✅ ใช้จริง | ✅ **เทสต์ผ่านจริง** | ⚠️ compile ผ่าน ยังไม่เคยรัน |
| `desktop/` — terminal | ✅ ใช้จริง | ✅ **เทสต์ผ่านบนเคอร์เนลจริง** | ⚠️ โค้ดตัวเดียวกับ Linux แต่ **ยังไม่เคยคอมไพล์หรือรันบน mac เลย** |
| `desktop/` — ที่เหลือ (GUI, browser) | ✅ ใช้จริง | ❌ ยังคอมไพล์ไม่ผ่าน (เฟส 2–3a) | ❌ ยังคอมไพล์ไม่ผ่าน (เฟส 2–3b) |
| binary ภายนอก (tesseract, ffmpeg) | ✅ | ✅ มีข้อความติดตั้งแยก OS อยู่แล้ว | ✅ auto-install ผ่าน brew อยู่แล้ว |

> **ระวังการอ่านตาราง:** ✅ ของ Linux แปลว่า *รันจริงบนเคอร์เนล Linux แล้วผ่าน* ส่วน macOS ยัง **ไม่มีอะไรถูกพิสูจน์เลยสักอย่าง** — ไม่มี container ไม่มี VM ที่ถูกกฎ และ job `unix` ใน CI ยังไม่แตะ `./desktop/...` เพราะแพ็กเกจยังคอมไพล์ไม่ผ่านนอก Windows **เฟส 2 คือจุดที่ mac ได้รับการตรวจครั้งแรกในชีวิต**

## เฟส

| # | งาน | สถานะ |
|:--|:---|:---|
| **0** | CI matrix — job `unix` (ubuntu + macos) รัน `vet` + `test` บน `./cmd/... ./internal/...`, Linux เพิ่ม `-race` | ✅ **เสร็จ** |
| **1** | `terminal.go` → `ptySession` interface + `terminal_windows.go` / `terminal_unix.go` (`creack/pty` v1.1.24) | ✅ **เสร็จ** — Windows + Linux พิสูจน์แล้ว, mac ยังไม่ถูกตรวจ |
| 2 | `browser.go` → แยก `hostBackend`/`tabView` + `browser_windows.go` + `browser_other.go` (stub) → `desktop/` เขียวทั้ง 3 OS | ยังไม่เริ่ม |
| 3a | `browser_linux.go` — WebKitGTK widget ใน `GtkOverlay`/`GtkFixed` | ยังไม่เริ่ม |
| 3b | `browser_darwin.go` — `WKWebView` เป็น subview ของ Wails `NSView` | ยังไม่เริ่ม |
| 4 | packaging (`.deb`/`.rpm`/tar.gz + `.app`/`.dmg`) + `bench.sh` | ยังไม่เริ่ม |

## แผนที่ไฟล์ — ไฟล์ไหนของ OS ไหน

กฎ: **แยกไฟล์ก็ต่อเมื่อ type หรือ import ต่างกัน** ถ้าต่างแค่ชื่อคำสั่งที่จะรัน ใช้ `switch runtime.GOOS` ในไฟล์เดิม (เช่น `presets.go` / `subagents.go` ที่เลือกระหว่าง `explorer`/`open`/`xdg-open`) และ**ไม่แยกโฟลเดอร์** เพราะ Go นับ 1 โฟลเดอร์ = 1 package ส่วน `App` ต้องอยู่ใน `package main` เดียวไม่งั้น Wails generate `App.d.ts` ออกมาคนละชุด (ARCHITECTURE.md §48 Decision 2)

| ไฟล์ | build tag | Win | Linux | mac | มีอะไร |
|:---|:---|:-:|:-:|:-:|:---|
| `terminal.go` | — | ✅ | ✅ | ✅ | `ptySession`, `App.Terminal*` ทั้งหมด, ลูปอ่าน |
| `terminal_windows.go` | ชื่อไฟล์ | ✅ | | | ConPTY + รายชื่อเชลล์ของ Windows |
| `terminal_unix.go` | `//go:build unix` | | ✅ | ✅ | `creack/pty` + `$SHELL` + การกวาด session |
| `browser.go` | — | ✅ | ✅ | ✅ | JS builders, `sameOrigin`, `onMessage`, `App.Browser*` (เฟส 2) |
| `browser_windows.go` | ชื่อไฟล์ | ✅ | | | Win32 + WebView2 (เฟส 2) |
| `browser_linux.go` | ชื่อไฟล์ | | ✅ | | WebKitGTK (เฟส 3a) |
| `browser_darwin.go` | ชื่อไฟล์ | | | ✅ | WKWebView (เฟส 3b) |

`_unix.go` ต้องเขียน `//go:build unix` เอง เพราะ Go ไม่ถือว่า `_unix` เป็นคำลงท้ายพิเศษเหมือน `_windows`/`_linux`/`_darwin` — เลือก `unix` แทน `!windows` (แบบที่ `internal/proc` ใช้) เพราะมันตรงกับชื่อไฟล์และไม่ลากไป js/wasm กับ plan9 ที่ `creack/pty` ไม่รองรับ

**เฟส 2 ต้องการ stub อะไรบ้าง** — วัดจากการ type-check จริงบน Linux: `browserElement` · `browserImage` · `browserSnapshot` · `browserTab{navDone}` + `meta()` · `browserHost{mu, lastID, tabs}` + `tab()` · `App.browserSnapshot` · `App.BrowserClickRef` · `App.BrowserTypeRef` · `App.CloseAllBrowserTabs` · `App.BrowserClose` เท่านี้พอให้ `desktop/` ทั้งแพ็กเกจ vet ผ่านบน Linux

พิมพ์เขียวของเฟส 3 อยู่ที่ [docs/architecture/native-browser-embedding-2026-07-24.md](docs/architecture/native-browser-embedding-2026-07-24.md) §"macOS / Linux port blueprint" — เขียนจากรอบดีบักจริง ไม่ใช่ทฤษฎี **ยึดกฎ 5 ข้อท้ายเอกสารนั้นเป็นหลัก**

## เฟส 0 — วัดอะไรได้บ้าง (2026-07-27)

รันใน Docker `golang:1.25` บนเคอร์เนล Linux จริง ไม่ใช่ cross-compile

| เช็ค | ผล |
|:---|:---|
| `go vet ./cmd/... ./internal/...` | สะอาด (vet คอมไพล์ `_test.go` ด้วย ซึ่ง `go build` ไม่ทำ) |
| `go test` 23 แพ็กเกจ | **ok ทั้งหมด** |
| `go test -race` 23 แพ็กเกจ | **ok ทั้งหมด ไม่มี data race** — เช็คที่ `verify.sh` skip มาตลอดเพราะเครื่อง dev ไม่มี C compiler |
| `TestShellSkillCancelKillsGrandchild` | **PASS 2.55s** — `tree_other.go` (`Setpgid` + `kill(-pgid)`) ถูกพิสูจน์กับ process หลานจริง |
| `libwebkit2gtk-4.0-dev` บน Ubuntu 24.04 | **ไม่มีแล้ว** — เหลือ 4.1 เท่านั้น เฟส 3a ต้องใช้ `-tags webkit2_41` |

## สิ่งที่ยังไม่รู้ และรู้ได้ทางเดียว

1. **macOS ทุกอย่าง** — ไม่มี container ให้รัน และรัน macOS ใน VM บนเครื่องที่ไม่ใช่ Apple ผิดสัญญาอนุญาต เฟส 3b จะเป็นเฟสที่ iterate ช้าที่สุด (แก้ 1 บรรทัด = รอ CI 1 รอบ) นี่คือเหตุผลที่ทำ Linux ให้จบก่อน
2. **z-order / compositing บน WM จริง** — WSLg หรือ xvfb บอกได้แค่ "รันได้ วาดออก" บอกไม่ได้ว่าบน GNOME/KDE/tiling WM หน้าต่างลูกจะอยู่ถูกชั้น ซึ่งเป็นหัวใจทั้งหมดของ `browser.go`
3. **`proc.KillTreeOnExit` เป็น no-op นอก Windows** — Job Object ไม่มีบน Unix `shutdown()` เก็บ mcp/lsp/terminal เองอยู่แล้ว จึงพังเฉพาะตอนถูก force-kill ไม่ใช่ตอนปิดปกติ
4. **`WebviewUserDataPath` มีเฉพาะ `options.Windows`** — `options.Linux` และ `options.Mac` ไม่มีฟิลด์นี้ กฎ §14 (webview profile อยู่ใต้ `DataRoot` เสมอ) จึงบังคับกับ webview ของตัวแอปไม่ได้นอก Windows แต่**แท็บ browser ยังบังคับได้ทุก OS** เพราะเป็นโค้ดที่เราคุมเอง (`WebKitWebsiteDataManager` / `WKWebsiteDataStore` รับ path ตรงๆ)
5. **Wails v2.13 ไม่เปิด native window handle** — ไม่มี `NativeWindowHandle`/`GetNativeHandle` ในโมดูลเลย ทำให้กฎข้อ 1 ของ blueprint ("ห้ามหาหน้าต่างตัวเองจาก global") ทำตามไม่ได้ ถ้าไม่ patch Wails เพิ่มใน `third_party/` (ดู §48 ท้ายสุด)

## การจัดจำหน่าย — ตัวเลขที่ต้องปกป้อง

[BENCHMARK.md](BENCHMARK.md) §4 ขายว่า Aetox **33 MB** เล็กกว่าคู่แข่ง 4–35 เท่า ด้วยเหตุผลว่า *ใช้ webview ที่ OS มีอยู่แล้ว ไม่ได้แบก Chromium มาเอง*

เหตุผลนั้นยังจริงบน macOS เสมอ แต่**บน Linux ขึ้นกับวิธีแพ็ก**:

| แบบ | ขนาด | สรุป |
|:---|:---|:---|
| `.deb` / `.rpm` / tar.gz ประกาศ `Depends: libwebkit2gtk-4.1-0` | ~35 MB | ✅ **เลือกอันนี้** |
| AppImage บรรจุ WebKitGTK มาด้วย | ~150 MB+ | ❌ ลบจุดขายหลักทิ้ง |
| Flatpak | ~40 MB | ❌ sandbox ตีกับตัวตนของสินค้า (รัน shell/git/MCP ทั้งเครื่อง) |

macOS มีค่าใช้จ่ายที่ไม่ใช่โค้ด: แอปไม่เซ็นชื่อโดน Gatekeeper บล็อก ถ้าจะให้เนียนต้องมี Apple Developer Program **$99/ปี** — ตัดสินใจได้ตั้งแต่ตอนนี้ ไม่ต้องรอเฟส 4

`bench.ps1` เป็น PowerShell ล้วน → เฟส 4 ต้องมี `bench.sh` ไม่งั้นตัวเลข Linux/mac ขึ้น README ไม่ได้ตามกติกาข้อเดียวของ BENCHMARK.md
