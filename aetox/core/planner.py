import json
import logging
from typing import Dict, List, Any
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine

logger = logging.getLogger("aetox.core.planner")

class Planner:
    """
    Decomposes user goals into a structured TaskPlan.
    """
    PLAN_SCHEMA = {
        "plan_id": "string",
        "goal": "string",
        "steps": [
            {
                "step_id": "integer",
                "description": "string",
                "agent": "executor | researcher | coder",
                "tool": "string",
                "memory_needed": ["list of strings"],
                "success_criteria": "string"
            }
        ],
        "estimated_steps": "integer"
    }

    def __init__(self, client: OllamaClient, engine: PromptEngine):
        self.client = client
        self.engine = engine
        self.model = "qwen2.5:14b"

    def create_plan(self, user_goal: str) -> Dict[str, Any]:
        logger.info(f"Planning for goal: {user_goal}")
        
        messages = self.engine.build_chat_messages(
            role="planner",
            user_input=user_goal,
            json_schema=self.PLAN_SCHEMA
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
            
            logger.info(f"Plan created with {len(plan.get('steps', []))} steps.")
            return plan
            
        except Exception as e:
            logger.error(f"Failed to create plan: {str(e)}")
            raise
