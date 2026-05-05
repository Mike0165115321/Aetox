# aetox/agents/main_agent.py
import json
import logging
import asyncio
from typing import Dict, Any, List, Optional
from aetox.memory.working import WorkingMemory, MemoryContextBuilder
from aetox.core.ollama_client import OllamaClient
from aetox.core.config_loader import config_loader
from aetox.tools.file_manager import MasterFileManager
from aetox.tools.web_scraper import WebPulseScraper

logger = logging.getLogger("aetox.agents.main")

class MainAgent:
    """
    Main Orchestrator Agent for AetoxClaw.
    Implements a robust reasoning loop with 3-layer memory integration.
    """
    def __init__(self):
        # 1. Load Config & Memory
        self.memory_config = config_loader.get_memory_config()
        self.memory = WorkingMemory(self.memory_config)
        
        # 2. Setup Ollama Client & Model
        self.model = config_loader.get_model("main")
        self.options = config_loader.get_options("main")
        self.ollama = OllamaClient(host=config_loader.get_ollama_url())
        
        # 3. Initialize Tools with memory reference
        self.tools = {
            "master_file_manager": MasterFileManager(),
            "web_pulse_scraper": WebPulseScraper(memory_ref=self.memory)
            # สามารถเพิ่ม http_client หรืออื่นๆ ได้ที่นี่
        }
        
        logger.info(f"AetoxClaw MainAgent initialized using model: {self.model}")

    async def execute_task(self, task_id: str, instruction: str) -> Dict:
        """
        Executes a complex task using the Reasoning Loop.
        """
        # 1. Setup Initial Context
        self.memory.set_active_context(task_id, {
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
เน้นความปลอดภัย ความถูกต้อง และความประหยัดทรัพยากร (ใช้ข้อมูลสรุปแทนข้อมูลดิบถ้าเป็นไปได้)

ตอบกลับเป็น JSON เท่านั้น:
{{
  "steps": [
    {{ "id": 1, "description": "ขั้นตอนที่ 1...", "reasoning": "ทำไมต้องทำขั้นนี้" }},
    ...
  ],
  "estimated_complexity": "high/medium/low"
}}
"""
        plan_response = await self.ollama.generate(
            model=self.model,
            prompt=plan_prompt,
            format="json",
            options=self.options
        )
        
        try:
            plan_data = json.loads(plan_response.get("response", "{}"))
            steps = plan_data.get("steps", [])
        except Exception as e:
            logger.error(f"Failed to parse plan: {e}")
            return {"status": "failure", "error": "Planning failed due to invalid JSON response."}

        if not steps:
            return {"status": "failure", "error": "No steps generated in plan."}

        # 3. Execution Phase (Iterative)
        logger.info(f"[EXECUTION] Starting execution of {len(steps)} steps")
        for i, step in enumerate(steps):
            step_desc = step.get("description")
            logger.info(f"[STEP {i+1}] {step_desc}")
            
            # Update Active Context
            self.memory.set_active_context(task_id, {
                "instruction": instruction,
                "current_step_id": i + 1,
                "total_steps": len(steps),
                "step_description": step_desc,
                "status": "executing"
            })
            
            # Build Context for this specific step
            step_context = MemoryContextBuilder.build_for_task(
                self.memory, 
                "execution", 
                f"กำลังทำขั้นตอน: {step_desc}"
            )
            
            # Execute Single Step (Tool Selection & Calling)
            result = await self._execute_single_step(step_desc, step_context)
            
            # Record Result to Working Memory (Layer 1)
            self.memory.add_to_working(
                content=str(result.get("output", "No output")),
                source=f"step_{i+1}",
                keywords=[step_desc[:20]],
                metadata={"step_id": i + 1, "status": result.get("status")}
            )
            
            # Record to Episodic Memory (Layer 2) if important/failure
            if result.get("status") == "failure" or i == len(steps) - 1:
                 from aetox.memory.episodic import EpisodicMemory
                 epi = EpisodicMemory()
                 epi.save_lesson(
                     task_goal=step_desc,
                     outcome=result.get("output", ""),
                     key_learnings=[result.get("error")] if result.get("error") else ["Success"],
                     success=(result.get("status") == "success")
                 )

            if result.get("status") == "failure":
                logger.error(f"Step {i+1} failed: {result.get('error')}")
                return {"status": "failure", "failed_step": i+1, "error": result.get("error")}

        # 4. Final Summarization
        logger.info("[SUMMARY] Generating final report")
        final_context = MemoryContextBuilder.build_for_task(self.memory, "research", "สรุปผลงานทั้งหมด")
        summary_prompt = f"""สรุปผลการดำเนินงานสำหรับคำสั่ง: {instruction}
โดยใช้ข้อมูลประวัติการทำงานด้านล่างนี้:

{final_context}

ตอบกลับเป็น JSON:
{{
  "summary": "บทสรุปความสำเร็จ...",
  "success": true,
  "artifacts_created": ["path/to/file1", ...],
  "next_suggestions": ["ควรทำอะไรต่อ..."]
}}
"""
        summary_response = await self.ollama.generate(
            model=self.model,
            prompt=summary_prompt,
            format="json",
            options=self.options
        )
        
        try:
            return json.loads(summary_response.get("response", "{}"))
        except:
            return {"status": "success", "raw_summary": summary_response.get("response")}

    async def _execute_single_step(self, step_description: str, context: str) -> Dict:
        """
        Reasoning to select tool, parameters, and execute.
        """
        tools_doc = "\n".join([t.get_prompt_doc() for t in self.tools.values()])
        
        selection_prompt = f"""คุณคือสมองของ AetoxClaw กำลังอยู่ในขั้นตอน: {step_description}
บริบทปัจจุบัน:
{context}

เครื่องมือที่เลือกใช้ได้:
{tools_doc}

กรุณาเลือกเครื่องมือที่เหมาะสมที่สุด (ถ้าไม่ต้องใช้เครื่องมือให้ใช้ 'chat') พร้อมระบุพารามิเตอร์

ตอบกลับเป็น JSON:
{{
  "tool": "ชื่อเครื่องมือ",
  "action": "ชื่อคำสั่ง",
  "params": {{ ... }},
  "thought": "เหตุผลที่เลือก"
}}
"""
        selection_response = await self.ollama.generate(
            model=self.model,
            prompt=selection_prompt,
            format="json",
            options=self.options
        )
        
        try:
            call_data = json.loads(selection_response.get("response", "{}"))
            tool_name = call_data.get("tool")
            action = call_data.get("action")
            params = call_data.get("params", {})
        except:
            return {"status": "failure", "error": "Failed to decide tool call."}

        if tool_name == "chat" or tool_name not in self.tools:
            return {"status": "success", "output": f"ดำเนินการ (Chat Mode): {step_description}"}

        # Execute Tool
        tool = self.tools[tool_name]
        logger.info(f"[TOOL CALL] {tool_name} -> {action}")
        
        try:
            # Note: WebPulseScraper อาจมีการเรียก asyncio.run ภายใน ต้องระวังถ้าเรียกใน Event Loop เดียวกัน
            result = tool.execute({**params, "action": action})
            return result
        except Exception as e:
            return {"status": "failure", "error": str(e)}