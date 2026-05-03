import subprocess
import time
import os
import logging
import uuid
from typing import Dict, Any, Optional

class CodeRunnerTool:
    """
    Safely executes Python and PowerShell scripts.
    Enforces timeouts, workspace restrictions, and simple code scanning.
    """
    def __init__(self, temp_dir: str = "C:/AetoxOS_Workspace/temp", outbox_dir: str = "C:/AetoxOS_Workspace/outbox"):
        self.temp_dir = temp_dir
        self.outbox_dir = outbox_dir
        self.timeout = 30 # seconds
        self.logger = logging.getLogger("aetox.tools.coderunner")
        
        # Ensure directories exist
        os.makedirs(self.temp_dir, exist_ok=True)
        os.makedirs(self.outbox_dir, exist_ok=True)

    def _is_safe(self, code: str, language: str) -> bool:
        """Simple heuristic check for network-related imports/commands."""
        forbidden = ["requests", "socket", "urllib", "http.client", "flask", "django"]
        if language == "powershell":
            forbidden += ["Invoke-WebRequest", "Invoke-RestMethod", "Net.WebClient"]
            
        for word in forbidden:
            if word in code:
                self.logger.warning(f"Forbidden keyword '{word}' detected in code.")
                return False
        return True

    def run_python(self, code: str) -> Dict[str, Any]:
        if not self._is_safe(code, "python"):
            return {"error": "SECURITY VIOLATION: Network-related code detected.", "status": "failure"}

        file_id = str(uuid.uuid4())
        script_path = os.path.join(self.temp_dir, f"script_{file_id}.py")
        
        try:
            with open(script_path, "w", encoding="utf-8") as f:
                f.write(code)

            start_time = time.time()
            # Run in a subprocess
            process = subprocess.run(
                ["python", script_path],
                capture_output=True,
                text=True,
                timeout=self.timeout
            )
            execution_time = time.time() - start_time

            return {
                "stdout": process.stdout,
                "stderr": process.stderr,
                "return_code": process.returncode,
                "timed_out": False,
                "execution_time": round(execution_time, 4),
                "status": "success"
            }

        except subprocess.TimeoutExpired:
            return {"timed_out": True, "error": "Execution timed out.", "status": "failure"}
        except Exception as e:
            return {"error": str(e), "status": "failure"}
        finally:
            if os.path.exists(script_path):
                os.remove(script_path)

    def run_powershell(self, code: str) -> Dict[str, Any]:
        if not self._is_safe(code, "powershell"):
            return {"error": "SECURITY VIOLATION: Network-related code detected.", "status": "failure"}

        file_id = str(uuid.uuid4())
        script_path = os.path.join(self.temp_dir, f"script_{file_id}.ps1")
        
        try:
            with open(script_path, "w", encoding="utf-8") as f:
                f.write(code)

            start_time = time.time()
            # Run with Bypass execution policy for this specific run
            process = subprocess.run(
                ["powershell", "-ExecutionPolicy", "Bypass", "-File", script_path],
                capture_output=True,
                text=True,
                timeout=self.timeout
            )
            execution_time = time.time() - start_time

            return {
                "stdout": process.stdout,
                "stderr": process.stderr,
                "return_code": process.returncode,
                "timed_out": False,
                "execution_time": round(execution_time, 4),
                "status": "success"
            }

        except subprocess.TimeoutExpired:
            return {"timed_out": True, "error": "Execution timed out.", "status": "failure"}
        except Exception as e:
            return {"error": str(e), "status": "failure"}
        finally:
            if os.path.exists(script_path):
                os.remove(script_path)

    def handle_long_output(self, output: str, task_name: str = "output") -> str:
        """Saves long output to outbox if it exceeds Discord limits."""
        if len(output) > 1800:
            file_id = str(uuid.uuid4())[:8]
            file_name = f"{task_name}_{file_id}.txt"
            path = os.path.join(self.outbox_dir, file_name)
            with open(path, "w", encoding="utf-8") as f:
                f.write(output)
            return f"Output is too long. Saved to: {path}"
        return output
