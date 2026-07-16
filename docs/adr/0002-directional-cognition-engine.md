# ADR 0002: Directional Cognition Engine — Multi-AI Architecture

Date: 2026-07-10
Status: Proposed
Scope: Long-term architecture vision — multi-provider orchestration, parallel ensemble, specialist routing, cross-validation consensus

---

# Directional Cognition Architecture — v2.0 (Multi-AI)

> **วันที่:** 10 กรกฎาคม 2026
> **ผู้แต่ง:** Mike (ชยพล พรมสะวะนา)
> **ธีสิส:** เราชนะโมเดล 1 ล้านล้านพารามิเตอร์ได้ ไม่ใช่ด้วยโมเดลของเราเอง — แต่ด้วย **สถาปัตยกรรมที่รวมจุดแข็งของ AI หลายตัว** มาทำงานร่วมกันอย่างชาญฉลาด

---

## ปัญหาที่แก้

| แนวทางเดิม | ปัญหา |
|:--|:--|
| ฝึกโมเดลตัวเอง | งบประมาณระดับสิบล้าน — ไม่ไหว |
| ใช้โมเดลเดียว (GPT-5.6, Mytos 5, Gemini 3) | จ่ายแพง, โดน lock-in, จุดอ่อนของโมเดลนั้นก็คือจุดอ่อนของระบบ |
| Multi-agent (LangGraph crew) | ซับซ้อน, orchestration overhead, แต่ละ agent ยังใช้โมเดลตัวเดียว |
| Simple router (ส่งไป provider ที่เก่งที่สุด) | ไม่ได้ leverage **การทำงานร่วมกัน** — แต่ละคำถามได้แค่คำตอบเดียว |

**Solution:** แทนที่จะใช้ provider ตัวเดียว → ใช้ **หลาย providers** ใน layer ต่าง ๆ กัน โดยแต่ละตัวทำในสิ่งที่มันถนัดที่สุด แล้ว assembly ผลลัพธ์จากทุกตัว

---

## สถาปัตยกรรมรวม

```
┌──────────────────────────────────────────────────────┐
│                    API Gateway                        │ ← REST / GraphQL / gRPC
│              (Rate limit, Auth, Logging)              │
├──────────────────────────────────────────────────────┤
│                                                       │
│              Orchestrator Layer                       │ ← ตัวตัดสินใจ —
│  ┌──────────────────────────────────────────────┐     │   คำถามนี้ต้องใช้
│  │  Question Analyzer     │  Intent Classifier  │     │   architecture แบบไหน?
│  │  Task Decomposer       │  Route Planner      │     │
│  └──────────────────────────────────────────────┘     │
├──────────────────────────────────────────────────────┤
│                                                       │
│         Reasoning Layer (Multi-Provider)              │ ← ส่งไปหลาย providers
│  ┌──────────┐ ┌──────────┐ ┌──────────┐              │   พร้อมกัน
│  │ Provider │ │ Provider │ │ Provider │              │   แต่ละตัวตอบ
│  │    A     │ │    B     │ │    C     │              │   ในมุมของตัวเอง
│  └──────────┘ └──────────┘ └──────────┘              │
│         ↓                                            │
│  ┌──────────────────────────────────────────────┐     │
│  │     Response Comparator & Cross-Validator     │     │ ← เปรียบเทียบ, หา conflict,
│  │     Conflict Resolver   │  Consensus Mgr      │     │   weighted vote
│  └──────────────────────────────────────────────┘     │
├──────────────────────────────────────────────────────┤
│                                                       │
│         Synthesis / Assembly Layer                    │ ← รวมคำตอบจากหลายทาง
│  ┌──────────────────────────────────────────────┐     │   สร้างคำตอบสุดท้าย
│  │  Evidence Merger    │  Citation Builder       │     │   พร้อม traceability
│  │  Confidence Scorer  │  Uncertainty Reporter   │     │
│  └──────────────────────────────────────────────┘     │
├──────────────────────────────────────────────────────┤
│                                                       │
│         Provider Runtime Layer                        │ ← abstraction layer
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐       │   เสียบ/ถอด provider ได้
│  │OpenAI│ │Anthro│ │Google│ │DeepS │ │Open  │       │   โดยไม่กระทบ layer บน
│  │      │ │ pic  │ │Gemini│ │ eek  │ │ Src  │       │
│  └──────┘ └──────┘ └──────┘ └──────┘ └──────┘       │
└──────────────────────────────────────────────────────┘
```

