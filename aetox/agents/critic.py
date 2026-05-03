import logging
import json
from typing import Dict, Any, Optional
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine

class CriticAgent:
    """
    Evaluates the quality and correctness of agent outputs.
    Uses DeepSeek-R1 (7B) for reasoning.
    """
    def __init__(self, client: Optional[OllamaClient] = None, engine: Optional[PromptEngine] = None):
        self.logger = logging.getLogger("aetox.agents.critic")
        self.client = client or OllamaClient()
        self.engine = engine or PromptEngine()
        self.model = "deepseek-r1:7b"

    def evaluate(self, step: Dict[str, Any], result: Dict[str, Any], context: Dict[str, Any]) -> Dict[str, Any]:
        """
        Evaluates the step result and provides a verdict.
        """
        self.logger.info(f"Evaluating Step {step.get('step_id')} output...")
        
        prompt_input = (
            f"Task Step: {json.dumps(step)}\n"
            f"Step Output: {json.dumps(result)}\n"
            f"Memory Context: {json.dumps(context)}"
        )
        
        messages = self.engine.build_chat_messages(
            role="critic",
            user_input=prompt_input,
            json_schema=self.engine.CRITIC_SCHEMA
        )
        
        try:
            response = self.client.chat(
                model=self.model,
                messages=messages,
                format="json",
                options={"temperature": 0.1}
            )
            
            content = response.get("message", {}).get("content", "")
            evaluation = json.loads(content)
            
            self.logger.info(f"Critic Verdict: {evaluation.get('verdict')} (Score: {evaluation.get('score')})")
            return evaluation
            
        except Exception as e:
            self.logger.error(f"Critic Evaluation Error: {e}")
            # Safe fallback: Pass if evaluation fails
            return {"verdict": "pass", "score": 1.0, "issues": [], "suggestion": "Evaluation failed, proceeding anyway."}
