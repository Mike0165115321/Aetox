import logging
from typing import Dict, List, Any, Optional
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.safety.permissions import PermissionManager

logger = logging.getLogger("aetox.agents.executor")

class ExecutorAgent:
    """
    Executor Agent - Clean Slate Edition.
    All legacy tools have been removed. Waiting for new tool designs.
    """
    def __init__(self):
        self.client = OllamaClient()
        self.engine = PromptEngine()
        self.permission_manager = PermissionManager()
        # All tools have been disconnected.

    def extract_action(self, task_step: Dict[str, Any], context: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """
        Extracts intent. If it's just a conversation, returns 'chat'.
        """
        description = task_step.get("description", "").lower()
        # Heuristic for simple chat
        if any(k in description for k in ["สวัสดี", "hello", "hi", "หวัดดี", "เป็นไงบ้าง"]):
            return {"tool": "chat", "action": "reply", "params": {"message": description}, "confidence": 1.0}
            
        return {"tool": "none", "action": "none", "params": {}, "confidence": 0.0}

    def run_action(self, extraction: Dict[str, Any], memory_context: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """
        Handles 'chat' or reports 'no tools'.
        """
        tool = extraction.get("tool")
        if tool == "chat":
            # Just let the model talk back normally
            return {
                "status": "success",
                "output": "AI is ready to chat. (Clean State Mode)",
                "memory_updates": {}
            }
            
        return {
            "status": "failure", 
            "error": "No tools available. AetoxOS is currently in clean-slate mode.",
            "output": None
        }
