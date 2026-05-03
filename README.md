# 🌌 AetoxOS
**The Agentic Local OS Orchestrator (Master Edition)**

AetoxOS คือระบบปฏิบัติการ AI แบบ Agentic ที่ทำงานบนเครื่องของคุณโดยสมบูรณ์ (Local-first) ออกแบบมาเพื่อเป็นสมองกลางในการจัดการไฟล์ วางแผนงาน และควบคุมระบบปฏิบัติการผ่านคำสั่งภาษาไทยที่ยืดหยุ่น

---

## 🚀 ความสามารถในปัจจุบัน (Current Capabilities)

### 🧠 Core Brain (Multi-Agent Ready)
*   **Dynamic Intent Extraction**: สกัดพารามิเตอร์และเลือกเครื่องมืออัตโนมัติด้วยระบบ **Dynamic Tool Discovery** (ไม่ต้องแก้พรอมต์เมื่อเพิ่มเครื่องมือ)
*   **Neutral Prompting**: ระบบพรอมต์ที่เป็นกลาง ลดหนี้ทางเทคนิค (No Technical Debt) โดยแยกการตั้งค่าไว้ใน YAML
*   **Short-term Memory**: จดจำบริบทการสนทนาและพาธล่าสุด 3 ขั้นตอน (Rolling Buffer)
*   **Ghost Protection**: ระบบ Single-Instance ป้องกันบอทรันซ้ำซ้อนอัตโนมัติ

### 👁️ AetoxVision (Intelligence)
*   **Deep Document Analysis**: อ่านและสรุปเนื้อหาจากไฟล์ **PDF (สูงสุด 20 หน้า)**, **Word (.docx)**, **Markdown (.md)** และไฟล์โค้ดต่างๆ
*   **Super Summary**: สรุปเนื้อหาใจความสำคัญแบบ "สรุปขั้นสุด" ไม่พ่นข้อความรกหน้าจอ
*   **ASCII Tree View**: แสดงโครงสร้างโฟลเดอร์ในรูปแบบแผนผังที่สวยงามและชัดเจน

### 🕹️ AetoxControl (Execution)
*   **Application Control**: สั่งเปิดโปรแกรมในเครื่องได้โดยตรง (เช่น Notepad, Calculator, Chrome)
*   **Multi-Launch**: รองรับการสั่งเปิดหลายแอปพลิเคชันพร้อมกันในคำสั่งเดียว
*   **Master File Manager**: จัดระเบียบไฟล์จำนวนมากแยกตามหมวดหมู่ (Images, Documents, Code, etc.) อัตโนมัติ

---

## 🛠 เทคโนโลยีเบื้องหลัง (Architecture)
*   **Language Models**: Ollama (Qwen 2.5:14b สำหรับงานวิเคราะห์ / 7b สำหรับงานสกัดคำสั่ง)
*   **Interface**: **Discord Bot** (Command Center หลัก)
*   **Backend**: Python 3.11+ (Windows Optimized)
*   **Documentation**: มีมาตรฐานการสร้าง Tool ([tool_standard.md](aetox/tools/doc/tool_standard.md))

---

## 📋 แผนการพัฒนา (Roadmap)
*   [x] **Interface**: Discord Bot Integration
*   [x] **File Intelligence**: PDF/Word/MD Summarization
*   [ ] **Phase 3 (Trinity)**: WebPulse (Web Search & Navigation)
*   [ ] **Phase 4**: Multi-Agent Orchestration (Planner, Researcher, Critic)
*   [ ] **Phase 5**: Desktop GUI (Aetox Dashboard)

---

## 💡 ตัวอย่างคำสั่งที่ใช้งานได้
*   *"เข้าไปดูใน Documents หน่อย มีอะไรอยู่ข้างในบ้าง"*
*   *"สรุปไฟล์ Aetox_ข้อเสนอโครงการ.docx ให้ผมเข้าใจที แบบสั้นที่สุดนะ"*
*   *"เปิด Notepad กับเครื่องคิดเลขขึ้นมาหน่อย"*
*   *"จัดระเบียบไฟล์ในหน้า Desktop ให้เข้าที่ให้หมด"*

---
*Created with ❤️ by Antigravity for the Aetox Ecosystem*