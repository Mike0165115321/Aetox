from typing import Dict, List, Any, Optional

class PromptEngine:
    """
    Formats prompts for different agent roles and model sizes.
    Ensures consistent instructions and output schemas.
    """

    SYSTEM_PROMPTS = {
        "planner": (
            "You are the Strategic Planner of AetoxOS. Your goal is to decompose complex user "
            "requests into clear, sequential steps. Each step must be assigned to the correct agent "
            "and tool. You must respond ONLY in valid JSON format."
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
            "You are the Critic Agent. Your role is to evaluate the quality of outputs from other agents. "
            "Be rigorous, detect errors, and suggest improvements. "
            "You must respond ONLY in valid JSON format."
        ),
        "coder": (
            "You are the Coder Agent. You specialize in software development, debugging, and technical tasks. "
            "Write clean, efficient, and well-documented code. "
            "You must respond ONLY in valid JSON format."
        ),
        "executor_extraction": (
            "You are a Tool Parameter Extractor. Your task is to analyze a TaskStep and extract "
            "the specific tool, action, and parameters required. "
            "Be precise and follow the JSON schema strictly. "
            "Respond ONLY in valid JSON format."
        )
    }

    PLANNER_SCHEMA = {
        "plan_id": "string",
        "goal": "string",
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
