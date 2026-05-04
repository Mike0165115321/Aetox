# AetoxOS: มาตรฐานการสร้างเครื่องมือ (Tool Standard) - Dynamic Edition

เอกสารฉบับนี้กำหนดโครงสร้างมาตรฐานสำหรับการสร้างเครื่องมือ (Tools) ใหม่ใน AetoxOS เพื่อให้ระบบคงความยืดหยุ่น (Flexibility) และรองรับการขยายตัว (Scalability) ได้อย่างไร้รอยต่อผ่านระบบ **Dynamic Tool Discovery**

---

## 1. โครงสร้างพื้นฐาน (Core Architecture)
ทุกเครื่องมือต้องสืบทอดคุณสมบัติ (Inherit) จากคลาส `BaseTool` ใน `aetox/tools/base.py`

### ส่วนประกอบที่สำคัญ:
1. **Name:** ชื่อเครื่องมือ (ภาษาอังกฤษ lowercase)
2. **Description:** คำอธิบายหน้าที่ (ภาษาไทย) AI จะใช้ข้อมูลนี้เป็นหลักในเบื้องต้น
3. **Actions:** รายการคำสั่งที่เครื่องมือนี้รองรับ (List of Strings)
4. **get_prompt_doc():** (สำคัญมาก) เมธอดสำหรับส่งคืนคู่มือการใช้งานแบบละเอียดและตัวอย่าง JSON เพื่อสอน AI (Prompt Injection)
5. **Execute Method:** จุดรับคำสั่งและพารามิเตอร์จาก AI

---

## 2. รูปแบบการเขียนคลาส (Class Structure)

```python
from typing import Dict, Any
from aetox.tools.base import BaseTool

class YourNewTool(BaseTool):
    def __init__(self):
        super().__init__(
            name="your_tool_name",
            description="อธิบายสั้นๆ ว่าเครื่องมือนี้ทำอะไร",
            actions=["action_1", "action_2"]
        )

    def get_prompt_doc(self) -> str:
        """ส่งคืนคำแนะนำแบบละเอียดเพื่อให้ AI ใช้งานเครื่องมือได้อย่างแม่นยำ"""
        return (
            f"Tool: {self.name}\n"
            f"หน้าที่: อธิบายหน้าที่แบบละเอียด พร้อมเงื่อนไขการใช้\n"
            f"คำสั่ง: {', '.join(self.actions)}\n"
            f"ตัวอย่าง JSON:\n"
            f'  {{"tool": "{self.name}", "action": "action_1", '
            f'"params": {{"target": "value"}}, "confidence": 0.95}}\n'
            f"ใช้เมื่อ: ระบุคีย์เวิร์ดที่ผู้ใช้มักจะสั่ง\n"
        )

    def execute(self, params: Dict[str, Any]) -> Dict[str, Any]:
        action = params.get("action")
        target = params.get("target") or params.get("path")

        if action == "action_1":
            return self._logic_function(target)

        return {"status": "failure", "error": f"ไม่รู้จักคำสั่ง: {action}"}
```

---

## 2.1 สถาปัตยกรรมแบบเลเยอร์ (Layered Tool Architecture)
เพื่อให้เครื่องมือมีความเสถียรและบำรุงรักษาง่าย เครื่องมือที่ซับซ้อนควรจัดเลเยอร์ภายในดังนี้:

1. **Layer 1: Router (การคัดแยก)**: อยู่ในเมธอด `execute()` ทำหน้าที่รับพารามิเตอร์และส่งต่อไปยังเมธอดที่เหมาะสมเท่านั้น
2. **Layer 2: Atomic Actions (การทำงานพื้นฐาน)**: เมธอดที่จัดการงานพื้นฐาน (เช่น `_handle_create`, `_handle_move`) เน้นความแม่นยำ
3. **Layer 3: Intelligent Engine (ระบบประมวลผลขั้นสูง)**: เมธอดที่ทำงานซับซ้อน (เช่น `_organize_directory`) แยกส่วนนี้ออกจากงานพื้นฐานเพื่อความปลอดภัย

---

## 3. มาตรฐานการรับ-ส่งข้อมูล (Input/Output Standard)

### การรับข้อมูล (Input):
- รับพารามิเตอร์ในรูปแบบ `Dict[str, Any]` (JSON จาก AI)
- ชื่อตัวแปรควรใช้มาตรฐานกลาง: `path`, `target`, `targets`, `message`

### การส่งข้อมูล (Output):
ต้องคืนค่าเป็น `Dict` ที่มีโครงสร้างดังนี้เสมอ:
- `status`: "success" หรือ "failure"
- `output`: ข้อความสรุปผลงานสำหรับแจ้งผู้ใช้ (ถ้าสำเร็จ)
- `error`: ข้อความอธิบายสาเหตุ (ถ้าล้มเหลว)

---

## 4. ขั้นตอนการลงทะเบียน (Dynamic Discovery)
ระบบ AetoxOS ใช้การสแกนไฟล์อัตโนมัติ (**Auto-scan**) คุณไม่จำเป็นต้องแก้ไข `ExecutorAgent` อีกต่อไป:
1. สร้างไฟล์เครื่องมือใหม่ในโฟลเดอร์ `aetox/tools/` (เช่น `my_new_tool.py`)
2. ตรวจสอบว่าคลาสของคุณสืบทอดจาก `BaseTool`
3. ระบบ `ToolLoader` จะตรวจพบไฟล์ใหม่และลงทะเบียนเข้าสู่ `ToolRegistry` โดยอัตโนมัติเมื่อ Start ระบบ
4. คู่มือใน `get_prompt_doc()` จะถูกฉีดเข้าไปใน Prompt ของ AI โดยอัตโนมัติผ่าน placeholder `{tools}`

---

## 5. กฎเหล็ก (Best Practices)
- **Examples are King:** ใน `get_prompt_doc()` ควรให้ตัวอย่าง JSON ที่ถูกต้อง เพื่อลดโอกาสที่ AI จะส่งพารามิเตอร์มาผิดรูปแบบ
- **Atomic Actions:** หนึ่ง Action ควรทำงานอย่างเดียวให้จบ (Single Responsibility)
- **Error Handling:** ต้องมี `try-except` ครอบ Logic หลักเสมอ เพื่อไม่ให้ระบบค้าง
- **No Manual Imports:** หลีกเลี่ยงการ Import Tool ข้ามกันโดยตรง ให้ใช้ผ่านระบบ Registry ของ Agent แทน
