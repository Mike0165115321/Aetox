# Competitor Research — 17 Jul 2026

> Reference document, not a decision. เก็บไว้ดูตอนออกแบบฟีเจอร์
>
> **อัปเดต 1 ส.ค. 2026** — คอลัมน์ Aetox เดิมล้าสมัยไปหลายช่อง (MCP, Desktop UI,
> sub-agents ทำเสร็จไปแล้วตั้งแต่ ก.ค.) แก้ให้ตรงกับโค้ดจริงแล้ว
> **หมายเหตุจุดยืน:** ตารางนี้เทียบกับ coding agent เพราะ
> เป็นสนามที่ engine ทับกัน — ไม่ใช่สนามที่ Aetox ตั้งใจไปแข่ง (ดู [COMPANY.md](../COMPANY.md) หัวข้อ "จุดยืน")

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

## จุดเด่นที่ควร Borrow

### จาก OpenCode
- ✅ Skill system (discoverable workflows) → Aetox มี skill-library อยู่แล้ว
- ✅ Plugin hooks (event-driven) → ไว้ Phase plugin
- ✅ Agent permission system → ตรงกับ agent profile ที่ออกแบบ

### จาก Claude Code
- ✅ Sub-agent config (tools, model, isolation) → ตรงกับ design
- ⏳ Agent teams + dynamic workflows → ขั้น advance
- 🔜 Hooks + background tasks → ไว้ทีหลัง

### จาก Codex CLI
- ✅ **/goal mode** → autonomous multi-turn — ตรงกับ sub-agent ลูปยาว
- ✅ **Verifier subagent** — maker ไม่ตรวจงานตัวเอง
- ✅ **Token budget control** — ตรงที่ชอบ token-calc
- 🔲 **Mobile monitor** — Mike บอกจำไว้ ควรทำ

### จาก Cursor
- ✅ Composer multi-file edit with diff — สำหรับ coding mode
- ✅ Parallel agents — sub-agent ขนานกัน
- 🔜 Cloud agents — ไว้ทีหลัง

### จาก Aider
- ✅ **Git auto-commit per turn** — audit trail ชัด
- ✅ **Architect mode** — โมเดลแพงวางแผน → โมเดลถูก implement
- ✅ **Repo map** — structure map ของ codebase
- ❌ Voice coding — ไม่จำเป็น

### จาก CrewAI / AutoGPT
- ✅ Role-based agents (researcher, coder, reviewer)
- ✅ Human-in-the-loop
- ✅ Goal decomposition

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
| Skill library | ✅ connect แล้ว 2026-07-22 — `DiscoverSkills` scan `~/.agents/skills/`, `~/.claude/skills/` |
| Knowledge base (Obsidian vault) | 📦 มี MCP server — ต่อผ่าน MCP ได้แล้ว แต่ยังไม่ได้ตั้งค่าใช้จริง |
| Web search | ✅ `web_search` + `web_fetch` เป็น builtin tool แล้ว |
| Token cost calculator | ⚠️ `token_usage` เก็บลง SQLite แล้ว (ผูกกับ session) — ยังไม่มี attribution ต่อ agent/tool |

---

## โน้ตสำคัญจาก Mike

- **Mobile monitor** — จำไว้, ควรทำ (Codex Remote เป็นแรงบันดาลใจ)
- **Knowledge base** — ควรทำของเราเอง, ไม่พึ่ง外人
- **สิ่งที่เรามีแล้ว** — เยอะ, แค่ยังไม่ได้ implement
- **ยังไม่ตัดสินใจทั้งหมด** — เอกสารนี้คือ reference, ไม่ใช่ decision
