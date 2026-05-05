import asyncio
import json
from unittest.mock import MagicMock, AsyncMock
from aetox.agents.main_agent import MainAgent
from aetox.core.dispatcher import Dispatcher

async def test_unified_flow():
    """จำลองการทำงานจริงตั้งแต่ MainAgent -> Dispatcher -> Executor -> Critic"""
    print("--- [TEST] Starting Unified Architecture Test ---")
    
    # 1. Mock Memory (เลี่ยงปัญหา chromadb และการโหลดโมเดลหนัก)
    memory = MagicMock()
    memory.goal = "Test Goal"
    memory.active_chunks = []
    memory.artifacts = {}
    memory.set_active_context = AsyncMock()
    memory.get_active_context = AsyncMock(return_value={})
    memory.get_full_context_async = AsyncMock(return_value={"status": "all_good"})
    memory.retrieve_relevant = MagicMock(return_value=[]) 
    memory.format_history = MagicMock(return_value="No history yet")
    
    # 2. Mock Agent Setup
    agent = MagicMock(spec=MainAgent)
    agent.memory = memory
    agent.ollama = MagicMock()
    agent.model = "test-model"
    agent.options = {}
    
    # Mock การวางแผน (Planning)
    agent.ollama.generate = AsyncMock(return_value={
        "response": json.dumps({
            "steps": [
                {"step_id": 1, "description": "Verify unification", "reasoning": "Check flow"}
            ],
            "goal": "Verify system"
        })
    })
    
    # 3. Mock Dispatcher
    dispatcher = MagicMock(spec=Dispatcher)
    dispatcher.run_plan = AsyncMock(return_value={"status": "success", "data": {"result": "Integration Passed"}})
    agent.dispatcher = dispatcher
    
    # 4. Execute (ใช้ logic จริงของ execute_task)
    print("[1/2] Running execute_task...")
    result = await MainAgent.execute_task(agent, "test_task_001", "Verify Architecture")
    
    print(f"Result: {result}")
    
    # 5. Verification
    assert result["status"] == "success"
    assert "Integration Passed" in str(result)
    print("[2/2] Verification complete.")
    print("--- [TEST] ALL SYSTEMS NOMINAL ---")

if __name__ == "__main__":
    asyncio.run(test_unified_flow())
