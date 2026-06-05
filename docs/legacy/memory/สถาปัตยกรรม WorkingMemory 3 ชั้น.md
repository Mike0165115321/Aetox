┌─────────────────────────────────────┐
│  🧠 Layer 1: Working Memory (RAM)   │
│  • Context ปัจจุบัน (≤4K tokens)    │
│  • ข้อมูลที่กำลังประมวลผลอยู่      │
│  • Fast access, volatile            │
└─────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────┐
│  🗂️ Layer 2: Episodic Memory (Disk) │
│  • สรุปงานย่อยเป็น "Episodes"       │
│  • Metadata: {task_id, summary,     │
│               keywords, timestamp}  │
│  • ค้นหาด้วย Keyword / Semantic     │
└─────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────┐
│  🗄️ Layer 3: Long-term Memory (Vector DB) │
│  • ข้อมูลดิบจากเว็บ / ไฟล์ใหญ่      │
│  • แบ่งเป็น Chunks + Embedding      │
│  • ค้นหาด้วย Semantic Similarity    │
└─────────────────────────────────────┘