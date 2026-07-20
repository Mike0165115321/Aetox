# Aetox — Architecture > Parameters

> **AI Agent + ผู้ช่วยส่วนตัว** — ไม่ผูกมัดกับระบบใด เป็นอิสระจากทุกข้อจำกัด
> ไม่ได้เกิดมาเพื่อเป็น framework อีกตัว  
> แต่เป็นรากฐานของ AGI ที่สถาปัตยกรรมคือหัวใจ ไม่ใช่โมเดล

เลือก provider ไหนก็ได้ ใช้ knowledge ของคุณ ตั้งค่าอะไรก็ได้ตามที่คุณต้องการ —  
เพราะ Aetox ไม่ได้เก่งที่โมเดล แต่เก่งที่ **วิธีคิด** ที่ควบคุมโมเดลอีกที

No lock-in. No subscription pressure. No boundaries.  
Your rules, your data, your AGI.

---

## Aetox คืออะไร

Aetox คือ **AI Agent + ผู้ช่วยส่วนตัว (Personal AI Assistant)**  
ที่ไม่ได้เกิดมาเพื่อแข่งกับใคร — แต่เกิดมาเพื่อ **อยู่เหนือข้อจำกัดของระบบเดิม**

- **เป็น AI Agent** — วางแผน ใช้ tools แก้ปัญหา real-world tasks ได้  
- **เป็น Personal Assistant** — ไม่ได้จำกัดแค่ agent workflow แต่พร้อมช่วยทุกเรื่อง  
- **ไม่ใช่ IDE plugin** — ไม่ผูกติดกับ IDE ใด  
- **ไม่ใช่แค่ chatbot** — คือ architecture ที่วิธีคิดสำคัญกว่าพลังดิบ

Aetox คือ **ยุคใหม่ของ AGI** ที่:

- **ไม่มีทีมใหญ่** — แต่วิสัยทัศน์ไกล  
- **ไม่มีคนมากมาย** — แต่มีไฟจากใจผู้สร้าง  
- **ไม่มีข้อจำกัด** — ไม่มี lock-in, ไม่มี rate limit, ไม่มี vendor ตัดฟีเจอร์  
- **พร้อมเติบโต** — วันนี้คือผู้ช่วยส่วนตัว วันหน้า ecosystem ของตัวเอง

> "หัวใจไม่ใช่ความรู้ในโมเดล — แต่คือ Architecture ที่ควบคุมวิธีคิด"

---

## ปรัชญา

| หลักการ | ความหมาย |
|:--------|:---------|
| **Architecture > Parameters** | สถาปัตยกรรมที่ดีเอาชนะ parameter ล้านล้านได้ |
| **Freedom > Convenience** | ไม่มี lock-in คุ้มค่ากว่าความสะดวกที่ผูกมัด |
| **You Own It** | คุณเป็นเจ้าของระบบ — ข้อมูล, โมเดล, การตั้งค่า ทั้งหมดเป็นของคุณ |
| **Direction > Execution** | ทิศทางสำคัญกว่าพลังดิบ — รู้ว่าต้องไปไหนก่อนลงมือ |
| **Pattern > Ad-hoc** | สร้าง pattern ไม่ใช่สร้างเฉพาะกิจ — ทำครั้งเดียว, automate ถาวร |
| **Simplicity > Complexity** | แก้ปัญหาด้วยวิธีที่ง่ายที่สุด ไม่ใช่เพิ่ม layer โดยไม่จำเป็น |

---

## ปัญหาที่ AI เจ้าอื่นแก้ไม่ได้

| ปัญหา | AI ทั่วไป | Aetox |
|:------|:----------|:------|
| **Vendor lock-in** | Claude Code = Claude เท่านั้น, Codex = OpenAI เท่านั้น, Cursor = IDE นั้น | **เลือก provider เอง** — 11 providers, สลับเมื่อไหร่ก็ได้ |
| **One model = one blind spot** | ใช้โมเดลเดียว → จุดอ่อนของโมเดลนั้นคือจุดอ่อนของระบบ | **ใช้หลาย providers** → cross-validation, weighted vote, เลือกคำตอบที่ดีที่สุด |
| **Data = product** | ข้อมูลคุณเข้าไปเทรนโมเดลต่อ | **ข้อมูลคุณเป็นของคุณ** — คุณเลือก provider ที่ไว้ใจได้ หรือรัน local |
| **Context waste** | Agent อ่านทั้ง codebase โดยไม่รู้ว่าอะไรสำคัญ | **Architecture-aware** — รู้ว่าต้องอ่านอะไร แค่ไหน ไม่ผลาญ context |
| **Subscription pressure** | จ่ายเดือนละ $20+ ต่อคน ต่อ tool | **จ่ายเฉพาะ token ที่ใช้** — เลือก provider ถูก/แพงได้ตามงาน |
| **Over-engineering** | LangChain 10 ชั้น, abstractions พรึบ | **ตรงไปตรงมา** — Go core, 15 packages, ไม่มี unnecessary layers |
| **Use case กำจัด** | IDE plugin → แก้โค้ดอย่างเดียว | **ผู้ช่วยส่วนตัว** — โค้ดได้, ค้นเว็บได้, สร้าง automation ได้, อะไรก็ได้ |
| **Zero architecture thinking** | แก้โค้ดตาม prompt ไม่รู้จัก system design | **Directional Cognition** — รู้ว่าคำถามนี้ควรใช้ architecture แบบไหน |
| **Community > Product** | ต้องรอ roadmap คนอื่น | **คุณเป็นเจ้าของ** — built โดยคนคนเดียวที่ใช้จริง ตัดสินใจได้ทันที |

