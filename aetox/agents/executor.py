import logging
import json
from typing import Dict, Any, Optional
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.safety.permission import PermissionManager
from aetox.tools.loader import create_default_registry

logger = logging.getLogger("aetox.agents.executor")

class ExecutorAgent:
    """
    Executor Agent - Asynchronous Edition
    Handles intent extraction and action execution without blocking.
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

        self.tools = create_default_registry()
        self.last_path = None
        self.history = []

        logger.info(f"ExecutorAgent initialized with async support using model: {self.model}")

    def add_to_history(self, question: str, answer: str):
        q_trunc = question[:200]
        a_trunc = answer[:200] if isinstance(answer, str) else str(answer)[:200]
        self.history.append({"q": q_trunc, "a": a_trunc})
        if len(self.history) > 3:
            self.history.pop(0)

    def _get_tools_info(self) -> str:
        return self.tools.build_prompt_doc()

    async def extract_action(
        self,
        task_step: Dict[str, Any],
        context: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """Asynchronously extract intent using LLM."""
        description = task_step.get("description", "")
        
        # 🧠 SMART HISTORY: Use WorkingMemory history if available, otherwise fallback to local history
        if context and context.get("history"):
            history_str = context.get("history")
        else:
            history_str = "".join([f"{i+1}. ถาม: {h['q']} -> ตอบ: {h['a']}\n" for i, h in enumerate(self.history)])

        prompt_data = self.engine.get_external_template("config/prompts/executor.yaml", "intent_extraction")
        system_msg = prompt_data.get("system_template", "").format(
            tools=self._get_tools_info(),
            history=history_str or "ไม่มี",
            last_path=self.last_path or "ยังไม่มี",
            global_goal=context.get("global_goal", "ไม่ได้ระบุ") if context else "ไม่ได้ระบุ"
        )

        user_msg = prompt_data.get("user_input_template", "").format(description=description)

        # 🚀 FORCE IDENTITY: ตอกย้ำว่าห้ามปฏิเสธงาน
        messages = [
            {"role": "system", "content": system_msg + "\nIMPORTANT: You ARE AetoxOS. You MUST use tools to fulfill requests. NEVER say you cannot access files."},
            {"role": "user", "content": user_msg}
        ]

        # 🧠 INCREASE BRAIN CAPACITY: ตั้งค่า num_ctx เป็น 8192 เพื่อให้จำประวัติงานได้ครบถ้วน
        options = {"num_ctx": 8192, "temperature": 0}

        try:
            result = await self.client.chat(model=self.model, messages=messages, format="json", options=options)

            content = result.get("message", {}).get("content", "{}")
            extraction = json.loads(content)

            if extraction.get("confidence", 0) < 0.5:
                return {"tool": "chat", "action": "reply", "params": {"message": description}, "confidence": 1.0}
            return extraction
        except Exception as e:
            logger.error(f"Async Extraction failed: {e}")
            return {"tool": "chat", "action": "reply", "params": {"message": description}, "confidence": 1.0}

    async def run_action(self, extraction: Dict[str, Any], context: Dict[str, Any]) -> Dict[str, Any]:
        """Asynchronously executes the tool based on extraction."""
        tool_name = extraction.get("tool")
        action = extraction.get("action")
        params = extraction.get("params", {})
        
        # 🛠️ FIX: Inject action into params for the Tool's internal Router layer
        if action:
            params["action"] = action

        
        # --- TERMINAL LOG: Show what's actually happening ---
        print(f"[TOOL] ⚙️ Calling: {tool_name} -> {action} with {params}")

        if tool_name == "none" or tool_name == "other":
            return {"status": "failure", "error": "No valid tool selected."}

        if params.get("path") and params.get("path") != ".":
            self.last_path = params.get("path")

        if tool_name == "chat":
            return await self._handle_chat(extraction)

        # Dynamic execution remains synchronous for now as tools are synchronous
        # but the agent wrapper around them is async.
        result = self.tools.execute(tool_name, params)

        # --- TERMINAL LOG: Show the actual result from the tool ---
        if result.get("status") == "success":
            print(f"[TOOL] ✅ Success: {result.get('output')}")
        else:
            print(f"[TOOL] ❌ Failure: {result.get('error')}")

        if tool_name == "aetox_vision" and result.get("status") == "success" and action == "summarize":
            result = await self._summarize_vision_result(result)
        return result

    async def _handle_chat(self, extraction: Dict[str, Any]) -> Dict[str, Any]:
        system_prompt = (
            "คุณคือ AetoxOS ระบบปฏิบัติการอัจฉริยะที่พัฒนาโดยทีม Aetox "
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
        result = await self.client.chat(model=self.model, messages=messages)
        response = result.get("message", {}).get("content", "ขออภัยครับ ผมนึกไม่ออก")
        return {"status": "success", "output": response, "memory_updates": {}}

    async def _summarize_vision_result(self, result: Dict[str, Any]) -> Dict[str, Any]:
        summary_prompt = f"สรุปเนื้อหาต่อไปนี้แบบสั้น กระชับ ตรงประเด็น ภาษาไทย:\n\n{result['raw_text'][:8000]}\n\nสรุป:"
        res = await self.client.chat(model=self.model, messages=[{"role": "user", "content": summary_prompt}])
        summary_text = res.get("message", {}).get("content", "สรุปไม่ได้ครับ")
        result["output"] = f"👁️ **[AetoxVision - Summary]**\n\n{summary_text}"
        return result

    async def run_chat_stream(self, message: str):
        """Asynchronous stream generator for chat tokens."""
        system_prompt = "คุณคือ AetoxOS ตอบกลับเป็นภาษาไทยที่สุภาพและเป็นธรรมชาติ"
        messages = [
            {"role": "system", "content": system_prompt},
            {"role": "user", "content": message}
        ]
        async for token in self.client.chat_stream(model=self.model, messages=messages):
            yield token
