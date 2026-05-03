import logging
import asyncio
from aetox.core.dispatcher import Dispatcher
from aetox.memory.working import WorkingMemory
from aetox.agents.executor import ExecutorAgent

logging.basicConfig(level=logging.INFO)

async def test_direct_task():
    print("\n--- Testing Direct Task Lane ---")
    memory = WorkingMemory("List files in current directory")
    dispatcher = Dispatcher(memory)
    
    # Mock progress callback
    def progress(msg):
        print(f"[PROGRESS] {msg}")
    dispatcher.progress_callback = progress
    
    result = dispatcher.run_direct_step("List files in current directory")
    print(f"Result Status: {result.get('status')}")
    print(f"Output: {result.get('output')}")

async def test_smart_suggestion():
    print("\n--- Testing Smart Suggestion (Complex Task) ---")
    memory = WorkingMemory("Build a full stack e-commerce application with React and Node.js")
    dispatcher = Dispatcher(memory)
    
    result = dispatcher.run_direct_step("Build a full stack e-commerce application with React and Node.js")
    print(f"Result Status: {result.get('status')}")
    print(f"Needs Planning: {result.get('needs_planning')}")
    print(f"Error: {result.get('error')}")

if __name__ == "__main__":
    asyncio.run(test_direct_task())
    asyncio.run(test_smart_suggestion())
