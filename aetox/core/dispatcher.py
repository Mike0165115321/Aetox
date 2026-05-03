import logging
from typing import Dict, List, Any
from aetox.memory.working import WorkingMemory
from aetox.agents.executor import ExecutorAgent
from aetox.memory.manager import MemoryManager

class Dispatcher:
    """
    Orchestrates the execution of a TaskPlan.
    """
    def __init__(self, memory: WorkingMemory):
        self.logger = logging.getLogger("aetox.core.dispatcher")
        self.memory = memory
        self.executor = ExecutorAgent()
        self.memory_manager = MemoryManager()

    def run_plan(self, plan: Dict[str, Any]) -> Dict[str, Any]:
        plan_id = plan.get("plan_id", "unknown")
        steps = plan.get("steps", [])
        
        self.logger.info(f"Starting execution for Plan ID: {plan_id}")
        
        all_success = True
        for step in steps:
            step_id = step.get("step_id")
            
            # 1. Execute
            result = self.executor.execute_step(step, self.memory.__dict__)
            
            # 2. Update memory
            self.memory.add_step_result(
                step_id=step_id,
                result=result.get("output"),
                status=result.get("status", "success"),
                error=result.get("error")
            )
            
            if "memory_updates" in result:
                self.memory.update_context(result["memory_updates"])

            if result.get("status") != "success":
                all_success = False

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
