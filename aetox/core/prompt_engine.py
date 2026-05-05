import yaml
import os
import logging
from typing import Dict, List, Any, Optional

logger = logging.getLogger("aetox.core.prompt_engine")

class PromptEngine:
    """
    Formats prompts for different agent roles and model sizes.
    Ensures consistent instructions and output schemas.
    """

    SYSTEM_PROMPTS = {
        "planner": (
            "You are the Strategic Planner of AetoxClaw. Your goal is to decompose complex user "
            "requests into clear, sequential steps.\n\n"
            "CRITICAL: You must provide a polite narrative in THAI explaining your plan to the user "
            "before listing the steps. This narrative should sound natural and helpful.\n"
            "You must respond ONLY in valid JSON format."
        ),
        "executor": (
            "You are the Executor Agent. You carry out specific task steps precisely. "
            "Follow instructions exactly and report results clearly. "
            "You must respond ONLY in valid JSON format."
        ),
        "researcher": (
            "You are the Researcher Agent. Your goal is to find, analyze, and synthesize information. "
            "Provide accurate facts and cite sources where possible. "
            "You must respond ONLY in valid JSON format."
        ),
        "critic": (
            "You are the Critic Agent for AetoxClaw. Your role is to verify if a task step was successful.\n\n"
            "GUIDELINES:\n"
            "1. If 'Status' is 'success', and the output contains success markers like 'สำเร็จ', 'เรียบร้อย', '✅', '📁', or '📄', give a HIGH score (1.0) and verdict 'pass'.\n"
            "2. DO NOT require external verification (like 'checking permissions') if the tool already reported success.\n"
            "3. Only reject if there is a clear 'failure' status or an obvious error message.\n"
            "4. Respond ONLY in valid JSON format."
        ),
        "coder": (
            "You are the Coder Agent. You specialize in software development, debugging, and technical tasks. "
            "Write clean, efficient, and well-documented code. "
            "You must respond ONLY in valid JSON format."
        ),
        "executor_extraction": (
            "You are a Tool Parameter Extractor. Your task is to analyze a TaskStep and extract "
            "the specific tool, action, and parameters required.\n\n"
            "Respond ONLY in valid JSON format."
        )
    }

    PLANNER_SCHEMA = {
        "plan_id": "string",
        "goal": "string",
        "narrative": "string (A polite summary of the plan in THAI for the user)",
        "steps": [
            {
                "step_id": "integer",
                "description": "string",
                "agent": "executor | researcher | coder",
                "tool": "string",
                "memory_needed": ["list of strings"],
                "success_criteria": "string"
            }
        ],
        "estimated_steps": "integer"
    }

    CRITIC_SCHEMA = {
        "verdict": "pass | retry | escalate",
        "score": "float (0.0 to 1.0)",
        "issues": ["list of strings"],
        "suggestion": "string"
    }

    def build_chat_messages(
        self, 
        role: str, 
        user_input: str, 
        context: Optional[Dict[str, Any]] = None,
        json_schema: Optional[Dict[str, Any]] = None
    ) -> List[Dict[str, str]]:
        """
        Builds a list of chat messages for the Ollama API.
        """
        system_content = self.SYSTEM_PROMPTS.get(role, "You are a helpful assistant.")
        
        if json_schema:
            system_content += f"\n\nYour output must adhere to this JSON schema:\n{json_schema}"
        else:
            system_content += "\n\nResponse must be a valid JSON object."

        messages = [
            {"role": "system", "content": system_content},
            {"role": "user", "content": user_input}
        ]

        if context:
            # In a real scenario, we might want to inject context differently, 
            # but for now, we'll append it to the user message or as a separate message.
            context_str = f"\n\nContext Information:\n{context}"
            messages[-1]["content"] += context_str

        return messages
    def get_external_template(self, file_path: str, template_name: str) -> Dict[str, Any]:
        """Loads a specific template from an external YAML file."""
        if not os.path.exists(file_path):
            logger.warning(f"Template file not found: {file_path}")
            return {}
        
        try:
            with open(file_path, "r", encoding="utf-8") as f:
                data = yaml.safe_load(f)
                return data.get(template_name, {}) if data else {}
        except yaml.YAMLError as ye:
            logger.error(f"Failed to parse YAML template {file_path}: {ye}")
            return {}
        except Exception as e:
            logger.error(f"Unexpected error loading template {file_path}: {e}")
            return {}
