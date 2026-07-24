# Settings Parity Plan — ทำระบบ + หน้าตั้งค่าให้ "พร้อมจริง"

บันทึกแผน 2026-07-24 หลังเทียบ Settings sidebar ของ ZCode (General / Code
preview / Model settings / Skills / Subagents / MCP Servers / Plugins /
Commands / Indexing / Usage stats / Onboard) กับสิ่งที่ Aetox มีจริงในโค้ด —
ไม่ก็อป 1:1: เอาเฉพาะของที่มีประโยชน์กับบริบทเรา เรียงถูก→แพง ทุก phase จบ
แล้ว ship ได้ commit แยก ของที่แตะ architecture ต้องได้ Decision section ใน
[ARCHITECTURE.md](ARCHITECTURE.md) ก่อนลงมือ (ดู §24)

ผลสำรวจโค้ดที่แผนนี้อิง (2026-07-24):

| หัวข้อ ZCode | backend เรา | UI เรา | ข้อสรุป |
|---|---|---|---|
| MCP Servers | ✅ | ✅ | **เสร็จแล้ว** (commit `c805a9b`: toggle/edit/badge/env/preset/remote) |
| Skills | ✅ `internal/skill/discovery.go` | บางส่วน (ToolsPane, read-only) | Phase 1 — ถูกสุด |
| Plugins | ครึ่งเดียว (`plugin_install` เขียนไฟล์แล้วไม่มีใครโหลดกลับ) | ❌ | Phase 1 — รวมกับ Skills เป็นหน้าเดียว (กลไกเดียวกัน: โฟลเดอร์ SKILL.md) |
| Onboard | ❌ | ❌ | Phase 2 — bindings ที่ต้องใช้มีครบแล้ว (provider/key/approval) |
| Usage stats | มีข้อมูลแล้ว (`model.Usage` per response) แต่ไม่ได้เก็บ | ❌ | Phase 3 — ต่อท่อลง SQLite + หน้าโชว์ |
| Commands | มีแค่ slash hardcode ใน `internal/grammar` | ❌ | Phase 4 — custom `.md` commands ตาม pattern Claude Code |
| Code preview | ❌ (ค่า editor อยู่ใน Appearance แล้ว) | ❌ | Phase 5 — ไม่ทำเป็นหน้า Settings; ทำ markdown preview ใน workbench แทน |
| Subagents | scaffold รออยู่แล้ว (`internal/orchestrator`, §10) | ❌ | Phase 6 — ใหญ่สุด, design ก่อนเขียนโค้ด |
| Indexing | FTS5 ทำงานใต้น้ำ (`desktop/db.go`) | — | **ตัดทิ้ง** — ไม่มี knob ให้ user ตั้ง; ZCode มีเพราะเขา index ทั้ง repo แบบ RAG ซึ่งเราไม่ได้ทำ |

## สถานะรวม (อัปเดต 2026-07-24 คืนเดียวกัน)

- Phase 0 ✅ (`761e5e0`) · Phase 1 ✅ · Phase 2 ✅ · Phase 3 ✅ · Phase 4 ✅ ·
  Phase 5 ✅ — ทุก phase ทดสอบ (unit + สดบน exe จริงเป็นส่วนใหญ่) และ commit แยกแล้ว
- Phase 6: Decision §25 ร่างแล้ว (Proposed) — **รอ owner อนุมัติก่อนเขียนโค้ด**
- หมายเหตุ: CLI ยังไม่ได้ wire `command.ExpandCustom` (desktop-first);
  เพิ่มได้ที่จุดรับ input ของ `cmd/aetox` เมื่อต้องการ

## Phase 0 — เก็บงานค้าง + hardening (✅ เสร็จ, `761e5e0`)

- [x] commit UI fix ฟอร์ม MCP (`2643a59`)
- [x] แก้ MCP/child process ค้างหลังปิดแอป: Windows **Job Object**
      (`internal/proc.KillTreeOnExit`, `KILL_ON_JOB_CLOSE`) — พิสูจน์ด้วย
      force-kill จริง: ลูก 8 ตัวตายหมดทั้งต้นไม้
- [x] เอกสารแผนนี้ + ลิงก์เข้า Document Map + Decision §24 (commit เดียวกัน)

## Phase 1 — Skills & Plugins → หน้า Settings เดียว

