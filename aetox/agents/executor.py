import logging
from typing import Dict, List, Any, Optional
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.safety.permission import PermissionManager
from aetox.tools.file_manager import MasterFileManager

logger = logging.getLogger("aetox.agents.executor")

class ExecutorAgent:
    """
    Executor Agent - Master Edition.
    Equipped with the MasterFileManager.
    """
    def __init__(self):
        self.client = OllamaClient()
        self.engine = PromptEngine()
        self.permission_manager = PermissionManager()
        self.file_manager = MasterFileManager()

    def extract_action(self, task_step: Dict[str, Any], context: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """
        Extracts intent. Detects if it's an organization task or just chat.
        """
        description = task_step.get("description", "").lower()
        
        # 1. Organization Heuristic (Master Tool)
        if any(k in description for k in ["จัดระเบียบ", "จัดโครงสร้าง", "organize"]):
            # Find a path in the description (naive check)
            path = "."
            if ":" in description: # Likely contains a path like E:\Mike
                import re
                path_match = re.search(r'[a-zA-Z]:\\[^ ]*', description)
                if path_match:
                    path = path_match.group(0)
            
            return {
                "tool": "master_file_manager", 
                "action": "organize", 
                "params": {"path": path}, 
                "confidence": 1.0
            }
        
        # 2. Default to chat
        return {
            "tool": "chat", 
            "action": "reply", 
            "params": {"message": description}, 
            "confidence": 1.0
        }

    def run_action(self, extraction: Dict[str, Any], memory_context: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """
        Executes tools based on extraction.
        """
        tool = extraction.get("tool")
        action = extraction.get("action")
        params = extraction.get("params", {})

        if tool == "master_file_manager":
            # High-risk: Ask for approval
            if not self.permission_manager.request_permission("organize", f"จัดระเบียบไฟล์ในตำแหน่ง: {params.get('path')}"):
                return {"status": "failure", "error": "การอนุมัติถูกปฏิเสธโดยผู้ใช้ครับ"}
            
            return self.file_manager.execute(params)

        if tool == "chat":
            # Real AI Chat response with strong persona
            system_prompt = (
                "คุณคือ AetoxOS ระบบปฏิบัติการอัจฉริยะที่พัฒนาโดยทีม Aetox "
                "คุณมีบุคลิกที่เป็นมิตร มืออาชีพ และพร้อมช่วยเหลือผู้ใช้เสมอ "
                "คุณคือ AetoxOS เท่านั้น! ตอบกลับเป็นภาษาไทยเท่านั้นที่สุภาพและเป็นธรรมชาติ"
            )
            messages = [
                {"role": "system", "content": system_prompt},
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
