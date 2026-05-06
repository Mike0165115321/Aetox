import logging
import json
import os
import yaml
import asyncio
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.planner import AetoxPlanner
from aetox.core.dispatcher import Dispatcher
from aetox.agents.executor import ExecutorAgent
from aetox.memory.working import WorkingMemory

# Setup logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(name)s - %(levelname)s - %(message)s')
logger = logging.getLogger("aetox.main")

def init_workspace():
    """Initializes the workspace folders based on settings.yaml."""
    settings_path = "config/settings.yaml"
    if not os.path.exists(settings_path):
        logger.warning(f"Settings file {settings_path} not found. Skipping workspace init.")
        return

    try:
        with open(settings_path, 'r', encoding='utf-8') as f:
            settings = yaml.safe_load(f)
            workspace = settings.get('workspace', {})
            
            for key, path in workspace.items():
                if not os.path.exists(path):
                    os.makedirs(path, exist_ok=True)
                    logger.info(f"Created workspace folder: {path} ({key})")
    except Exception as e:
        logger.error(f"Failed to initialize workspace: {e}")

async def run_aetox_mvp():
    logger.info("--- AetoxClaw System Startup ---")
    
    # 0. Initialize Workspace
    init_workspace()
    
    # 1. Initialize Foundation
    client = OllamaClient()
    engine = PromptEngine()
    
    if not await client.check_health():
        logger.error("Ollama not found. Please ensure Ollama is running at localhost:11434")
        return

    # 2. Receive User Goal (CLI Demo)
    user_goal = "List the files in the current directory and create a new file named 'summary.txt' with the count."
    
    # 3. Plan
    planner = AetoxPlanner(client, engine)
    try:
        plan = await planner.create_plan(user_goal)
        print("\n[PLAN GENERATED]")
        print(json.dumps(plan, indent=2, ensure_ascii=False))
    except Exception as e:
        logger.error(f"Planning failed: {e}")
        return

    # 4. Initialize Memory & Dispatcher
    memory_config = config_loader.get_memory_config()
    memory_config["goal"] = user_goal
    memory = WorkingMemory(memory_config) 
    dispatcher = Dispatcher(memory)
    dispatcher.executor = ExecutorAgent() # ExecutorAgent init is still sync for now
    
    # 5. Run Execution Loop
    print("\n[STARTING EXECUTION]")
    final_result = await dispatcher.run_plan(plan)
    
    # 6. Show Final Result
    print("\n[FINAL MEMORY STATE]")
    print(json.dumps(final_result, indent=2, ensure_ascii=False))

if __name__ == "__main__":
    asyncio.run(run_aetox_mvp())
