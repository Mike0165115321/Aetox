import logging
import json
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.core.planner import Planner
from aetox.core.dispatcher import Dispatcher
from aetox.agents.executor import ExecutorAgent
from aetox.memory.working import WorkingMemory

# Setup logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(name)s - %(levelname)s - %(message)s')
logger = logging.getLogger("aetox.main")

def run_aetox_mvp():
    logger.info("--- AetoxOS Smart Executor Test (Phase 1, Step 4) ---")
    
    # 1. Initialize Foundation
    client = OllamaClient()
    engine = PromptEngine()
    
    if not client.check_health():
        logger.error("Ollama not found. Test aborted.")
        return

    # 2. Receive User Goal
    user_goal = "List the files in the current directory and create a new file named 'summary.txt' with the count."
    
    # 3. Plan
    planner = Planner(client, engine)
    try:
        plan = planner.create_plan(user_goal)
        print("\n[PLAN GENERATED]")
        print(json.dumps(plan, indent=2, ensure_ascii=False))
    except Exception as e:
        logger.error(f"Planning failed: {e}")
        return

    # 4. Initialize Memory & Dispatcher (Passing engine/client for Smart Executor)
    memory = WorkingMemory(user_goal)
    dispatcher = Dispatcher(memory)
    # Inject client/engine into executor if needed (currently Dispatcher creates its own, let's fix that)
    dispatcher.executor = ExecutorAgent(client=client, engine=engine)
    
    # 5. Run Execution Loop
    print("\n[STARTING EXECUTION]")
    final_result = dispatcher.run_plan(plan)
    
    # 6. Show Final Result
    print("\n[FINAL MEMORY STATE]")
    # Remove large fields for cleaner output if necessary, but here we'll show all
    print(json.dumps(final_result, indent=2, ensure_ascii=False))

if __name__ == "__main__":
    run_aetox_mvp()
