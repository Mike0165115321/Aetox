# Aetox — Architecture > Parameters

Aetox คือระบบ AI agent บนเครื่องคุณ ที่ถูกสร้างมาเพื่อทำงานแทนคุณในงานที่ซ้ำซาก เอาชนะ Codex CLI หรือ Claude Code ไม่ใช่ด้วยโมเดลที่ใหญ่กว่า — แต่ด้วย **สถาปัตยกรรมที่ควบคุมวิธีคิดของโมเดล**

## ปรัชญา

- **ทิศทางสำคัญกว่าพลังดิบ.** สถาปัตยกรรมที่ดีเอาชนะ parameter ล้านล้านได้
- **คุณเป็นเจ้าของระบบ.** ไม่มี lock-in, ไม่มี rate limit, ไม่มี vendor ตัดฟีเจอร์
- **Agent ต้องรับใช้ ไม่ใช่กินทรัพยากร.** MAIN วางแผนและสังเคราะห์ — sub-agent ทำงานหนักแทน
- **สร้าง pattern ไม่ใช่สร้างเฉพาะกิจ.** ทำครั้งเดียว, automate ถาวร

## ความสามารถ

### Agent Runtime
- **MAIN agent** — ใช้ tools ตรงๆ สำหรับงานสั้น (grep, read, shell, web search)
- **Sub-agents** — spawn เฉพาะงานที่ต้องลูปยาว / isolation / specialist model
  - แต่ละตัวมี profile ของตัวเอง: model, allowed tools, role description
  - working → คืนผล plain text → จบ
- **Agent Profile** — user กำหนดเองว่า agent แต่ละตัวใช้ model อะไร, มี tools อะไร
  - เก็บเป็น config file, จัดการผ่าน Desktop UI

### Core Tools (ทุก agent ใช้ชุดเดียวกัน, profile filter ตามบทบาท)
| หมวด | Tools |
|------|-------|
| File | `read`, `write`, `delete`, `list` |
| Search | `grep`, `glob`, `find` |
| Terminal | `shell`, `git`, `ps`, `disk`, `docker` |
| Web | `web_search`, `web_fetch` |
| Utils | `echo`, `time` |

### Automation Engine 🚀

คุณบอก Aetox เป็นภาษาไทยว่าอยากให้ทำอะไรอัตโนมัติ — Aetox สร้าง script + ลง schedule + ตรวจผลให้เอง

**Aetox สร้าง automation อะไรให้คุณได้บ้าง:**

| หมวด | ตัวอย่าง |
|:-----|:---------|
| 🔍 **Monitor** | token cost spike, disk space ใกล้เต็ม, Docker container dead, git repo ahead/behind |
| 🔄 **Sync** | git auto-commit/push, journal backup, dotfiles sync, WSL2 disk compact |
| 🧹 **Maintenance** | cleanup temp/logs, npm/pip cache clear, dependency update check |
| 📡 **Watch** | tech news tracking, model release alert, GitHub release watcher |
| 📝 **Log** | auto-summarize การทำงานแต่ละวัน, สรุป decision log |
| ⚙️ **Pipeline** | auto-build → test → report, auto-deploy staging |

**สั่งยังไง:**
| Command | ทำอะไร |
|:--------|:-------|
| `aetox auto "..."` | สร้าง automation จากภาษาธรรมชาติ |
| `aetox auto list` | ดู automation ทั้งหมด |
| `aetox auto logs <name>` | ดู history + ผลล่าสุด |
| `aetox auto run <name>` | รันเดี๋ยวนี้ |
| `aetox auto remove <name>` | ลบ automation |

### Directional Cognition Engine (กำลังออกแบบ)
- Multi-provider parallel reasoning
- Response comparison, confidence scoring
- Route strategies: ensemble, refinement, debate

### Desktop UI (Wails + TypeScript)
- cockpit แสดง MAIN + sub-agents
- จัดการ agent profiles, tools, providers
- ประวัติ session, ค่าใช้จ่าย, logs

### Provider Flexibility
- 11 providers: OpenAI, Anthropic, DeepSeek, Google Gemini, Groq, Mistral, Ollama, Perplexity, Together, OpenRouter, LM Studio
- abstraction layer เดียวกัน — เปลี่ยน provider โดยไม่กระทบ architecture
- ควบคุม thinking/reasoning level ต่อ provider

## สถาปัตยกรรม

```
┌──────────────────────────────────────────┐
│         Aetox Desktop (UI)               │
│   Wails + TypeScript (Svelte/React)      │
├──────────────────────────────────────────┤
│                                          │
│    Multi-Agent Layer                     │
│    MAIN → ใช้ tools ตรงๆ                │
│    └── spawn sub-agent → งานลูปยาว       │
│                                          │
├──────────────────────────────────────────┤
│    Directional Cognition Engine          │
│    Parallel Ensemble | Specialist Route  │
│    Cross-Validation | Synthesis          │
├──────────────────────────────────────────┤
│    Core Runtime                          │
│    Providers | 16 Tools | Turn Loop      │
│    Safety | Audit | Config               │
├──────────────────────────────────────────┤
│    Terminal + File System                │
│    shell | git | read | write | search   │
└──────────────────────────────────────────┘
```

## สถานะ

| Layer | สถานะ |
|-------|--------|
| Core Runtime | ✅ v0.4.0 |
| Terminal + File | ✅ core tools พร้อม |
| Multi-Agent Orchestration | ❌ กำลังออกแบบ |
| Directional Cognition | 📄 ADR 0002 (Proposed) |
| Desktop UI | 📄 Aetox Desktop.md (Planned) |
| Plugin / MCP | 🔜 หลังจาก core แข็ง |

## วิธีใช้

```powershell
# โหมดโต้ตอบ
aetox

# one-shot
aetox chat "refactor module นี้หน่อย"

# เลือก provider
aetox --model-provider anthropic --model-name claude-sonnet-4

# สร้าง automation (บอกเป็นภาษาธรรมชาติ)
aetox auto "ตรวจ token cost ทุกเช้า 9 โมง ถ้าเกิน 50 บาทให้แจ้ง"

# ดู automation ทั้งหมด
aetox auto list

# รัน automation ทันที
aetox auto run check-token-cost

# ดู logs
aetox auto logs check-token-cost

# ลบ automation
aetox auto remove check-token-cost
```

สร้างโดย Mike. Architecture > Parameters.
