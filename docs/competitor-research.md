# Competitor Research — 17 Jul 2026

> Reference document, not a decision. เก็บไว้ดูตอนออกแบบฟีเจอร์

---

## Capability Matrix

| Capability | OpenCode | Claude Code | Codex CLI | Cursor | Aider | Aetox |
|-----------|----------|-------------|-----------|--------|-------|-------|
| Multi-provider | ✅ 40+ | ❌ Anthropic | ❌ OpenAI | ✅ หลายตัว | ✅ หลายตัว | ✅ 11 |
| Sub-agents | ✅ primary/sub | ✅ custom | ✅ verifier | ✅ | ❌ | ❌ กำลังออกแบบ |
| MCP | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| Plugins | ✅ JS/TS | ✅ | ❌ | ✅ | ❌ | ❌ |
| Desktop UI | ✅ TUI+Web | ✅ CLI+Desktop+Web | ✅ TUI+Desktop | ✅ IDE | ❌ CLI | ❌ CLI |
| Autonomous mode | ❌ | ✅ routines | ✅ /goal | ✅ bg agents | ❌ | ❌ |
| Git integration | ✅ | ✅ | ✅ | ✅ | ✅ (ดีสุด) | ✅ บางส่วน |
| Voice | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ |
| HTML output | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Knowledge base | AGENTS.md | CLAUDE.md | AGENTS.md | .cursor/rules | AGENTS.md | ❌ |

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
| 11 providers abstraction | ✅ ใช้ได้เลย |
| Tool calling loop | ✅ ใช้ได้เลย |
| Safety + audit | ✅ ใช้ได้เลย |
| Git integration (skill) | ✅ ใช้ได้เลย |
| Multi-provider thinking/reasoning | ✅ ใช้ได้เลย |
| Agent model switching | ✅ ใช้ได้เลย |
| Skill library (67 skills) | 📦 มีแต่ยังไม่ connect กับ Aetox runtime |
| Knowledge base (Obsidian vault) | 📦 มี MCP server แต่ Aetox ยังไม่ใช้ |
| Web search (Firecrawl CLI + Exa) | 📦 มี CLI แต่ยังไม่เป็น tool ใน Aetox |
| Token cost calculator | 📦 skill มี แต่ยังไม่ integrate |

---

## โน้ตสำคัญจาก Mike

- **Mobile monitor** — จำไว้, ควรทำ (Codex Remote เป็นแรงบันดาลใจ)
- **Knowledge base** — ควรทำของเราเอง, ไม่พึ่ง外人
- **สิ่งที่เรามีแล้ว** — เยอะ, แค่ยังไม่ได้ implement
- **ยังไม่ตัดสินใจทั้งหมด** — เอกสารนี้คือ reference, ไม่ใช่ decision
