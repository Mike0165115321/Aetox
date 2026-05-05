import asyncio
import json
import unittest
from unittest.mock import MagicMock, AsyncMock
from aetox.agents.main_agent import MainAgent

class TestUnifiedOrchestration(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self):
        # Mock memory to avoid heavy dependencies
        self.memory = MagicMock()
        self.memory.goal = "Test"
        self.memory.active_chunks = []
        self.memory.artifacts = {}
        self.memory.set_active_context = AsyncMock()
        self.memory.get_active_context = AsyncMock(return_value={})
        self.memory.get_full_context_async = AsyncMock(return_value={"res": "ok"})
        self.memory.retrieve_relevant = MagicMock(return_value=[])
        self.memory.format_history = MagicMock(return_value="None")

        # Mock Agent
        self.agent = MagicMock(spec=MainAgent)
        self.agent.memory = self.memory
        self.agent.ollama = MagicMock()
        self.agent.model = "qwen"
        self.agent.options = {}
        
        self.agent.ollama.generate = AsyncMock(return_value={
            "response": json.dumps({
                "steps": [{"step_id": 1, "description": "Step 1"}],
                "goal": "Test Goal"
            })
        })

        self.dispatcher = MagicMock()
        self.dispatcher.run_plan = AsyncMock(return_value={"status": "success", "data": {"final": "done"}})
        self.agent.dispatcher = self.dispatcher

    async def test_execute_task_flow(self):
        result = await MainAgent.execute_task(self.agent, "task_001", "Hello")
        self.assertEqual(result["status"], "success")
        self.assertIn("done", str(result))

if __name__ == "__main__":
    unittest.main()
