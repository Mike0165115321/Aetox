import logging
from typing import Dict, List, Any, Optional, Callable
from aetox.memory.working import WorkingMemory
from aetox.agents.executor import ExecutorAgent
from aetox.agents.critic import CriticAgent
from aetox.memory.manager import MemoryManager

class Dispatcher:
    """
    Orchestrates the execution of a TaskPlan with quality control (Critic).
    """
    def __init__(self, memory: WorkingMemory):
        self.logger = logging.getLogger("aetox.core.dispatcher")
        self.memory = memory
        self.executor = ExecutorAgent()
        self.critic = CriticAgent()
        self.memory_manager = MemoryManager()
        self.progress_callback: Optional[Callable[[str], None]] = None

    def run_direct_step(self, goal: str) -> Dict[str, Any]:
        """
        Executes a single step directly based on a user goal.
        Skips planning and uses optimized context.
        """
        self.logger.info(f"Running direct step for goal: {goal}")
        
        if self.progress_callback:
            self.progress_callback(f"[TASK] Analyzing direct action: {goal}")

        # 1. Extract parameters using Executor's LLM
        # Optimization: Pass minimal context for speed
        minimal_context = {"context": {}} 
        extraction = self.executor.extract_action({"description": goal}, minimal_context)

        if not extraction or extraction.get("confidence", 0) < 0.6 or extraction.get("tool") == "other" or extraction.get("action") == "unknown":
            # Smart Suggestion logic will be handled by the interface, 
            # but we return a clear error here.
            return {
                "status": "failure",
                "error": "Task too complex for direct execution or extraction failed.",
                "needs_planning": True
            }

        # 2. Execute the action

        if self.progress_callback:
            self.progress_callback(f"[EXEC] Executing: {extraction.get('action')} using {extraction.get('tool')}")
            
        result = self.executor.run_action(extraction, minimal_context)
        
        # 3. Update memory
        self.memory.add_step_result(
            step_id=1,
            result=result.get("output"),
            status=result.get("status", "success"),
            error=result.get("error")
        )
        
        return result

    def run_plan(self, plan: Dict[str, Any]) -> Dict[str, Any]:
        plan_id = plan.get("plan_id", "unknown")
        steps = plan.get("steps", [])
        
        self.logger.info(f"Starting execution for Plan ID: {plan_id}")
        
        all_success = True
        for step in steps:
            step_id = step.get("step_id")
            description = step.get("description")
            
            if self.progress_callback:
                self.progress_callback(f"[PLAN] Working on Step {step_id}: {description}")

            retry_count = 0
            max_retries = 2
            step_passed = False
            
            while retry_count <= max_retries and not step_passed:
                # 1. Execute
                result = self.executor.execute_step(step, self.memory.__dict__)
                
                # 2. Evaluate with Critic
                if self.progress_callback:
                    self.progress_callback(f"[QC] Critic is evaluating Step {step_id}...")
                
                eval_result = self.critic.evaluate(step, result, self.memory.context)
                verdict = eval_result.get("verdict", "pass")
                score = eval_result.get("score", 1.0)
                
                if verdict == "pass" or score >= 0.7:
                    if self.progress_callback:
                        self.progress_callback(f"✨ Step {step_id} passed quality check! (Score: {score})")
                    step_passed = True
                elif verdict == "retry" and retry_count < max_retries:
                    retry_count += 1
                    self.logger.warning(f"Critic requested RETRY for Step {step_id} (Attempt {retry_count}). Issues: {eval_result.get('issues')}")
                    if self.progress_callback:
                        self.progress_callback(f"🔄 **Retry needed!** Attempt {retry_count}/{max_retries}. Issues: {', '.join(eval_result.get('issues', []))}")
                else:
                    # Escalate or too many retries
                    self.logger.error(f"Step {step_id} FAILED quality check. Verdict: {verdict}")
                    if self.progress_callback:
                        self.progress_callback(f"❌ Step {step_id} failed quality check: {eval_result.get('suggestion')}")
                    all_success = False
                    break # Stop or escalate

            # Update memory
            self.memory.add_step_result(
                step_id=step_id,
                result=result.get("output"),
                status=result.get("status", "success") if step_passed else "failure",
                error=result.get("error") if not step_passed else None
            )
            
            if "memory_updates" in result:
                self.memory.update_context(result["memory_updates"])

            self.logger.info(f"Step {step_id} finished with status: {result.get('status')}")

        self.logger.info("Plan execution completed.")
        
        # 3. Save to Episodic Memory
        outcome = "success" if all_success else "partial_failure"
        self.memory_manager.save_episode(
            event_id=plan_id,
            event_type="task_execution",
            summary=getattr(self.memory, "goal", "Task Execution"),
            outcome=outcome,
            facts=getattr(self.memory, "context", {}),
            tags=["task_execution", outcome]
        )
        
        return self.memory.get_full_context()
