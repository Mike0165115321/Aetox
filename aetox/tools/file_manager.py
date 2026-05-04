import os
import shutil
import logging
from pathlib import Path
from typing import Dict, Any

logger = logging.getLogger("aetox.tools.file_manager")
from aetox.tools.base import BaseTool

class MasterFileManager(BaseTool):
    """
    The core file management tool for AetoxOS.
    Enhanced with Create, Rename, Move, and Organize capabilities.
    """
    def __init__(self):
        super().__init__(
            name="master_file_manager",
            description="จัดการไฟล์และโฟลเดอร์ (สร้าง, เปลี่ยนชื่อ, ย้าย, จัดระเบียบ)",
            actions=["create_folder", "create_file", "rename", "move", "organize"]
        )
        self.categories = {
            "Images": [".jpg", ".jpeg", ".png", ".gif", ".svg", ".webp"],
            "Documents": [".pdf", ".docx", ".doc", ".txt", ".xlsx"],
            "Videos": [".mp4", ".mkv", ".mov"],
            "Archives": [".zip", ".rar", ".7z"],
            "Code": [".py", ".js", ".ts", ".html", ".css", ".json"],
        }

    def get_prompt_doc(self) -> str:
        return (
            f"Tool: {self.name}\n"
            f"คำสั่งที่มี:\n"
            f"- create_folder: สร้างโฟลเดอร์ใหม่ (params: path)\n"
            f"- create_file: สร้างไฟล์ใหม่ (params: path, content)\n"
            f"- rename: เปลี่ยนชื่อหรือย้ายไฟล์/โฟลเดอร์ (params: path, new_path)\n"
            f"- move: ย้ายไฟล์ไปโฟลเดอร์ปลายทาง (params: path, destination)\n"
            f"- organize: จัดระเบียบไฟล์ลงโฟลเดอร์หมวดหมู่ (params: path)\n"
            f"ตัวอย่าง JSON:\n"
            f'  {{"tool": "master_file_manager", "action": "create_folder", "params": {{"path": "E:/Project/NewFolder"}}}}\n'
        )

    def execute(self, params: Dict[str, Any]) -> Dict[str, Any]:
        action = params.get("action", "organize")
        path = params.get("path")
        
        if not path:
            return {"status": "failure", "error": "Missing 'path' parameter."}

        try:
            if action == "create_folder":
                os.makedirs(path, exist_ok=True)
                return {"status": "success", "output": f"📁 สร้างโฟลเดอร์เรียบร้อย: {path}"}
            
            elif action == "create_file":
                content = params.get("content", "")
                with open(path, "w", encoding="utf-8") as f:
                    f.write(content)
                return {"status": "success", "output": f"📄 สร้างไฟล์เรียบร้อย: {path}"}
            
            elif action == "rename" or action == "move":
                new_path = params.get("new_path") or params.get("destination")
                if not new_path: return {"status": "failure", "error": "Missing 'new_path' or 'destination'."}
                shutil.move(path, new_path)
                return {"status": "success", "output": f"🔄 ย้าย/เปลี่ยนชื่อ: {path} -> {new_path}"}
                
            elif action == "organize":
                return self._organize_directory(path)
            
            return {"status": "failure", "error": f"Unknown action: {action}"}
        except Exception as e:
            return {"status": "failure", "error": str(e)}

    def _organize_directory(self, target_path: str) -> Dict[str, Any]:
        try:
            p = Path(target_path)
            if not p.exists() or not p.is_dir():
                return {"status": "failure", "error": f"Path not found: {target_path}"}

            moved_files = []
            for item in p.iterdir():
                if item.is_file():
                    ext = item.suffix.lower()
                    target_category = "Others"
                    for category, extensions in self.categories.items():
                        if ext in extensions:
                            target_category = category
                            break
                    
                    dest_dir = p / target_category
                    dest_dir.mkdir(parents=True, exist_ok=True)
                    dest_path = dest_dir / item.name
                    if not dest_path.exists():
                        shutil.move(str(item), str(dest_path))
                        moved_files.append(f"{item.name} -> {target_category}")

            return {"status": "success", "output": f"Organized {len(moved_files)} files in {target_path}"}
        except Exception as e:
            return {"status": "failure", "error": str(e)}
