# aetox/agents/main_agent.py
import json
import logging
import asyncio
from typing import Dict, Any, List, Optional
from aetox.memory.working import WorkingMemory, MemoryContextBuilder
from aetox.core.dispatcher import Dispatcher
from aetox.core.ollama_client import OllamaClient
from aetox.core.config_loader import config_loader

logger = logging.getLogger("aetox.agents.main")

class MainAgent:
    """
    Main Orchestrator Agent for AetoxClaw.
    Implements a robust reasoning loop with 3-layer memory integration.
    """
    def __init__(self):
        try:
            # 1. Load Config & Memory
            self.memory_config = config_loader.get_memory_config()
            self.memory = WorkingMemory(self.memory_config)
            
            # 2. Setup Ollama Client & Model
            self.model = config_loader.get_model("main")
            self.options = config_loader.get_options("main")
            self.ollama = OllamaClient(host=config_loader.get_ollama_url())
            
            # 3. Initialize Dispatcher (Delegate)
            self.dispatcher = Dispatcher(self.memory)
            
            logger.info(f"[MAIN] AetoxClaw MainAgent initialized using model: {self.model}")
        except Exception as e:
            logger.error(f"[MAIN] Initialization failed: {e}")
            raise RuntimeError(f"ไม่สามารถเริ่มระบบ MainAgent ได้: {e}")

    async def execute_task(self, task_id: str, instruction: str) -> Dict:
        """
        Executes a complex task by generating a plan and delegating to Dispatcher.
        """
        try:
            # 1. Setup Initial Context
            await self.memory.set_active_context(task_id, {
                "instruction": instruction,
                "step": 0,
                "status": "planning"
            })
            
            # 2. Planning Phase
            logger.info(f"[PLANNING] Generating plan for task: {task_id}")
            context = MemoryContextBuilder.build_for_task(self.memory, "planning", instruction)
            
            plan_prompt = f"""คุณคือผู้ช่วยอัจฉริยะ (MainAgent) ประจำระบบ AetoxClaw
งานที่ได้รับ: {instruction}

{context if context else "[ยังไม่มีประวัติงานก่อนหน้า]"}

กรุณาวางแผนขั้นตอนการทำงานอย่างเป็นระบบ (Step-by-Step) 
ตอบกลับเป็น JSON เท่านั้น:
{{
  "steps": [
    {{ "step_id": 1, "description": "ขั้นตอนที่ 1...", "reasoning": "เหตุผล" }},
    ...
  ],
  "goal": "{instruction}"
}}
"""
            plan_response = await self.ollama.generate(
                model=self.model,
                prompt=plan_prompt,
                format="json",
                options=self.options
            )
            
            plan_data = json.loads(plan_response.get("response", "{}"))
            plan_data["plan_id"] = task_id
            
            if not plan_data.get("steps"):
                # Fallback to direct execution if planning returns no steps
                logger.info(f"[MAIN] No steps generated, attempting direct execution.")
                return await self.dispatcher.run_direct_step(instruction, task_id)

            # 3. Delegate to Dispatcher (The New System)
            logger.info(f"[MAIN] Delegating plan execution to Dispatcher.")
            result = await self.dispatcher.run_plan(plan_data)
            
            if result.get("status") == "failure":
                return {
                    "status": "failure",
                    "error": f"การดำเนินการล้มเหลวที่ขั้นตอน {result.get('failed_step')}: {result.get('reason')}",
                    "suggestion": "โปรดลองสั่งใหม่อีกครั้งด้วยรายละเอียดที่ชัดเจนขึ้น"
                }

            # 4. Final Summarization (Optional)
            return {
                "status": "success",
                "summary": f"ดำเนินการสำเร็จตามเป้าหมาย: {instruction}",
                "data": result.get("data")
            }

        except Exception as e:
            logger.error(f"[MAIN] Task execution error: {e}")
            return {"status": "failure", "error": f"เกิดข้อผิดพลาดในระบบ: {str(e)}"}