### ทำไมต้อง Aetox?

เพราะ AI ที่ดีที่สุดในโลก **แต่คุณเลือก provider ไม่ได้** — ก็เหมือนมีรถยนต์ที่เติมได้แค่ปั๊มเดียว

เพราะ Agent ที่เก่งที่สุด **แต่ใช้ได้แค่ IDE นั้น** — ก็เหมือนมีสมาร์ทโฟนที่โทรได้แค่ค่ายเดียว

เพราะ Architecture ที่ฉลาดที่สุด **แต่คุณควบคุมวิธีคิดไม่ได้** — ก็เหมือนมีผู้ช่วยที่ทำตามคำสั่งอย่างเดียว ไม่รู้ว่าทำไปทำไม

Aetox ไม่ได้เกิดมาเพื่อเป็น "อีกตัวเลือกหนึ่ง"  
แต่เกิดมาเพื่อให้คุณ **เป็นเจ้าของระบบของคุณเอง**

---

## สถานะปัจจุบัน — v0.4.0

Aetox ยังอยู่ในช่วงหล่อหลอม — แกนกลางพร้อมแล้ว ชั้นถัดไปกำลังถูกสร้าง

### ✅ พร้อมใช้ตอนนี้

| ความสามารถ | รายละเอียด |
|:-----------|:-----------|
| **CLI โต้ตอบ** | โหมด interactive + one-shot |
| **11 Providers** | OpenAI, Anthropic, DeepSeek, Google Gemini, Groq, Mistral, Together, Perplexity, Cohere, LM Studio, Ollama |
| **Tool Calling** | model-driven tool loop — agent เลือกใช้ tools เอง |
| **17 Tools ในตัว** | read, write, list, shell, git, grep, echo, และอื่นๆ |
| **Safety 3 ระดับ** | ถามก่อน → คำสั่งเสี่ยง → รันเต็มที่ |
| **Multi-provider** | ใช้ providers ต่างกันใน session เดียวกัน |
| **Model Switching** | เปลี่ยน provider/model ได้ทันที โดยไม่เสีย context |
| **Streaming** | แสดงผลแบบ real-time สำหรับ conversation |
| **Auto-save Preference** | ค่า provider, model, API key, approval mode จำอัตโนมัติ |
| **Desktop App** | Wails + Svelte 5 — Sidebar (file tree + chat history), Chat, Workbench (tabs: Review, Terminal, Files, Browser, File Editor), TopBar |
| **Persistent Sessions** | ประวัติแชททุกโปรเจกต์เก็บใน SQLite ท้องถิ่น (ไม่มีข้อมูลออกจากเครื่อง) — ค้นหาแบบ full-text ได้ทั้งไทย/อังกฤษ |
| **Agent-controlled Browser** | Agent เปิดเว็บจริงในแท็บ Workbench ได้เอง (`browser_open`/`browser_read`) — ไม่ติด X-Frame-Options เหมือน iframe |

### 🚧 กำลังสร้าง

| ความสามารถ | สถานะ |
|:-----------|:------|
| Directional Cognition Engine | ADR 0002 — กำลังออกแบบ |
| Multi-Agent Orchestration | วางแผน |
| Knowledge Base (Obsidian + codebase + web) | วางแผน |

### 🔮 อนาคต

- **Automation Engine** — `aetox auto` บอกเป็นไทย → สร้าง script + schedule อัตโนมัติ
- Aetox Ecosystem — plugin, marketplace, community skills
- AGI-level reasoning — ensemble, debate, cross-validation
- Personal AI that grows with you

---

## เริ่มต้นใช้

```powershell
# build
./build.ps1

# โหมดโต้ตอบ (เลือก provider ครั้งแรก)
aetox

# one-shot
aetox chat "ช่วยดูโค้ด module นี้หน่อย"

# เลือก provider และ model
aetox --model-provider deepseek --model-name deepseek-v4-flash

# กำหนด thinking level
aetox --model-provider deepseek --model-name "deepseek-v4-flash(high)"

# approval mode
aetox --approval full-access
```

