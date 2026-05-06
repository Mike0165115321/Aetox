import logging
import json
from typing import Dict, Any, Optional, List
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.core.config_loader import config_loader
from aetox.safety.permission import PermissionManager
from aetox.tools.loader import create_default_registry

logger = logging.getLogger("aetox.agents.executor")

class ExecutorAgent:
    """
    Executor Agent — Stateless Edition
    ไม่มี internal history — รับ history จาก context ภายนอก
    """
    def __init__(self, client: Optional[OllamaClient] = None, engine: Optional[PromptEngine] = None):
        self.client = client or OllamaClient()
        self.engine = engine or PromptEngine()
        self.permission_manager = PermissionManager()

        # Load Model Config
        self.model = config_loader.get_model("executor")
        self.options = config_loader.get_options("executor")
        self.extraction_model = config_loader.get_model("extraction")
        self.extraction_options = config_loader.get_options("extraction")

        self.tools = create_default_registry()
        self.last_path = None

        logger.info(f"ExecutorAgent initialized (stateless) using model: {self.model}")

    def _get_tools_info(self) -> str:
        return self.tools.build_prompt_doc()

    async def extract_action(
        self,
        task_step: Dict[str, Any],
        context: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """Asynchronously extract intent using LLM."""
        description = task_step.get("description", "")
        
        # 🧠 History จากภายนอก (Dispatcher หรือ SessionContext ส่งมา)
        history_str = context.get("history", "ไม่มี") if context else "ไม่มี"

        prompt_data = self.engine.get_external_template("config/prompts/executor.yaml", "intent_extraction")
        system_msg = prompt_data.get("system_template", "").format(
            tools=self._get_tools_info(),
            history=history_str,
            last_path=self.last_path or "ยังไม่มี",
            global_goal=context.get("global_goal", "ไม่ได้ระบุ") if context else "ไม่ได้ระบุ"
        )

        user_msg = prompt_data.get("user_input_template", "").format(description=description)

        messages = [
            {"role": "system", "content": system_msg + "\nIMPORTANT: You ARE AetoxClaw. You are a capable OS agent with access to tools. If a task is truly impossible (e.g. file doesn't exist), suggest an alternative or ask for clarification, but always try your best to fulfill the request using tools first."},
            {"role": "user", "content": user_msg}
        ]

        options = self.extraction_options

        try:
            result = await self.client.chat(model=self.extraction_model, messages=messages, format="json", options=options)

            content = result.get("message", {}).get("content", "{}")
            extraction = json.loads(content)

            if extraction.get("confidence", 0) < 0.5:
                return {"tool": "chat", "action": "reply", "params": {"message": description}, "confidence": 1.0}
            return extraction
        except json.JSONDecodeError as je:
            logger.error(f"JSON Parsing failed: {je}")
            return {"tool": "chat", "action": "reply", "params": {"message": description}, "confidence": 1.0}
        except Exception as e:
            logger.error(f"Async Extraction failed: {e}")
            return {"tool": "chat", "action": "reply", "params": {"message": description}, "confidence": 1.0}

    async def run_action(self, extraction: Dict[str, Any], context: Dict[str, Any]) -> Dict[str, Any]:
        """Asynchronously executes the tool based on extraction."""
        tool_name = extraction.get("tool")
        action = extraction.get("action")
        params = extraction.get("params", {})
        
        # Inject action into params for the Tool's internal Router layer
        if action:
            params["action"] = action

        # --- TERMINAL LOG ---
        print(f"[TOOL] ⚙️ Calling: {tool_name} -> {action} with {params}")

        if tool_name == "none" or tool_name == "other":
            return {"status": "failure", "error": "No valid tool selected."}

        if params.get("path") and params.get("path") != ".":
            self.last_path = params.get("path")

        if tool_name == "chat":
            return await self._handle_chat(extraction, context)

        # Dynamic execution (tools are synchronous)
        result = self.tools.execute(tool_name, params)

        # --- TERMINAL LOG ---
        status = result.get("status")
        if status == "success":
            print(f"[TOOL] ✅ Success: {result.get('output')}")
        elif status == "chat":
            print(f"[TOOL] 💬 Chat hand-off: Switching to conversation mode...")
        else:
            print(f"[TOOL] ❌ Failure: {result.get('error')}")

        if result.get("status") == "chat":
            extraction["params"]["message"] = result.get("output", "")
            return await self._handle_chat(extraction, context)

        if tool_name == "aetox_vision" and result.get("status") == "success" and action == "summarize":
            result = await self._summarize_vision_result(result)
        return result

    async def _handle_chat(self, extraction: Dict[str, Any], context: Dict[str, Any] = None) -> Dict[str, Any]:
        """Handle chat mode with history from context."""
        system_prompt = (
            "คุณคือ AetoxClaw ระบบปฏิบัติการอัจฉริยะที่พัฒนาโดยทีม Aetox "
            "ตอบกลับเป็นภาษาไทยที่สุภาพและเป็นธรรมชาติ"
        )
        
        # สร้าง history messages จาก context
        history_messages = []
        if context and context.get("history"):
            # history เป็น string format — ไม่ต้องแปลง
            system_prompt += f"\n\nประวัติการคุย:\n{context['history']}"

        messages = [
            {"role": "system", "content": system_prompt},
            *history_messages,
            {"role": "user", "content": extraction.get("params", {}).get("message", "")}
        ]
        result = await self.client.chat(model=self.model, messages=messages, options=self.options)
        response = result.get("message", {}).get("content", "ขออภัยครับ ผมนึกไม่ออก")
        return {"status": "success", "output": response}

    async def _summarize_vision_result(self, result: Dict[str, Any]) -> Dict[str, Any]:
        summary_prompt = f"สรุปเนื้อหาต่อไปนี้แบบสั้น กระชับ ตรงประเด็น ภาษาไทย:\n\n{result['raw_text'][:8000]}\n\nสรุป:"
        res = await self.client.chat(model=self.model, messages=[{"role": "user", "content": summary_prompt}], options=self.options)
        summary_text = res.get("message", {}).get("content", "สรุปไม่ได้ครับ")
        result["output"] = f"👁️ **[AetoxVision - Summary]**\n\n{summary_text}"
        return result

    async def run_chat_stream(self, message: str, context: str = None):
        """Asynchronous stream generator for chat tokens."""
        system_prompt = "คุณคือ AetoxClaw ตอบกลับเป็นภาษาไทยที่สุภาพและเป็นธรรมชาติ"
        if context:
            system_prompt += f"\nบริบทเพิ่มเติม: {context}"
            
        messages = [
            {"role": "system", "content": system_prompt},
            {"role": "user", "content": message}
        ]
        async for token in self.client.chat_stream(model=self.model, messages=messages, options=self.options):
            yield token
