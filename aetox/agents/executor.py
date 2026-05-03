import logging
import os
from typing import Dict, List, Any, Optional
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.safety.permission import PermissionManager
from aetox.tools.file_manager import MasterFileManager
from aetox.tools.vision import AetoxVision

logger = logging.getLogger("aetox.agents.executor")

class ExecutorAgent:
    """
    Executor Agent - Master Edition.
    Equipped with MasterFileManager and AetoxVision.
    """
    def __init__(self):
        self.client = OllamaClient()
        self.engine = PromptEngine()
        self.permission_manager = PermissionManager()
        self.file_manager = MasterFileManager()
        self.vision = AetoxVision()
        self.last_path = None
        self.history = [] # Rolling buffer of last 3 interactions

    def add_to_history(self, question: str, answer: str):
        """Adds a Q&A pair to history, keeping only the last 3 and truncating to 200 chars."""
        q_trunc = question[:200]
        a_trunc = answer[:200] if isinstance(answer, str) else str(answer)[:200]
        self.history.append({"q": q_trunc, "a": a_trunc})
        if len(self.history) > 3:
            self.history.pop(0)

    def extract_action(self, task_step: Dict[str, Any], context: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """
        Uses LLM to extract intent with limited 3-step history context.
        """
        description = task_step.get("description", "")
        
        # Format history for prompt
        history_str = ""
        for i, h in enumerate(self.history):
            history_str += f"{i+1}. ถาม: {h['q']} -> ตอบ: {h['a']}\n"

        # High-Precision Extraction Prompt
        extraction_prompt = (
            "คุณคือผู้ช่วยสกัดคำสั่ง (Intent Extractor) สำหรับ AetoxOS\n"
            "กฎการเลือกเครื่องมือ:\n"
            "1. aetox_vision: ใช้เมื่อต้องการ 'ดู' (view/list), 'อ่าน' (read), 'สรุป' (summarize), 'แสดงไฟล์'\n"
            "   - คำสำคัญ: ดู, มีอะไรบ้าง, สรุปที, อ่านไฟล์, ลิสต์ไฟล์\n"
            "2. master_file_manager: ใช้เมื่อต้องการ 'จัดระเบียบ' (organize), 'แยกหมวดหมู่', 'ย้ายไฟล์เข้าโฟลเดอร์'\n"
            "   - คำสำคัญ: จัดระเบียบ, จัดตำแหน่ง, แยกไฟล์, จัดเข้าที่\n"
            "3. chat: ใช้สำหรับการพูดคุยทั่วไป\n\n"
            f"ประวัติ 3 ครั้งล่าสุด:\n{history_str or 'ไม่มี'}\n"
            f"ความจำล่าสุด (Path): '{self.last_path or 'ยังไม่มี'}'\n\n"
            "กฎเหล็ก:\n"
            "- รวมพาธจากความจำล่าสุดถ้าผู้ใช้ระบุแค่ชื่อไฟล์\n"
            "- ห้ามซ้ำคำเวลาต่อพาธ (เช่น memory + '\\' + filename)\n"
            "- แยกพาธไฟล์ (Path) ออกจากคำสั่งภาษาไทยให้ชัดเจน\n\n"
            "โปรดตอบกลับเป็น JSON เท่านั้น:\n"
            "{\n"
            "  \"tool\": \"ชื่อเครื่องมือ\",\n"
            "  \"action\": \"ชื่อคำสั่ง\",\n"
            "  \"params\": {\"path\": \"ที่อยู่ไฟล์/โฟลเดอร์\", \"message\": \"ข้อความแชท\"},\n"
            "  \"confidence\": 0.0-1.0\n"
            "}\n\n"
            f"คำสั่งผู้ใช้: \"{description}\""
        )
        
        try:
            result = self.client.chat(model="qwen2.5:14b", messages=[{"role": "user", "content": extraction_prompt}], format="json")
            content = result.get("message", {}).get("content", "{}")
            import json
            extraction = json.loads(content)
            
            # Smart defaults if extraction is weak
            if extraction.get("confidence", 0) < 0.5:
                return {"tool": "chat", "action": "reply", "params": {"message": description}, "confidence": 1.0}
                
            return extraction
        except Exception as e:
            logger.error(f"Extraction failed, falling back to chat: {e}")
            return {"tool": "chat", "action": "reply", "params": {"message": description}, "confidence": 1.0}

    def run_action(self, extraction: Dict[str, Any], memory_context: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """
        Executes tools based on extraction.
        """
        tool = extraction.get("tool")
        action = extraction.get("action")
        params = extraction.get("params", {})
        
        # Save last path if it's a valid directory or file
        if params.get("path") and params.get("path") != ".":
            self.last_path = params.get("path")

        if tool == "master_file_manager":
            return self.file_manager.execute(params)
        
        if tool == "aetox_vision":
            result = self.vision.execute(params)
            # If it's a successful read and user asked for summary, wrap it with LLM summary
            if result.get("status") == "success" and "raw_text" in result:
                # Decide if we need to summarize based on intent
                if action == "summarize" or any(k in description for k in ["สรุป", "summary"]):
                    summary_prompt = (
                        "คุณคือผู้เชี่ยวชาญด้านการสรุปข้อมูล (Super Summarizer)\n"
                        "หน้าที่ของคุณคือ: อ่านข้อความต่อไปนี้แล้วสรุปแบบ 'ขั้นสุด' (สั้น กระชับ ตรงประเด็น)\n"
                        "ไม่ต้องอ่านเนื้อหาให้ฟัง! ให้บอกแค่ว่า 'เอกสารนี้เกี่ยวกับอะไร' เท่านั้น\n\n"
                        f"เนื้อหาจากไฟล์:\n{result['raw_text'][:8000]}\n\n"
                        "สรุปแบบขั้นสุด (ภาษาไทย):"
                    )
                    res = self.client.chat(model="qwen2.5:14b", messages=[{"role": "user", "content": summary_prompt}])
                    summary_text = res.get("message", {}).get("content", "สรุปไม่ได้ครับ")
                    result["output"] = f"👁️ **[AetoxVision - Super Summary]**\n\n{summary_text}"
            
            return result

        if tool == "chat":
            # Real AI Chat response with strong persona
            system_prompt = (
                "คุณคือ AetoxOS ระบบปฏิบัติการอัจฉริยะที่พัฒนาโดยทีม Aetox "
                "คุณมีบุคลิกที่เป็นมิตร มืออาชีพ และพร้อมช่วยเหลือผู้ใช้เสมอ "
                "คุณคือ AetoxOS เท่านั้น! ตอบกลับเป็นภาษาไทยเท่านั้นที่สุภาพและเป็นธรรมชาติ"
            )
            # Include history for chat context
            history_messages = []
            for h in self.history:
                history_messages.append({"role": "user", "content": h["q"]})
                history_messages.append({"role": "assistant", "content": h["a"]})

            messages = [
                {"role": "system", "content": system_prompt},
                *history_messages,
                {"role": "user", "content": extraction.get('params', {}).get('message', '')}
            ]
            
            result = self.client.chat(model="qwen2.5:14b", messages=messages)
            response = result.get("message", {}).get("content", "ขออภัยครับ ผมนึกไม่ออก")
            
            return {
                "status": "success",
                "output": response,
                "memory_updates": {}
            }
            
        return {
            "status": "failure", 
            "error": "ขออภัยครับ ตอนนี้ผมยังไม่มีเครื่องมือสำหรับทำสิ่งนี้ (โหมดพื้นฐาน)",
            "output": None
        }

    def run_chat_stream(self, message: str):
        """
        Yields tokens from the LLM for a chat message with a strong persona.
        """
        system_prompt = (
            "คุณคือ AetoxOS ระบบปฏิบัติการอัจฉริยะที่พัฒนาโดยทีม Aetox "
            "คุณมีบุคลิกที่เป็นมิตร มืออาชีพ และพร้อมช่วยเหลือผู้ใช้เสมอ "
            "คุณคือ AetoxOS เท่านั้น! ตอบกลับเป็นภาษาไทยเท่านั้นที่สุภาพและเป็นธรรมชาติ"
        )
        messages = [
            {"role": "system", "content": system_prompt},
            {"role": "user", "content": message}
        ]
        
        # Stream tokens from Ollama
        for token in self.client.chat_stream(model="qwen2.5:14b", messages=messages):
            yield token