ZCode แยกสองหน้า แต่ของเรามันกลไกเดียวกัน — skill ภายนอก = โฟลเดอร์ที่มี
`SKILL.md` ใต้ `~/.agents/skills` หรือ `~/.claude/skills` และ plugin_install
ก็เขียนลง `~/.agents/skills/<name>/` อยู่แล้ว:

- list skill ที่ discover เจอ (`ListSkills` มีอยู่แล้ว) + path ที่มา
- ปุ่ม install จาก GitHub repo URL — reuse ตัว `pluginInstallSkill.execute`
  เป็น desktop binding ตรง (ไม่ต้องให้ AI เรียก) แล้ว re-discover ทันที
  → **ปิด half-finished loop ของ plugin_install** (ติดตั้งแล้วใช้ได้เลย
  ไม่ต้อง restart; เงื่อนไขเดิมคง: bundle ต้องมี SKILL.md ถึงจะ discover เจอ)
- ปุ่มลบ (ลบโฟลเดอร์ skill) + refresh (re-bootstrap engine)

## Phase 2 — Onboarding (first-run wizard)

เปิดครั้งแรก (ยังไม่มี provider/API key) → wizard 3 ขั้น ข้ามได้ทุกขั้น:
ภาษา+ธีม → เลือก provider + ใส่ key (reuse `SupportedProviders`/
`submitAPIKey`) → โหมด approval เก็บ flag "onboarded" ใน localStorage
(ไม่ใช่ config กลาง — เป็นเรื่องของเครื่อง/หน้าจอ ไม่ใช่ engine)

## Phase 3 — Usage stats

- ท่อข้อมูลมีแล้ว: provider คืน `model.Usage{PromptTokens, CompletionTokens}`
  ทุก response — เหลือให้ turn executor ส่งขึ้นมาแล้ว desktop เก็บลง SQLite
  (ตาราง `usage`: session_id, ts, model, prompt_tokens, completion_tokens)
- หน้า Settings: รวมวันนี้ / 7 วัน / ทั้งหมด แยกตาม model — v1 ตัวเลขล้วน
  ไม่มีกราฟ (เพิ่มเมื่ออยากได้จริง)

## Phase 4 — Commands (custom slash commands)

ตาม pattern Claude Code (แหล่งอ้างอิงหลักของโปรเจกต์): custom command =
ไฟล์ `.md` ใน `<DataRoot>/commands/<name>.md` → พิมพ์ `/<name>` ใน chat =
ยิงเนื้อไฟล์เป็น prompt (แทนที่ `$ARGUMENTS` ด้วยข้อความหลังชื่อ command)
เพิ่ม lookup ชั้นเดียวก่อน grammar เดิม (built-in ชนะเสมอ) + หน้า Settings
list + ปุ่มเปิดโฟลเดอร์

## Phase 5 — Code preview (workbench ไม่ใช่ Settings)

ค่า editor (ฟอนต์/ธีม) อยู่ใน Appearance แล้ว — สิ่งที่ยังไม่มีจริงคือ
มุมมอง preview: เปิดไฟล์ `.md` ใน workbench แล้วสลับ Rendered/Source ได้
(ใช้ markdown renderer ตัวเดียวกับ Chat)

## Phase 6 — Subagents (design ก่อนเขียนโค้ด)

`internal/orchestrator` ถูกออกแบบรอเรื่องนี้ไว้แล้ว (§10 — "MAIN agent plus
sub-agents", มี Spawn/Get/Stop/List ครบ แต่ยังไม่มีใครเรียก) ขั้นตอน:

1. ร่าง Decision section ใหม่ใน ARCHITECTURE.md: `task` tool — main agent
   spawn sub-agent ผ่าน orchestrator, context/message history แยก,
   จำกัด depth 1, ผลลัพธ์กลับเป็น tool output; UI แสดงใน timeline
2. ผ่านแล้วค่อยทำ walking skeleton (tool + wiring + badge) — ไม่ทำ
   ensemble/routing/consensus ของ ADR 0002 ในรอบนี้

## สิ่งที่ตัดทิ้งโดยตั้งใจ

- **Indexing page** — ไม่มีอะไรให้ user ตั้ง (เหตุผลในตารางบนสุด)
- **Default MCP servers** — preset เป็น opt-in เท่านั้น ห้าม auto-install
  (บทเรียน obsidian ค้าง config, ดู MCP-SUPPORT-PLAN.md)
