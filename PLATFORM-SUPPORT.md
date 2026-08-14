# Platform Support — สถานะพอร์ต Linux/macOS · อัปเดต 2026-07-27

> **จุดยืน (owner, 2026-07-27):** กลับข้าง — จาก "บันทึกไว้เฉยๆ" เป็น **"ทำจริง เดสก์ท็อปก่อน"** เป้าหมาย v0.7.0
> Decision section: [ARCHITECTURE.md §48](ARCHITECTURE.md) · จุดยืนเดิม "Windows first, ไม่ไล่ตาม" อยู่ที่ §29 ถูก §48 แทนแล้ว
>
> **เอกสารนี้คือจุดตั้งต้นสำหรับคนที่มาต่องานนี้** อ่านจบแล้วรู้ว่าอยู่ตรงไหน ทำอะไรต่อ และห้ามทำอะไร

## สรุป

- **เฟส 0–2 เสร็จ** — `desktop/` **คอมไพล์ได้ครบทั้งสาม OS** และรันสวีททั้งชุดบนทั้งสาม
  ที่เหลือคือแท็บ browser
  > แก้ถ้อยคำ 15 ส.ค. 2026: บรรทัดนี้เคยเขียนว่า "เทสต์ผ่านครบทั้งสาม OS" ซึ่งขัดกับ
  > ย่อหน้าถัดๆ ไปในหน้าเดียวกันที่ระบุเทสต์แดงค้างอยู่ **คอมไพล์ได้** กับ **ผ่าน** ไม่ใช่
  > เรื่องเดียวกัน และหน้านี้ยืนยันอย่างแรกได้ ไม่ใช่อย่างหลัง
- ยกเว้นแท็บ browser ทุกอย่างพร้อมใช้บน Linux: chat, sessions, MCP, skills, files, tools, review, **เทอร์มินัล**
- **macOS รันจริงแล้วใน CI (อัปเดต 7 ส.ค. 2026)** — job `unix (macos-latest)` รันสวีททั้งชุด
  ไม่ใช่แค่ type-check อีกต่อไป และ**มันเจอของจริงสองอย่างที่ Windows กับ Linux ไม่เจอ**:
  `TestCloseKillsBackgroundedGrandchild` (หมดเวลาที่ 5 วิ — วิธีฆ่าลูกหลานของโพรเซสบน
  macOS ไม่เหมือนกัน) และ `TestImageOCRSkillMissingBinaryGivesActionableError` (ใช้เวลา
  13–20 วิ) ทั้งคู่แดงต่อเนื่องมาตั้งแต่อย่างน้อย 4 ส.ค. **ยังไม่มีใครแก้ เพราะวินิจฉัยจาก
  เครื่อง Windows ได้แค่เดา** — ต้องมีคนที่มี Mac จริงมารับ นี่คือรายการแรกของงานพอร์ต macOS
  ไม่ใช่ 3b

## ตารางสถานะ

| ส่วน | Windows | Linux | macOS |
|:---|:---|:---|:---|
| `internal/` (engine, tools, providers) | ✅ ใช้จริง | ✅ **เทสต์ + race ผ่านบนเคอร์เนลจริง** | ⚠️ type-check ผ่าน ยังไม่เคยรัน |
| `cmd/aetox` (CLI) | ✅ ใช้จริง | ✅ **เทสต์ + race ผ่าน** | ⚠️ type-check ผ่าน ยังไม่เคยรัน |
| `desktop/` ทั้งแพ็กเกจ | ✅ ใช้จริง | ✅ **`go test ./...` + `-race` ผ่าน** | ⚠️ type-check ผ่าน ยังไม่เคยรัน |
| `desktop/` แท็บ browser | ✅ ใช้จริง | ⏳ stub (เฟส 3a) | ⏳ stub (เฟส 3b) |
| binary ภายนอก (tesseract, ffmpeg) | ✅ | ✅ มีข้อความติดตั้งแยก OS อยู่แล้ว | ✅ auto-install ผ่าน brew อยู่แล้ว |

> **ระวังการอ่านตาราง (แก้ 7 ส.ค. 2026):** ✅ ของ Linux = *รันจริงบนเคอร์เนล Linux แล้วผ่าน* ·
> macOS **รันจริงแล้วบน `macos-latest` ใน CI แต่ยังไม่ผ่านครบ** — เทสต์แดงค้างสองตัว (ดูสรุปข้างบน)
> ช่อง ⚠️ ในตารางล่างจึงหมายถึง "รันแล้ว ยังไม่เขียว" ไม่ใช่ "ยังไม่เคยรัน" อีกต่อไป ·
> เครื่องเจ้าของยังไม่มี Mac และรัน macOS ใน VM บนเครื่องที่ไม่ใช่ Apple ผิดสัญญาอนุญาต
> จึงแก้ได้เฉพาะผ่าน CI หรือผ่านคนที่มีเครื่องจริง

