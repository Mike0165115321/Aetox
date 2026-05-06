# 🌌 AetoxClaw Architecture: Stateless Core & History Injection
**The Lightweight Orchestration Blueprint**

เอกสารฉบับนี้อธิบายโครงสร้างเวอร์ชัน **Lightweight Refactor** ซึ่งเน้นความเร็วสูงสุด ประหยัดทรัพยากร และการส่งต่อบริบทที่แม่นยำ

---

## 1. High-Level Orchestration (The Stateless Flow)

AetoxClaw ใช้สถาปัตยกรรมแบบ **Stateless Core** โดยที่เอเจนต์แต่ละตัวจะไม่เก็บสถานะ (State) ไว้ถาวร แต่จะได้รับบริบท (Context) ผ่านการฉีดข้อมูล (Injection) จากตัวกลาง (Dispatcher)

### 🗺️ Data Journey (The Chain of Thought)

```text
[ USER ] ──► [ MAIN AGENT ] ──► [ DISPATCHER ] ──► [ EXECUTOR ] ──► [ CRITIC ]
  (Request)    (Plan Steps)      (Inject History)  (Tool Call)      (Result Audit)
                                       ▲                                 │
                                       └─────────────────────────────────┘
                                            (Feedback / Retry Loop)
```

### 🏛️ Core Components

1.  **Main Agent (The Planner):** ทำหน้าที่วางแผนงาน (Planning) เป็นขั้นตอน โดยไม่ต้องสนใจประวัติที่ซับซ้อน เน้นที่เป้าหมายปัจจุบันของผู้ใช้
2.  **Dispatcher (The Orchestrator):** เป็น "หัวใจ" ของระบบ ทำหน้าที่บริหารจัดการบริบท (Context Management) โดยจะตัดสินใจว่าควรส่งประวัติอะไรให้ Executor ในแต่ละจังหวะ
3.  **Executor Agent (The Hands):** รับบริบทที่ถูกฉีดเข้ามา (Injected History) และเรียกใช้เครื่องมือตามคำสั่ง
4.  **Critic Agent (The Auditor):** ตรวจสอบผลลัพธ์เทียบกับเป้าหมายในก้าวนั้นๆ

---

## 2. ระบบจัดการบริบท (Smart History Injection)

เพื่อแก้ปัญหาเรื่อง Context Bloat และการกินทรัพยากร เรายกเลิกระบบ RAG/Summarize อัตโนมัติ และเปลี่ยนมาใช้ระบบฉีดบริบทตามความจำเป็น:

### 📱 Chat Mode (การสนทนาทั่วไป)
*   **Strategy:** Sliding Window History.
*   **Logic:** ส่งประวัติการคุยล่าสุด **3-5 ชุด** (ถาม-ตอบ) เพื่อให้ระบบยังคงจำเรื่องที่คุยกันล่าสุดได้ โดยไม่ทำให้ Prompt ยาวเกินไปจนโมเดลสับสน

### 📝 Plan Mode (การทำงานหลายขั้นตอน)
*   **Strategy:** Dynamic Task History.
*   **Logic:** 
    *   ส่ง **ผลลัพธ์ของขั้นตอนก่อนหน้า (Immediate Context)** เพื่อความต่อเนื่อง
    *   ส่ง **สรุปสถานะของแผนงาน (Plan Summary)** ว่าขั้นตอนไหนสำเร็จหรือล้มเหลวไปแล้วบ้าง
    *   *ไม่ส่ง* ผลลัพธ์ดิบ (Full Output) ของทุกขั้นตอน เพื่อประหยัด Token

---

## 3. Lightweight Session Context

`SessionContext` ถูกนำมาแทนที่ `WorkingMemory` เพื่อลด Overhead:
*   **No Background Tasks:** ไม่มีระบบ Summarization หรือ Embedding ที่รันใน Background
*   **Memory Efficiency:** เก็บข้อมูลเป็น Plain List ใน RAM ทำให้เข้าถึงได้เร็วและกินทรัพยากรต่ำสุด
*   **Task Isolation:** ประวัติของแต่ละงานจะถูกจัดการแยกกันอย่างชัดเจน

---

## 4. มาตรฐานความปลอดภัย (Safety)

1.  **Permission Manager:** ตรวจสอบสิทธิ์การเข้าถึงก่อนรันเครื่องมือ
2.  **Path Sandboxing:** จำกัดขอบเขตการทำงานของ File Tool
3.  **Timeout Protection:** ป้องกันการรันงานค้างในระบบด้วยการกำหนดเวลาตายตัว

---
*Updated: May 2026 | Refactored for Speed*
