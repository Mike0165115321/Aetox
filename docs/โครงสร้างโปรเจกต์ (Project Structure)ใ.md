# 🗂️ โครงสร้างโปรเจกต์ (Project Structure)

โครงสร้างไฟล์และโฟลเดอร์ของ AetoxClaw เวอร์ชัน Trinity

```text
AetoxClaw/
├── 📁 aetox/                    # แกนหลัก (Source Code)
│   ├── 📁 agents/              # เอเจนต์ผู้ชำนาญการ
│   │   ├── main_agent.py       # 🧠 Planner & Delegator
│   │   ├── executor.py         # ⚡ ศูนย์กลางการเรียก Tool
│   │   ├── critic.py           # 🔍 ผู้ตรวจสอบคุณภาพงาน
│   │   └── base.py             # 🧱 โครงสร้างพื้นฐานเอเจนต์
│   ├── 📁 core/                # ระบบควบคุมส่วนกลาง
│   │   ├── dispatcher.py       # 🎯 Orchestrator (Async)
│   │   ├── ollama_client.py    # 🔌 ตัวเชื่อมต่อ Ollama
│   │   ├── prompt_engine.py    # 📝 จัดการพรอมต์
│   │   └── config_loader.py    # ⚙️ โหลดค่าตั้งค่า (YAML)
│   ├── 📁 memory/              # ระบบความจำ 3 ชั้น
│   │   ├── working.py          # 💾 RAM Context (L1)
│   │   ├── episodic.py         # 📜 History Log (L2)
│   │   ├── vector_store.py     # 🗂️ Semantic DB (L3)
│   │   └── embedder.py         # 🧮 ตัวแปลงเวกเตอร์ (BGE-M3)
│   ├── 📁 tools/               # เครื่องมือและความสามารถ
│   │   ├── loader.py           # 🔍 ตัวโหลด Tool แบบไดนามิก
│   │   ├── file_manager.py     # 🗂️ จัดการไฟล์ (Master)
│   │   └── web_scraper.py      # 🌐 ดึงข้อมูลเว็บ (WebPulse)
│   └── 📁 safety/              # ความปลอดภัย
│       └── permission.py       # 🛡️ ตัวจัดการสิทธิ์
├── 📁 config/                  # ไฟล์ตั้งค่า (YAML)
├── 📁 data/                    # ฐานข้อมูลและไฟล์เก็บข้อมูล (Disk)
├── 📁 docs/                    # เอกสารประกอบการพัฒนา
├── main.py                     # 🚀 จุดเริ่มต้นโปรแกรม
├── requirements.txt            # 📦 รายการไลบรารี
└── README.md                   # 📘 คู่มือภาพรวม
```

---
*Updated: May 2026*