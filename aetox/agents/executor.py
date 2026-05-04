import logging
import json
from typing import Dict, Any, Optional
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.safety.permission import PermissionManager
from aetox.tools.loader import create_default_registry  # ← ใหม่

logger = logging.getLogger("aetox.agents.executor")

class ExecutorAgent:
    """
    Executor Agent - Dynamic Tool Discovery Edition
    ไม่ hardcode tool list อีกต่อไป
    """
    def __init__(self):
        self.client = OllamaClient()
        self.engine = PromptEngine()
        self.permission_manager = PermissionManager()

        # Load Model Config
        try:
            import yaml
            with open("config/models.yaml", 'r') as f:
                config = yaml.safe_load(f)
                self.model = config.get("executor", "qwen2.5:14b")
        except Exception:
            self.model = "qwen2.5:14b"

        # ✅ Dynamic Registry แทน hardcode list
        self.tools = create_default_registry()

        self.last_path = None
        self.history = []

        logger.info(
            f"ExecutorAgent ready using model {self.model}. "
            f"Tools loaded: {self.tools.list_names()}"
        )

    def add_to_history(self, question: str, answer: str):
        q_trunc = question[:200]
        a_trunc = answer[:200] if isinstance(answer, str) else str(answer)[:200]
        self.history.append({"q": q_trunc, "a": a_trunc})
        if len(self.history) > 3:
            self.history.pop(0)

    def _get_tools_info(self) -> str:
        """
        ✅ ดึง prompt doc จาก registry แทน hardcode
        เพิ่ม tool ใหม่ → ไม่ต้องแตะ method นี้เลย
        """
        return self.tools.build_prompt_doc()

    def extract_action(
        self,
        task_step: Dict[str, Any],
        context: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """ใช้ LLM สกัด intent พร้อม dynamic tool discovery"""
        description = task_step.get("description", "")

        history_str = ""
        for i, h in enumerate(self.history):
            history_str += f"{i+1}. ถาม: {h['q']} -> ตอบ: {h['a']}\n"

        prompt_data = self.engine.get_external_template(
            "config/prompts/executor.yaml",
            "intent_extraction"
        )

        system_msg = prompt_data.get("system_template", "").format(
            tools=self._get_tools_info(),  # ← inject จาก registry
            history=history_str or "ไม่มี",
            last_path=self.last_path or "ยังไม่มี"
        )
        user_msg = prompt_data.get("user_input_template", "").format(
            description=description
        )

        messages = [
            {"role": "system", "content": system_msg},
            {"role": "user", "content": user_msg}
        ]

        try:
            result = self.client.chat(
                model=self.model,
                messages=messages,
                format="json"
            )
            content = result.get("message", {}).get("content", "{}")
            extraction = json.loads(content)

            if extraction.get("confidence", 0) < 0.5:
                return {
                    "tool": "chat",
                    "action": "reply",
                    "params": {"message": description},
                    "confidence": 1.0
                }

            return extraction

        except Exception as e:
            logger.error(f"Extraction failed: {e}")
            return {
                "tool": "chat",
                "action": "reply",
                "params": {"message": description},
                "confidence": 1.0
            }

    def run_action(
        self,
        extraction: Dict[str, Any],
        memory_context: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """
        ✅ Dynamic lookup แทน if/elif hardcode
        เพิ่ม tool ใหม่ → method นี้ไม่ต้องแตะเลย
        """
        tool_name = extraction.get("tool")
        action = extraction.get("action")
        params = extraction.get("params", {})

        # บันทึก last_path ถ้ามี
        if params.get("path") and params.get("path") != ".":
            self.last_path = params.get("path")

        # Chat mode (special case ไม่ใช่ tool จริง)
        if tool_name == "chat":
            return self._handle_chat(extraction)

        # ✅ Dynamic execution ผ่าน registry
        result = self.tools.execute(tool_name, params)

        # Vision special case: ถ้าต้องสรุปเพิ่ม
        if (
            tool_name == "aetox_vision"
            and result.get("status") == "success"
            and "raw_text" in result
            and action == "summarize"
        ):
            result = self._summarize_vision_result(result)

        return result

    def _handle_chat(self, extraction: Dict[str, Any]) -> Dict[str, Any]:
        """จัดการ chat mode แยกออกมาให้สะอาด"""
        system_prompt = (
            "คุณคือ AetoxOS ระบบปฏิบัติการอัจฉริยะที่พัฒนาโดยทีม Aetox "
            "คุณมีบุคลิกที่เป็นมิตร มืออาชีพ และพร้อมช่วยเหลือผู้ใช้เสมอ "
            "ตอบกลับเป็นภาษาไทยที่สุภาพและเป็นธรรมชาติ"
        )

        history_messages = []
        for h in self.history:
            history_messages.append({"role": "user", "content": h["q"]})
            history_messages.append({"role": "assistant", "content": h["a"]})

        messages = [
            {"role": "system", "content": system_prompt},
            *history_messages,
            {"role": "user", "content": extraction.get("params", {}).get("message", "")}
        ]

        result = self.client.chat(model=self.model, messages=messages)
        response = result.get("message", {}).get("content", "ขออภัยครับ ผมนึกไม่ออก")

        return {"status": "success", "output": response, "memory_updates": {}}

    def _summarize_vision_result(self, result: Dict[str, Any]) -> Dict[str, Any]:
        """สรุปเนื้อหาจาก vision แยกออกมา"""
        summary_prompt = (
            "คุณคือผู้เชี่ยวชาญด้านการสรุปข้อมูล\n"
            "สรุปเนื้อหาต่อไปนี้แบบสั้น กระชับ ตรงประเด็น ภาษาไทย:\n\n"
            f"{result['raw_text'][:8000]}\n\nสรุป:"
        )
        res = self.client.chat(
            model=self.model,
            messages=[{"role": "user", "content": summary_prompt}]
        )
        summary_text = res.get("message", {}).get("content", "สรุปไม่ได้ครับ")
        result["output"] = f"👁️ **[AetoxVision - Super Summary]**\n\n{summary_text}"
        return result

    def run_chat_stream(self, message: str):
        """Stream tokens สำหรับ chat mode"""
        system_prompt = (
            "คุณคือ AetoxOS ระบบปฏิบัติการอัจฉริยะที่พัฒนาโดยทีม Aetox "
            "ตอบกลับเป็นภาษาไทยที่สุภาพและเป็นธรรมชาติ"
        )
        messages = [
            {"role": "system", "content": system_prompt},
            {"role": "user", "content": message}
        ]
        for token in self.client.chat_stream(model=self.model, messages=messages):
            yield token
