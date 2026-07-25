# Aetox — Architecture > Parameters

> **AI Agent + ผู้ช่วยส่วนตัว** — ไม่ผูกมัดกับระบบใด เป็นอิสระจากทุกข้อจำกัด
> ไม่ได้เกิดมาเพื่อเป็น framework อีกตัว  
> แต่เป็นรากฐานของ AGI ที่สถาปัตยกรรมคือหัวใจ ไม่ใช่โมเดล

No lock-in. No subscription pressure. No boundaries.  
Your rules, your data, your AGI.

**🌐 [ดูหน้าเว็บแนะนำ Aetox](https://mike0165115321.github.io/Aetox/)** — เห็นภาพว่ามันทำอะไรได้บ้างใน 1 นาที

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

## ขยายศักยภาพให้โมเดล — โมเดลตาบอดก็เห็นได้ หูหนวกก็ฟังได้ มือด้วนก็ทำงานได้

นี่คือหัวใจของ **Architecture > Parameters** แบบจับต้องได้:
โมเดล 9B ที่รันบนเครื่องคุณ อ่านภาพไม่ได้ ดูวิดีโอไม่ได้ ฟังเสียงไม่ได้ ลงมือทำในเว็บไม่ได้ —
แต่พอวิ่งผ่าน Aetox มันทำได้ทั้งหมด เพราะสถาปัตยกรรมเติมความสามารถให้ ไม่ใช่ตัวโมเดล

| โมเดลทำไม่ได้ | Aetox เติมให้ด้วย | ผลลัพธ์ |
|:--------------|:------------------|:--------|
| มองไม่เห็นรูปภาพ | `image_ocr` (Tesseract ไทย+อังกฤษ) | อ่านข้อความในรูปได้ทุกโมเดล |
| ดูวิดีโอไม่ได้ | `video_ocr` (ffmpeg แตกเฟรม + OCR) | ได้ข้อความพร้อม timestamp `[m:ss]` จากวิดีโอ |
| ฟังเสียงไม่ได้ | `audio_transcribe` (whisper.cpp ในเครื่อง) | ถอดเสียงพูดเป็นข้อความพร้อม `[m:ss]` ทั้งไฟล์เสียงและวิดีโอ |
| ลงมือทำในเว็บไม่ได้ | `browser_open`/`read`/`click`/`type` | อ่านหน้าเว็บจริง คลิก กรอกฟอร์ม เลือก dropdown แทนได้ |

ลูปเต็มของโมเดลที่ไม่มีตาไม่มีหู: รูปหรือคลิป → `image_ocr` / `video_ocr` / `audio_transcribe` แปลงเป็นข้อความ → ตัดสินใจ → `browser_click/type` ลงมือทำต่อในเว็บ
คลิปหนึ่งไฟล์ยิงทั้ง `video_ocr` และ `audio_transcribe` ได้ — ทั้งคู่คืน `[m:ss]` ฟอร์แมตเดียวกัน อ่านต่อกันเป็นสคริปต์เดียว

> โมเดลไหนก็ได้ ยิ่งถูกยิ่งดี — ความสามารถอยู่ที่สถาปัตยกรรม ไม่ใช่ราคาโมเดล

---

## 🔒 ความปลอดภัยของข้อมูลคุณ

Aetox ไม่ส่งข้อมูลอะไรออกจากเครื่องคุณนอกจากที่คุณสั่งให้ทำ

| เรื่อง | สถานะตอนนี้ |
|:---|:---|
| **ประวัติแชท / ไฟล์โปรเจกต์** | อยู่ในเครื่องคุณล้วนๆ (SQLite ท้องถิ่น) ไม่มี server ของเราคั่นกลาง |
| **อยากตัดขาดจาก cloud ทั้งหมด** | รันโมเดลผ่าน **LM Studio / Ollama** บนเครื่องคุณเองได้ — prompt ไม่ออกจากเครื่องแม้แต่ byte เดียว |
| **คำสั่งที่ agent รัน** | รันเฉพาะในโฟลเดอร์โปรเจกต์ที่คุณเปิด ออกนอกโฟลเดอร์ไม่ได้ และมีบันทึกไว้ทุกคำสั่ง |
| **API key** | ตอนนี้เก็บเป็นไฟล์ plaintext ในเครื่อง ยังไม่เข้ารหัส — 🔜 กำลังย้ายเข้า Windows DPAPI ก่อน แล้วขยายไป macOS Keychain / Linux secret service ตามแพลตฟอร์มที่รองรับเพิ่ม |

ถ้าใช้ cloud provider (OpenAI, Anthropic ฯลฯ) ข้อมูลที่ส่งออกคือสิ่งที่ provider นั้นเห็นตามปกติของ API เขา — Aetox เองไม่มี server หรือ analytics คั่นกลางเพื่อเก็บข้อมูลคุณต่อ

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

## สถานะปัจจุบัน — v0.4.0

Aetox ยังอยู่ในช่วงหล่อหลอม — แกนกลางพร้อมแล้ว ชั้นถัดไปกำลังถูกสร้าง

### ✅ พร้อมใช้ตอนนี้

| ความสามารถ | รายละเอียด |
|:-----------|:-----------|
| **13 Providers** | OpenAI, Anthropic, DeepSeek, Google Gemini, Groq, Mistral, Together, Perplexity, Cohere, OpenRouter, Z.ai, LM Studio, Ollama |
| **Tool Calling** | model-driven tool loop — agent เลือกใช้ tools เอง |
| **26 Tools ในตัว** | read, write, edit, list, shell, git, grep, image_ocr, video_ocr, audio_transcribe, browser_* และอื่นๆ |
| **Safety 3 ระดับ** | ถามก่อน → คำสั่งเสี่ยง → รันเต็มที่ |
| **Multi-provider** | ใช้ providers ต่างกันใน session เดียวกัน |
| **Model Switching** | เปลี่ยน provider/model ได้ทันที โดยไม่เสีย context |
| **Streaming** | แสดงผลแบบ real-time สำหรับ conversation |
| **Auto-save Preference** | ค่า provider, model, API key, approval mode จำอัตโนมัติ |
| **Desktop App** | Wails + Svelte 5 — Sidebar (file tree + chat history), Chat, Workbench (tabs: Review, Terminal, Files, Browser, File Editor), TopBar |
| **Persistent Sessions** | ประวัติแชททุกโปรเจกต์เก็บใน SQLite ท้องถิ่น (ไม่มีข้อมูลออกจากเครื่อง) — ค้นหาแบบ full-text ได้ทั้งไทย/อังกฤษ |
| **Agent-controlled Browser** | Agent เปิด/อ่าน/คลิก/พิมพ์ในเว็บจริงบนแท็บ Workbench ได้เอง (`browser_open`/`browser_read`/`browser_click`/`browser_type`) — ไม่ติด X-Frame-Options เหมือน iframe, คลิกด้วย ref ที่ `browser_read` แปะให้ (แบบเดียวกับ Playwright MCP/browser-use) |
| **Interactive Agent** | เมื่อ agent ติดสินใจไม่ได้ มันหยุดถามคุณเป็นตัวเลือก A/B/C/D ให้กดเลือก · งานใหญ่ agent ทำ checklist ให้เห็นความคืบหน้าสดๆ · ความคิดของโมเดลถูกเก็บไว้ทุกคำตอบ กดย้อนดูได้ว่า "คิดเป็นเวลากี่วินาที คิดอะไร" · ปุ่ม Stop หยุดงานกลางคันได้ตลอด |
| **ลองก่อนมีคีย์** | provider `aetox` ในตัวมีโมเดลจำลองให้ลองทุกฟีเจอร์โดยไม่ต้องสมัครอะไรเลย — โมเดลที่เรียก tools จริง, โมเดลคิดยาว, แกลเลอรีรูป, markdown ครบชุด |

### 🚧 เส้นทางถัดไป — จาก Agent เดี่ยว สู่ทีม Agent

Aetox วันนี้คือ agent เดี่ยวที่ใช้เครื่องมือเป็น เป้าหมายถัดไปคือให้มันทำงานเป็นทีมได้:

| ขั้น | คืออะไร | สถานะ |
|:---|:---|:---|
| **1. Subagent** | agent หลักมอบงานย่อยให้ agent ลูกไปทำในพื้นที่ความจำแยก แล้วรับเฉพาะข้อสรุปกลับมา — งานค้นหา/อ่านไฟล์เยอะๆ ไม่กินความจำของบทสนทนาหลักอีกต่อไป | ออกแบบเสร็จ กำลังจะสร้าง |
| **2. ทีม Agent ขนาน** | agent ลูกหลายตัวทำงานพร้อมกัน เช่น ค้นข้อมูล 5 มุมพร้อมกัน หรือ review โค้ดหลายแง่มุมในคราวเดียว — แต่ละตัวใช้โมเดลคนละค่ายได้ | ถัดจากขั้น 1 |
| **3. Multi-Agent Orchestration** | ทีมที่มีบทบาทชัดเจน (วางแผน / เขียนโค้ด / ตรวจ) ส่งงานต่อกันเอง รวมถึงให้หลายโมเดลถกเถียงกันเพื่อหาคำตอบที่ดีที่สุด | วิสัยทัศน์ระยะยาว |
| **4. Automation Engine** | บอกเป็นภาษาไทยว่าอยากให้ทำอะไรซ้ำๆ เมื่อไหร่ → Aetox สร้าง script และตั้งเวลารันให้เอง | หลัง core agent แข็งแรง |

รากฐานสำหรับเส้นทางนี้ถูกวางไว้แล้วตั้งแต่ต้น: agent แต่ละตัวเป็นหน่วยจบในตัวที่มีโมเดลและความจำของตัวเอง เครื่องมือทั้งหมดแชร์ผ่าน interface กลาง และคำขออนุญาตทุกอย่างวิ่งผ่านด่านความปลอดภัยจุดเดียว — การเพิ่มทีม agent จึงเป็นการต่อยอด ไม่ใช่การรื้อสร้างใหม่

### 🔮 อนาคตไกลกว่านั้น

- Aetox Ecosystem — plugin, marketplace, community skills
- Knowledge Base — Obsidian + codebase + web
- AGI-level reasoning — ensemble, debate, cross-validation
- Personal AI that grows with you

---

## เริ่มต้นใช้

Aetox เป็นแอปเดสก์ท็อปบน Windows โหลดแล้วเปิดใช้ได้เลย ไม่ต้องมี API key ก่อน —
provider `aetox` ในตัวมีโมเดลจำลองให้ลองทุกฟีเจอร์ก่อนตัดสินใจ

**ตัวติดตั้ง** — [ดาวน์โหลดตัวล่าสุด](https://github.com/Mike0165115321/Aetox/releases/latest/download/aetox-amd64-installer.exe)
ติดตั้งแล้วมีใน Start Menu จัดการ Tesseract (OCR) ให้อัตโนมัติ

**Scoop** — ถ้าใช้ scoop อยู่แล้ว

```powershell
scoop install https://raw.githubusercontent.com/Mike0165115321/Aetox/main/scoop/aetox.json
```

**Portable** — [โหลด zip ไม่ต้องติดตั้ง](https://github.com/Mike0165115321/Aetox/releases/latest/download/aetox-windows-amd64-portable.zip) แตกแล้วรัน `aetox.exe` · [ดูไฟล์ทั้งหมดของรุ่นล่าสุด](https://github.com/Mike0165115321/Aetox/releases/latest)

> ตัวติดตั้งยังไม่ได้ code signing ครั้งแรก Windows SmartScreen จะขึ้นเตือน "unknown publisher" —
> กด More info → Run anyway

### build เอง

```powershell
cd desktop
wails build          # ได้ desktop/build/bin/aetox.exe
wails build -nsis    # พร้อมตัวติดตั้ง
```
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
│    13 Providers | Tools | Turn Loop      │
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
| **Z.ai** | `ZAI_API_KEY` | ✅ | ✅ |
| **LM Studio** | ท้องถิ่น (localhost) | ✅ | ❌ |
| **Ollama** | ท้องถิ่น (localhost) | ✅ | ❌ |

---

## Tools ในตัว (26 ตัว)

| Tool | ใช้ทำอะไร |
|:-----|:----------|
| `read` | อ่านไฟล์ |
| `write` | เขียนไฟล์ทั้งไฟล์ |
| `edit` | แก้ไขไฟล์แบบ search & replace เป๊ะๆ |
| `delete` | ลบไฟล์ |
| `list` | ดูรายการไฟล์ใน directory |
| `shell` | รันคำสั่ง shell |
| `git` | คำสั่ง git |
| `grep` | ค้นหาข้อความในไฟล์ (regex) |
| `fs` | file system operations |
| `echo` | ทดสอบ output |
| `time` | แสดงเวลาปัจจุบัน |
| `help` | แสดง help |
| `github_repo_summary` | สรุป repo จาก GitHub |
| `plugin_install` | ติดตั้ง plugin จาก GitHub |
| `image_ocr` | อ่านข้อความจากรูป (Tesseract ไทย+อังกฤษ) |
| `video_ocr` | อ่านข้อความจากวิดีโอ (ffmpeg แตกเฟรม + OCR พร้อม timestamp) |
| `audio_transcribe` | ถอดเสียงพูดในไฟล์เสียง/วิดีโอเป็นข้อความ (whisper.cpp ออฟไลน์ ไทย+อังกฤษ) |
| `web_fetch` | อ่านเนื้อหาหน้าเว็บ |
| `web_search` | ค้นเว็บ (DuckDuckGo, ไม่ต้องมี API key) |
| `github_search` | ค้นโค้ด/repo บน GitHub |
| `github_read_file` | อ่านไฟล์จาก repo GitHub |
| `github_list_files` | ดูโครงสร้างไฟล์ของ repo GitHub |
| `browser_open` | เปิดหน้าเว็บในแท็บ Workbench |
| `browser_read` | อ่านหน้าเว็บ พร้อมแปะเลข ref ให้ทุก element |
| `browser_click` | คลิก element ด้วยเลข ref |
| `browser_type` | พิมพ์ลงช่อง หรือเลือก dropdown ด้วยเลข ref |

---

## โครงสร้างโปรเจค

```
Aetox/
├── cmd/aetox/              # CLI entry point — ยังไม่ปล่อยเป็นโปรดักต์
│   ├── main.go             # flags, provider selection, bootstrap
│   ├── main_windows.go     # UTF-8 console support
│   ├── main_other.go       # cross-platform
│   └── main_test.go
│
├── internal/               # core packages
│   ├── app/                # bootstrap + terminal loop (desktop ใช้แค่บางส่วน)
│   ├── audit/              # execution logging
│   ├── cognitive/          # Agent — tool loop, respond, stream
│   ├── command/            # intent parsing
│   ├── config/             # config loading, model preference persistence
│   ├── debuglog/           # debug logging
│   ├── grammar/            # input grammar
│   ├── memory/             # context management
│   ├── model/              # provider types, factory, bootstrap
│   ├── plan/               # execution planning
│   ├── provider/           # provider catalog (13 providers)
│   ├── safety/             # 3-tier approval
│   ├── skill/              # 22 built-in tools
│   ├── think/              # thinking level configuration
│   └── turn/               # turn pipeline — explicit command → model tool loop → chat (§17)
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
│   ├── workbench.go        # agent-facing browser_open/browser_read/browser_click/browser_type skills
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
| **Tool Calling** | ✅ model-driven เท่านั้น — โมเดลเลือก tool เองทุกครั้ง (§17) |
| **15 Built-in Tools** | ✅ read, write, list, shell, git, grep และอื่นๆ |
| **Desktop App** | ✅ Wails + Svelte 5 — Sidebar, Chat, Workbench (tabbed dock), TopBar |
| **Persistent Sessions** | ✅ SQLite ท้องถิ่น + FTS5 search (ไทย/อังกฤษ) ต่อโปรเจกต์ |
| **Agent-controlled Browser** | ✅ native WebView2 tab — agent เปิด/อ่านหน้าเว็บได้เอง |
| **Interactive Agent** | ✅ ถามผู้ใช้กลางงาน, checklist สด, ความคิดย้อนดูได้, ปุ่ม Stop |
| **Subagent** | 🔜 ขั้นแรกสู่ทีม agent — ออกแบบเสร็จแล้ว |
| **ทีม Agent ขนาน** | 🔜 ถัดจาก Subagent |
| **Multi-Agent Orchestration** | 📄 วิสัยทัศน์ระยะยาว — หลังสองขั้นแรกแข็งแรง |
| **Automation Engine** | 🔜 บอกเป็นไทย → script + ตั้งเวลารันอัตโนมัติ |
| **API key ในที่เก็บรหัสของ OS** | 🔜 ตอนนี้เป็นไฟล์ plaintext — ถัดไป Windows DPAPI ก่อน แล้วขยายไป macOS Keychain / Linux secret service |
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
> พรุ่งนี้คือ ecosystem ของตัวเอง
>
> ไม่มีทีมใหญ่ แต่วิสัยทัศน์ไกล  
> ไม่มีคนมากมาย แต่มีไฟจากใจผู้สร้าง
>
> — Mike (ชยพล พรมสะวะนา)

---

Project: [github.com/Mike0165115321/Aetox](https://github.com/Mike0165115321/Aetox)  
License: MIT
