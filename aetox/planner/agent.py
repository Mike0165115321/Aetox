import json
import logging
import yaml
from typing import Dict, Any, Optional
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.core.config_loader import config_loader
from aetox.tools.loader import create_default_registry

logger = logging.getLogger("aetox.planner.agent")

class AetoxPlanner:
    """
    Asynchronous Strategic Planner for AetoxClaw.
    Now with Tool-Awareness for realistic task decomposition.
    """
    def __init__(self, client: Optional[OllamaClient] = None, engine: Optional[PromptEngine] = None):
        self.client = client or OllamaClient()
        self.engine = engine or PromptEngine()
        self.tools = create_default_registry()
        
        self.model = config_loader.get_model("planner")
        self.options = config_loader.get_options("planner")
            
        logger.info(f"AetoxPlanner ready using model: {self.model}")

    async def create_plan(self, user_goal: str) -> Dict[str, Any]:
        """Generates a realistic plan based on available tools."""
        logger.info(f"Generating tool-aware plan for: {user_goal}")
        
        tools_info = self.tools.build_prompt_doc()
        
        # We inject tool info into the planning context
        planning_context = (
            f"รายการเครื่องมือที่ระบบมี (Capabilities):\n{tools_info}\n\n"
            f"เป้าหมายผู้ใช้: {user_goal}"
        )

        messages = self.engine.build_chat_messages(
            role="planner",
            user_input=planning_context,
            json_schema=self.engine.PLANNER_SCHEMA
        )

        try:
            response = await self.client.chat(
                model=self.model,
                messages=messages,
                format="json",
                options=self.options
            )
            
            content = response.get("message", {}).get("content", "{}")
            plan = json.loads(content)
            return plan
            
        except Exception as e:
            logger.error(f"Planning failed: {str(e)}")
            return {"plan_id": "error", "goal": user_goal, "steps": [], "error": str(e)}
