# Competitor Research — 17 Jul 2026

> Reference document, not a decision. เก็บไว้ดูตอนออกแบบฟีเจอร์
>
> **อัปเดต 1 ส.ค. 2026** — คอลัมน์ Aetox เดิมล้าสมัยไปหลายช่อง (MCP, Desktop UI,
> sub-agents ทำเสร็จไปแล้วตั้งแต่ ก.ค.) แก้ให้ตรงกับโค้ดจริงแล้ว
> **หมายเหตุจุดยืน:** ตารางนี้เทียบกับ coding agent เพราะ
> เป็นสนามที่ engine ทับกัน — ไม่ใช่สนามที่ Aetox ตั้งใจไปแข่ง (ดู [COMPANY.md](../COMPANY.md) หัวข้อ "จุดยืน")
>
> **ขอบเขตของไฟล์นี้ (22 ส.ค. 2026):** ที่นี่เก็บได้เฉพาะ**การเทียบความสามารถระดับหัวข้อ**
> ซึ่งเป็นเหตุผลเดียวที่ไฟล์นี้เผยแพร่ได้ ([.gitignore](../.gitignore) ระบุไว้) โน้ตที่ไล่อ่าน
> ของโปรเจกต์อื่น หรือรายการว่าจะหยิบอะไรจากใคร **ไม่เก็บที่นี่และไม่ขึ้น GitHub** —
> เจ้าของตัดสินไว้เมื่อ 18 ส.ค. และหัวข้อแบบนั้นถูกถอดออกจากไฟล์นี้เมื่อ 22 ส.ค.

---

## Capability Matrix

| Capability | OpenCode | Claude Code | Codex CLI | Cursor | Aider | Aetox |
|-----------|----------|-------------|-----------|--------|-------|-------|
| Multi-provider | ✅ 40+ | ❌ Anthropic | ❌ OpenAI | ✅ หลายตัว | ✅ หลายตัว | ✅ 14 |
| Sub-agents | ✅ primary/sub | ✅ custom | ✅ verifier | ✅ | ❌ | ✅ `task`/`task_result`, profiles explore/general/plan (depth 1) |
| MCP | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ stdio + remote, tools + resources |
| Plugins | ✅ JS/TS | ✅ | ❌ | ✅ | ❌ | ⚠️ skill auto-discovery ✅ / `plugin_install` ยังไม่ครบ / hooks ไม่ทำ |
| Desktop UI | ✅ TUI+Web | ✅ CLI+Desktop+Web | ✅ TUI+Desktop | ✅ IDE | ❌ CLI | ✅ CLI + Desktop (Wails, browser + terminal pane) |
| Browser automation | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ `browser_open/read/click/type` — ไม่ต้องใช้ vision model |
| Autonomous mode | ❌ | ✅ routines | ✅ /goal | ✅ bg agents | ❌ | 🔜 workflow builder หลัง 1.0.0 |
| Git integration | ✅ | ✅ | ✅ | ✅ | ✅ (ดีสุด) | ✅ บางส่วน (`git` tool + snapshot/undo) |
| Voice | ❌ | ❌ | ❌ | ❌ | ✅ | ⚠️ `audio_transcribe` (input เท่านั้น ไม่ใช่ voice coding) |
| HTML output | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Knowledge base | AGENTS.md | CLAUDE.md | AGENTS.md | .cursor/rules | AGENTS.md | ✅ AETOX.md → AGENTS.md → CLAUDE.md |
| ข้อมูลอยู่ที่ไหน | local | cloud model | cloud model | cloud | local+cloud | ✅ local 100% |

---

## สิ่งที่ Aetox มีแล้วแต่ยังไม่ได้ทำ

| ของมีแล้ว | ใช้ตอนไหน |
|-----------|-----------|
| 14 providers abstraction | ✅ ใช้ได้เลย |
| Tool calling loop | ✅ ใช้ได้เลย |
| Safety + audit | ✅ ใช้ได้เลย |
| Git integration (skill) | ✅ ใช้ได้เลย |
| Multi-provider thinking/reasoning | ✅ ใช้ได้เลย |
| Agent model switching | ✅ ใช้ได้เลย |
| Skill library | ✅ connect แล้ว 2026-07-22 — `DiscoverSkills` scan `~/.aetox/skills/` (โฟลเดอร์ของ Aetox เอง ไม่ใช่ของเครื่องมืออื่น) |
| Knowledge base (Obsidian vault) | 📦 มี MCP server — ต่อผ่าน MCP ได้แล้ว แต่ยังไม่ได้ตั้งค่าใช้จริง |
| Web search | ✅ `web_search` + `web_fetch` เป็น builtin tool แล้ว |
| Token cost calculator | ⚠️ `token_usage` เก็บลง SQLite แล้ว (ผูกกับ session) — ยังไม่มี attribution ต่อ agent/tool |

---

## โน้ตสำคัญจาก Mike

- **Mobile monitor** — จำไว้, ควรทำ
- **Knowledge base** — ควรทำของเราเอง, ไม่พึ่ง外人
- **สิ่งที่เรามีแล้ว** — เยอะ, แค่ยังไม่ได้ implement
- **ยังไม่ตัดสินใจทั้งหมด** — เอกสารนี้คือ reference, ไม่ใช่ decision
