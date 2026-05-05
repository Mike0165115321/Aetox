import asyncio
from aetox.memory.working import WorkingMemory
from aetox.agents.executor import ExecutorAgent
from aetox.core.config_loader import get_memory_config, get_model_config

async def test_extraction():
    print("[*] Initializing diagnostic test...")
    memory = WorkingMemory(get_memory_config())
    executor = ExecutorAgent(memory, get_model_config())
    
    goal = "กลับมาที่ E:\\Aetox\\งานเอกสาร\\เอกสารสำคัญ ดูเนื้อหาใน Aetox_สัญญาจ้าง_1.0.0 หน่อย"
    context = {"context": {}}
    
    print(f"[*] Testing extraction for goal: {goal}")
    try:
        # We wrap it in a timeout to see if it hangs
        extraction = await asyncio.wait_for(
            executor.extract_action({"description": goal}, context),
            timeout=30
        )
        print(f"[*] Extraction result: {extraction}")
    except asyncio.TimeoutError:
        print("[!] ERROR: Extraction timed out after 30 seconds!")
    except Exception as e:
        print(f"[!] ERROR: Extraction failed: {e}")

if __name__ == "__main__":
    asyncio.run(test_extraction())
