# 🌌 AetoxClaw: The Agentic Local OS Orchestrator
**Next-Generation AI System for Local Task Orchestration**

AetoxClaw คือระบบปฏิบัติการ AI แบบ **Agentic** ที่ทำงานบนเครื่องของคุณโดยสมบูรณ์ (Local-first) ออกแบบมาเพื่อเป็น "สมองกลาง" ในการจัดการไฟล์ วางแผนงานซับซ้อน และควบคุมระบบปฏิบัติการผ่านคำสั่งภาษาไทยที่ยืดหยุ่น โดยใช้สถาปัตยกรรมที่เน้นความปลอดภัย ความเสถียร และความเป็นส่วนตัว

---

## 🚀 ความสามารถหลัก (Core Capabilities)

### 🧠 Hybrid Memory System (3-Layer)
ระบบความจำที่จำลองการทำงานของมนุษย์ เพื่อให้เอเจนต์จดจำบริบทได้แม่นยำแม้รันบนโมเดลขนาดเล็ก:
1.  **Working Memory (RAM):** เก็บ Context ปัจจุบันที่กำลังทำอยู่ (Async-Safe & Stateless).
2.  **Episodic Memory (Disk):** บันทึก "บทเรียน" และประวัติการทำงานย่อยๆ ในรูปแบบ JSONL เพื่อการเรียกใช้ซ้ำ.
3.  **Long-term Memory (Vector DB):** เก็บข้อมูลดิบจากไฟล์และเว็บไซต์ผ่านระบบ RAG (BGE-M3 Embedder) เพื่อการค้นหาเชิงความหมาย.

### 🎯 Async Orchestration (Dispatcher-based)
สถาปัตยกรรมแบบกระจายศูนย์ที่ลดความซับซ้อนของโค้ดหลัก:
*   **MainAgent (Planner):** รับคำสั่งและวางแผนงาน (Planning) แล้วส่งต่อให้ Dispatcher.
*   **Dispatcher (Orchestrator):** ควบคุมการรันงานแบบ Async รองรับ Timeout, Retry และการจัดการ Error อย่างเป็นระบบ.
*   **ExecutorAgent (Doer):** ศูนย์กลางการเรียกใช้เครื่องมือ (Tool Registry) ที่มีความปลอดภัยสูง.
*   **CriticAgent (Auditor):** ตรวจสอบผลลัพธ์ของแต่ละขั้นตอนและให้ Feedback เพื่อแก้ไขงานอัตโนมัติ.

### 👁️ AetoxVision & Control
*   **Deep Analysis:** อ่านและสรุปไฟล์ PDF, Word, Markdown และ Code ได้อย่างแม่นยำ.
*   **Master File Manager:** จัดระเบียบไฟล์และจัดการเส้นทาง (Path Navigation) อัตโนมัติ.
*   **Dynamic Tool Discovery:** รองรับการเพิ่มเครื่องมือใหม่โดยไม่ต้องแก้ไขระบบหลัก.

---

## 🛠 เทคโนโลยี (Tech Stack)
*   **LLM Engine:** [Ollama](https://ollama.com/) (Qwen 2.5 / Llama 3)
*   **Embedder:** BAAI/bge-m3 (State-of-the-art Multi-lingual Embedder)
*   **Vector DB:** ChromaDB (Local-first)
*   **Language:** Python 3.11+ (Asynchronous / Task-oriented)
*   **Safety:** Built-in Permission Manager & Path Sandbox

---

## 🗄️ โครงสร้างโปรเจกต์ (Project Structure)
```text
AetoxClaw/
├── 📁 aetox/                    # แกนหลักของระบบ
│   ├── 📁 agents/              # เอเจนต์ (Main, Executor, Critic)
│   ├── 📁 core/                # ระบบควบคุม (Dispatcher, PromptEngine, Client)
│   ├── 📁 memory/              # ระบบความจำ 3 ชั้น (Working, Episodic, Vector)
│   ├── 📁 tools/               # เครื่องมือ (FileManager, WebScraper, Vision)
│   └── 📁 safety/              # ระบบความปลอดภัยและการจัดการสิทธิ์
├── 📁 config/                  # การตั้งค่าเอเจนต์และเครื่องมือ (YAML)
├── 📁 data/                    # ฐานข้อมูลและไฟล์ Snapshot (Git Ignored)
├── 📁 docs/                    # เอกสารทางเทคนิคและคู่มือ
└── README.md                   # 📘 เอกสารสรุปโครงการ
```

---

## 📋 Roadmap การพัฒนา
- [x] **Phase 1-2**: ระบบฐานรากและการเชื่อมต่อ Discord/CLI
- [x] **Phase 3**: WebPulse Integration (ดึงข้อมูลเว็บเข้าสู่ Memory)
- [x] **Phase 4**: **Trinity Update** (Unified Async Architecture + 3-Layer Memory) ⬅️ *Current Status*
- [ ] **Phase 5**: Aetox Dashboard (Desktop GUI สำหรับควบคุมและดู Progress)
- [ ] **Phase 6**: Auto-Optimization (ระบบเรียนรู้วิธีแก้ปัญหาจากความผิดพลาดในอดีตอัตโนมัติ)

---

## 💡 ตัวอย่างคำสั่ง
*   *"สรุปเนื้อหาจากไฟล์ PDF ในโฟลเดอร์ดาวน์โหลด แล้วส่งเข้ากลุ่ม Discord"*
*   *"ช่วยหาข้อมูลเกี่ยวกับโปรเจกต์ใหม่จากเว็บ แล้ววางแผนขั้นตอนการทำงานให้ผมที"*
*   *"จัดระเบียบหน้า Desktop ให้หน่อย แยกไฟล์รูปกับไฟล์เอกสารออกจากกัน"*

---
*Created with ❤️ by Antigravity for the Aetox Ecosystem. Build for Local, Scale for Life.*
