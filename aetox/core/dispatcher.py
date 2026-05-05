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

    async def run_direct_step(self, goal: str, task_id: str = "direct_task") -> Dict[str, Any]:
        """Executes a single step asynchronously."""
        self.logger.debug(f"[DISPATCHER] Starting direct step | task_id: {task_id} | goal: {goal}")
        
        if self.progress_callback:
            await self.progress_callback(f"🔍 **กำลังวิเคราะห์:** {goal}")

        # 1. Prepare Context (Stateless-Safe)
        current_context = await self.memory.get_active_context(task_id)
        current_context["global_goal"] = goal

        # 2. Extract and Execute
        extraction = await self.executor.extract_action({"description": goal}, current_context)

        if not extraction or extraction.get("confidence", 0) < 0.5 or extraction.get("tool") == "other":
            self.logger.warning(f"[DISPATCHER] Low confidence or unknown tool for: {goal}")
            return {
                "status": "failure",
                "error": "ไม่สามารถดำเนินการโดยตรงได้ (งานซับซ้อนเกินไปหรือเครื่องมือไม่รองรับ)",
                "needs_planning": True
            }

        if self.progress_callback:
            await self.progress_callback(f"⚙️ **เรียกใช้:** {extraction.get('tool')} ({extraction.get('action')})")
            
        result = await self.executor.run_action(extraction, current_context)
        
        # 3. Update state (Standardized)
        self.executor.add_to_history(goal, result.get("output", ""))
        self.memory.add_step_result(
            step_id="direct",
            output=result.get("output"),
            status=result.get("status", "success"),
            error=result.get("error")
        )
        
        self.logger.debug(f"[DISPATCHER] Direct step completed | status: {result.get('status')}")
        return result

    async def run_direct_chat_stream(self, goal: str, task_id: str = "chat_task"):
        """Asynchronous stream generator for pure chat."""
        context = await self.memory.get_active_context(task_id)
        extraction = await self.executor.extract_action({"description": goal}, context)
        
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
        
        self.logger.info(f"[DISPATCHER] Executing Plan: {plan_id} | Steps: {len(steps)}")
        
        for step in steps:
            step_id = step.get("step_id") or step.get("id")
            description = step.get("description")
            retries = 0
            
            while retries < max_retries:
                if self.progress_callback:
                    retry_text = f" (รอบที่ {retries+1})" if retries > 0 else ""
                    await self.progress_callback(f"🛠️ **ขั้นตอนที่ {step_id}:** {description}{retry_text}")

                # 1. Prepare Context (Standardized)
                current_context = await self.memory.get_full_context_async() # We'll need this async helper
                current_context["global_goal"] = goal
                if "hint" in step:
                    current_context["hint"] = step["hint"]
                
                try:
                    # 2. Extract and Run (with timeout)
                    async def execute_logic():
                        self.logger.debug(f"[DISPATCHER] Extracting action for step {step_id}")
                        extraction = await self.executor.extract_action(step, current_context)
                        return await self.executor.run_action(extraction, current_context)

                    result = await asyncio.wait_for(execute_logic(), timeout=timeout_per_step)
                    
                    # 3. Quality Check (Critic)
                    active_ctx = await self.memory.get_active_context(plan_id)
                    eval_result = await self.critic.evaluate(step, result, active_ctx)
                    is_success = (eval_result.get("verdict") == "pass")
                    
                    if is_success:
                        self.memory.add_step_result(step_id, result.get("output"), "success")
                        if "memory_updates" in result and result["memory_updates"]:
                            await self.memory.update_context(plan_id, result["memory_updates"])
                        
                        self.logger.debug(f"[DISPATCHER] Step {step_id} PASSED critic.")
                        break # Success! Go to next step
                    else:
                        retries += 1
                        feedback = await self.critic.analyze_failure(step, result)
                        step["hint"] = feedback # Inject feedback for next retry
                        
                        # บันทึกสถานะ Retry
                        self.memory.add_step_result(step_id, result.get("output"), "retry", feedback, metadata={"eval": eval_result})
                        
                        if self.progress_callback:
                            await self.progress_callback(f"⚠️ **ไม่ผ่านเกณฑ์:** {eval_result.get('suggestion')}\n🔍 **คำแนะนำ:** {feedback}")
                        self.logger.warning(f"[DISPATCHER] Step {step_id} FAILED critic | retry: {retries}")
                
                except asyncio.TimeoutError:
                    retries += 1
                    error_msg = f"ขั้นตอนที่ {step_id} หมดเวลา (Timeout {timeout_per_step}s)"
                    step["hint"] = "การทำงานใช้เวลานานเกินไป โปรดทำให้ขั้นตอนสั้นลงหรือเพิ่ม timeout"
                    
                    self.memory.add_step_result(step_id, None, "failure", error_msg)
                    
                    if self.progress_callback:
                        await self.progress_callback(f"⏱️ **หมดเวลา:** {error_msg}")
                    self.logger.error(f"[DISPATCHER] Step {step_id} TIMEOUT")
                
                if retries == max_retries:
                    self.memory.add_step_result(step_id, None, "failed", "พยายามสูงสุดแล้วแต่ไม่สำเร็จ")
                    return {"status": "failure", "plan_id": plan_id, "failed_step": step_id, "reason": "Max retries exceeded"}

        final_data = await self.memory.get_full_context_async()
        return {"status": "success", "data": final_data}
