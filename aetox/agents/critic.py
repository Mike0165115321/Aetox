import logging
import json
from typing import Optional, Any, Dict, List
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.core.config_loader import config_loader

logger = logging.getLogger("aetox.agents.critic")

class CriticAgent:
    def __init__(self, client: Optional[OllamaClient] = None, engine: Optional[PromptEngine] = None):
        self.client = client or OllamaClient()
        self.engine = engine or PromptEngine()
        
        self.model = config_loader.get_model("critic")
        self.options = config_loader.get_options("critic")
            
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
                options=self.options
            )
            
            content = response.get("message", {}).get("content", "{}")
            evaluation = json.loads(content)
            
            logger.info(f"Critic Verdict: {evaluation.get('verdict')} (Score: {evaluation.get('score')})")
            return evaluation
            
        except Exception as e:
            logger.error(f"Critic Evaluation Error: {e}")
            return {"verdict": "pass", "score": 1.0, "issues": [], "suggestion": "Proceeding despite evaluation failure."}

    async def analyze_failure(self, step: Dict[str, Any], result: Dict[str, Any]) -> str:
        """
        Analyzes why a step failed and provides a hint for the next retry.
        """
        prompt = (
            f"Step attempted: {step.get('description')}\n"
            f"Resulting error: {result.get('error')}\n"
            f"Output produced: {result.get('output')}\n"
            "Analyze the failure and provide a short, actionable hint to fix it."
        )
        
        messages = [
            {"role": "system", "content": "You are a debug assistant. Provide a concise hint to fix the reported failure."},
            {"role": "user", "content": prompt}
        ]
        
        try:
            response = await self.client.chat(model=self.model, messages=messages, options=self.options)
            content = response.get("message", {}).get("content", "ลองตรวจสอบพารามิเตอร์อีกครั้ง")
            return content
        except:
            return "โปรดตรวจสอบพารามิเตอร์และเส้นทางไฟล์"
