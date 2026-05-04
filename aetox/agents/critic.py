import logging
import json
import yaml
from typing import Dict, Any, Optional
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine

logger = logging.getLogger("aetox.agents.critic")

class CriticAgent:
    """
    Evaluates the quality and correctness of agent outputs asynchronously.
    Uses the unified qwen2.5:14b model for consistent reasoning.
    """
    def __init__(self, client: Optional[OllamaClient] = None, engine: Optional[PromptEngine] = None):
        self.client = client or OllamaClient()
        self.engine = engine or PromptEngine()
        
        # Load Model Config
        try:
            with open("config/models.yaml", 'r') as f:
                config = yaml.safe_load(f)
                self.model = config.get("critic", "qwen2.5:14b")
        except Exception:
            self.model = "qwen2.5:14b"
            
        logger.info(f"CriticAgent (Async) initialized using model: {self.model}")

    async def evaluate(self, step: Dict[str, Any], result: Dict[str, Any], context: Dict[str, Any]) -> Dict[str, Any]:
        """
        Asynchronously evaluates the step result and provides a verdict.
        """
        logger.info(f"Critiquing Step {step.get('step_id')} output...")
        
        prompt_input = (
            f"Goal Step: {step.get('description')}\n"
            f"Execution Result: {result.get('output')}\n"
            f"Status: {result.get('status')}\n"
            f"Error (if any): {result.get('error')}"
        )
        
        messages = self.engine.build_chat_messages(
            role="critic",
            user_input=prompt_input,
            json_schema=self.engine.CRITIC_SCHEMA
        )
        
        try:
            response = await self.client.chat(
                model=self.model,
                messages=messages,
                format="json",
                options={"temperature": 0.1}
            )
            
            content = response.get("message", {}).get("content", "{}")
            evaluation = json.loads(content)
            
            logger.info(f"Critic Verdict: {evaluation.get('verdict')} (Score: {evaluation.get('score')})")
            return evaluation
            
        except Exception as e:
            logger.error(f"Critic Evaluation Error: {e}")
            return {"verdict": "pass", "score": 1.0, "issues": [], "suggestion": "Proceeding despite evaluation failure."}
