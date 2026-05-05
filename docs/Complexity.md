Runtime Memory Usage:
├── WorkingMemory:        O(K)        (K = ขนาด context ที่จำกัด ~6K tokens)
├── Tool Buffers:         O(B)        (B = ขนาดข้อมูลชั่วคราว ~1-2 MB)
├── LLM Context:          O(C)        (C = context window ~8K tokens)
├── Embedding Cache:      O(E)        (E = จำนวนเวกเตอร์ที่ cache)
└── Python Overhead:      ~200 MB

Total RAM: ~1.5-2.5 GB (ไม่รวม VRAM ของโมเดล)

VRAM Usage (RTX 4060 8GB):
├── Qwen3-8B (Q4):        ~5.8 GB
├── BGE-M3 (CPU):         ~0 GB     (รันบน CPU)
├── ChromaDB Index:       ~0.1 GB   (ใน RAM)
└── System Reserve:       ~1.5 GB
          ↓
เหลือว่างสำหรับ Context: ~0.6 GB → เพียงพอสำหรับงานส่วนใหญ่ ✅