AetoxClaw/
├── 📁 aetox/                    # Core Source Code
│   ├── 📁 agents/              # Agent Components
│   │   ├── intent_extractor.py # 🧠 สกัดความตั้งใจจากผู้ใช้
│   │   ├── executor.py         # ⚡ รันแอ็กชัน/เรียกเครื่องมือ
│   │   ├── critic.py           # 🔍 ตรวจสอบคุณภาพผลลัพธ์
│   │   └── base.py             # 🧱 Base class สำหรับ Agent
│   ├── 📁 core/                # ระบบหลัก
│   │   ├── dispatcher.py       # 🎯 Orchestrator หลัก (Async)
│   │   ├── ollama_client.py    # 🔌 เชื่อมต่อ Ollama API
│   │   └── prompt_engine.py    # 📝 จัดการพรอมต์แบบไดนามิก
│   ├── 📁 memory/              # ระบบความจำ
│   │   ├── working.py          # 💾 WorkingMemory (RAM + Disk)
│   │   ├── vector_store.py     # 🗂️ Vector DB (BGE-M3 + Chroma)
│   │   └── embedder.py         # 🧮 Embedding Engine
│   ├── 📁 tools/               # เครื่องมือภายนอก
│   │   ├── base.py             # 🧱 BaseTool Interface
│   │   ├── loader.py           # 🔍 Dynamic Tool Discovery
│   │   ├── web_scraper.py      # 🌐 WebPulse Scraper
│   │   ├── file_manager.py     # 🗂️ Master File Manager + PathNavigator
│   │   └── safety.py           # 🛡️ Safety Checker
│   └── 📁 utils/               # Utility Functions
├── 📁 config/                  # Configuration
│   ├── models.yaml             # 🤖 Model assignments + parameters
│   └── tools.yaml              # 🔧 Tool registration + prompts
├── 📁 data/                    # Persistent Storage (gitignored)
│   ├── tasks/                  # 💾 WorkingMemory snapshots
│   ├── vector_db/              # 🗂️ ChromaDB index
│   └── episodes.jsonl          # 📜 Episodic memory logs
├── 📁 docs/                    # Documentation
│   └── tool_standard.md        # 📋 มาตรฐานการสร้าง Tool
├── main.py                     # 🚀 Entry Point
├── requirements.txt            # 📦 Dependencies
└── README.md                   # 📘 Project Overview