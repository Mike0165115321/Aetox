# 📂 โครงสร้างโปรเจกต์ AetoxClaw (Project Structure)

โครงสร้างไฟล์และโมดูลของระบบ AetoxClaw เวอร์ชัน **Lightweight Core**

---

## 🏗️ แผนผังโฟลเดอร์ (Directory Tree)

```text
AetoxClaw/
├── 📁 aetox/                    # 🧠 แกนหลักของระบบ (Core Logic)
│   ├── 📁 agents/              # เอเจนต์ผู้เชี่ยวชาญด้านต่างๆ
│   │   ├── main_agent.py       # ผู้วางแผนงานและรับคำสั่งหลัก
│   │   ├── executor.py         # ผู้ลงมือทำ (เรียกใช้เครื่องมือ)
│   │   └── critic.py           # ผู้ตรวจสอบผลงาน (Auditor)
│   ├── 📁 core/                # ระบบควบคุมส่วนกลาง
│   │   ├── dispatcher.py       # ตัวจัดการงานแบบ Async และ History Injection
│   │   ├── ollama_client.py    # ตัวเชื่อมต่อกับ Ollama API
│   │   ├── prompt_engine.py    # ระบบจัดการ Template ของ Prompt
│   │   └── config_loader.py    # ตัวโหลดการตั้งค่าจาก YAML
│   ├── 📁 memory/              # ระบบจัดการบริบท (Context Management)
│   │   └── working.py          # SessionContext (เก็บประวัติชั่วคราวใน RAM)
│   ├── 📁 tools/               # เครื่องมือที่เอเจนต์เรียกใช้ได้ (Tools)
│   │   ├── file_manager.py     # จัดการไฟล์และโฟลเดอร์
│   │   ├── web_scraper.py      # ดึงข้อมูลจากเว็บไซต์
│   │   ├── vision.py           # อ่านและวิเคราะห์ไฟล์เอกสาร (PDF/Word)
│   │   └── registry.py         # ศูนย์รวมการลงทะเบียนเครื่องมือ
│   └── 📁 safety/              # ระบบความปลอดภัย
│       └── permission.py       # ตัวจัดการสิทธิ์และการขออนุมัติ
├── 📁 config/                  # ⚙️ ไฟล์ตั้งค่า (Configuration)
│   ├── models.yaml             # ตั้งค่าโมเดลและ Session
│   └── prompts/                # โฟลเดอร์เก็บ System Prompts
├── 📁 data/                    # 💾 โฟลเดอร์เก็บข้อมูลชั่วคราว (Git Ignored)
├── 📁 docs/                    # 📘 เอกสารประกอบการใช้งาน
├── 📁 tests/                   # 🧪 ระบบทดสอบ (Pytest)
├── main.py                     # 🚀 จุดรันโปรแกรมหลัก (CLI Demo)
├── health_check.py             # 🏥 ตรวจสอบความพร้อมของระบบ
└── requirements.txt            # 📦 รายการไลบรารีที่ต้องใช้
```

---

## 🔍 คำอธิบายโมดูลสำคัญ

### 1. `aetox/core/dispatcher.py`
เป็นโมดูลที่สำคัญที่สุดในเวอร์ชันปัจจุบัน ทำหน้าที่เป็น "ศูนย์กลาง" ในการรันงานแบบ Asynchronous และจัดการการส่งประวัติ (History) ให้กับเอเจนต์แต่ละตัวอย่างเหมาะสมตามประเภทงาน (Chat vs Plan)

### 2. `aetox/memory/working.py` (SessionContext)
เก็บประวัติการคุยและการทำงานล่าสุดไว้ใน RAM รูปแบบ Plain List ทำให้ระบบทำงานได้รวดเร็วมากและไม่กินทรัพยากรเครื่องเหมือนระบบ RAG เดิม

### 3. `aetox/agents/executor.py`
เอเจนต์ตัวนี้ถูกปรับปรุงให้เป็นแบบ **Stateless** คือไม่จำอะไรด้วยตัวเอง แต่จะรอรับคำสั่งและประวัติที่ส่งมาจาก Dispatcher เท่านั้น ทำให้โค้ดสะอาดและดูแลง่าย

### 4. `aetox/tools/`
ศูนย์รวมความสามารถของระบบ โดยเครื่องมือแต่ละตัวจะถูกแยกเป็นโมดูลอิสระ (Modular Design) สามารถเพิ่มหรือลดเครื่องมือได้โดยไม่กระทบต่อระบบหลัก

---
*ปรับปรุงล่าสุด: พฤษภาคม 2026 | สถาปัตยกรรมเน้นประสิทธิภาพสูง*