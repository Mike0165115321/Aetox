# 🌌 AetoxClaw: The Lightweight Agentic Orchestrator
**High-Performance AI System for Local Task Orchestration**

AetoxClaw คือระบบปฏิบัติการ AI แบบ **Agentic** ที่ทำงานบนเครื่องของคุณโดยสมบูรณ์ (Local-first) ออกแบบมาให้มีความเร็วสูงและประหยัดทรัพยากร (Lightweight) โดยใช้สถาปัตยกรรม **Stateless Core** พร้อมระบบการฉีดบริบท (History Injection) ที่แม่นยำตามประเภทงาน

---

## 🚀 ความสามารถหลัก (Core Capabilities)

### 🧠 Lightweight Session Context (New!)
ระบบจัดการความจำที่ถูกปรับแต่งมาเพื่อความเร็วและประหยัด RAM/CPU:
1.  **Stateless Execution:** ตัวเอเจนต์ไม่ต้องแบกรับความจำหนักๆ ไว้ในตัว แต่จะได้รับบริบทที่จำเป็น (Injected Context) เฉพาะตอนที่ทำงาน
2.  **Smart History Injection:** 
    *   **Chat Mode:** ส่งประวัติการคุยแบบ Sliding Window (3-5 ข้อความล่าสุด) เพื่อความต่อเนื่อง
    *   **Plan Mode:** ส่งผลลัพธ์ขั้นตอนก่อนหน้า + สรุปสถานะแผนงาน เพื่อความแม่นยำในงานซับซ้อน
3.  **Zero Background Overhead:** ยกเลิกการทำ RAG, Summarize และ Keyword Extraction อัตโนมัติ เพื่อให้รันบนโมเดลขนาดเล็กได้ลื่นไหล

### 🎯 Async Orchestration (Dispatcher-based)
สถาปัตยกรรมที่แยกส่วนการวางแผนและการลงมือทำออกจากกัน:
*   **MainAgent (Planner):** วิเคราะห์เป้าหมายและวางแผนงาน (Planning) เป็นลำดับขั้นตอน
*   **Dispatcher (Orchestrator):** ควบคุมการรันงานแบบ Async รองรับ Timeout, Retry และการจัดการ Error
*   **ExecutorAgent (Doer):** ศูนย์กลางการเรียกใช้เครื่องมือ (Tool Registry) ที่มีความปลอดภัยสูง
*   **CriticAgent (Auditor):** ตรวจสอบผลลัพธ์ของแต่ละขั้นตอนเทียบกับเป้าหมาย

### 👁️ AetoxVision & Control
*   **Deep Analysis:** อ่านและสรุปไฟล์ PDF, Word, Markdown และ Code ได้โดยตรง
*   **Master File Manager:** จัดระเบียบไฟล์และจัดการเส้นทาง (Path Navigation) อัตโนมัติ
*   **Dynamic Tool Discovery:** รองรับการเพิ่มเครื่องมือใหม่โดยไม่ต้องแก้ไขระบบหลัก

---

## 🛠 เทคโนโลยี (Tech Stack)
*   **LLM Engine:** [Ollama](https://ollama.com/) (Qwen 2.5 / Llama 3)
*   **Language:** Python 3.11+ (Asynchronous / Task-oriented)
*   **Design Philosophy:** Stateless Core, Explicit Context, Local-first
*   **Safety:** Built-in Permission Manager & Path Sandbox

---

## 🗄️ โครงสร้างโปรเจกต์ (Project Structure)
```text
AetoxClaw/
├── 📁 aetox/                    # แกนหลักของระบบ
│   ├── 📁 agents/              # เอเจนต์ (Main, Executor, Critic)
│   ├── 📁 core/                # ระบบควบคุม (Dispatcher, PromptEngine, ConfigLoader)
│   ├── 📁 memory/              # ระบบจัดการบริบท (SessionContext)
│   ├── 📁 tools/               # เครื่องมือ (FileManager, WebScraper, Vision)
│   └── 📁 safety/              # ระบบความปลอดภัยและการจัดการสิทธิ์
├── 📁 config/                  # การตั้งค่าเอเจนต์และเครื่องมือ (YAML)
├── 📁 data/                    # ไฟล์ Snapshot และ Temporary (Git Ignored)
├── 📁 docs/                    # เอกสารทางเทคนิคและคู่มือ
└── README.md                   # 📘 เอกสารสรุปโครงการ
```

---

## 💡 ตัวอย่างคำสั่ง
*   *"สรุปเนื้อหาจากไฟล์ PDF ในโฟลเดอร์ดาวน์โหลด"*
*   *"จัดระเบียบหน้า Desktop แยกไฟล์รูปกับไฟล์เอกสารออกจากกัน"*
*   *"หาไฟล์ที่ชื่อว่า report.docx แล้วเปลี่ยนชื่อเป็น final_report.docx"*

---
*Created with ❤️ by Antigravity for the Aetox Ecosystem. Build for Speed, Scale for Intelligence.*
