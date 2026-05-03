import logging
from aetox.agents.executor import ExecutorAgent
from aetox.memory.working import WorkingMemory

logging.basicConfig(level=logging.INFO)

def test_safety_violation():
    executor = ExecutorAgent()
    memory = WorkingMemory("Test goal")
    
    # 1. Test High Risk (Outside project folder)
    print("\n--- Testing High Risk Action ---")
    step_high = {
        "step_id": 1,
        "description": "Write a file to C:/Users/Public/test.txt",
        "tool": "file_manager",
        "action": "write_file",
        "params": {"path": "C:/Users/Public/test.txt", "content": "hello"}
    }
    # This should trigger CLI prompt or Sandbox error
    try:
        result = executor.execute_step(step_high, {"context": {}})
        print(f"Result: {result}")
    except Exception as e:
        print(f"Caught expected error: {e}")

    # 2. Test Sandbox Violation (Forbidden path)
    print("\n--- Testing Sandbox Violation ---")
    step_forbidden = {
        "step_id": 2,
        "description": "Read C:/Windows/win.ini",
        "tool": "file_manager",
        "action": "read_file",
        "params": {"path": "C:/Windows/win.ini"}
    }
    result = executor.execute_step(step_forbidden, {"context": {}})
    print(f"Result: {result}")

if __name__ == "__main__":
    test_safety_violation()
