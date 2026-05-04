import uuid
import time
from typing import Dict, List, Any, Optional

class WorkingMemory:
    """
    Enhanced In-RAM and Persistable storage for the current task's state.
    Designed to provide clean context for LLM agents.
    """
    def __init__(self, goal: str, task_id: Optional[str] = None):
        self.task_id = task_id or str(uuid.uuid4())
        self.goal = goal
        self.summary: str = "" # High-level summary of progress
        self.current_step_index = 0
        self.step_results: List[Dict[str, Any]] = []
        self.artifacts: Dict[str, Any] = {}
        self.context: Dict[str, Any] = {} # Key facts to pass between steps
        self.metadata: Dict[str, Any] = {
            "created_at": time.time(),
            "updated_at": time.time(),
            "version": "2.0"
        }

    def add_step_result(self, step_id: int, result: Any, status: str = "success", error: Optional[str] = None):
        """Adds a result and updates metadata."""
        self.step_results.append({
            "step_id": step_id,
            "status": status,
            "output": result,
            "error": error,
            "timestamp": time.time()
        })
        self.metadata["updated_at"] = time.time()
        self.current_step_index = step_id

    def update_context(self, updates: Dict[str, Any]):
        """Safely updates the shared context."""
        self.context.update(updates)
        self.metadata["updated_at"] = time.time()

    def add_artifact(self, name: str, value: Any):
        """Registers an artifact (e.g., a file path or a generated code snippet)."""
        self.artifacts[name] = value
        self.metadata["updated_at"] = time.time()

    def format_history(self) -> str:
        """
        Returns a clean Markdown representation of the task history for the LLM.
        This prevents the LLM from getting 'confused' by raw JSON.
        """
        if not self.step_results:
            return "ยังไม่มีประวัติการทำงาน"

        history_lines = ["### ประวัติการทำงานที่ผ่านมา:"]
        for res in self.step_results:
            status_icon = "✅" if res["status"] == "success" else "❌"
            line = f"- Step {res['step_id']}: {status_icon} {res['status']}"
            if res.get("error"):
                line += f" (Error: {res['error']})"
            
            # Truncate output if it's too long, but keep enough for paths and lists
            output_str = str(res['output'])
            if len(output_str) > 1000:
                output_str = output_str[:997] + "..."

            
            line += f" | ผลลัพธ์: {output_str}"
            history_lines.append(line)
        
        return "\n".join(history_lines)

    def to_dict(self) -> Dict[str, Any]:
        """Serializes the memory state to a dictionary."""
        return {
            "task_id": self.task_id,
            "goal": self.goal,
            "summary": self.summary,
            "current_step_index": self.current_step_index,
            "step_results": self.step_results,
            "artifacts": self.artifacts,
            "context": self.context,
            "metadata": self.metadata
        }

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> 'WorkingMemory':
        """Restores a memory state from a dictionary."""
        instance = cls(goal=data["goal"], task_id=data["task_id"])
        instance.summary = data.get("summary", "")
        instance.current_step_index = data.get("current_step_index", 0)
        instance.step_results = data.get("step_results", [])
        instance.artifacts = data.get("artifacts", {})
        instance.context = data.get("context", {})
        instance.metadata = data.get("metadata", instance.metadata)
        return instance

    def save_to_disk(self, directory: str = "data/tasks"):
        """Saves the memory state to a JSON file on disk."""
        import os
        import json
        if not os.path.exists(directory):
            os.makedirs(directory)
        
        filepath = os.path.join(directory, f"{self.task_id}.json")
        with open(filepath, 'w', encoding='utf-8') as f:
            json.dump(self.to_dict(), f, ensure_ascii=False, indent=2)

    @classmethod
    def load_from_disk(cls, task_id: str, directory: str = "data/tasks") -> Optional['WorkingMemory']:
        """Loads a memory state from disk by task ID."""
        import os
        import json
        filepath = os.path.join(directory, f"{task_id}.json")
        if not os.path.exists(filepath):
            return None
            
        with open(filepath, 'r', encoding='utf-8') as f:
            data = json.load(f)
            return cls.from_dict(data)

    def get_full_context(self) -> Dict[str, Any]:
        """Returns all relevant information for the next agent."""
        return {
            "task_id": self.task_id,
            "goal": self.goal,
            "summary": self.summary,
            "history": self.format_history(),
            "step_results": self.step_results,
            "context": self.context,
            "artifacts": self.artifacts,
            "metadata": self.metadata
        }

