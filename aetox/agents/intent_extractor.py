import logging
import json
from typing import Dict, Any, Optional, List
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.core.config_loader import config_loader
from aetox.tools.loader import create_default_registry

logger = logging.getLogger("aetox.agents.extractor")

class IntentExtractor:
    """
    Intent Extractor Agent
    Specializes in parsing user intent and mapping it to specific tools/actions.
    Separated from Executor to follow SRP (Single Responsibility Principle).
    """
    def __init__(self):
        self.client = OllamaClient()
        self.engine = PromptEngine()
        
        # Load Model Config for 'extraction' role
        self.model = config_loader.get_model("extraction")
        self.options = config_loader.get_options("extraction")
        
        # We need tool info to build the system prompt
        self.tools = create_default_registry()
        
        self.history: List[Dict[str, str]] = []
        self.last_path: Optional[str] = None

        logger.info(f"IntentExtractor initialized using model: {self.model}")

    def add_to_history(self, question: str, answer: str):
        """Maintains a short history for context-aware extraction."""
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
        """
        Asynchronously extract intent using LLM.
        Returns a dictionary containing 'tool', 'action', and 'params'.
        """
        description = task_step.get("description", "")
        
        # 🧠 SMART HISTORY: Use WorkingMemory history if available, otherwise fallback to local history
        if context and context.get("history"):
            history_str = context.get("history")
        else:
            history_str = "".join([f"{i+1}. ถาม: {h['q']} -> ตอบ: {h['a']}\n" for i, h in enumerate(self.history)])

        # Load prompt template
        prompt_data = self.engine.get_external_template("config/prompts/executor.yaml", "intent_extraction")
        system_msg = prompt_data.get("system_template", "").format(
            tools=self._get_tools_info(),
            history=history_str or "ไม่มี",
            last_path=self.last_path or "ยังไม่มี",
            global_goal=context.get("global_goal", "ไม่ได้ระบุ") if context else "ไม่ได้ระบุ"
        )

        user_msg = prompt_data.get("user_input_template", "").format(description=description)

        messages = [
            {"role": "system", "content": system_content(system_msg)},
            {"role": "user", "content": user_msg}
        ]

        try:
            result = await self.client.chat(
                model=self.model, 
                messages=messages, 
                format="json", 
                options=self.options
            )

            content = result.get("message", {}).get("content", "{}")
            extraction = json.loads(content)

            # Fallback to chat if confidence is too low
            if extraction.get("confidence", 0) < 0.5:
                return self._fallback_chat(description)
                
            return extraction
        except Exception as e:
            logger.error(f"Async Extraction failed: {e}")
            return self._fallback_chat(description)

    def _fallback_chat(self, message: str) -> Dict[str, Any]:
        return {
            "tool": "chat", 
            "action": "reply", 
            "params": {"message": message}, 
            "confidence": 1.0
        }

def system_content(base_msg: str) -> str:
    """Helper to inject standard identity constraints."""
    return (
        f"{base_msg}\n"
        "IMPORTANT: You ARE AetoxClaw. You MUST use tools to fulfill requests. "
        "NEVER say you cannot access files. Always respond in valid JSON."
    )
