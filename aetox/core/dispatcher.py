import logging
from typing import Dict, List, Any
from aetox.memory.working import WorkingMemory
from aetox.agents.executor import ExecutorAgent

logger = logging.getLogger("aetox.core.dispatcher")

class Dispatcher:
    """
    Routes tasks and manages the execution flow of a TaskPlan.
    """
    def __init__(self, memory: WorkingMemory):
        self.memory = memory
        self.executor = ExecutorAgent()
        # In the future, we'll have more agents here

    def run_plan(self, plan: Dict[str, Any]) -> Dict[str, Any]:
        steps = plan.get("steps", [])
        logger.info(f"Starting execution for Plan ID: {plan.get('plan_id', 'unknown')}")

        for step in steps:
            step_id = step.get("step_id")
            
            # 1. Fetch context from memory
            context = self.memory.get_full_context()
            
            # 2. Dispatch to agent (currently only Executor)
            # In Step 2, we just use the executor regardless of the 'agent' field
            result = self.executor.execute_step(step, context)
            
            # 3. Update memory
            self.memory.add_step_result(
                step_id=step_id,
                result=result.get("output"),
                status=result.get("status", "success"),
                error=result.get("error")
            )
            
            if "memory_updates" in result:
                self.memory.update_context(result["memory_updates"])

            logger.info(f"Step {step_id} finished with status: {result.get('status')}")

        logger.info("Plan execution completed.")
        return self.memory.get_full_context()