---

## แต่ละ Layer — Concept

### 1. API Gateway
- Interface สำหรับ service อื่นเรียกใช้
- Input: prompt + metadata (desired latency, cost tier, required confidence)
- Output: response + confidence score + provider trace
- Authentication, rate limiting, logging

### 2. Orchestrator Layer (ชั้นสำคัญที่สุด)
**นี่คือที่ "Directional Cognition" อยู่จริง**

| Component | หน้าที่ |
|:--|:--|
| **Question Analyzer** | วิเคราะห์ประเภทคำถาม — factual/reasoning/creative/coding/planning |
| **Intent Classifier** | รู้ว่า user ต้องการอะไร — ไม่ใช่แค่ prompt literal |
| **Task Decomposer** | แยกคำถามซับซ้อนเป็น subtask |
| **Route Planner** | ตัดสินใจ: ส่ง parallel หรือ sequential? ใช้กี่ provider? เอาแบบเร็วหรือถูกหรือแม่น? |

**Route Strategy Options:**
- **Parallel Ensemble** — ส่งให้ N providers, vote หาคำตอบที่ดีสุด → accuracy สูง
- **Sequential Refinement** — Provider A → Provider B refine → accuracy + depth
- **Specialist Routing** — โจทย์ code → Claude + GPT, โจทย์ logic → DeepSeek, โจทย์ multimodal → Gemini
- **Debate** — ส่ง prompt เดียวให้ 2 providers แล้วให้คนที่ 3 เป็น judge

### 3. Reasoning Layer
ของใหม่เมื่อเทียบกับ v1.0 — ไม่ใช่ layer เดียว แต่เป็น **multi-provider parallel reasoning**

แต่ละ provider ได้ prompt *เดียวกัน* หรือ *ต่างมุม* กัน แล้วส่งผลลัพธ์มาที่:
- **Response Comparator** — ดูความแตกต่าง, similarity score
- **Conflict Resolver** — ถ้า provider ขัดแย้งกัน, ใช้ voting หรือ confidence weight
- **Consensus Manager** — ถ้าทุกตัวเห็นตรงกัน → confidence สูง, ถ้าไม่ตรง → อาจต้องวนรอบ debate

### 4. Synthesis / Assembly Layer
- **Evidence Merger** — รวมข้อมูลจากทุก provider, deduplicate
- **Citation Builder** — ติด tag ว่าข้อมูลนี้มาจาก provider ไหน (transparency)
- **Confidence Scorer** — คะแนนความมั่นใจของคำตอบ (ขึ้นกับ consensus level)
- **Uncertainty Reporter** — ถ้าข้อมูลขัดแย้งหรือไม่มี consensus → รายงาน uncertainty กลับไป

### 5. Provider Runtime
- Abstraction layer — interface เดียวกันทุก provider
- เสียบเข้า/ถอดได้โดยไม่กระทบ layer บน
- แต่ละ provider มี metadata: cost/1M tokens, latency profile, strengths/weaknesses, max context
- **Dynamic provider registry** — ถ้า Mytos 6 ออกมา, แค่เพิ่ม provider entry

---

## กลยุทธ์ "Beat Mytos 5 / GPT-5.6"

คำถาม $64,000 — architecture นี้ชนะโมเดลเดี่ยวได้ยังไง?

