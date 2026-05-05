import os
import subprocess
import logging
from typing import Dict, Any
from aetox.tools.base import BaseTool

logger = logging.getLogger("aetox.tools.system_control")

class SystemControl(BaseTool):
    """
    SystemControl - The 'Hands' of AetoxClaw.
    Allows the agent to interact with the local operating system, 
    open applications, and run files.
    """
    def __init__(self):
        super().__init__(
            name="system_control",
            description="ใช้สำหรับการ 'เปิด' (Open/Run) แอปพลิเคชัน, โปรแกรมในเครื่อง, หรือเปิดไฟล์งานขึ้นมาใช้งานบนหน้าจอโดยตรง",
            actions=["open"]
        )

    def get_prompt_doc(self) -> str:
        return (
            f"Tool: {self.name}\n"
            f"หน้าที่: เปิดแอปพลิเคชันหรือไฟล์บนเครื่อง\n"
            f"คำสั่ง: open\n"
            f"ตัวอย่าง JSON (เปิดแอปเดียว):\n"
            f'  {{"tool": "system_control", "action": "open", '
            f'"params": {{"target": "notepad"}}, "confidence": 0.95}}\n'
            f"ตัวอย่าง JSON (เปิดหลายแอป):\n"
            f'  {{"tool": "system_control", "action": "open", '
            f'"params": {{"targets": ["notepad", "calc"]}}, "confidence": 0.95}}\n'
            f"ใช้เมื่อ: ผู้ใช้พูดว่า 'เปิด', 'รัน', 'open', 'launch'\n"
        )

    def execute(self, params: Dict[str, Any]) -> Dict[str, Any]:
        action = params.get("action")
        target = params.get("target") or params.get("path")
        targets = params.get("targets") # Support multiple apps in a list

        if not action:
            return {"status": "failure", "error": "โปรดระบุคำสั่ง (action) ที่ต้องการให้ทำครับ"}

        if action == "open":
            if targets and isinstance(targets, list):
                results = []
                for t in targets:
                    results.append(self._open_target(t))
                return {
                    "status": "success", 
                    "output": f"🚀 **[AetoxControl]** สั่งเปิด {len(targets)} รายการให้แล้วครับ:\n" + "\n".join([r.get("output", "") for r in results])
                }
            return self._open_target(target)
        
        return {"status": "failure", "error": f"ไม่รู้จักคำสั่ง: {action}"}

    def _open_target(self, target: str) -> Dict[str, Any]:
        """Opens an application or a file using the system's default handler."""
        if not target:
            return {"status": "failure", "error": "โปรดระบุชื่อแอปหรือไฟล์ที่ต้องการเปิดครับ"}

        try:
            # On Windows, os.startfile is the cleanest way to open apps/files
            # It's like double-clicking the item.
            os.startfile(target)
            return {
                "status": "success",
                "output": f"🚀 **[AetoxControl]** กำลังเปิด '{target}' ให้แล้วครับ!"
            }
        except Exception as e:
            # If direct open fails, try searching in common app paths or use shell
            try:
                subprocess.Popen(target, shell=True)
                return {
                    "status": "success",
                    "output": f"🚀 **[AetoxControl]** พยายามเปิด '{target}' ผ่าน Shell ให้แล้วครับ"
                }
            except Exception as e2:
                logger.error(f"Failed to open {target}: {e2}")
                return {"status": "failure", "error": f"ไม่สามารถเปิด '{target}' ได้ครับ: {str(e2)}"}
