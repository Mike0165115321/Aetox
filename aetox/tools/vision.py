import logging
import fitz  # PyMuPDF
import docx  # python-docx
from typing import Dict, Any
from pathlib import Path
from aetox.tools.base import BaseTool

logger = logging.getLogger("aetox.tools.vision")

class AetoxVision(BaseTool):
    """
    AetoxVision - The 'Eyes' of AetoxClaw.
    Specialized in reading and understanding document contents (PDF, TXT, etc.).
    Supports proper Thai encoding to prevent gibberish text.
    """
    def __init__(self):
        super().__init__(
            name="aetox_vision",
            description="ใช้สำหรับการ 'ดู' โครงสร้างโฟลเดอร์, ลิสต์รายชื่อไฟล์, 'อ่าน' เนื้อหาในเอกสาร (PDF, Word, TXT), และ 'สรุป' เนื้อหาภาษาไทยจากไฟล์งานต่างๆ",
            actions=["read", "summarize", "list"]
        )

    def get_prompt_doc(self) -> str:
        return (
            f"Tool: {self.name}\n"
            f"หน้าที่: อ่านเนื้อหาไฟล์ (PDF, Word, TXT, Code) และดูโครงสร้างโฟลเดอร์\n"
            f"คำสั่ง: read, summarize, list\n"
            f"ตัวอย่าง JSON:\n"
            f'  {{"tool": "aetox_vision", "action": "read", '
            f'"params": {{"path": "C:/docs/report.pdf"}}, "confidence": 0.95}}\n'
            f"ใช้เมื่อ: ผู้ใช้พูดว่า 'อ่าน', 'สรุป', 'ดูไฟล์', 'เปิดดู', 'list'\n"
        )

    def execute(self, params: Dict[str, Any]) -> Dict[str, Any]:
        action = params.get("action", "read")
        path = params.get("path")

        if not path:
            return {"status": "failure", "error": "โปรดระบุตำแหน่งไฟล์ที่ต้องการให้อ่านครับ"}

        if action in ["read", "summarize", "list"]:
            return self._read_document(path)
        
        return {"status": "failure", "error": f"ไม่รู้จักคำสั่ง: {action}"}

    def _read_document(self, file_path: str) -> Dict[str, Any]:
        try:
            p = Path(file_path)
            if not p.exists():
                return {"status": "failure", "error": f"ไม่พบตำแหน่ง '{file_path}' ในเครื่องครับ"}

            # 1. Handle Directory (Tree View)
            if p.is_dir():
                items = sorted(p.iterdir(), key=lambda x: (not x.is_dir(), x.name.lower()))
                
                tree_output = f"📂 **[AetoxVision]** โครงสร้างโฟลเดอร์: `{p.name}`\n```\n"
                tree_output += f"{p.name}/\n"
                
                for i, item in enumerate(items):
                    is_last = (i == len(items) - 1)
                    prefix = "└── " if is_last else "├── "
                    
                    symbol = "📂 " if item.is_dir() else "📄 "
                    tree_output += f"{prefix}{symbol}{item.name}\n"
                
                tree_output += "```"
                return {"status": "success", "output": tree_output}

            # 2. Handle File (Existing logic)
            ext = p.suffix.lower()
            text_content = ""

            # 1. Handle PDF
            if ext == ".pdf":
                doc = fitz.open(str(p))
                # Read up to first 20 pages
                for i in range(min(20, len(doc))):
                    page_text = doc[i].get_text("text", sort=True)
                    text_content += page_text + "\n"
                doc.close()
                source_type = "PDF"

            # 2. Handle Text & Code files
            elif ext in [".txt", ".log", ".py", ".js", ".ts", ".json", ".yaml", ".yml", ".csv", ".md", ".html", ".css", ".env", ".gitignore"]:
                # Force UTF-8 for Thai support
                with open(p, "r", encoding="utf-8", errors="replace") as f:
                    text_content = f.read(20000) # Increased to 20k characters
                source_type = "Text/Code"

            # 3. Handle Word documents
            elif ext == ".docx":
                doc = docx.Document(str(p))
                full_text = [para.text for para in doc.paragraphs]
                text_content = "\n".join(full_text)
                source_type = "Word"

            else:
                return {"status": "failure", "error": f"ขออภัยครับ ตอนนี้ผมยังไม่อ่านไฟล์นามสกุล {ext} ไม่ได้"}

            if not text_content.strip():
                return {"status": "success", "output": f"📄 อ่านไฟล์ {source_type} สำเร็จ แต่ไม่พบข้อความข้างในครับ"}

            # --- CLEANING LAYER ---
            # Remove non-printable characters that cause "garbage" output
            import re
            clean_text = "".join(c for c in text_content if c.isprintable() or c in "\n\r\t")
            clean_text = re.sub(r'\n{3,}', '\n\n', clean_text) # Remove excessive newlines
            clean_text = clean_text.strip()
            
            if not clean_text:
                return {"status": "failure", "error": "ไฟล์นี้อาจเป็นสแกน (Image-only PDF) หรือเข้ารหัสลับ ทำให้ไม่สามารถดึงข้อความออกมาได้ครับ"}

            return {
                "status": "success",
                "output": f"👁️ **[AetoxVision]** อ่านข้อมูลจากไฟล์ {source_type} เรียบร้อยครับ:\n\n{clean_text[:1500]}..." if len(clean_text) > 1500 else f"👁️ **[AetoxVision]** อ่านข้อมูลเสร็จแล้วครับ:\n\n{clean_text}",
                "raw_text": clean_text
            }


        except Exception as e:
            logger.error(f"Error in AetoxVision: {e}")
            return {"status": "failure", "error": f"เกิดข้อผิดพลาดในการอ่านไฟล์: {str(e)}"}