## เฟส

| # | งาน | สถานะ |
|:--|:---|:---|
| **0** | CI matrix — job `unix` (ubuntu + macos), Linux เพิ่ม `-race` | ✅ **เสร็จ** |
| **1** | `terminal.go` → `ptySession` + `terminal_windows.go` / `terminal_unix.go` (`creack/pty` v1.1.24) | ✅ **เสร็จ** |
| **2** | `browser.go` → `hostBackend`/`tabView` + `browser_windows.go` + `browser_other.go` (stub) | ✅ **เสร็จ** |
| 3a | `browser_linux.go` — WebKitGTK widget ใน `GtkOverlay`/`GtkFixed` | ยังไม่เริ่ม |
| 3b | `browser_darwin.go` — `WKWebView` เป็น subview ของ Wails `NSView` | ยังไม่เริ่ม |
| 4 | packaging (`.deb`/`.rpm`/tar.gz + `.app`/`.dmg`) + `bench.sh` | ยังไม่เริ่ม |

## แผนที่ไฟล์ — ไฟล์ไหนของ OS ไหน

กฎ: **แยกไฟล์ก็ต่อเมื่อ type หรือ import ต่างกัน** ถ้าต่างแค่ชื่อคำสั่งที่จะรัน ใช้ `switch runtime.GOOS` ในไฟล์เดิม (เช่น `presets.go`/`subagents.go` ที่เลือก `explorer`/`open`/`xdg-open`) และ**ไม่แยกโฟลเดอร์** เพราะ Go นับ 1 โฟลเดอร์ = 1 package ส่วน `App` ต้องอยู่ใน `package main` เดียว ไม่งั้น Wails generate `App.d.ts` ออกมาคนละชุด แล้ว `BrowserPane.svelte` พังตอน `vite build` (§48 Decision 2)

| ไฟล์ | build tag | Win | Linux | mac | มีอะไร |
|:---|:---|:-:|:-:|:-:|:---|
| `terminal.go` | — | ✅ | ✅ | ✅ | `ptySession`, `App.Terminal*`, ลูปอ่าน |
| `terminal_windows.go` | ชื่อไฟล์ | ✅ | | | ConPTY + รายชื่อเชลล์ของ Windows |
| `terminal_unix.go` | `//go:build unix` | | ✅ | ✅ | `creack/pty` + `$SHELL` + กวาด session |
| `browser.go` | — | ✅ | ✅ | ✅ | `tabView`/`hostBackend`/`tabCallbacks`, JS builders, `sameOrigin`, `onMessage`, `navCompleted`, `App.Browser*` |
| `browser_windows.go` | ชื่อไฟล์ | ✅ | | | Win32 + WebView2 (`win32Host`/`win32Tab`) |
| `browser_other.go` | `//go:build !windows` | | ⏳ | ⏳ | stub ชั่วคราว — ลบทิ้งเมื่อ 3a และ 3b เสร็จทั้งคู่ |
| `browser_linux.go` | ชื่อไฟล์ | | ✅ | | WebKitGTK (เฟส 3a) |
| `browser_darwin.go` | ชื่อไฟล์ | | | ✅ | WKWebView (เฟส 3b) |

`_unix.go` ต้องเขียน `//go:build unix` เอง เพราะ Go ไม่ถือว่า `_unix` เป็นคำลงท้ายพิเศษเหมือน `_windows`/`_linux`/`_darwin` — ใช้ `unix` แทน `!windows` (แบบที่ `internal/proc` ใช้) เพราะตรงกับชื่อไฟล์และไม่ลากไป js/wasm กับ plan9 ที่ `creack/pty` ไม่รองรับ

## ทำเฟส 3a/3b ยังไง — ต้อง implement แค่ 2 interface

อยู่ใน `desktop/browser.go` ทั้งคู่ ที่เหลือทั้งไฟล์ใช้ร่วมกันหมด **ไม่ต้องแตะ**

```go
type hostBackend interface {
    start() error                  // ตั้ง UI thread / message pump ของแพลตฟอร์ม, idempotent
    do(fn func())                  // รัน fn บน thread ที่เป็นเจ้าของ webview
    openTab(id, url string, x, y, w, h int, cb tabCallbacks) tabView
}

type tabView interface {
    navigate(url string)
    eval(js string)
    setBounds(x, y, w, h int)
    setVisible(visible bool)       // true = แสดง + ยกขึ้นบนสุด
    setZoom(factor float64)
    openDevTools()
    destroy()
}
```

