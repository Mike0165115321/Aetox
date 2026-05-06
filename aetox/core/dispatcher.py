import logging
import asyncio
import inspect
from typing import Dict, List, Any, Optional, Callable
from aetox.agents.executor import ExecutorAgent
from aetox.agents.critic import CriticAgent


class Dispatcher:
    """
    Stateless Orchestrator for AetoxClaw.
    ไม่ต้องการ WorkingMemory — จัดการ history เองภายในแต่ละโหมด
    
    Chat Mode:  รับ history จากภายนอก (SessionContext)
    Plan Mode:  สร้าง plan_history ภายในแล้วเคลียร์เมื่อจบงาน
    """
    def __init__(self):
        self.logger = logging.getLogger("aetox.core.dispatcher")
        self.executor = ExecutorAgent()
        self.critic = CriticAgent()
        self.progress_callback: Optional[Callable[[str], None]] = None

    async def _safe_callback(self, message: str):
        """Internal helper to handle sync or async callbacks safely."""
        if not self.progress_callback:
            return
            
        try:
            if inspect.iscoroutinefunction(self.progress_callback):
                await self.progress_callback(message)
            else:
                self.progress_callback(message)
        except Exception as e:
            self.logger.error(f"[DISPATCHER] Callback execution failed: {e}")

    async def run_direct_step(self, goal: str, history: List[Dict] = None) -> Dict[str, Any]:
        """
        Executes a single step (Chat Mode).
        รับ history จากภายนอก (SessionContext) — ส่งแค่ 3-5 ข้อความล่าสุด
        """
        self.logger.debug(f"[DISPATCHER] Starting direct step | goal: {goal}")
        
        await self._safe_callback(f"🔍 **กำลังวิเคราะห์:** {goal}")

        # สร้าง context จาก history ที่ได้รับ
        history_str = ""
        if history:
            history_str = "\n".join([f"{i+1}. ถาม: {h['q']} -> ตอบ: {h['a']}" for i, h in enumerate(history)])
        
        context = {
            "history": history_str,
            "global_goal": goal
        }

        # Extract and Execute
        extraction = await self.executor.extract_action({"description": goal}, context)

        if not extraction or extraction.get("confidence", 0) < 0.5 or extraction.get("tool") == "other":
            self.logger.warning(f"[DISPATCHER] Low confidence or unknown tool for: {goal}")
            return {
                "status": "failure",
                "error": "ไม่สามารถดำเนินการโดยตรงได้ (งานซับซ้อนเกินไปหรือเครื่องมือไม่รองรับ)",
                "needs_planning": True
            }

        await self._safe_callback(f"⚙️ **เรียกใช้:** {extraction.get('tool')} ({extraction.get('action')})")
            
        result = await self.executor.run_action(extraction, context)
        
        self.logger.debug(f"[DISPATCHER] Direct step completed | status: {result.get('status')}")
        return result

    async def run_direct_chat_stream(self, goal: str, history: List[Dict] = None):
        """Asynchronous stream generator for pure chat."""
        history_str = ""
        if history:
            history_str = "\n".join([f"{i+1}. ถาม: {h['q']} -> ตอบ: {h['a']}" for i, h in enumerate(history)])
        
        context = {"history": history_str, "global_goal": goal}
        extraction = await self.executor.extract_action({"description": goal}, context)
        
        if extraction.get("tool") == "chat":
            async for token in self.executor.run_chat_stream(goal):
                yield token
        else:
            yield "__NOT_CHAT__"

    async def run_plan(self, plan: Dict[str, Any], max_retries: int = 3, timeout_per_step: int = 300) -> Dict[str, Any]:
        """
        Executes a multi-step plan (Plan Mode).
        สร้าง plan_history ภายในและส่งต่อให้แต่ละ step:
        - ผลลัพธ์ขั้นตอนก่อนหน้า 1 ขั้น (immediate context)
        - สรุปสถานะแผนทั้งหมด (plan summary)
        """
        plan_id = plan.get("plan_id", "unknown")
        goal = plan.get("goal", "งานหลายขั้นตอน")
        steps = plan.get("steps", [])
        
        # 🧠 Plan-scoped history: เก็บผลลัพธ์แต่ละ step
        plan_history: List[Dict[str, Any]] = []
        
        self.logger.info(f"[DISPATCHER] Executing Plan: {plan_id} | Steps: {len(steps)}")
        
        for step in steps:
            step_id = step.get("step_id") or step.get("id")
            description = step.get("description")
            retries = 0
            
            while retries < max_retries:
                retry_text = f" (รอบที่ {retries+1})" if retries > 0 else ""
                await self._safe_callback(f"🛠️ **ขั้นตอนที่ {step_id}:** {description}{retry_text}")

                # สร้าง context จาก plan_history
                plan_summary = "\n".join([
                    f"Step {r['step_id']}: {r['status']} — {r.get('output', '')[:100]}" 
                    for r in plan_history
                ])
                
                context = {
                    "global_goal": goal,
                    "plan_summary": plan_summary or "ยังไม่มีขั้นตอนที่เสร็จ",
                    "previous_result": plan_history[-1] if plan_history else None,
                    "history": ""  # Plan mode ไม่ส่ง chat history
                }
                
                if "hint" in step:
                    context["hint"] = step["hint"]
                
                try:
                    # Extract and Run (with timeout)
                    async def execute_logic():
                        self.logger.debug(f"[DISPATCHER] Extracting action for step {step_id}")
                        extraction = await self.executor.extract_action(step, context)
                        return await self.executor.run_action(extraction, context)

                    result = await asyncio.wait_for(execute_logic(), timeout=timeout_per_step)
                    
                    # Quality Check (Critic)
                    eval_result = await self.critic.evaluate(step, result, context)
                    is_success = (eval_result.get("verdict") == "pass")
                    
                    if is_success:
                        # บันทึกผลลัพธ์ลง plan_history
                        plan_history.append({
                            "step_id": step_id,
                            "description": description,
                            "status": "success",
                            "output": str(result.get("output", ""))[:500]  # ตัดไม่ให้ยาวเกิน
                        })
                        self.logger.debug(f"[DISPATCHER] Step {step_id} PASSED critic.")
                        break  # Success! Go to next step
                    else:
                        retries += 1
                        feedback = await self.critic.analyze_failure(step, result)
                        step["hint"] = feedback  # Inject feedback for next retry
                        
                        plan_history.append({
                            "step_id": step_id,
                            "description": description,
                            "status": "retry",
                            "output": str(result.get("output", ""))[:200],
                            "feedback": feedback
                        })
                        
                        await self._safe_callback(f"⚠️ **ไม่ผ่านเกณฑ์:** {eval_result.get('suggestion')}\n🔍 **คำแนะนำ:** {feedback}")
                        self.logger.warning(f"[DISPATCHER] Step {step_id} FAILED critic | retry: {retries}")
                
                except asyncio.TimeoutError:
                    retries += 1
                    error_msg = f"ขั้นตอนที่ {step_id} หมดเวลา (Timeout {timeout_per_step}s)"
                    step["hint"] = "การทำงานใช้เวลานานเกินไป โปรดทำให้ขั้นตอนสั้นลง"
                    
                    plan_history.append({
                        "step_id": step_id,
                        "description": description,
                        "status": "timeout",
                        "output": ""
                    })
                    
                    await self._safe_callback(f"⏱️ **หมดเวลา:** {error_msg}")
                    self.logger.error(f"[DISPATCHER] Step {step_id} TIMEOUT")
                
                if retries == max_retries:
                    return {
                        "status": "failure", 
                        "plan_id": plan_id, 
                        "failed_step": step_id, 
                        "reason": "Max retries exceeded",
                        "plan_history": plan_history
                    }

        return {"status": "success", "plan_id": plan_id, "plan_history": plan_history}
