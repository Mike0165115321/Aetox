# 🌌 AetoxClaw: Architecture & Trinity Intelligence
**The Modern Agentic Orchestration Blueprint**

เอกสารฉบับนี้อธิบายโครงสร้างและกระบวนการทำงานของ AetoxClaw เวอร์ชัน **Trinity Update** ซึ่งเน้นความเร็ว (Async), ความแม่นยำ (Critic), และความจำที่ชาญฉลาด (3-Layer Memory)

---

## 1. High-Level Orchestration (The Trinity Flow)

กระบวนการทำงานหลักถูกขับเคลื่อนด้วยระบบ **Unified Async Orchestration** ซึ่งแยกส่วนการวางแผน (Planning) และการลงมือทำ (Execution) ออกจากกันอย่างชัดเจน

### 🗺️ Data Journey (The Chain of Thought)

```text
[ USER ] ──► [ MAIN AGENT ] ──► [ DISPATCHER ] ──► [ EXECUTOR ] ──► [ CRITIC ]
  (Request)    (Strategic Plan)  (Task Queue)      (Tool Call)      (Audit Result)
                                      ▲                                 │
                                      └─────────────────────────────────┘
                                           (Feedback / Retry Loop)
```

### 🏛️ Component Architecture

1.  **Main Agent (The Planner):** รับคำสั่งภาษาไทยจากผู้ใช้ วิเคราะห์บริบทจาก Memory และวางแผนงานเป็นขั้นตอน (Steps) ในรูปแบบ JSON.
2.  **Dispatcher (The Orchestrator):** หัวใจหลักของการรันงานแบบ Asynchronous ทำหน้าที่จัดการคิวงาน, ควบคุม Timeout, และบริหารจัดการ Retry Loop เมื่อได้รับ Feedback จาก Critic.
3.  **Executor Agent (The Hands):** ทำหน้าที่สกัดพารามิเตอร์ (Intent Extraction) และเรียกใช้เครื่องมือจาก **Unified Tool Registry**.
4.  **Critic Agent (The Auditor):** ตรวจสอบผลลัพธ์ของแต่ละขั้นตอนเทียบกับความตั้งใจของผู้ใช้ หากไม่ผ่านเกณฑ์จะส่ง Feedback (Hints) กลับไปให้ Dispatcher เพื่อสั่ง Retry.

---

## 2. ระบบความจำ 3 ชั้น (3-Layer Hybrid Memory)

AetoxClaw ใช้สถาปัตยกรรมความจำที่จำลองสมองมนุษย์เพื่อแก้ปัญหา Context Window ของโมเดลขนาดเล็ก:

*   **Layer 1: Working Memory (RAM - Fast):** เก็บสถานะปัจจุบัน, ประวัติการทำงานล่าสุด (Async-Safe) และ Context ของงานที่กำลังทำอยู่.
*   **Layer 2: Episodic Memory (Disk - Structured):** บันทึกสรุปงานที่สำเร็จ (Episodes) พร้อม Metadata และ Keywords เพื่อใช้ในการวางแผนงานที่คล้ายคลึงกันในอนาคต.
*   **Layer 3: Long-term Memory (Vector DB - Semantic):** ใช้ BGE-M3 Embedder แปลงข้อมูลจากไฟล์และเว็บเป็นเวกเตอร์ เก็บลง ChromaDB เพื่อทำ RAG (Retrieval-Augmented Generation).

---

## 3. หัวใจของระบบ: Dynamic Tool Discovery

เรายังคงยึดถือมาตรฐาน **Class-Driven Prompting**:
*   เครื่องมือ (Tools) จะต้องรายงานความสามารถของตัวเอง (Self-Reporting) ผ่าน `description` และ `actions`.
*   **Executor** จะกวาดข้อมูลเหล่านี้ไปบอก AI อัตโนมัติ ทำให้ระบบสามารถขยายตัวได้แบบ Zero-Debt.

---

## 4. มาตรฐานความปลอดภัย (Safety Standards)

1.  **Permission Manager:** ตรวจสอบสิทธิ์การเข้าถึงไฟล์และคำสั่งระบบก่อนดำเนินการจริง.
2.  **Path Sandboxing:** จำกัดการทำงานของ File Manager ให้อยู่ในขอบเขตที่ปลอดภัย.
3.  **Ghost Protector:** ป้องกันการรัน Process ซ้อนกันเพื่อความเสถียรของระบบ.

---
*Updated: May 2026 | Created with ❤️ by Antigravity*
