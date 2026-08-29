# Test Report — by Module

> **What this document is for, and what it is not.**
>
> **Not the test count.** The count is whatever `go test ./...` says right now, and any number
> written down here is stale the moment somebody adds a test. This file said "207 passing
> across 16 packages" from 2026-07-21 until 2026-08-07, by which point the truth was
> **1,438 across 34** — seven times off, and wrong in the direction that undersells the
> project. Run the command; do not read a number here.
>
> ```powershell
> go test ./...                        # the whole suite
> cd desktop/frontend; npx vitest run  # the UI suite
> golangci-lint run ./...              # the linters that gate — see below
> ```
>
> Or `bash ./verify.sh`, which runs all three plus vet, build, the race detector and
> `svelte-check` in one pass and prints one summary line.
>
> **What it *is* for:** the seams that cannot be tested and why, and the conventions new
> tests follow. That part does not go stale on its own and is not written down anywhere
> else — it is why the per-module breakdown below is still worth reading even where its
> counts have moved on.

> Per-module figures below: 2026-07-21
> Method: `go test ./<package>/... -v` per package, counting top-level `--- PASS`/`--- FAIL` lines (subtests roll up into their parent test's count). `go build ./...` and `go vet ./...` also clean across the whole repo.
> Grouping: the 5 modules match the split discussed for [ARCHITECTURE.md §10](ARCHITECTURE.md#10-decision--agent-orchestrator-layer-proposed-approved-2026-07-21) (model management / model-control layer / orchestrator / UI-CLI / desktop app). A 6th "shared/cross-cutting" bucket is added for packages that don't cleanly belong to any one of the 5 — noted explicitly rather than forced into a module they don't fit.

## Convention — whole-path tests run on Aetox's own model (2026-07-26, ARCHITECTURE.md §45)

A test that exercises a **whole path** — a turn, a tool loop, a delegation to a
sub-agent — uses the built-in provider, not a fake written for the test:

```go
provider := model.NewNoopProvider("aetox-tools:test") // real Provider, no key, no network
agent := cognitive.NewAgent(cognitive.AgentConfig{Provider: provider, Model: "aetox-tools:test", ...})
exec := turn.NewExecutor(turn.ExecutorOptions{Agent: agent, Dispatcher: skill.NewDispatcher(registry), ...})
```

It goes down the same channel a real provider does, costs nothing, needs no key,
and is deterministic (its scripts read the next round off the transcript). A
per-test fake is a second implementation of the thing under test and can pass
while the real path is broken. Reference: `internal/subagent/spawn_demo_test.go` (since removed).

Hand-written stubs stay right for **provider edge cases** only — a truncated tool
call, leaked DSML, a 401 — where the point is to produce one exact wire condition.

---

## Checks that are not tests — vet, lint, race (เขียน 2026-08-29 · decision 2026-08-18, [docs/DECISIONS.md §141](docs/DECISIONS.md))

เทสไม่ใช่ทุกอย่างที่ต้องเขียวก่อน merge สามอย่างข้างล่างนี้รันคู่กับ `go test` เสมอ
และแต่ละอันตอบคนละคำถาม — อันไหน **gate** (แดงแล้วหยุด) กับอันไหน **report**
(อ่านทุก push แต่ไม่กั้น) เขียนไว้ตรงนี้เพราะเคยมี stage ที่ทุกคนเชื่อว่ารันอยู่แล้วไม่ได้รัน

| Check | รันที่ไหน | Gate? |
|---|---|---|
| `go vet ./...` | `verify.sh` stage `vet` · ทั้งสาม job ใน [ci.yml](.github/workflows/ci.yml) | ✅ gate |
| `golangci-lint run ./...` | `verify.sh` stage `lint` · job `windows` (gate) และ job `unix` (report ตาม `continue-on-error` ของ job) | ✅ gate บน Windows |
| `golangci-lint ... --enable=gosec` | `verify.sh` (พิมพ์บรรทัดสรุป) · ทั้ง `windows` และ `unix` | ❌ report เท่านั้น |
| `go test -race` | `verify.sh` stage `race` (ข้ามถ้าไม่มี C compiler) · job `ubuntu-latest` | ✅ ใน verify.sh · ❌ ใน CI (job `unix` เป็น report) |

**ลินเตอร์ตัวไหนเปิด และตัวไหนยังไม่เปิดเพราะอะไร อยู่ที่ [DECISIONS.md §141](docs/DECISIONS.md) ที่เดียว**
— ตัวไฟล์คอนฟิกคือ [`.golangci.yml`](.golangci.yml) ไม่ต้องคัดรายชื่อมาไว้ที่นี่อีกที่หนึ่ง
(ที่สองที่ตอบคำถามเดียวกันคือหนี้) สั้นๆ คือ: gate เฉพาะชุดที่ **นับได้ศูนย์ในวันที่เปิด**
เพราะ gate ที่แดงตั้งแต่วันแรกไม่ใช่ gate ส่วน `gosec` report อย่างเดียว เพราะโปรแกรมนี้
อ่านไฟล์จากตัวแปรและรันคำสั่งที่ประกอบตอนรันเป็นงานปกติของมัน (G304/G204)

**ถ้า `golangci-lint` ไม่ได้อยู่บนเครื่อง** `verify.sh` จะ skip แบบ **ดังๆ** พร้อมคำสั่งติดตั้ง —
เจตนาเดียวกับ stage `race`: check ที่เชื่อว่ารันอยู่แต่ไม่ได้รัน แย่กว่า check ที่ไม่เคยมี

```powershell
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

การเพิ่ม/ปิดลินเตอร์ตัวใหม่แก้ที่ `.golangci.yml` แล้วบันทึกเหตุผลใน §141 ไม่ใช่โรย
`//nolint` ไว้ตามไซต์ — ยี่สิบ annotation คือยี่สิบที่ที่ตอบคำถามซึ่งคอนฟิกถูกสร้างมาตอบ

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

---

## Addendum 2026-07-25 — Engine parity batch + interactive tools (§27)

New coverage added with the §27 work (all green, `go test ./...` + vitest 16/16):

| Area | Tests | Pins |
|---|---|---|
| `internal/cognitive` | `TestRespondEphemeralDoesNotTouchContext` · `TestToolLoopFirstCallFailureDoesNotDuplicateUserMessage` · `TestToolLoopCompactsMidLoop` · `TestRespondUsesPerProviderOutputCeiling` | summary prompts stay out of history; no duplicate user msg on first-call failure; compaction fires mid-loop; the flat-768 cap stays dead |
| `internal/turn` | `TestExecuteTool_AbnormallySlowToolReportsBackToModel` · `TestExecuteTool_InteractiveToolExemptFromTimeout` | 60s slow-tool receipt; ask_user never abandoned |
| `internal/model` | `TestNewProviderDeepSeekEmptyBaseURLStaysOnDeepSeek` · `TestNewProviderDeepSeekAltFormatReplacesDefaultURL` · `TestNewProviderDeepSeekDefaultFormatReplacesStaleAltURL` · `TestModelDiscovery*` (2) · `TestNoopToolsModelScriptsToolLoop` · `TestNoopThinkModelProducesLongSectionedReasoning` | the 401-to-real-Anthropic restart bug (both directions); alt-endpoint discovery routing; the scripted UI-test models |
| `desktop` | `TestAskUser*` (4) · `TestAnswerUserQuestionNoPendingIsNoop` · `TestTodoWrite*` (2) · `TestApplyConfigInheritsPriorAgentContext` | ask_user round-trip/cancel/single-flight; todo sanitize+counts; model switch keeps tool history |
| frontend (vitest) | `cockpitAskTodo.test.ts` (5) | ask/answer/clear store flow; todo replace-wholesale; junk payload safety |
