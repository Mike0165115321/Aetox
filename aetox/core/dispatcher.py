import logging
from typing import Dict, List, Any, Optional, Callable
from aetox.memory.working import WorkingMemory
from aetox.agents.executor import ExecutorAgent
from aetox.agents.critic import CriticAgent


class Dispatcher:
    """
    Asynchronous Orchestrator for AetoxOS.
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

    async def run_plan(self, plan: Dict[str, Any]) -> Dict[str, Any]:
        """Executes a multi-step plan asynchronously with full intent extraction for each step."""
        plan_id = plan.get("plan_id", "unknown")
        steps = plan.get("steps", [])
        
        self.logger.info(f"Executing Plan {plan_id} (Async)")
        
        all_success = True
        for step in steps:
            step_id = step.get("step_id")
            description = step.get("description")
            
            if self.progress_callback:
                await self.progress_callback(f"🛠️ **ขั้นตอนที่ {step_id}:** {description}")

            # 1. Extract specific intent for THIS step (Dynamic Step Execution)
            extraction = await self.executor.extract_action({"description": description}, self.memory.__dict__)
            
            # 2. Run action
            result = await self.executor.run_action(extraction, self.memory.__dict__)
            
            # ✅ บันทึกประวัติขั้นตอนย่อย (Sub-step History)
            self.executor.add_to_history(description, result.get("output", ""))

            # 3. Quality Check (Async Critic)
            if self.progress_callback:
                await self.progress_callback(f"⚖️ ตรวจสอบคุณภาพงานขั้นตอนที่ {step_id}...")
                
            eval_result = await self.critic.evaluate(step, result, self.memory.context)
            
            if eval_result.get("verdict") == "pass":
                status = "success"
            else:
                status = "failure"
                all_success = False
                if self.progress_callback:
                    await self.progress_callback(f"⚠️ **แจ้งเตือน:** ตรวจพบประเด็นในขั้นตอนที่ {step_id}: {eval_result.get('suggestion')}")

            # 4. Update memory
            self.memory.add_step_result(
                step_id=step_id,
                result=result.get("output"),
                status=status,
                error=result.get("error")
            )
            
            if "memory_updates" in result:
                self.memory.update_context(result["memory_updates"])

        return self.memory.get_full_context()