### Flags

| Flag | คำอธิบาย |
|:-----|:---------|
| `--model-provider` | `openai`, `anthropic`, `deepseek`, `gemini`, `groq`, `mistral`, `together`, `perplexity`, `cohere`, `lmstudio`, `ollama` |
| `--model-name` | ชื่อ model หรือ `model(think-level)` เช่น `deepseek-v4-flash(high)` |
| `--think` | thinking level — `off-think`, `high`, `max` (แล้วแต่ provider) |
| `--approval` | approval mode — `ask`, `unsafe-only`, `full-access` |
| `--no-banner` | ไม่แสดง banner ตอนเข้า interactive mode |
| `--debug` | เขียน debug log |

---

## Architecture

```
┌──────────────────────────────────────────┐
│         Aetox Desktop (UI)               │ ← Wails + Svelte cockpit
│ Sidebar(tree+history) · Chat · Workbench │   Workbench = tabs: Review,
│ (tabs) · TopBar                          │   Terminal, Files, Browser, Editor
├──────────────────────────────────────────┤
│    Local Store (SQLite, FTS5)            │ ← ประวัติแชททุกโปรเจกต์ ค้นหาได้
│    เก็บในเครื่อง ไม่มีข้อมูลออกไปไหน        │   ทั้งไทย/อังกฤษ, ไม่มี cloud
├──────────────────────────────────────────┤
│                                          │
│    Directional Cognition Engine          │ ← วิธีคิด — ensemble, routing,
│    Parallel Ensemble | Specialist Route  │   cross-validation, synthesis
│    Cross-Validation | Synthesis          │   (ออกแบบ)
│                                          │
├──────────────────────────────────────────┤
│    Multi-Provider Orchestration          │ ← ใช้หลาย providers พร้อมกัน
│    Router | Comparator | Consensus       │   เปรียบเทียบ เลือกคำตอบที่ดีที่สุด
│                                          │
├──────────────────────────────────────────┤
│    Core Runtime                          │ ← แกนกลางที่ทำงานแล้ว
│    11 Providers | Tools | Turn Loop      │
│    Safety | Audit | Config               │
├──────────────────────────────────────────┤
│    Terminal + File System                │
│    shell | git | read | write | search   │
└──────────────────────────────────────────┘
```

---

## Providers ที่รองรับ

| Provider | API Key | Tool Calling | Reasoning |
|:---------|:--------|:------------|:----------|
| **OpenAI** | `OPENAI_API_KEY` | ✅ | ✅ |
| **Anthropic** | `ANTHROPIC_API_KEY` | ✅ | ✅ |
| **DeepSeek** | `DEEPSEEK_API_KEY` | ✅ | ✅ |
| **Google Gemini** | `GEMINI_API_KEY` | ✅ | ✅ |
| **Groq** | `GROQ_API_KEY` | ✅ | ✅ |
| **Mistral** | `MISTRAL_API_KEY` | ✅ | ❌ |
| **Together** | `TOGETHER_API_KEY` | ✅ | ❌ |
| **Perplexity** | `PERPLEXITY_API_KEY` | ✅ | ❌ |
| **Cohere** | `COHERE_API_KEY` | ✅ | ❌ |
| **OpenRouter** | `OPENROUTER_API_KEY` | ✅ | ✅ |
| **LM Studio** | ท้องถิ่น (localhost) | ✅ | ❌ |
| **Ollama** | ท้องถิ่น (localhost) | ✅ | ❌ |

---

## Tools ในตัว (17 ตัว)

| Tool | ใช้ทำอะไร |
|:-----|:----------|
| `read` | อ่านไฟล์ |
| `write` | เขียน/แก้ไขไฟล์ |
| `delete` | ลบไฟล์ |
| `list` | ดูรายการไฟล์ใน directory |
| `shell` | รันคำสั่ง shell |
| `git` | คำสั่ง git |
| `grep` | ค้นหาข้อความในไฟล์ |
| `echo` | ทดสอบ output |
| `time` | แสดงเวลาปัจจุบัน |
| `help` | แสดง help |
| `input` | ขอ input จากผู้ใช้ |
| `output` | แสดง output |
| `fs` | file system operations |
| `defaults` | ค่าเริ่มต้น |
| `github_repo_summary` | สรุป repo |
| `plugin_install` | ติดตั้ง plugin |
| `dispatcher` | skill dispatching |

---

## โครงสร้างโปรเจค

