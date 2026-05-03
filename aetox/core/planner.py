import json
import logging
from typing import Dict, List, Any, Optional
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.memory.manager import MemoryManager

class Planner:
    """
    Transforms a user goal into a structured TaskPlan using LLM (14B).
    Injects long-term memory context.
    """
    def __init__(self, client: Optional[OllamaClient] = None, engine: Optional[PromptEngine] = None):
        self.logger = logging.getLogger("aetox.core.planner")
        self.client = client or OllamaClient()
        self.engine = engine or PromptEngine()
        self.memory_manager = MemoryManager()
        self.model = "qwen2.5:14b"

    def create_plan(self, user_goal: str) -> Dict[str, Any]:
        self.logger.info(f"Planning for goal: {user_goal}")
        
        # Fetch Memory Context
        past_context = self.memory_manager.get_context_for_planner(user_goal)
        
        full_input = user_goal
        if past_context:
            full_input = f"{user_goal}\n\n[PAST CONTEXT & PREFERENCES]\n{past_context}"
            self.logger.info("Injected past context into planner.")
            
        messages = self.engine.build_chat_messages(
            role="planner",
            user_input=full_input,
            json_schema=self.engine.PLANNER_SCHEMA
        )

        try:
            response = self.client.chat(
                model=self.model,
                messages=messages,
                format="json",
                options={"temperature": 0.3}
            )
            
            content = response.get("message", {}).get("content", "")
            plan = json.loads(content)
            
            self.logger.info(f"Plan created with {len(plan.get('steps', []))} steps.")
            return plan
            
        except Exception as e:
            self.logger.error(f"Failed to create plan: {str(e)}")
            raise