| กลไก | หลักการ | ทำไมชนะ |
|:--|:--|:--|
| **Parallel Ensemble** | ส่งให้ 3-5 providers → vote | ถ้าความแม่นยำต่อตัว p=0.85, ensemble 5 ตัวได้ >0.99 |
| **Specialist Routing** | แต่ละ provider ถนัดคนละอย่าง | รวมจุดแข็ง, ตัดจุดอ่อน |
| **Cross-Validation** | Provider A ตรวจคำตอบ Provider B | จับ hallucination ได้ดีกว่า single model |
| **Confidence Scoring** | รู้ว่าตัวเองไม่รู้ | ตัดสินใจส่ง provider เพิ่มหรือตอบว่า "ไม่แน่ใจ" |
| **Cost Distribution** | ใช้โมเดลถูกสำหรับ routing, โมเดลแพงสำหรับ deep reasoning | จ่ายเท่า GPT-5.6 ได้ quality ที่เหนือกว่า |

**ตัวอย่าง:** Mytos 5 อาจแม่นเรื่อง factual knowledge — แต่ถาม logic puzzle ที่ซับซ้อน, DeepSeek R1 (ราคาถูกกว่า 50x) อาจตอบได้ดีกว่า + Claude ตรวจทานอีกครั้ง = ชนะ Mytos 5 ทั้ง accuracy และ cost

---

## API Output — ตัวอย่าง

```json
{
  "id": "req_abc123",
  "answer": "...",
  "confidence": 0.92,
  "providers_used": ["openai/gpt-5.6", "anthropic/claude-4.6", "google/gemini-3-pro"],
  "consensus": "unanimous",
  "reasoning_trace": {
    "route": "parallel_ensemble",
    "provider_results": [
      { "provider": "openai/gpt-5.6", "confidence": 0.88 },
      { "provider": "anthropic/claude-4.6", "confidence": 0.94 },
      { "provider": "google/gemini-3-pro", "confidence": 0.91 }
    ],
    "latency_ms": 2450,
    "cost_usd": 0.042
  },
  "uncertainty_report": null
}
```

---

## เปรียบเทียบ v1.0 → v2.0

| มิติ | v1.0 (วันนี้) | v2.0 (ที่คุยนี้) |
|:--|:--|:--|
| ผู้เล่น | ชั้น conceptual 6 ชั้น | สถาปัตยกรรมระบบจริง |
| Core thesis | Architecture > Parameters | Multi-AI Collaboration > Single Model |
| Provider | เปลี่ยนได้แต่ทีละตัว | **ใช้หลายตัวพร้อมกัน** |
| Competition | ไม่ได้พูด | **Beat GPT-5.6 / Mytos 5** |
| Output | conceptual | **API first** |
| Key mechanism | Directional Cognition | Parallel Ensemble + Specialist Routing |
| Cost model | ไม่ได้พูด | Leverage ราคา model ที่ถูกลง |

---

## สถานะ

- ✅ สถาปัตยกรรมรวม — v2.0 draft
- ❌ ยังไม่มีโค้ด
- 🔜 ขั้นตอนถัดไป:
  1. experiment จริง — เลือก provider 3-5 ตัว, เขียน orchestrator บน Local
  2. ทดสอบกับชุดคำถาม benchmark (MMLU, GSM8K, HumanEval)
  3. วัดต้นทุน vs GPT-5.6 ล้วน ๆ
  4. ปรับ Route Strategy ตามข้อมูลจริง

สิ่งที่ Aetox ต้องเป็น (ทั้งหมด)
┌──────────────────────────────────────────┐
│         Aetox Desktop (UI)               │ ← Wails + TS desktop cockpit
│   visualize agents, logs, cost, sessions │
├──────────────────────────────────────────┤
│                                          │
│    Multi-Agent Orchestration             │ ← Supervisor วางแผน →
│    ┌──────┐ ┌──────┐ ┌──────┐           │    dispatch ให้ specialist
│    │Coding│ │DevOps│ │Research│          │    แต่ละตัวใช้ Directional Cognition
│    └──────┘ └──────┘ └──────┘           │
│                                          │
├──────────────────────────────────────────┤
│    Directional Cognition Engine          │ ← Parallel ensemble, voting, synthesis
│    Specialist Routing, Cross-Validation  │
├──────────────────────────────────────────┤
│    Agent Core Runtime                    │ ← มีแล้ว: providers, tools, turn loop
│    + Plugin System + MCP Client          │
├──────────────────────────────────────────┤
│    Terminal + File Operations            │ ← shell, git, read/write, search