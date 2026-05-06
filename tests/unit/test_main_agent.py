import pytest
import json
from unittest.mock import MagicMock, AsyncMock, patch
from aetox.agents.main_agent import MainAgent

@pytest.fixture
def main_agent():
    with patch("aetox.agents.main_agent.config_loader"), \
         patch("aetox.agents.main_agent.WorkingMemory"), \
         patch("aetox.agents.main_agent.OllamaClient"), \
         patch("aetox.agents.main_agent.Dispatcher"):
        agent = MainAgent()
        return agent

@pytest.mark.asyncio
async def test_execute_task_with_plan(main_agent):
    # Setup mocks
    main_agent.ollama.generate = AsyncMock(return_value={
        "response": json.dumps({
            "steps": [{"step_id": 1, "description": "Step 1"}],
            "goal": "Test Goal"
        })
    })
    main_agent.dispatcher.run_plan = AsyncMock(return_value={"status": "success", "data": {}})
    main_agent.memory.set_active_context = AsyncMock()
    main_agent.memory.format_history.return_value = "History"
    main_agent.memory.goal = "Goal"
    
    result = await main_agent.execute_task("task_001", "Do something")
    
    assert result["status"] == "success"
    main_agent.dispatcher.run_plan.assert_called_once()
    assert main_agent.memory.set_active_context.called

@pytest.mark.asyncio
async def test_execute_task_direct_fallback(main_agent):
    # Setup mocks to return no steps
    main_agent.ollama.generate = AsyncMock(return_value={
        "response": json.dumps({"steps": [], "goal": "Test Goal"})
    })
    main_agent.dispatcher.run_direct_step = AsyncMock(return_value={"status": "success", "output": "Direct OK"})
    main_agent.memory.set_active_context = AsyncMock()
    main_agent.memory.format_history.return_value = "History"
    main_agent.memory.goal = "Goal"
    
    result = await main_agent.execute_task("task_001", "Simple task")
    
    assert result["status"] == "success"
    main_agent.dispatcher.run_direct_step.assert_called_once()

@pytest.mark.asyncio
async def test_execute_task_failure(main_agent):
    main_agent.ollama.generate = AsyncMock(return_value={
        "response": json.dumps({"steps": [{"step_id": 1}], "goal": "G"})
    })
    main_agent.dispatcher.run_plan = AsyncMock(return_value={
        "status": "failure", "failed_step": 1, "reason": "Timeout"
    })
    main_agent.memory.set_active_context = AsyncMock()
    main_agent.memory.format_history.return_value = "History"
    main_agent.memory.goal = "Goal"
    
    result = await main_agent.execute_task("task_001", "Fail task")
    
    assert result["status"] == "failure"
    assert "Timeout" in result["error"]
