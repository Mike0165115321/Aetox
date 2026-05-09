import os
import shutil
import logging
from pathlib import Path
from typing import Dict, Any, List, Optional
from aetox.tools.base import BaseTool

logger = logging.getLogger("aetox.tools.file_manager")

class PathNavigator:
    """จัดการ 'ตำแหน่งปัจจุบัน' ของระบบไฟล์ เหมือน terminal มี cd"""
    
    def __init__(self, root: str = None, config_path: str = "config/permissions.yaml"):
        # ตั้ง root ที่ปัจจุบัน
        self.root = Path(root) if root else Path.cwd()
        self.cwd = self.root
        
        # โหลดค่าจาก config/permissions.yaml
        self.allowed_roots = [Path.home(), Path.cwd()]
        self.denied_paths = {"C:\\Windows", "C:\\Program Files", "/etc", "/root", "/sys"}
        
        try:
            import yaml
            if os.path.exists(config_path):
                with open(config_path, 'r', encoding='utf-8') as f:
                    config = yaml.safe_load(f)
                    # โหลด Allowed Roots
                    extra_roots = config.get('allowed_roots', [])
                    for r in extra_roots:
                        # ขยายค่า %USERPROFILE%, %APPDATA% หรือ ~ อัตโนมัติ
                        expanded_path = os.path.expandvars(os.path.expanduser(r))
                        p = Path(expanded_path)
                        if p not in self.allowed_roots:
                            self.allowed_roots.append(p)
                    
                    # โหลด Sensitive Zones (Denied Paths)
                    extra_denied = config.get('sensitive_zones', [])
                    for d in extra_denied:
                        expanded_denied = os.path.expandvars(os.path.expanduser(d))
                        self.denied_paths.add(expanded_denied)
                logger.info(f"PathNavigator: Loaded {len(self.allowed_roots)} allowed roots from {config_path}")
        except Exception as e:
            logger.error(f"PathNavigator: Failed to load config: {e}")

        # เพิ่มระบบทางลัด (Shortcuts) สำหรับโฟลเดอร์มาตรฐาน
        self.shortcuts = {
            "dropbox": os.path.expandvars(os.path.expanduser("~/Dropbox")),
            "desktop": os.path.expandvars(os.path.expanduser("~/Desktop")),
            "downloads": os.path.expandvars(os.path.expanduser("~/Downloads")),
            "documents": os.path.expandvars(os.path.expanduser("~/Documents")),
            "pictures": os.path.expandvars(os.path.expanduser("~/Pictures")),
            "videos": os.path.expandvars(os.path.expanduser("~/Videos")),
        }

    def is_safe_path(self, path: Path) -> bool:
        """ตรวจสอบว่า path ปลอดภัยที่จะเข้าถึง"""
        try:
            resolved = path.resolve()
            # ตรวจสอบว่าอยู่ใน allowed_roots ไหม
            for allowed in self.allowed_roots:
                try:
                    resolved.relative_to(allowed.resolve())
                    break
                except ValueError:
                    continue
            else:
                return False  # ไม่อยู่ใน allowed_roots ไหนเลย
            
            # ตรวจสอบ denied_paths
            for denied in self.denied_paths:
                if str(resolved).startswith(denied):
                    return False
            
            return True
        except Exception:
            return False
    
    def resolve(self, path: str) -> Path:
        """แปลง path (relative/absolute/shortcut) → absolute path ที่ปลอดภัย"""
        # 1. เช็คระบบทางลัด (Shortcuts)
        clean_path = path.strip().lower().replace("/", "\\").rstrip("\\")
        if clean_path in self.shortcuts:
            candidate = Path(self.shortcuts[clean_path])
        elif os.path.isabs(path):
            candidate = Path(path)
        else:
            candidate = self.cwd / path
        
        resolved = candidate.resolve()
        
        if not self.is_safe_path(resolved):
            raise PermissionError(f"Access denied: {resolved}")
        
        return resolved
    
    def cd(self, path: str) -> Dict[str, Any]:
        """เปลี่ยน directory ปัจจุบัน (เหมือนคำสั่ง cd)"""
        try:
            new_cwd = self.resolve(path)
            if not new_cwd.is_dir():
                return {"status": "failure", "error": f"Not a directory: {path}"}
            
            old_cwd = self.cwd
            self.cwd = new_cwd
            return {
                "status": "success",
                "output": f"📁 เปลี่ยนไปยัง: {self.cwd}",
                "meta": {"previous": str(old_cwd), "current": str(self.cwd)}
            }
        except PermissionError as e:
            return {"status": "failure", "error": str(e)}
        except Exception as e:
            return {"status": "failure", "error": f"cd failed: {str(e)}"}
    
    def pwd(self) -> str:
        """คืนตำแหน่งปัจจุบัน"""
        return str(self.cwd)

