# aetox/agents/main_agent.py
import json
import logging
import asyncio
from typing import Dict, Any, List, Optional
from aetox.core.dispatcher import Dispatcher
from aetox.core.ollama_client import OllamaClient
from aetox.core.config_loader import config_loader

logger = logging.getLogger("aetox.agents.main")

class MainAgent:
    """
    Main Orchestrator Agent for AetoxClaw.
    Lightweight version — no WorkingMemory, uses Dispatcher directly.
    """
    def __init__(self):
        try:
            # 1. Setup Ollama Client & Model
            self.model = config_loader.get_model("main")
            self.options = config_loader.get_options("main")
            self.ollama = OllamaClient(host=config_loader.get_ollama_url())
            
            # 2. Initialize Dispatcher (Stateless)
            self.dispatcher = Dispatcher()
            
            logger.info(f"[MAIN] AetoxClaw MainAgent initialized using model: {self.model}")
        except Exception as e:
            logger.error(f"[MAIN] Initialization failed: {e}")
            raise RuntimeError(f"ไม่สามารถเริ่มระบบ MainAgent ได้: {e}")

    async def execute_task(self, task_id: str, instruction: str) -> Dict:
        """
        Executes a complex task by generating a plan and delegating to Dispatcher.
        """
        try:
            # 1. Planning Phase
            logger.info(f"[PLANNING] Generating plan for task: {task_id}")
            
            plan_prompt = f"""คุณคือผู้ช่วยอัจฉริยะ (MainAgent) ประจำระบบ AetoxClaw
งานที่ได้รับ: {instruction}

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
                return await self.dispatcher.run_direct_step(instruction)

            # 2. Delegate to Dispatcher
            logger.info(f"[MAIN] Delegating plan execution to Dispatcher.")
            result = await self.dispatcher.run_plan(plan_data)
            
            if result.get("status") == "failure":
                return {
                    "status": "failure",
                    "error": f"การดำเนินการล้มเหลวที่ขั้นตอน {result.get('failed_step')}: {result.get('reason')}",
                    "suggestion": "โปรดลองสั่งใหม่อีกครั้งด้วยรายละเอียดที่ชัดเจนขึ้น"
                }

            # 3. Return result
            return {
                "status": "success",
                "summary": f"ดำเนินการสำเร็จตามเป้าหมาย: {instruction}",
                "data": result.get("plan_history")
            }

        except Exception as e:
            logger.error(f"[MAIN] Task execution error: {e}")
            return {"status": "failure", "error": f"เกิดข้อผิดพลาดในระบบ: {str(e)}"}
