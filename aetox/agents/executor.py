import logging
import json
import asyncio
from typing import Dict, Any, Optional
from aetox.tools.file_manager import FileManagerTool
from aetox.tools.code_runner import CodeRunnerTool
from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.safety.permission import PermissionManager
from aetox.safety.sandbox import SafetyViolation
from aetox.memory.manager import MemoryManager

class ExecutorAgent:
    """
    Executes task steps using available tools.
    Uses LLM (7B) for smart parameter extraction with a heuristic fallback.
    Includes safety and permission checks.
    """
    EXTRACTION_SCHEMA = {
        "tool": "file_manager | discord_manager | code_runner | other",
        "action": "list_files | read_file | write_file | create_category | create_channel | run_python | run_powershell",
        "params": {
            "path": "string (for files)",
            "code": "string (for code_runner)",
            "name": "string (for discord)",
            "guild_id": "integer",
            "content": "string"
        },
        "confidence": "float (0.0 to 1.0)"
    }

    def __init__(
        self, 
        client: Optional[OllamaClient] = None, 
        engine: Optional[PromptEngine] = None,
        allowed_paths: Optional[list] = None,
        discord_tool: Any = None
    ):
        self.logger = logging.getLogger("aetox.agents.executor")
        self.file_manager = FileManagerTool(allowed_paths=allowed_paths)
        self.discord_tool = discord_tool
        self.code_runner = CodeRunnerTool()
        self.permission_manager = PermissionManager()
        self.memory_manager = MemoryManager()
        self.client = client or OllamaClient()
        self.engine = engine or PromptEngine()
        self.model = "qwen2.5:7b"

    def execute_step(self, step: Dict[str, Any], memory_context: Dict[str, Any]) -> Dict[str, Any]:
        step_id = step.get("step_id", 0)
        description = step.get("description", "No description")
        
        self.logger.info(f"Analyzing Step {step_id}: {description}")
        
        # 1. Try LLM Extraction
        extraction = self._extract_with_llm(step, memory_context)
        
        source = "LLM"
        if not extraction or extraction.get("confidence", 0) < 0.7:
            self.logger.warning(f"LLM extraction failed or low confidence. Falling back to Heuristics.")
            extraction = self._extract_with_heuristics(step, memory_context)
            source = "Heuristic"
        
        action = extraction.get("action", "unknown")
        params = extraction.get("params", {})
        self.logger.info(f"Execution source: {source} | Extracted Action: {action}")

        # 2. Permission Check
        risk = self.permission_manager.get_risk_level(action, params)
        if risk == "high":
            details = f"{action} -> {params}"
            if not self.permission_manager.request_permission(action, details):
                # LEARN FROM DENIAL
                self.memory_manager.learn_from_denial(action, details)
                return {
                    "status": "failure", 
                    "error": "User DENIED high-risk operation.",
                    "output": None
                }
        elif risk == "medium":
            self.logger.info(f"MEDIUM risk action '{action}' logged.")

        # 3. Execute based on extraction
        return self._run_extraction(extraction, step_id, memory_context)

    def _extract_with_llm(self, step: Dict[str, Any], memory_context: Dict[str, Any]) -> Optional[Dict[str, Any]]:
        try:
            prompt_input = f"Task Step: {json.dumps(step)}\n\nAvailable Context: {json.dumps(memory_context.get('context', {}))}"
            messages = self.engine.build_chat_messages(
                role="executor_extraction",
                user_input=prompt_input,
                json_schema=self.EXTRACTION_SCHEMA
            )
            
            response = self.client.chat(
                model=self.model,
                messages=messages,
                format="json",
                options={"temperature": 0.1}
            )
            
            content = response.get("message", {}).get("content", "")
            return json.loads(content)
        except Exception as e:
            self.logger.error(f"LLM Extraction Error: {e}")
            return None

    def _extract_with_heuristics(self, step: Dict[str, Any], memory_context: Dict[str, Any]) -> Dict[str, Any]:
        tool_name = step.get("tool", "").lower()
        description = step.get("description", "No description").lower()
        
        # Priority: Write > Count > List
        if any(k in tool_name or k in description for k in ["write", "create", "touch", "summary"]):
            count = memory_context.get("context", {}).get("file_count", 0)
            return {
                "tool": "file_manager",
                "action": "write_file",
                "params": {"path": "summary.txt", "content": f"Total files found: {count}"}
            }
        elif any(k in tool_name or k in description for k in ["count", "number"]):
            return {"tool": "internal", "action": "count_files", "params": {}}
        elif any(k in tool_name or k in description for k in ["list", "ls", "directory"]):
            return {"tool": "file_manager", "action": "list_files", "params": {"path": "."}}
            
        return {"tool": "unknown", "action": "unknown", "params": {}}

    def _run_extraction(self, extraction: Dict[str, Any], step_id: int, memory_context: Dict[str, Any]) -> Dict[str, Any]:
        tool = extraction.get("tool", "").lower()
        action = extraction.get("action", "").lower()
        params = extraction.get("params", {})

        try:
            # Flexible mapping: check action first, then tool/action combination
            if action == "list_files" or (tool == "file_manager" and "list" in action):
                files = self.file_manager.list_files(params.get("path", "."))
                return {
                    "status": "success",
                    "output": f"Found {len(files)} files: {', '.join(files)}",
                    "memory_updates": {"file_list": files, "file_count": len(files)}
                }
            elif action == "write_file" or (tool == "file_manager" and "write" in action) or tool == "write_file":
                path = params.get("path")
                content = params.get("content")
                if not path or not content:
                    count = memory_context.get("context", {}).get("file_count", 0)
                    content = content or f"Total files found: {count}"
                    path = path or "summary.txt"
                    
                result = self.file_manager.write_file(path, content)
                return {
                    "status": "success",
                    "output": result,
                    "memory_updates": {"created_file": path}
                }
            elif action == "read_file" or (tool == "file_manager" and "read" in action):
                content = self.file_manager.read_file(params.get("path"))
                return {"status": "success", "output": content}
            elif action == "create_directory" or (tool == "file_manager" and "directory" in action):
                result = self.file_manager.create_directory(params.get("path"))
                return {"status": "success", "output": result}

            elif tool == "discord_manager" or action in ["create_category", "create_channel"]:
                if not self.discord_tool:
                    return {"status": "failure", "error": "Discord tool not initialized.", "output": None}
                
                name = params.get("name")
                guild_id = params.get("guild_id")
                
                if action == "create_category":
                    future = asyncio.run_coroutine_threadsafe(
                        self.discord_tool.create_category(int(guild_id), name), 
                        self.discord_tool.bot.loop
                    )
                    output = future.result()
                    return {"status": "success", "output": output}
                
                elif action == "create_channel":
                    cat_id = params.get("category_id")
                    future = asyncio.run_coroutine_threadsafe(
                        self.discord_tool.create_channel(int(guild_id), name, int(cat_id) if cat_id else None), 
                        self.discord_tool.bot.loop
                    )
                    output = future.result()
                    return {"status": "success", "output": output}

            elif tool == "code_runner" or action in ["run_python", "run_powershell"]:
                code = params.get("code")
                if not code:
                    return {"status": "failure", "error": "No code provided.", "output": None}

                # High risk action check
                if not self.permission_manager.request_permission("run_code", f"Executing {action} script."):
                    return {"status": "failure", "error": "Permission denied by user.", "output": None}

                if action == "run_python":
                    result = self.code_runner.run_python(code)
                else:
                    result = self.code_runner.run_powershell(code)
                
                if result["status"] == "success":
                    stdout = result.get("stdout", "")
                    stderr = result.get("stderr", "")
                    execution_time = result.get("execution_time", 0)
                    
                    combined_output = f"Stdout:\n{stdout}\n\nStderr:\n{stderr}\n\nExecution Time: {execution_time}s"
                    final_output = self.code_runner.handle_long_output(combined_output, action)
                    
                    return {
                        "status": "success",
                        "output": final_output,
                        "memory_updates": {"last_execution_time": execution_time}
                    }
                else:
                    return {"status": "failure", "error": result.get("error", "Unknown error"), "output": None}

            return {
                "status": "failure", 
                "error": f"Unsupported tool/action combination: {tool}/{action}",
                "output": None
            }

        except Exception as e:
            self.logger.error(f"Execution Error: {str(e)}")
            return {"status": "failure", "error": str(e), "output": None}