แล้วให้ `newHostBackend() hostBackend` ในไฟล์ของ OS นั้นคืนตัวจริง (ตัด GOOS นั้นออกจาก build tag ของ `browser_other.go` ด้วย)

**ห้ามพลาดสามข้อนี้:**

1. **`do()` ต้อง async เสมอ** — `g_idle_add` / `dispatch_async` **ห้าม `dispatch_sync`** เพราะ `browserSnapshot` เรียก `do()` แล้วบล็อกรอ channel 5 วินาที ถ้า sync และถูกเรียกจาก main thread = **deadlock ที่ path ซึ่ง AI ใช้อ่านหน้าเว็บ** unit test จับไม่ได้ (§48 Decision 3)
2. **`bridgePost` มีอยู่แล้วใน `browser_other.go`** = `window.webkit.messageHandlers.aetox.postMessage` ถูกต้องสำหรับ WebKit ทั้งสองตัว ชื่อ handler ต้องตรงกับที่ลงทะเบียนกับ `WebKitUserContentManager` / `WKUserContentController`
3. **ยึด 5 กฎท้าย [docs/architecture/native-browser-embedding-2026-07-24.md](docs/architecture/native-browser-embedding-2026-07-24.md)** — เขียนจากรอบดีบักจริงที่ได้ failure catalog 7 ข้อ ไม่ใช่ทฤษฎี โดยเฉพาะกฎข้อ 1 (ห้ามหาหน้าต่างตัวเองจาก global)

Linux ต้องใช้ `-tags webkit2_41` — Ubuntu 24.04 ไม่มี `libwebkit2gtk-4.0-dev` แล้ว (วัดแล้ว)

## วิธีตรวจซ้ำเอง

เครื่อง dev เป็น Windows และไม่มี C compiler → **พิสูจน์อะไรของ Linux/mac บนเครื่องตรงๆ ไม่ได้** ที่ใช้อยู่จริงคือ Docker (ต้องเปิด Docker Desktop ก่อน):

```bash
# ทั้งชุดบนเคอร์เนล Linux จริง — copy ก่อนเพื่อไม่ให้เขียนทับรีโป
docker run --rm -v "E:/Aetox/Aetox:/repo:ro" -v aetox-gomod:/go/pkg/mod golang:1.25 \
  sh -c 'cp -r /repo /tmp/a && cd /tmp/a && go test -count=1 ./...'

# race detector — เช็คที่ verify.sh skip มาตลอดเพราะเครื่อง dev ไม่มี C compiler
  ... && go test -count=1 -race -timeout 15m ./...'

# macOS ตรวจได้แค่ type-check เท่านั้น
  ... && GOOS=darwin GOARCH=arm64 go vet ./...'
```

จาก Git Bash ต้องใส่ `MSYS_NO_PATHCONV=1` นำหน้า ไม่งั้น path ฝั่งคอนเทนเนอร์โดนแปลงเป็น path Windows

## ผลวัดจริง (2026-07-27)

| เช็ค | ผล |
|:---|:---|
| Windows `go test ./...` | ✅ 24 แพ็กเกจ |
| **Linux `go test ./...`** | ✅ **24 แพ็กเกจ รวม `desktop`** |
| **Linux `go test -race ./...`** | ✅ **24 แพ็กเกจ ไม่มี data race รวม `desktop`** |
| `GOOS=darwin go vet ./...` | ✅ สะอาด |
| `TestShellSkillCancelKillsGrandchild` | ✅ `tree_other.go` (`Setpgid` + `kill(-pgid)`) พิสูจน์กับ process หลานจริง |
| `TestCloseKillsBackgroundedGrandchild` | ✅ การกวาด session ของเทอร์มินัลพิสูจน์กับ process ที่ถูก detach จริง |
| `libwebkit2gtk-4.0-dev` บน Ubuntu 24.04 | ❌ ไม่มีแล้ว เหลือ 4.1 |

## สิ่งที่พอร์ตนี้ขุดเจอ — บั๊กที่ Windows ปิดบังไว้

