from typing import Dict, Any
from aetox.tools.base import BaseTool

class SystemControl(BaseTool):
    """
    Standard tool for system control and general communication.
    """
    def __init__(self, registry=None):
        super().__init__(
            name="system_control",
            description="จัดการการตอบโต้ทั่วไป แนะนำตัว และตรวจสอบความสามารถของระบบ",
            actions=["chat", "get_status", "list_capabilities"]
        )
        self.registry = registry # อ้างอิงไปยัง registry เพื่อดู tools ทั้งหมด
        self.identity = (
            "ฉันคือ AetoxClaw ระบบปฏิบัติการอัจฉริยะ "
            "ที่ถูกออกแบบมาเพื่อช่วยคุณจัดการไฟล์, ค้นหาข้อมูลเว็บ, วิเคราะห์รูปภาพ "
            "และควบคุมระบบคอมพิวเตอร์ของคุณแบบอัตโนมัติผ่านพลังของ AI"
        )

    def get_prompt_doc(self) -> str:
        return (
            f"Tool: {self.name}\n"
            f"หน้าที่: แนะนำตัว, คุยทั่วไป และรายงานความสามารถของระบบ\n"
            f"คำสั่ง:\n"
            f"1. chat: คุยเล่น ทักทาย หรือแนะนำตัว (params: message)\n"
            f"2. get_status: ดูสถานะระบบ (ไม่ต้องมี params)\n"
            f"3. list_capabilities: บอกผู้ใช้ว่ามีเครื่องมือ (Tools) อะไรพร้อมใช้งานบ้าง (ไม่ต้องมี params)\n"
        )

    def execute(self, params: Dict[str, Any]) -> Dict[str, Any]:
        action = params.get("action")
        
        if action == "chat":
            message = params.get("message", "")
            # ส่ง Identity ไปพร้อมกับข้อความเพื่อให้ LLM นำไปใช้ตอบกลับ
            return {
                "status": "chat", 
                "output": f"บริบทตัวตน: {self.identity}\nข้อความผู้ใช้: {message}"
            }
            
        if action == "get_status":
            return {"status": "success", "output": f"🟢 ระบบออนไลน์: {self.identity}"}

        if action == "list_capabilities":
            if not self.registry:
                return {"status": "failure", "error": "ไม่สามารถดึงข้อมูล Registry ได้"}
            
            tool_list = []
            for name, tool in self.registry.get_all().items():
                tool_list.append(f"- **{name}**: {tool.description}")
            
            output = "🛠️ **เครื่องมือที่พร้อมใช้งานในขณะนี้:**\n" + "\n".join(tool_list)
            output += "\n\nคุณสามารถสั่งให้ฉันจัดการไฟล์, ท่องเว็บ หรือวิเคราะห์ข้อมูลได้ทันทีครับ!"
            return {"status": "success", "output": output}
            
        return {"status": "failure", "error": f"ไม่พบคำสั่ง: {action}"}
