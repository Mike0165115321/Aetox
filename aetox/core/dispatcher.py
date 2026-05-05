import logging
import asyncio
from typing import Dict, List, Any, Optional, Callable
from aetox.memory.working import WorkingMemory
from aetox.agents.executor import ExecutorAgent
from aetox.agents.critic import CriticAgent


class Dispatcher:
    """
    Asynchronous Orchestrator for AetoxClaw.
    Manages task execution and quality control without blocking the event loop.
    """
    def __init__(self, memory: WorkingMemory):
        self.logger = logging.getLogger("aetox.core.dispatcher")
        self.memory = memory
        self.executor = ExecutorAgent()
        self.critic = CriticAgent()
        self.progress_callback: Optional[Callable[[str], None]] = None

    async def run_direct_step(self, goal: str) -> Dict[str, Any]:
        """Executes a single step asynchronously."""
        self.logger.info(f"Running direct step (Async) for goal: {goal}")
        
        if self.progress_callback:
            await self.progress_callback(f"[TASK] Analyzing: {goal}")

        # 1. Extract intent
        minimal_context = {"context": {}} 
        extraction = await self.executor.extract_action({"description": goal}, minimal_context)

        if not extraction or extraction.get("confidence", 0) < 0.6 or extraction.get("tool") == "other":
            return {
                "status": "failure",
                "error": "Task too complex for direct execution.",
                "needs_planning": True
            }

        # 2. Execute
        if self.progress_callback:
            await self.progress_callback(f"[EXEC] Using {extraction.get('tool')} ({extraction.get('action')})")
            
        result = await self.executor.run_action(extraction, minimal_context)
        
        # 3. Update state
        self.executor.add_to_history(goal, result.get("output", ""))
        self.memory.add_step_result(
            step_id=1,
            result=result.get("output"),
            status=result.get("status", "success"),
            error=result.get("error")
        )
        self.memory.save_to_disk() # 💾 AUTO-SAVE
        return result

    async def run_direct_chat_stream(self, goal: str):
        """Asynchronous stream generator for pure chat."""
        minimal_context = {"context": {}} 
        extraction = await self.executor.extract_action({"description": goal}, minimal_context)
        
        if extraction.get("tool") == "chat":
            async for token in self.executor.run_chat_stream(goal):
                yield token
        else:
            yield "__NOT_CHAT__"

    async def run_plan(self, plan: Dict[str, Any], max_retries: int = 3, timeout_per_step: int = 300) -> Dict[str, Any]:
        """
        Executes a multi-step plan asynchronously with retry logic and critic feedback.
        """
        plan_id = plan.get("plan_id", "unknown")
        goal = plan.get("goal", "งานหลายขั้นตอน")
        steps = plan.get("steps", [])
        
        self.logger.info(f"Executing Plan {plan_id} (Async) with {max_retries} max retries")
        
        for step in steps:
            step_id = step.get("step_id")
            description = step.get("description")
            retries = 0
            
            while retries < max_retries:
                if self.progress_callback:
                    retry_text = f" (พยายามครั้งที่ {retries+1})" if retries > 0 else ""
                    await self.progress_callback(f"🛠️ **ขั้นตอนที่ {step_id}:** {description}{retry_text}")

                # 1. Prepare Context
                current_context = self.memory.get_full_context()
                current_context["global_goal"] = goal
                if "hint" in step:
                    current_context["hint"] = step["hint"]
                
                try:
                    # 2. Extract and Run (with timeout)
                    async def execute_logic():
                        extraction = await self.executor.extract_action(step, current_context)
                        return await self.executor.run_action(extraction, current_context)

                    result = await asyncio.wait_for(execute_logic(), timeout=timeout_per_step)
                    
                    # 3. Quality Check (Critic)
                    eval_result = await self.critic.evaluate(step, result, self.memory.get_active_context(plan_id) or {})
                    is_success = (eval_result.get("verdict") == "pass")
                    
                    if is_success:
                        self.memory.add_step_result(step_id, result.get("output"), "success")
                        if "memory_updates" in result:
                            self.memory.update_context(result["memory_updates"])
                        break # Success! Go to next step
                    else:
                        retries += 1
                        feedback = await self.critic.analyze_failure(step, result)
                        step["hint"] = feedback # Inject feedback for next retry
                        if self.progress_callback:
                            await self.progress_callback(f"⚠️ **ล้มเหลว:** {eval_result.get('suggestion')}\n🔍 **คำแนะนำ:** {feedback}")
                
                except asyncio.TimeoutError:
                    retries += 1
                    step["hint"] = "การทำงานใช้เวลานานเกินไป โปรดทำให้ขั้นตอนสั้นลงหรือเพิ่ม timeout"
                    if self.progress_callback:
                        await self.progress_callback(f"⏱️ **Timeout:** ขั้นตอนที่ {step_id} หมดเวลา")
                
                if retries == max_retries:
                    self.memory.add_step_result(step_id, None, "failed", "Max retries exceeded")
                    return {"status": "failure", "plan_id": plan_id, "failed_step": step_id, "reason": "Max retries exceeded"}

            self.memory.save_to_disk()

        return {"status": "success", "data": self.memory.get_full_context()}
