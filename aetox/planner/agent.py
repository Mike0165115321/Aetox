import json
import logging
import yaml
from typing import Dict, Any, Optional
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine

logger = logging.getLogger("aetox.planner.agent")

class AetoxPlanner:
    """
    Asynchronous Strategic Planner for AetoxOS.
    """
    def __init__(self, client: Optional[OllamaClient] = None, engine: Optional[PromptEngine] = None):
        self.client = client or OllamaClient()
        self.engine = engine or PromptEngine()
        
        try:
            with open("config/models.yaml", 'r') as f:
                config = yaml.safe_load(f)
                self.model = config.get("planner", "qwen2.5:14b")
        except Exception:
            self.model = "qwen2.5:14b"
            
        logger.info(f"AetoxPlanner (Async) ready using model: {self.model}")

    async def create_plan(self, user_goal: str) -> Dict[str, Any]:
        """Generates a plan asynchronously."""
        logger.info(f"Generating plan for: {user_goal}")
        
        messages = self.engine.build_chat_messages(
            role="planner",
            user_input=user_goal,
            json_schema=self.engine.PLANNER_SCHEMA
        )

        try:
            response = await self.client.chat(
                model=self.model,
                messages=messages,
                format="json",
                options={"temperature": 0.2}
            )
            
            content = response.get("message", {}).get("content", "{}")
            plan = json.loads(content)
            return plan
            
        except Exception as e:
            logger.error(f"Planning failed: {str(e)}")
            return {"plan_id": "error", "goal": user_goal, "steps": [], "error": str(e)}
