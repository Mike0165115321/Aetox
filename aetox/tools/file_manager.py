import os
import shutil
import logging
from pathlib import Path
from typing import Dict, Any, List
from aetox.tools.base import BaseTool

logger = logging.getLogger("aetox.tools.file_manager")

class MasterFileManager(BaseTool):
    """
    The core file management tool for AetoxOS.
    Organized into 3 Layers: Router, Atomic Actions, and Intelligent Engine.
    Follows the Aetox Tool Standard v2.0.
    """
    def __init__(self):
        super().__init__(
            name="master_file_manager",
            description="จัดการระบบไฟล์แบบครบวงจร (อ่าน, เขียน, ย้าย, ลบ, จัดระเบียบ)",
            actions=["create_folder", "create_file", "read_file", "delete", "rename", "move", "copy", "list_dir", "organize"]
        )
        # Category Definitions for Intelligent Layer (Organize)
        self.categories = {
            "Images": [".jpg", ".jpeg", ".png", ".gif", ".svg", ".webp"],
            "Documents": [".pdf", ".docx", ".doc", ".txt", ".xlsx", ".csv"],
            "Videos": [".mp4", ".mkv", ".mov"],
            "Archives": [".zip", ".rar", ".7z"],
            "Code": [".py", ".js", ".ts", ".html", ".css", ".json", ".yaml", ".yml"],
            "Audio": [".mp3", ".wav", ".flac"]
        }

    def get_prompt_doc(self) -> str:
        """Detailed documentation for LLM to ensure accurate tool calls."""
        return (
            f"Tool: {self.name}\n"
            f"หน้าที่: จัดการไฟล์และโฟลเดอร์ในระบบ\n"
            f"คำสั่งที่รองรับ:\n"
            f"1. create_folder: สร้างโฟลเดอร์ (params: path)\n"
            f"2. create_file: สร้าง/เขียนไฟล์ (params: path, content)\n"
            f"3. read_file: อ่านเนื้อหาในไฟล์ (params: path)\n"
            f"4. delete: ลบไฟล์หรือโฟลเดอร์ (params: path)\n"
            f"5. move/rename: ย้ายหรือเปลี่ยนชื่อ (params: path, destination)\n"
            f"6. copy: คัดลอกไฟล์/โฟลเดอร์ (params: path, destination)\n"
            f"7. list_dir: ดูรายชื่อไฟล์ในโฟลเดอร์ (params: path)\n"
            f"8. organize: จัดระเบียบไฟล์ลงโฟลเดอร์หมวดหมู่ (params: path)\n\n"
            f"ตัวอย่าง JSON:\n"
            f'  {{"tool": "{self.name}", "action": "create_file", "params": {{"path": "test.txt", "content": "Hello"}}, "confidence": 1.0}}\n'
            f'  {{"tool": "{self.name}", "action": "organize", "params": {{"path": "./downloads"}}, "confidence": 0.9}}\n'
        )

    # =========================================================================
    # LAYER 1: ROUTER
    # =========================================================================
    def execute(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Main entry point that routes requests to specific handlers."""
        action = params.get("action")
        path = params.get("path") or params.get("target")
        
        if not action:
            return {"status": "failure", "error": "Missing 'action' parameter."}
        if not path:
            return {"status": "failure", "error": "Missing 'path' or 'target' parameter."}

        try:
            # Atomic Routing
            if action == "create_folder":
                return self._handle_create_folder(path)
            elif action == "create_file":
                return self._handle_create_file(path, params.get("content", ""))
            elif action == "read_file":
                return self._handle_read_file(path)
            elif action == "delete":
                return self._handle_delete(path)
            elif action in ["move", "rename"]:
                return self._handle_move_rename(path, params.get("destination") or params.get("new_path"))
            elif action == "copy":
                return self._handle_copy(path, params.get("destination"))
            elif action == "list_dir":
                return self._handle_list_dir(path)
            
            # Intelligent Engine Routing
            elif action == "organize":
                return self._organize_directory(path)
            
            return {"status": "failure", "error": f"ไม่พบคำสั่ง: {action}"}
        except Exception as e:
            logger.error(f"Execution Error in {action}: {e}")
            return {"status": "failure", "error": f"Internal Error: {str(e)}"}

    # =========================================================================
    # LAYER 2: ATOMIC ACTIONS (Basic CRUD Operations)
    # =========================================================================
    
    def _handle_create_folder(self, path: str) -> Dict[str, Any]:
        try:
            os.makedirs(path, exist_ok=True)
            return {"status": "success", "output": f"📁 สร้างโฟลเดอร์สำเร็จ: {path}"}
        except Exception as e:
            return {"status": "failure", "error": f"สร้างโฟลเดอร์ไม่สำเร็จ: {str(e)}"}

    def _handle_create_file(self, path: str, content: str) -> Dict[str, Any]:
        try:
            p = Path(path)
            p.parent.mkdir(parents=True, exist_ok=True) # Auto-create parent dirs
            with open(path, "w", encoding="utf-8") as f:
                f.write(content)
            return {"status": "success", "output": f"📄 สร้าง/เขียนไฟล์สำเร็จ: {path}"}
        except Exception as e:
            return {"status": "failure", "error": f"สร้างไฟล์ไม่สำเร็จ: {str(e)}"}

    def _handle_read_file(self, path: str) -> Dict[str, Any]:
        try:
            if not os.path.exists(path):
                return {"status": "failure", "error": f"ไม่พบไฟล์: {path}"}
            with open(path, "r", encoding="utf-8") as f:
                content = f.read()
            return {"status": "success", "output": content}
        except Exception as e:
            return {"status": "failure", "error": f"อ่านไฟล์ไม่สำเร็จ: {str(e)}"}

    def _handle_delete(self, path: str) -> Dict[str, Any]:
        try:
            if not os.path.exists(path):
                return {"status": "failure", "error": f"ไม่พบไฟล์/โฟลเดอร์: {path}"}
            
            if os.path.isdir(path):
                shutil.rmtree(path)
                return {"status": "success", "output": f"🗑️ ลบโฟลเดอร์สำเร็จ: {path}"}
            else:
                os.remove(path)
                return {"status": "success", "output": f"🗑️ ลบไฟล์สำเร็จ: {path}"}
        except Exception as e:
            return {"status": "failure", "error": f"ลบไม่สำเร็จ: {str(e)}"}

    def _handle_move_rename(self, src: str, dest: str) -> Dict[str, Any]:
        try:
            if not dest:
                return {"status": "failure", "error": "ต้องการ 'destination' สำหรับการย้ายหรือเปลี่ยนชื่อ"}
            shutil.move(src, dest)
            return {"status": "success", "output": f"🔄 ย้าย/เปลี่ยนชื่อสำเร็จ: {src} -> {dest}"}
        except Exception as e:
            return {"status": "failure", "error": f"ดำเนินการไม่สำเร็จ: {str(e)}"}

    def _handle_copy(self, src: str, dest: str) -> Dict[str, Any]:
        try:
            if not dest:
                return {"status": "failure", "error": "ต้องการ 'destination' สำหรับการคัดลอก"}
            
            if os.path.isdir(src):
                shutil.copytree(src, dest)
                return {"status": "success", "output": f"📋 คัดลอกโฟลเดอร์สำเร็จ: {src} -> {dest}"}
            else:
                shutil.copy2(src, dest)
                return {"status": "success", "output": f"📋 คัดลอกไฟล์สำเร็จ: {src} -> {dest}"}
        except Exception as e:
            return {"status": "failure", "error": f"คัดลอกไม่สำเร็จ: {str(e)}"}

    def _handle_list_dir(self, path: str) -> Dict[str, Any]:
        try:
            if not os.path.isdir(path):
                return {"status": "failure", "error": f"'{path}' ไม่ใช่โฟลเดอร์หรือไม่มีอยู่จริง"}
            
            # --- TREE GENERATOR (Simplified) ---
            def build_tree(current_path: str, prefix: str = "", depth: int = 0) -> List[str]:
                if depth > 0: # 🛑 Limit to only first level as requested
                    return []
                
                lines = []
                try:
                    items = sorted(os.listdir(current_path))
                except Exception:
                    return []

                # Limit items to avoid blowing up the chat
                max_items = 20
                display_items = items[:max_items]
                
                for i, item in enumerate(display_items):
                    full_path = os.path.join(current_path, item)
                    is_last = (i == len(display_items) - 1 and len(items) <= max_items)
                    connector = "└── " if is_last else "├── "
                    
                    type_icon = "📁" if os.path.isdir(full_path) else "📄"
                    lines.append(f"{prefix}{connector}{type_icon} {item}")
                    
                    # Optional: Peek into subfolder (limit to 1-2 files)
                    if os.path.isdir(full_path):
                        try:
                            sub_items = sorted(os.listdir(full_path))[:2] # Show only first 2 items
                            sub_prefix = prefix + ("    " if is_last else "│   ")
                            for j, sub in enumerate(sub_items):
                                s_last = (j == len(sub_items) - 1)
                                s_conn = "└── " if s_last else "├── "
                                s_type = "📁" if os.path.isdir(os.path.join(full_path, sub)) else "📄"
                                lines.append(f"{sub_prefix}{s_conn}{s_type} {sub}")
                            if len(os.listdir(full_path)) > 2:
                                lines.append(f"{sub_prefix}└── ...")
                        except Exception:
                            pass

                if len(items) > max_items:
                    lines.append(f"{prefix}└── ... และอีก {len(items) - max_items} รายการ")
                
                return lines


            tree_lines = build_tree(path)
            output = "\n".join(tree_lines) if tree_lines else "โฟลเดอร์ว่างเปล่า"
            return {"status": "success", "output": f"### โครงสร้างไฟล์ใน {path}:\n```\n{output}\n```"}
        except Exception as e:
            return {"status": "failure", "error": f"ไม่สามารถดูรายการได้: {str(e)}"}


    # =========================================================================
    # LAYER 3: INTELLIGENT ENGINE (Complex Logic)
    # =========================================================================
    
    def _organize_directory(self, target_path: str) -> Dict[str, Any]:
        """Automatically categorizes files into folders based on extensions."""
        try:
            p = Path(target_path)
            if not p.exists() or not p.is_dir():
                return {"status": "failure", "error": f"ไม่พบโฟลเดอร์: {target_path}"}

            moved_count = 0
            skipped_count = 0
            
            for item in p.iterdir():
                # Skip directories and hidden files
                if item.is_dir() or item.name.startswith('.'):
                    continue
                
                ext = item.suffix.lower()
                target_category = "Others"
                
                # Find matching category
                for category, extensions in self.categories.items():
                    if ext in extensions:
                        target_category = category
                        break
                
                dest_dir = p / target_category
                dest_dir.mkdir(parents=True, exist_ok=True)
                dest_path = dest_dir / item.name
                
                # Handle filename collisions
                if dest_path.exists():
                    skipped_count += 1
                    continue
                
                shutil.move(str(item), str(dest_path))
                moved_count += 1

            return {
                "status": "success", 
                "output": f"✅ จัดระเบียบใน {target_path} สำเร็จ!\n- ย้ายแล้ว: {moved_count} รายการ\n- ข้าม (มีอยู่แล้ว): {skipped_count} รายการ"
            }
        except Exception as e:
            logger.error(f"Organization Error: {e}")
            return {"status": "failure", "error": f"การจัดระเบียบล้มเหลว: {str(e)}"}
