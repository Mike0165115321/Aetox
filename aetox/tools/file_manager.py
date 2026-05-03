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
    Focused on intelligent organization while preserving existing folder structures.
    """
    def __init__(self):
        super().__init__(
            name="master_file_manager",
            description="Organizes loose files in a directory into category folders (Images, Documents, etc.) while skipping existing folders."
        )
        self.categories = {
            "Images": [".jpg", ".jpeg", ".png", ".gif", ".svg", ".webp", ".bmp"],
            "Documents": [".pdf", ".docx", ".doc", ".txt", ".xlsx", ".pptx", ".csv"],
            "Videos": [".mp4", ".mkv", ".mov", ".avi", ".wmv"],
            "Archives": [".zip", ".rar", ".7z", ".tar", ".gz"],
            "Code": [".py", ".js", ".ts", ".html", ".css", ".json", ".yaml", ".cpp", ".java"],
            "Music": [".mp3", ".wav", ".flac", ".m4a"],
            "Executables": [".exe", ".msi", ".bat", ".sh"]
        }

    def execute(self, params: Dict[str, Any]) -> Dict[str, Any]:
        action = params.get("action", "organize")
        path = params.get("path")

        if not path:
            return {"status": "failure", "error": "โปรดระบุตำแหน่ง (Path) ที่ต้องการจัดระเบียบครับ"}

        if action == "organize":
            return self._organize_directory(path)
        
        return {"status": "failure", "error": f"ไม่รู้จักคำสั่ง: {action}"}

    def _organize_directory(self, target_path: str) -> Dict[str, Any]:
        try:
            p = Path(target_path)
            if not p.exists() or not p.is_dir():
                return {"status": "failure", "error": f"ไม่พบโฟลเดอร์หรือตำแหน่ง '{target_path}' ในเครื่องครับ"}

            moved_files = []
            # 1. Scan only loose files in the target directory
            for item in p.iterdir():
                if item.is_file():  # GOLEN RULE: Skip existing folders
                    ext = item.suffix.lower()
                    target_category = "Others"
                    
                    # 2. Determine category
                    for category, extensions in self.categories.items():
                        if ext in extensions:
                            target_category = category
                            break
                    
                    # 3. Target folder handling
                    dest_dir = p / target_category
                    if not dest_dir.exists():
                        dest_dir.mkdir(parents=True, exist_ok=True)
                    
                    # 4. Move file
                    dest_path = dest_dir / item.name
                    if not dest_path.exists():
                        shutil.move(str(item), str(dest_path))
                        moved_files.append(f"📄 {item.name} ➡️ 📁 {target_category}")

            count = len(moved_files)
            if count == 0:
                return {"status": "success", "output": "✨ ตำแหน่งนี้เป็นระเบียบเรียบร้อยอยู่แล้วครับ (ไม่พบไฟล์ที่ต้องย้าย)"}
            
            summary = f"✅ **จัดระเบียบเสร็จสมบูรณ์!** ย้ายไฟล์ทั้งหมด {count} รายการแล้วครับ:\n\n"
            summary += "\n".join(moved_files[:15])
            if count > 15:
                summary += f"\n...และรายการอื่นๆ อีก {count-15} รายการ"
                
            return {
                "status": "success", 
                "output": summary
            }
        except Exception as e:
            logger.error(f"Error organizing directory: {e}")
            return {"status": "failure", "error": f"เกิดข้อผิดพลาดระหว่างจัดระเบียบ: {str(e)}"}
