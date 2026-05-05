from abc import ABC, abstractmethod
from typing import Dict, Any, List

class BaseTool(ABC):
    """
    Abstract base class สำหรับทุก Tool ใน AetoxClaw
    ทุก Tool ต้อง implement get_prompt_doc() เพื่อบอก LLM ว่าใช้ตัวเองยังไง
    """
    def __init__(self, name: str, description: str, actions: List[str] = None):
        self.name = name
        self.description = description
        self.actions = actions or []

    @abstractmethod
    def execute(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Logic หลักของ tool คืน dict ที่มี status และ output เสมอ"""
        pass

    def get_prompt_doc(self) -> str:
        """
        คืน string ที่บอก LLM ว่าต้องใช้ tool นี้ยังไง
        Default: สร้างจาก name + description + actions อัตโนมัติ
        Override ใน subclass ถ้าต้องการ doc ที่ละเอียดกว่านี้
        """
        actions_str = ", ".join(self.actions) if self.actions else "ไม่ระบุ"
        return (
            f"Tool: {self.name}\n"
            f"หน้าที่: {self.description}\n"
            f"คำสั่งที่รองรับ (action): {actions_str}\n"
        )

    def get_schema(self) -> Dict[str, Any]:
        """JSON Schema สำหรับ validate params (optional, override ได้)"""
        return {
            "tool": self.name,
            "actions": self.actions,
            "params": {}
        }

    def get_metadata(self) -> Dict[str, str]:
        return {"name": self.name, "description": self.description}
