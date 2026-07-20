# MCP + External Skills — Readiness Notes

บันทึกจากการสำรวจโค้ดจริง (2026-07-21) ก่อนเริ่ม implement MCP support ภายนอก

## สถานะปัจจุบัน

**พร้อม:** [internal/skill/skill.go](internal/skill/skill.go) มี `Tool` interface
(`ToolDefinition()` คืน JSON schema + `ExecuteTool(ctx, args)`) ที่รูปร่างตรงกับ
MCP tool อยู่แล้ว. [internal/skill/dispatcher.go](internal/skill/dispatcher.go)
ดึง `ToolDefinition()` จากทุก skill ใน registry ไปให้ model เรียกผ่าน tool-calling
loop โดยไม่สนใจว่า skill นั้น implement เองหรือห่อ remote tool มา — **ไม่ต้องแก้
dispatcher/registry/tool-loop เลย** ถ้าจะเพิ่ม MCP.

**ยังไม่มีเลย:** ไม่มี MCP client ในโปรเจกต์ (ไม่มี stdio/SSE JSON-RPC transport,
ไม่มี process lifecycle ของ MCP server) — ต้องสร้างใหม่ทั้งหมด.

**Half-finished (พบระหว่างสำรวจ):** `plugin_install` skill
([internal/skill/github_tools.go:224](internal/skill/github_tools.go#L224))
ดาวน์โหลด `aetox-plugin.json` manifest + ไฟล์จาก GitHub repo แล้วเขียนลง
`~/.agents/skills/<name>/` ได้จริง — แต่ไม่มีจุดไหนใน codebase โหลดไฟล์พวกนั้น
กลับเข้า registry ตอน bootstrap เลย ([internal/skill/defaults.go](internal/skill/defaults.go)
ลงทะเบียนแต่ skill ที่ compile เข้ามาตรง ๆ เท่านั้น). เท่ากับตอนนี้
"ติดตั้ง skill ภายนอก" ผ่าน tool นี้ไม่มีผลอะไรหลังดาวน์โหลดเสร็จ.

## แผนที่แนะนำ — MCP

1. สร้าง `internal/mcp` package: stdio JSON-RPC client (`initialize` →
   `tools/list` → `tools/call`), spawn/monitor MCP server เป็น subprocess.
2. เขียน adapter struct ที่ implement `skill.Tool` ต่อ remote tool หนึ่งตัว
   (ดึง schema จาก `tools/list`, ส่งต่อ `ExecuteTool` เป็น `tools/call` JSON-RPC).
3. Config: รายชื่อ MCP server ที่จะ spawn (command + args) — ใส่ใน
   `internal/config` เหมือน provider preference ที่มีอยู่แล้ว.
4. Register adapter เข้า `Registry` ตอน `bootstrapFromConfig`
   ([desktop/app.go](desktop/app.go)) เหมือนที่ `workbenchTools` (browser_open/
   browser_read) ทำอยู่ตอนนี้ — pattern เดียวกันเป๊ะ ใช้ซ้ำได้เลย.

## ช่องว่างสถาปัตยกรรม: ไม่มีเส้นแบ่ง core vs user-added

`RegisterDefaults()` ([internal/skill/defaults.go](internal/skill/defaults.go))
ยัด built-in ทั้งหมด (`read`, `write`, `shell`, `git`, ...) กับของที่ควรเป็น
"เพิ่มทีหลังได้" (`plugin_install`, MCP adapter ในอนาคต) ลง `Registry` เดียวกัน
แบบแบนราบ — ไม่มี field หรือ namespace บอกว่า skill ไหนมาจากไหน. ผลคือ:

- Gate สิทธิ์ต่างกันไม่ได้ (built-in เชื่อใจได้ vs MCP/plugin จาก third-party ควรเข้มกว่า)
- แสดงใน UI แยกกลุ่มไม่ได้ (เช่น Settings.svelte ที่จะโชว์ "core tools" vs "installed skills")
- ชื่อชนกันได้เงียบ ๆ — `Registry.Register()` เขียนทับ key เดิมโดยไม่เตือน ถ้า user
  ติดตั้ง skill ชื่อซ้ำกับ built-in

ก่อนต่อ MCP/plugin_install ให้ใช้งานจริง ควรแยก `Registry` เป็นสอง scope (core /
user-added) หรืออย่างน้อยเพิ่ม field `Source string` ("builtin" | "mcp" |
"plugin") ใน `Skill`/`Tool` metadata ก่อน.

## ช่องว่างที่ต้องปิดก่อน production-ready

- **Safety tier**: 3 ระดับปัจจุบัน (ask / unsafe-only / full-access) ออกแบบมา
  สำหรับ 17 built-in tools ที่เขียนเอง เชื่อใจได้ — MCP server จาก third-party
  รันโค้ดที่เราไม่ควบคุม ต้องคิดว่าจะ gate ยังไง (อย่างน้อย MCP tool ทั้งหมดควร
  เข้าเงื่อนไข "unsafe" เป็น default ไม่ auto-run ใน full-access ทันที).
- **plugin_install loader**: ถ้าจะให้ทางนี้ใช้งานได้จริงด้วย ต้องเขียน loader
  ที่ scan `~/.agents/skills/` ตอน bootstrap แล้ว register กลับเข้า registry —
  แต่ skill ที่ดาวน์โหลดมาเป็น "ไฟล์" (ไม่ใช่ compiled Go) จะ execute ยังไงต้อง
  ตัดสินใจก่อน (interpreter เช่น script, หรือจำกัดเฉพาะ prompt/markdown skill
  ที่ไม่ต้องรันโค้ด).

## ไฟล์ที่เกี่ยวข้อง

- [internal/skill/skill.go](internal/skill/skill.go) — `Skill`/`Tool` interface, `Registry`
- [internal/skill/dispatcher.go](internal/skill/dispatcher.go) — tool-loop wiring
- [internal/skill/defaults.go](internal/skill/defaults.go) — built-in registration
- [internal/skill/github_tools.go](internal/skill/github_tools.go) — `plugin_install` (half-finished)
- [desktop/app.go](desktop/app.go) `bootstrapFromConfig` — ตัวอย่าง pattern ต่อ extra skills (`workbenchTools`)
- [desktop/workbench.go](desktop/workbench.go) — ตัวอย่าง skill ที่เรียก external process/UI จริง (ใกล้เคียงที่สุดกับ MCP adapter)
