import pytest
from unittest.mock import AsyncMock, MagicMock
from aetox.core.dispatcher import Dispatcher

class TestDispatcherFlow:
    @pytest.fixture
    def dispatcher(self):
        # Dispatcher is now stateless — no memory needed
        return Dispatcher()

    async def test_run_direct_step_success(self, dispatcher):
        # Mock executor methods
        dispatcher.executor.extract_action = AsyncMock(return_value={"tool": "t", "action": "a", "confidence": 0.9})
        dispatcher.executor.run_action = AsyncMock(return_value={"status": "success", "output": "Done"})
        
        result = await dispatcher.run_direct_step("Test Step", history=[{"q": "prev", "a": "ok"}])
        
        assert result["status"] == "success"
        assert dispatcher.executor.run_action.called

    async def test_run_plan_success(self, dispatcher, mock_plan_response):
        # Mock executor and critic
        dispatcher.executor = MagicMock()
        dispatcher.executor.extract_action = AsyncMock(return_value={"tool": "test", "action": "test"})
        dispatcher.executor.run_action = AsyncMock(return_value={"status": "success", "output": "Step Done"})
        
        dispatcher.critic = MagicMock()
        dispatcher.critic.evaluate = AsyncMock(return_value={"verdict": "pass", "score": 1.0})
        
        result = await dispatcher.run_plan(mock_plan_response)
        
        # 2 steps in mock_plan_response
        assert result["status"] == "success"
        assert dispatcher.executor.run_action.call_count == 2
        assert len(result["plan_history"]) == 2

    async def test_run_plan_retry_logic(self, dispatcher, mock_plan_response):
        dispatcher.executor = MagicMock()
        dispatcher.executor.extract_action = AsyncMock(return_value={"tool": "test", "action": "test"})
        dispatcher.executor.run_action = AsyncMock(return_value={"status": "failure", "error": "Fail"})
        
        # Mock critic to fail once then pass
        dispatcher.critic = MagicMock()
        dispatcher.critic.evaluate = AsyncMock()
        dispatcher.critic.evaluate.side_effect = [
            {"verdict": "retry", "score": 0.2},  # Fail first
            {"verdict": "pass", "score": 1.0}     # Pass second
        ]
        dispatcher.critic.analyze_failure = AsyncMock(return_value="Fix it")
        
        # 1 step plan to test retry easily
        plan = {"steps": [mock_plan_response["steps"][0]]}
        result = await dispatcher.run_plan(plan)
        
        # Should have called run_action twice due to retry
        assert dispatcher.executor.run_action.call_count == 2
