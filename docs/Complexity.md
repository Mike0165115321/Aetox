# System Complexity & Resource Usage (Lightweight Edition)

## 🧠 Memory Usage (RAM)
การใช้ทรัพยากรหลังจาก Refactor ระบบให้เบาลง:

| Component | Complexity | Usage (Est.) | Note |
|-----------|------------|--------------|------|
| **SessionContext** | O(K) | < 10 MB | K = ประวัติ 5 ข้อความล่าสุด |
| **Tool Buffers** | O(B) | 5 - 20 MB | B = ข้อมูลชั่วคราว (เช่น เนื้อหาเว็บ) |
| **LLM Context** | O(C) | ~1.2 GB | C = Context Window (8K tokens) |
| **Python Runtime**| - | ~150 MB | Core system overhead |
| **Total RAM** | - | **~1.4 - 1.6 GB** | **ลดลงจากเดิม ~2.5 GB** |

*หมายเหตุ: ไม่รวม VRAM ที่โมเดล (Ollama) ใช้งาน*

## 🎮 VRAM Usage (Example: RTX 4060 8GB)
เมื่อใช้งานร่วมกับโมเดล 8B (Quantized):

- **Qwen 2.5 / 3 (8B - Q4_K_M):** ~5.2 - 5.8 GB
- **System Reserve:** ~1.5 GB
- **Total VRAM:** ~6.7 - 7.3 GB
- **Remaining VRAM:** ~0.7 GB → **เพียงพอสำหรับ Context และงานทั่วไป ✅**

## ⚡ Performance Matrix
- **Inference Time:** ขึ้นอยู่กับความเร็วของ GPU (เฉลี่ย 30-50 tokens/s)
- **Context Injection:** O(1) (รวดเร็วมากเพราะไม่มีการประมวลผล Vector)
- **Cold Start:** < 2 วินาที (รวดเร็วเพราะไม่ต้องโหลดโมเดล Embedder)

---
*Updated: May 2026 | Focus on Lightweight Efficiency*