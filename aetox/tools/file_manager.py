import os
import shutil
import logging
from pathlib import Path
from typing import Dict, Any
from aetox.tools.base import BaseTool

logger = logging.getLogger("aetox.tools.file_manager")

class MasterFileManager(BaseTool):
    """
    The core file management tool for AetoxOS.
    Organized into 3 Layers: Router, Atomic Actions, and Intelligent Engine.
    """
    def __init__(self):
        super().__init__(
            name="master_file_manager",
            description="จัดการไฟล์และโฟลเดอร์ (สร้าง, เปลี่ยนชื่อ, ย้าย, จัดระเบียบ)",
            actions=["create_folder", "create_file", "rename", "move", "organize"]
        )
        # Category Definitions for Intelligent Layer
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
        )

    # =========================================================================
    # LAYER 1: ROUTER
    # =========================================================================
    def execute(self, params: Dict[str, Any]) -> Dict[str, Any]:
        action = params.get("action", "organize")
        path = params.get("path")
        
        if not path:
            return {"status": "failure", "error": "Missing 'path' parameter."}

        # Route to appropriate layer
        if action == "create_folder":
            return self._handle_create_folder(path)
        elif action == "create_file":
            return self._handle_create_file(path, params.get("content", ""))
        elif action in ["rename", "move"]:
            return self._handle_move_rename(path, params.get("new_path") or params.get("destination"))
        elif action == "organize":
            return self._organize_directory(path)
        
        return {"status": "failure", "error": f"Unknown action: {action}"}

    # =========================================================================
    # LAYER 2: ATOMIC ACTIONS (Basic Operations)
    # =========================================================================
    def _handle_create_folder(self, path: str) -> Dict[str, Any]:
        try:
            os.makedirs(path, exist_ok=True)
            return {"status": "success", "output": f"📁 สร้างโฟลเดอร์สำเร็จ: {path}"}
        except Exception as e:
            return {"status": "failure", "error": f"สร้างโฟลเดอร์ไม่สำเร็จ: {str(e)}"}

    def _handle_create_file(self, path: str, content: str) -> Dict[str, Any]:
        try:
            with open(path, "w", encoding="utf-8") as f:
                f.write(content)
            return {"status": "success", "output": f"📄 สร้างไฟล์สำเร็จ: {path}"}
        except Exception as e:
            return {"status": "failure", "error": f"สร้างไฟล์ไม่สำเร็จ: {str(e)}"}

    def _handle_move_rename(self, src: str, dest: str) -> Dict[str, Any]:
        try:
            if not dest: return {"status": "failure", "error": "Missing destination path."}
            shutil.move(src, dest)
            return {"status": "success", "output": f"🔄 ดำเนินการย้าย/เปลี่ยนชื่อสำเร็จ: {src} -> {dest}"}
        except Exception as e:
            return {"status": "failure", "error": f"ย้าย/เปลี่ยนชื่อไม่สำเร็จ: {str(e)}"}

    # =========================================================================
    # LAYER 3: INTELLIGENT ENGINE (Complex Logic)
    # =========================================================================
    def _organize_directory(self, target_path: str) -> Dict[str, Any]:
        """Original Organization Logic preserved and isolated in this layer."""
        try:
            p = Path(target_path)
            if not p.exists() or not p.is_dir():
                return {"status": "failure", "error": f"ไม่พบตำแหน่ง: {target_path}"}

            moved_count = 0
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
                        moved_count += 1

            return {"status": "success", "output": f"✅ จัดระเบียบไฟล์สำเร็จ {moved_count} รายการ ใน {target_path}"}
        except Exception as e:
            logger.error(f"Organization Error: {e}")
            return {"status": "failure", "error": f"เกิดข้อผิดพลาดในการจัดระเบียบ: {str(e)}"}