class MasterFileManager(BaseTool):
    """
    The core file management tool for AetoxClaw.
    Organized into 3 Layers: Router, Atomic Actions, and Intelligent Engine.
    Follows the Aetox Tool Standard v2.0.
    """
    def __init__(self):
        super().__init__(
            name="master_file_manager",
            description="จัดการระบบไฟล์แบบครบวงจร (อ่าน, เขียน, ย้าย, ลบ, จัดระเบียบ, นำทาง)",
            actions=[
                "create_folder", "create_file", "read_file", "delete", 
                "rename", "move", "copy", "list_dir", "organize",
                "navigate", "get_cwd"
            ]
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
        self.navigator = PathNavigator()

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
            f"8. organize: จัดระเบียบไฟล์ลงโฟลเดอร์หมวดหมู่ (params: path)\n"
            f"9. navigate: เปลี่ยนโฟลเดอร์ปัจจุบัน (เหมือน cd) (params: path)\n"
            f"10. get_cwd: ดูตำแหน่งปัจจุบัน (ไม่ต้องมี params)\n\n"
            f"ตัวอย่าง JSON:\n"
            f'  {{"tool": "{self.name}", "action": "navigate", "params": {{"path": "Downloads"}}, "confidence": 1.0}}\n'
            f'  {{"tool": "{self.name}", "action": "read_file", "params": {{"path": "test.txt"}}, "confidence": 1.0}}\n'
        )

    # =========================================================================
    # LAYER 1: ROUTER
    # =========================================================================
    def execute(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Main entry point that routes requests to specific handlers."""
        action = params.get("action")
        path = params.get("path") or params.get("target") or "."
        
        if not action:
            return {"status": "failure", "error": "Missing 'action' parameter."}

        try:
            # Navigation Actions
            if action == "navigate":
                return self.navigator.cd(path)
            elif action == "get_cwd":
                return {"status": "success", "output": self.navigator.pwd()}

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
            safe_path = self.navigator.resolve(path)
            safe_path.mkdir(parents=True, exist_ok=True)
            return {"status": "success", "output": f"📁 สร้างโฟลเดอร์สำเร็จ: {safe_path}"}
        except Exception as e:
            return {"status": "failure", "error": f"สร้างโฟลเดอร์ไม่สำเร็จ: {str(e)}"}

    def _handle_create_file(self, path: str, content: str) -> Dict[str, Any]:
        try:
            safe_path = self.navigator.resolve(path)
            safe_path.parent.mkdir(parents=True, exist_ok=True)
            with open(safe_path, "w", encoding="utf-8") as f:
                f.write(content)
            return {"status": "success", "output": f"📄 สร้าง/เขียนไฟล์สำเร็จ: {safe_path}"}
        except Exception as e:
            return {"status": "failure", "error": f"สร้างไฟล์ไม่สำเร็จ: {str(e)}"}

    def _handle_read_file(self, path: str) -> Dict[str, Any]:
        try:
            safe_path = self.navigator.resolve(path)
            if not safe_path.exists():
                return {"status": "failure", "error": f"ไม่พบไฟล์: {path}"}
            with open(safe_path, "r", encoding="utf-8") as f:
                content = f.read()
            return {"status": "success", "output": content}
        except Exception as e:
            return {"status": "failure", "error": f"อ่านไฟล์ไม่สำเร็จ: {str(e)}"}

    def _handle_delete(self, path: str) -> Dict[str, Any]:
        try:
            safe_path = self.navigator.resolve(path)
            if not safe_path.exists():
                return {"status": "failure", "error": f"ไม่พบไฟล์/โฟลเดอร์: {path}"}
            
            if safe_path.is_dir():
                shutil.rmtree(safe_path)
                return {"status": "success", "output": f"🗑️ ลบโฟลเดอร์สำเร็จ: {safe_path}"}
            else:
                os.remove(safe_path)
                return {"status": "success", "output": f"🗑️ ลบไฟล์สำเร็จ: {safe_path}"}
        except Exception as e:
            return {"status": "failure", "error": f"ลบไม่สำเร็จ: {str(e)}"}

    def _handle_move_rename(self, src: str, dest: str) -> Dict[str, Any]:
        try:
            if not dest:
                return {"status": "failure", "error": "ต้องการ 'destination' สำหรับการย้ายหรือเปลี่ยนชื่อ"}
            safe_src = self.navigator.resolve(src)
            safe_dest = self.navigator.resolve(dest)
            shutil.move(str(safe_src), str(safe_dest))
            return {"status": "success", "output": f"🔄 ย้าย/เปลี่ยนชื่อสำเร็จ: {safe_src} -> {safe_dest}"}
        except Exception as e:
            return {"status": "failure", "error": f"ดำเนินการไม่สำเร็จ: {str(e)}"}

    def _handle_copy(self, src: str, dest: str) -> Dict[str, Any]:
        try:
            if not dest:
                return {"status": "failure", "error": "ต้องการ 'destination' สำหรับการคัดลอก"}
            safe_src = self.navigator.resolve(src)
            safe_dest = self.navigator.resolve(dest)
            
            if safe_src.is_dir():
                shutil.copytree(safe_src, safe_dest)
                return {"status": "success", "output": f"📋 คัดลอกโฟลเดอร์สำเร็จ: {safe_src} -> {safe_dest}"}
            else:
                shutil.copy2(safe_src, safe_dest)
                return {"status": "success", "output": f"📋 คัดลอกไฟล์สำเร็จ: {safe_src} -> {safe_dest}"}
        except Exception as e:
            return {"status": "failure", "error": f"คัดลอกไม่สำเร็จ: {str(e)}"}

    def _handle_list_dir(self, path: str) -> Dict[str, Any]:
        try:
            safe_path = self.navigator.resolve(path)
            if not safe_path.is_dir():
                return {"status": "failure", "error": f"'{path}' ไม่ใช่โฟลเดอร์หรือไม่มีอยู่จริง"}
            
            # --- TREE GENERATOR (Simplified) ---
            def build_tree(current_path: Path, prefix: str = "", depth: int = 0) -> List[str]:
                if depth > 0: # 🛑 Limit to only first level
                    return []
                
                lines = []
                try:
                    items = sorted(os.listdir(current_path))
                except Exception:
                    return []

                max_items = 20
                display_items = items[:max_items]
                
                for i, item in enumerate(display_items):
                    full_path = current_path / item
                    is_last = (i == len(display_items) - 1 and len(items) <= max_items)
                    connector = "└── " if is_last else "├── "
                    
                    type_icon = "📁" if full_path.is_dir() else "📄"
                    lines.append(f"{prefix}{connector}{type_icon} {item}")
                    
                    if full_path.is_dir():
                        try:
                            sub_items = sorted(os.listdir(full_path))[:2]
                            sub_prefix = prefix + ("    " if is_last else "│   ")
                            for j, sub in enumerate(sub_items):
                                s_last = (j == len(sub_items) - 1)
                                s_conn = "└── " if s_last else "├── "
                                s_type = "📁" if (full_path / sub).is_dir() else "📄"
                                lines.append(f"{sub_prefix}{s_conn}{s_type} {sub}")
                            if len(os.listdir(full_path)) > 2:
                                lines.append(f"{sub_prefix}└── ...")
                        except Exception:
                            pass

                if len(items) > max_items:
                    lines.append(f"{prefix}└── ... และอีก {len(items) - max_items} รายการ")
                
                return lines


            tree_lines = build_tree(safe_path)
            output = "\n".join(tree_lines) if tree_lines else "โฟลเดอร์ว่างเปล่า"
            return {"status": "success", "output": f"### โครงสร้างไฟล์ใน {safe_path}:\n```\n{output}\n```"}
        except Exception as e:
            return {"status": "failure", "error": f"ไม่สามารถดูรายการได้: {str(e)}"}

    # =========================================================================
    # LAYER 3: INTELLIGENT ENGINE (Complex Logic)
    # =========================================================================
    
    def _organize_directory(self, target_path: str) -> Dict[str, Any]:
        """Automatically categorizes files into folders based on extensions."""
        try:
            safe_path = self.navigator.resolve(target_path)
            if not safe_path.exists() or not safe_path.is_dir():
                return {"status": "failure", "error": f"ไม่พบโฟลเดอร์: {target_path}"}

            moved_count = 0
            skipped_count = 0
            
            for item in safe_path.iterdir():
                if item.is_dir() or item.name.startswith('.'):
                    continue
                
                ext = item.suffix.lower()
                target_category = "Others"
                
                for category, extensions in self.categories.items():
                    if ext in extensions:
                        target_category = category
                        break
                
                dest_dir = safe_path / target_category
                dest_dir.mkdir(parents=True, exist_ok=True)
                dest_path = dest_dir / item.name
                
                if dest_path.exists():
                    skipped_count += 1
                    continue
                
                shutil.move(str(item), str(dest_path))
                moved_count += 1

            return {
                "status": "success", 
                "output": f"✅ จัดระเบียบใน {safe_path} สำเร็จ!\n- ย้ายแล้ว: {moved_count} รายการ\n- ข้าม (มีอยู่แล้ว): {skipped_count} รายการ"
            }
        except Exception as e:
            logger.error(f"Organization Error: {e}")
            return {"status": "failure", "error": f"การจัดระเบียบล้มเหลว: {str(e)}"}