```
Aetox/
├── cmd/aetox/              # entry point
│   ├── main.go             # CLI flags, provider selection, bootstrap
│   ├── main_windows.go     # UTF-8 console support
│   ├── main_other.go       # cross-platform
│   └── main_test.go
│
├── internal/               # core packages
│   ├── app/                # interactive CLI loop, banner, prompt
│   ├── audit/              # execution logging
│   ├── cognitive/          # Agent — tool loop, respond, stream
│   ├── command/            # intent parsing
│   ├── config/             # config loading, model preference persistence
│   ├── debuglog/           # debug logging
│   ├── grammar/            # input grammar
│   ├── memory/             # context management
│   ├── model/              # provider types, factory, bootstrap
│   ├── plan/               # execution planning
│   ├── provider/           # provider catalog (11 providers)
│   ├── safety/             # 3-tier approval
│   ├── skill/              # 17 built-in tools
│   ├── think/              # thinking level configuration
│   └── turn/               # 4-phase execution pipeline
│
├── desktop/                # Wails + Svelte 5 desktop app
│   ├── frontend/           # Svelte 5 UI
│   │   ├── src/lib/        # Chat, Sidebar, TopBar, Settings, TaskTimeline
│   │   ├── src/lib/workbench/ # tabbed dock — Review, Files, Browser panes
│   │   ├── src/lib/stores/ # cockpit + workbench state (Svelte 5 runes)
│   │   ├── src/lib/services/ # Go core bindings
│   │   └── src/style.css   # CSS custom properties theme system
│   ├── app.go              # Wails app binding (providers, model, project)
│   ├── browser.go          # native WebView2 browser tabs (agent + user)
│   ├── db.go                # local SQLite store (chat history, FTS5)
│   ├── sessions.go         # per-project session persistence + search
│   ├── workbench.go        # agent-facing browser_open/browser_read skills
│   ├── terminal.go         # embedded shell sessions
│   ├── main.go             # Desktop entry point
│   ├── wails.json           # Wails v2 config
│   └── desktop.exe         # build artifact
│
├── docs/
│   ├── adr/
│   │   ├── 0001-native-tool-calling-foundation.md   ✅
│   │   └── 0002-directional-cognition-engine.md      📄 Proposed
│   ├── architecture-reference-opencode.md
│   ├── architecture-review-aetox-cli.md
│   └── competitor-research.md
│
├── go.mod
├── go.sum
├── build.ps1               # build script
└── AETOX.md                # full project vision
```

---

## สถานะการพัฒนา

| Layer | สถานะ |
|-------|-------|
| **Core Runtime** | ✅ v0.4.0 — providers, tools, turn loop, safety |
| **CLI** | ✅ interactive + one-shot + auto-save preference |
| **Tool Calling** | ✅ model-driven + regex fallback |
| **17 Built-in Tools** | ✅ read, write, list, shell, git, grep และอื่นๆ |
| **Desktop App** | ✅ Wails + Svelte 5 — Sidebar, Chat, Workbench (tabbed dock), TopBar |
| **Persistent Sessions** | ✅ SQLite ท้องถิ่น + FTS5 search (ไทย/อังกฤษ) ต่อโปรเจกต์ |
| **Agent-controlled Browser** | ✅ native WebView2 tab — agent เปิด/อ่านหน้าเว็บได้เอง |
| **Automation Engine** | 🔜 `aetox auto` — บอกเป็นไทย → script + schedule |
| **Directional Cognition** | 📄 ADR — รอเริ่ม implement |
| **Multi-Provider Orchestration** | 🔜 ถัดจาก Directional Cognition |
| **Knowledge Base** | 🔜 Obsidian + codebase + web |
| **Ecosystem (plugin/marketplace)** | 🔜 หลังจาก core แข็ง |

---

## จากผู้สร้าง

> Aetox เกิดมาไม่ใช่เพื่อแข่งกับใคร  
> ไม่ใช่เพื่อเป็นอีกหนึ่ง agent framework  
> ไม่ใช่เพื่อ lock-in ผู้ใช้เข้าสู่ระบบใด
>
> Aetox คือ **ผู้ช่วยส่วนตัว** ที่พร้อมจะเติบโต  
> คือ AGI ที่ไม่ผูกมัดกับระบบใด  
> คือรากฐานของสถาปัตยกรรมที่จะควบคุมวิธีคิดของโมเดล
>
> วันนี้คือ CLI ไม่กี่พันบรรทัด  
> พรุ่งนี้คือ ecosystem ของตัวเอง
>
> ไม่มีทีมใหญ่ แต่วิสัยทัศน์ไกล  
> ไม่มีคนมากมาย แต่มีไฟจากใจผู้สร้าง
>
> — Mike (ชยพล พรมสะวะนา)

---

Project: [github.com/Mike0165115321/Aetox](https://github.com/Mike0165115321/Aetox)  
License: MIT
