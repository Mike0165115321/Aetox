import logging
from typing import Optional
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.agents.executor import ExecutorAgent
from aetox.agents.critic import CriticAgent
from aetox.planner.agent import AetoxPlanner

logger = logging.getLogger("aetox.core.factory")

class AgentFactory:
    """
    Centralized Factory for creating AetoxClaw Agents.
    Handles dependency sharing (Client, Engine) to optimize resource usage.
    """
    def __init__(self, client: Optional[OllamaClient] = None, engine: Optional[PromptEngine] = None):
        self.shared_client = client or OllamaClient()
        self.shared_engine = engine or PromptEngine()
        logger.info("AgentFactory initialized with shared resources.")

    def create_executor(self) -> ExecutorAgent:
        """Creates an ExecutorAgent with shared resources."""
        return ExecutorAgent(client=self.shared_client, engine=self.shared_engine)

    def create_critic(self) -> CriticAgent:
        """Creates a CriticAgent with shared resources."""
        return CriticAgent(client=self.shared_client, engine=self.shared_engine)

    def create_planner(self) -> AetoxPlanner:
        """Creates an AetoxPlanner with shared resources."""
        return AetoxPlanner(client=self.shared_client, engine=self.shared_engine)
