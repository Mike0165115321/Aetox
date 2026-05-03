import uuid
from typing import Dict, List, Any, Optional

class WorkingMemory:
    """
    In-RAM storage for the current task's state.
    Destroyed when the task completes or the process exits.
    """
    def __init__(self, goal: str):
        self.task_id = str(uuid.uuid4())
        self.goal = goal
        self.current_step_index = 0
        self.step_results: List[Dict[str, Any]] = []
        self.artifacts: Dict[str, Any] = {}
        self.context: Dict[str, Any] = {} # Key facts to pass between steps

    def add_step_result(self, step_id: int, result: Any, status: str = "success", error: Optional[str] = None):
        self.step_results.append({
            "step_id": step_id,
            "status": status,
            "output": result,
            "error": error
        })

    def update_context(self, updates: Dict[str, Any]):
        self.context.update(updates)

    def get_full_context(self) -> Dict[str, Any]:
        """Returns all relevant information for the next agent."""
        return {
            "task_id": self.task_id,
            "goal": self.goal,
            "step_results": self.step_results,
            "context": self.context,
            "artifacts": self.artifacts
        }
