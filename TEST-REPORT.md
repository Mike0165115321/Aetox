# Test Report — by Module

> Date: 2026-07-21
> Method: `go test ./<package>/... -v` per package, counting top-level `--- PASS`/`--- FAIL` lines (subtests roll up into their parent test's count). `go build ./...` and `go vet ./...` also clean across the whole repo.
> Grouping: the 5 modules match the split discussed for [ARCHITECTURE.md §10](ARCHITECTURE.md#10-decision--agent-orchestrator-layer-proposed-approved-2026-07-21) (model management / model-control layer / orchestrator / UI-CLI / desktop app). A 6th "shared/cross-cutting" bucket is added for packages that don't cleanly belong to any one of the 5 — noted explicitly rather than forced into a module they don't fit.

**Total: 207 passing tests, 0 failing, across 16 tested packages** (updated after Module 5 pass — see below).

---

## Module 1 — การจัดการโมเดล (Model Management)

| Package | Test files | Tests | Result |
|---|---|---|---|
| `internal/model` | 5 | 34 | ✅ PASS |
| `internal/provider` | 1 | 16 | ✅ PASS |

**สถานะ: ไม่มีปัญหา.** ครอบคลุม provider factory, bootstrap, thinking-level normalization, และ catalog ทั้งสองตัว (ดู [ARCHITECTURE.md §6.3](ARCHITECTURE.md) — สอง catalog นี้ยังไม่ได้ diff ว่าซ้ำกันแค่ไหน แต่ทั้งคู่มีเทสของตัวเองแยกกัน ไม่ใช่ปัญหาเรื่อง test coverage)

---

## Module 2 — ระบบควบคุมโมเดล (Skill/Tool + Agent Loop)

| Package | Test files | Tests | Result |
|---|---|---|---|
| `internal/skill` | 17 | 62 | ✅ PASS |
| `internal/cognitive` | 1 | 3 | ✅ PASS |
| `internal/turn` | 1 | 23 | ✅ PASS |
| `internal/command` | 1 | 4 | ✅ PASS |

**สถานะ: ไม่มีปัญหา.** `internal/skill` เพิ่งขยายจาก 2 → 17 ไฟล์เทสในรอบนี้ (ครบทั้ง 12 built-in skills + Registry + Dispatcher + shared helpers) `internal/turn`/`cognitive` คุม tool-call loop หลักไว้แน่นอยู่แล้วตั้งแต่ก่อนหน้า

---

## Module 3 — Orchestrator / พื้นที่ Multi-Agent (Proposed layer)

| Package | Test files | Tests | Result |
|---|---|---|---|
| `internal/orchestrator` | 1 | 1 | ✅ PASS |

**สถานะ: ผ่าน แต่ขอบเขตแคบตามที่ตั้งใจ.** เทสคุมแค่ `Spawn`/`Get`/`Stop`/`List` ของ package ที่เพิ่งสร้างในเซสชันนี้ — ไม่มี integration test เพราะยังไม่มี front end ไหนเรียกใช้จริง (ดู [ARCHITECTURE.md §10](ARCHITECTURE.md)) ไม่ใช่ gap ที่ต้องปิดตอนนี้ เพราะ package เองก็ยังไม่ถูก wire เข้าใช้งาน

---

## Module 4 — UI / CLI Front End

| Package | Test files | Tests | Result |
|---|---|---|---|
| `cmd/aetox` | 1 | 8 | ✅ PASS |
| `internal/app` | 1 | 2 | ✅ PASS |

**สถานะ: ผ่าน แต่ `internal/app` บางเมื่อเทียบกับขนาดไฟล์.** package นี้มี 4 source files (844 บรรทัดใน `app.go` คนเดียว) รวม banner, status bar, interactive loop, approval-mode picker แต่มีแค่ 2 เทส — ตรงกับ finding เดิมที่ [ARCHITECTURE.md §6.1](ARCHITECTURE.md#61-internalapp-mixes-orchestration-with-cli-terminal-presentation) ชี้ไว้ว่า package นี้ทำหลายหน้าที่ปนกัน ยังไม่ได้แก้ในรอบเทสนี้ (ไม่อยู่ในสโคปของ "internal/skill ก่อน" ที่ตกลงกันไว้)

---

## Module 5 — Desktop App (Extension / Browser / Terminal / Display)

| ไฟล์ | Test files | Tests | Result |
|---|---|---|---|
| `sessions.go` (pure helpers only) | 1 | 5 | ✅ PASS |
| `app.go` (pure/file/git parts only) | 1 | 11 | ✅ PASS |
| `terminal.go` (pure + real-conpty parts) | 1 | 6 | ✅ PASS |
| `db.go` | 0 | 0 | ⚠️ ยังทดสอบไม่ได้ — ดูเหตุผลด้านล่าง |
| `browser.go` | 0 | 0 | ❌ ทดสอบอัตโนมัติไม่ได้ (ดูเหตุผลด้านล่าง) |
| `workbench.go` | 0 | 0 | ผูกกับ `browser.go` — เหตุผลเดียวกัน |
| `main.go` | 0 | 0 | ไม่มี logic ให้ทดสอบ |

**สถานะ: ปิดไปได้ 3 จาก 7 ไฟล์ (22 เทส, ผ่านหมด).** รายละเอียด:

- **`sessions.go`** — เทส pure function ทั้ง 4 ตัว (`projectKey`, `newSessionID`, `sessionTitleFrom` รวม edge case ภาษาไทยที่ตัดด้วย rune ไม่ใช่ byte, `transcriptToModelMessages`)
- **`app.go`** — เทส `safeSandboxPath` (path traversal), `ReadFile`/`WriteFile` round-trip, `CommandHistory` (ลำดับ + cap 50 + กรอง event "result" ทิ้ง), `GitChangedFiles` (นอก repo / ตรวจ untracked file จริงผ่าน git repo ชั่วคราว), `ProjectTree` (แสดงไฟล์จริง + ข้าม `node_modules` ตาม `treeIgnore`)
- **`terminal.go`** — เทส `nextTerminalID` (unique), `TerminalShells` (path ที่คืนมาต้อง resolve ได้จริงทุกตัว), `TerminalWrite`/`TerminalResize` ผ่าน conpty จริงที่ inject เข้า `a.terminals` ตรงๆ (ไม่เรียกผ่าน `TerminalStart`)

**ทำไม `TerminalStart`/`TerminalClose`/`browser.go` ทั้งไฟล์ถึงทดสอบอัตโนมัติไม่ได้ (หลักฐานจริง ไม่ใช่เดา):**
อ่านซอร์สของ `wailsapp/wails/v2/pkg/runtime/runtime.go` แล้วพบว่า `wailsruntime.EventsEmit(ctx, ...)` เรียก `getEvents(ctx)` ซึ่งถ้า `ctx` ไม่ใช่ context จริงที่ Wails ผูกไว้ตอน runtime เริ่มทำงาน (แค่ `context.Background()` เฉยๆ ไม่พอ) จะเรียก **`log.Fatalf` = `os.Exit(1)` ทันที** ไม่ใช่ error ที่ recover ได้ `TerminalStart` spawn goroutine (`pumpTerminalOutput`) ที่เรียก `EventsEmit` ทุกครั้งที่ shell มี output — เท่ากับว่าถ้าเทสเรียก `TerminalStart` จริง โปรเซส `go test` จะถูกฆ่าทันทีที่ shell พิมพ์อะไรออกมา (เกือบจะทันทีเสมอ) `TerminalClose`/`closeSession` ก็เรียก `EventsEmit` ตรงๆ เช่นกัน `browser.go` ทั้งไฟล์อาศัย Win32 window/message loop จริงที่ไม่มีในสภาพแวดล้อมเทส เหตุผลเดียวกันโดยหลักการ (ต้องมี runtime context จริง)

**`db.go` — ทำไมยังไม่ทำ, ต้องตัดสินใจก่อน:**
`a.database()` เปิด SQLite ที่ path จริงของเครื่อง (`os.UserConfigDir()/aetox/aetox.db`) ตรงๆ ไม่มีช่องให้ override เป็น temp path ตอนเทส เทสอะไรที่เรียกผ่าน path นี้ (`appendTurn`, `ListSessions`, `SearchSessions`, `LoadSession`) จะเขียน/อ่านไฟล์ session จริงของผู้ใช้บนเครื่อง ไม่ได้ทำต่อในรอบนี้เพราะเป็นการเปลี่ยน production code (เพิ่มช่อง override DSN) ไม่ใช่แค่เพิ่มเทส — ต้องถามก่อนว่าจะแก้ `db.go` เพื่อเปิดช่องเทสไหม

---

## Module 6 — Shared / Cross-Cutting (ไม่เข้าโมดูลไหนใน 5 ข้อบน)

| Package | Test files | Tests | Result |
|---|---|---|---|
| `internal/safety` | 1 | 3 | ✅ PASS |
| `internal/config` | 1 | 3 | ✅ PASS |
| `internal/audit` | 1 | 7 | ✅ PASS |
| `internal/think` | 1 | 2 | ✅ PASS |
| `internal/plan` | 1 | 2 | ✅ PASS |
| `internal/grammar` | 1 | 15 | ✅ PASS |
| `internal/memory` | 0 | 0 | ⚠️ ไม่มีเทส |
| `internal/debuglog` | 0 | 0 | ⚠️ ไม่มีเทส |

**สถานะ: ส่วนใหญ่ไม่มีปัญหา.** `memory`/`debuglog` ไม่มีเทส แต่เป็นโมดูลความเสี่ยงต่ำ (context struct ธรรมดา / logging เฉยๆ) — ระบุไว้เป็น known gap ไม่ใช่ priority

---

## สรุปลำดับความสำคัญ (ถ้าจะปิดช่องว่างต่อ)

1. **ตัดสินใจเรื่อง `db.go` (Module 5)** — ต้องเลือกก่อนว่าจะเพิ่มช่อง override DSN ใน production code เพื่อให้เทส `appendTurn`/`ListSessions`/`SearchSessions`/`LoadSession` ได้แบบไม่แตะไฟล์จริงของผู้ใช้ ไหม
2. **`internal/app` (Module 4)** — เพิ่มเทสสำหรับ approval-mode picker และ command routing ก่อนที่จะแยก package ตาม §6.1
3. `internal/memory`/`internal/debuglog` (Module 6) — ต่ำสุด ทำเมื่อมีเวลาเหลือ
4. `browser.go`/`workbench.go`/`TerminalStart`/`TerminalClose` (Module 5) — ไม่ใช่ priority เพราะทดสอบอัตโนมัติไม่ได้ตามโครงสร้างปัจจุบัน (ต้องมี Wails runtime context จริง) ถ้าจะปิดช่องนี้จริงต้องเปลี่ยนสถาปัตยกรรม ไม่ใช่แค่เพิ่มเทส
