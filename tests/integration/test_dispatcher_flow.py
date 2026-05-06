import pytest
from unittest.mock import AsyncMock, MagicMock
from aetox.core.dispatcher import Dispatcher
from aetox.memory.working import WorkingMemory

class TestDispatcherFlow:
    @pytest.fixture
    def dispatcher(self, mock_memory_config, tmp_path):
        # Update config for memory
        cfg = mock_memory_config.copy()
        cfg["episodic_path"] = str(tmp_path / "episodes.jsonl")
        memory = WorkingMemory(cfg)
        return Dispatcher(memory)

    async def test_run_direct_step_success(self, dispatcher, mock_extraction_response):
        # Mock executor methods since we mocked the executor object
        dispatcher.executor.extract_action = AsyncMock(return_value={"tool": "t", "action": "a", "confidence": 0.9})
        dispatcher.executor.run_action = AsyncMock(return_value={"status": "success", "output": "Done"})
        
        goal = "Test Step"
        result = await dispatcher.run_direct_step(goal)
        
        assert result["status"] == "success"
        assert dispatcher.executor.run_action.called

    async def test_run_plan_success(self, dispatcher, mock_plan_response):
        # Mock executor and extractor
        dispatcher.executor = MagicMock()
        dispatcher.executor.extract_action = AsyncMock(return_value={"tool": "test", "action": "test"})
        dispatcher.executor.run_action = AsyncMock(return_value={"status": "success", "output": "Step Done"})
        
        # Mock critic to pass everything
        dispatcher.critic = MagicMock()
        dispatcher.critic.evaluate = AsyncMock(return_value={"verdict": "pass", "score": 1.0})
        
        # Mock memory.add_step_result to verify calls
        dispatcher.memory.add_step_result = AsyncMock()
        
        await dispatcher.run_plan(mock_plan_response)
        
        # 2 steps in mock_plan_response
        assert dispatcher.executor.run_action.call_count == 2
        assert dispatcher.memory.add_step_result.call_count == 2

    async def test_run_plan_retry_logic(self, dispatcher, mock_plan_response):
        dispatcher.executor = MagicMock()
        dispatcher.executor.extract_action = AsyncMock(return_value={"tool": "test", "action": "test"})
        dispatcher.executor.run_action = AsyncMock(return_value={"status": "failure", "error": "Fail"})
        
        # Mock critic to fail once then pass
        dispatcher.critic = MagicMock()
        dispatcher.critic.evaluate = AsyncMock()
        dispatcher.critic.evaluate.side_effect = [
            {"verdict": "retry", "score": 0.2}, # Fail first
            {"verdict": "pass", "score": 1.0}    # Pass second
        ]
        dispatcher.critic.analyze_failure = AsyncMock(return_value="Fix it")
        
        # We need a plan with only 1 step to test retry easily
        plan = {"steps": [mock_plan_response["steps"][0]]}
        await dispatcher.run_plan(plan)
        
        # Should have called run_action twice due to retry
        assert dispatcher.executor.run_action.call_count == 2
