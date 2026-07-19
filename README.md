# Aetox — Desktop AI Agent System

เลือก provider อะไรก็ได้ ใช้知识ของคุณ ตั้งค่า agent profile เอง — Architecture ที่ฉลาด orchestrate ให้ทั้งหมดทำงานร่วมกัน

No lock-in. No subscription pressure. Your agents, your providers, your rules.

---

## ปัญหาที่ Aetox แก้

| ตอนนี้ | Aetox |
|--------|-------|
| Claude Code = ใช้ได้แค่ Claude | **เลือก provider เอง** — 11 providers รองรับ |
| Codex = ใช้ได้แค่ OpenAI | **ไม่ lock-in** — สลับได้เมื่อไหร่ก็ได้ |
| Cursor = IDE lock-in | **Desktop app + CLI** — ไม่ผูกกับ IDE ไหน |
| CrewAI = ต้องเขียนโค้ด | **Config + UI** — ตั้งค่าได้ ไม่ต้องเขียน code |
| ความรู้คุณอยู่กระจัดกระจาย | **Knowledge base ในตัว** — Obsidian + codebase + web |
| แต่ละ tool ใช้ context แยกกัน | **Architecture ควบคุมทุกอย่าง** — Directional Cognition |

## Aetox คืออะไร

Desktop AI agent system ที่ให้คุณ **ควบคุม provider, tools, agent profile, knowledge base** ของตัวเองได้ 100% — โดยมีสถาปัตยกรรมที่ฉลาดคอย orchestrate ความสามารถเหล่านี้ให้ทำงานร่วมกัน

## จุดเด่น

- **Multi-provider** — ใช้ Claude, DeepSeek, Gemini, OpenAI หรืออื่นๆ ใน session เดียวกัน
- **Sub-agent orchestration** — MAIN วางแผน, sub-agent ทำงาน, MAIN สังเคราะห์
- **Knowledge base ในตัว** — Obsidian vault + codebase + web search
- **Desktop UI** — cockpit แสดง agent, session, cost, logs
- **Architecture ที่คุณควบคุมได้** — Directional Cognition, agent profile, tool permissions
- **สร้าง automation เองได้** — บอก Aetox เป็นไทย → สร้าง script + schedule ให้อัตโนมัติ
- **ไม่มี subscription lock-in** — คุณเลือก provider และจ่ายเฉพาะ token ที่ใช้

## ความสามารถ

- Chat ทั่วไป
- ค้นเว็บ → สรุปเป็น HTML
- ค้น codebase + knowledge base
- รัน shell, git, docker commands
- Refactor โค้ดด้วย sub-agent
- ออกแบบ architecture ด้วย Directional Cognition
- 🚀 **สร้าง automation อัตโนมัติ** — บอก Aetox เป็นภาษาไทย:
  - Monitor: token cost, disk space, Docker, git status
  - Sync: git auto-commit, backup, dotfiles
  - Maintenance: cleanup, cache, dependencies
  - Watch: tech news, model release, GitHub
  - Pipeline: auto-build → test → deploy

## เริ่มต้นใช้

```powershell
# โหมดโต้ตอบ
aetox

# one-shot
aetox chat "refactor module นี้หน่อย"

# เลือก provider
aetox --model-provider anthropic --model-name claude-sonnet-4
```

## Architecture

```
┌──────────────────────────────────────────┐
│         Aetox Desktop (UI)               │
├──────────────────────────────────────────┤
│    Multi-Agent Layer                     │
│    MAIN → แยกงาน → spawn sub-agents     │
├──────────────────────────────────────────┤
│    Directional Cognition Engine          │
│    Multi-provider ensemble | vote        │
├──────────────────────────────────────────┤
│    Core Runtime                          │
│    11 Providers | Tools | Turn Loop      │
│    Safety | Audit | Config               │
├──────────────────────────────────────────┤
│    Knowledge Base                        │
│    Obsidian | Codebase | Web             │
└──────────────────────────────────────────┘
```

## การพัฒนา

Aetox ถูกออกแบบโดย Mike (ชยพล พรมสะวะนา) จากปรัชญา:

> "หัวใจไม่ใช่ความรู้ในโมเดล — แต่คือ Architecture ที่ควบคุมวิธีคิด"

Project: [github.com/Mike0165115321/Aetox](https://github.com/Mike0165115321/Aetox)
