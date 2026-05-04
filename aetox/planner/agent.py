import json
import logging
import yaml
from typing import Dict, Any, Optional
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine

logger = logging.getLogger("aetox.planner.agent")

class AetoxPlanner:
    """
    Strategic Planner for AetoxOS.
    Responsible for decomposing complex user goals into executable TaskPlans.
    Uses qwen2.5:14b exclusively for high-reasoning accuracy.
    """
    def __init__(self, client: Optional[OllamaClient] = None, engine: Optional[PromptEngine] = None):
        self.client = client or OllamaClient()
        self.engine = engine or PromptEngine()
        
        # Load Model Config
        try:
            with open("config/models.yaml", 'r') as f:
                config = yaml.safe_load(f)
                self.model = config.get("planner", "qwen2.5:14b")
        except Exception:
            self.model = "qwen2.5:14b"
            
        logger.info(f"AetoxPlanner initialized using model: {self.model}")

    def create_plan(self, user_goal: str) -> Dict[str, Any]:
        """
        Generates a structured multi-step plan for the given goal.
        """
        logger.info(f"Generating plan for: {user_goal}")
        
        # Use PromptEngine to build messages for the planner role
        messages = self.engine.build_chat_messages(
            role="planner",
            user_input=user_goal,
            json_schema=self.engine.PLANNER_SCHEMA
        )

        try:
            response = self.client.chat(
                model=self.model,
                messages=messages,
                format="json",
                options={"temperature": 0.2} # Low temperature for consistent structure
            )
            
            content = response.get("message", {}).get("content", "{}")
            plan = json.loads(content)
            
            steps_count = len(plan.get('steps', []))
            logger.info(f"Successfully generated plan with {steps_count} steps.")
            return plan
            
        except Exception as e:
            logger.error(f"Planning failed: {str(e)}")
            return {
                "plan_id": "error",
                "goal": user_goal,
                "steps": [],
                "error": str(e)
            }