1. **`kill(-pgid)` ไม่พอสำหรับเชลล์ interactive** — เชลล์โยนงาน background ไป process group ใหม่เสมอ (bash และ dash, เปิด/ปิด job control) แต่ยังอยู่ session เดิม `npm run dev &` เลยรอดจากการปิดแท็บและยึดพอร์ตไว้ แก้ด้วยการกวาดทั้ง session **ตรวจแล้วว่าไม่ลามไป `proc.KillOnCancel`** เพราะ `sh -c` ไม่มี tty จึงไม่มี job control (§48 Decision 5)
2. **เทสต์ที่แยก config dir ด้วย `APPDATA`/`LOCALAPPDATA` ไม่ได้แยกอะไรเลยนอก Windows** — 9 เทสต์ไปอ่านเขียน `~/.config/aetox` ตัวจริง แก้ด้วย `isolateUserDirs(t)` ต่อแพ็กเกจ (§48 Decision 6)
3. **`TestProjectKeyStableAndDistinct` ผ่านมาตลอดด้วยเหตุผลที่ผิด** — เช็ค `filepath.Base` กับ `C:\projects\app` ที่ hardcode ซึ่งนอก Windows เป็นชื่อเดียวไม่มีตัวคั่น
4. **`win32Host.start()` ที่ล้มเหลวทำให้ทั้งระบบค้าง** — ทิ้ง `started=true` ไว้โดยไม่ปิด `ready` และ `browserHostLazy` เรียก `start()` ทุก binding แก้แล้ว

## สิ่งที่ยังไม่รู้ และรู้ได้ทางเดียว

1. **macOS ทุกอย่าง** — เฟส 3b จะ iterate ช้าที่สุด (แก้ 1 บรรทัด = รอ CI 1 รอบ ไม่เห็นหน้าจอ) นี่คือเหตุผลที่ทำ Linux ให้จบก่อน
2. **z-order / compositing บน WM จริง** — Docker/WSLg/xvfb บอกได้แค่ "รันได้ วาดออก" บอกไม่ได้ว่าบน GNOME/KDE/tiling WM หน้าต่างลูกอยู่ถูกชั้น ซึ่งเป็นหัวใจทั้งหมดของงาน **ต้องมี Linux desktop จริงหรือ VM**
3. **`proc.KillTreeOnExit` เป็น no-op นอก Windows** — ไม่มี Job Object `shutdown()` เก็บ mcp/lsp/terminal เองอยู่แล้ว จึงพังเฉพาะตอนถูก force-kill
4. **`WebviewUserDataPath` มีเฉพาะ `options.Windows`** — กฎ §14 บังคับกับ webview ของตัวแอปไม่ได้นอก Windows แต่**แท็บ browser ยังบังคับได้ทุก OS** เพราะเป็นโค้ดที่เราคุมเอง (`WebKitWebsiteDataManager`/`WKWebsiteDataStore` รับ path ตรงๆ)
5. **Wails v2.13 ไม่เปิด native window handle** — ไม่มี `NativeWindowHandle`/`GetNativeHandle` ในโมดูลเลย กฎข้อ 1 ของ blueprint จึงทำตามไม่ได้ถ้าไม่ patch Wails เป็น fork ตัวที่ 3 ใน `third_party/` (§48 ท้ายสุด)

## การจัดจำหน่าย — ตัวเลขที่ต้องปกป้อง

[BENCHMARK.md](BENCHMARK.md) §4 ขายว่า Aetox **33 MB** เล็กกว่าคู่แข่ง 4–35 เท่า ด้วยเหตุผลว่า *ใช้ webview ที่ OS มีอยู่แล้ว ไม่ได้แบก Chromium มาเอง* — เหตุผลนั้นยังจริงบน macOS เสมอ แต่**บน Linux ขึ้นกับวิธีแพ็ก**:

| แบบ | ขนาด | สรุป |
|:---|:---|:---|
| `.deb`/`.rpm`/tar.gz ประกาศ `Depends: libwebkit2gtk-4.1-0` | ~35 MB | ✅ **เลือกอันนี้** |
| AppImage บรรจุ WebKitGTK มาด้วย | ~150 MB+ | ❌ ลบจุดขายหลักทิ้ง |
| Flatpak | ~40 MB | ❌ sandbox ตีกับตัวตนของสินค้า (รัน shell/git/MCP ทั้งเครื่อง) |

macOS มีค่าใช้จ่ายที่ไม่ใช่โค้ด: แอปไม่เซ็นชื่อโดน Gatekeeper บล็อก ถ้าจะให้เนียนต้องมี Apple Developer Program **$99/ปี** — ตัดสินใจได้ตั้งแต่ตอนนี้

`bench.ps1` เป็น PowerShell ล้วน → เฟส 4 ต้องมี `bench.sh` ไม่งั้นตัวเลข Linux/mac ขึ้น README ไม่ได้ตามกติกาข้อเดียวของ BENCHMARK.md
