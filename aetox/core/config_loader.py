import yaml
import logging
import os
from typing import Dict, Any, Optional, List, Union
from pydantic import BaseModel, Field, model_validator

logger = logging.getLogger("aetox.core.config_loader")

# 🏛️ GLOBAL DEFAULTS - To avoid Technical Debt
# If you want to change the fallback model, change it here ONCE.
FALLBACK_MODEL = "qwen3:8b"

class ModelOptions(BaseModel):
    temperature: Optional[float] = Field(default=None, ge=0.0, le=2.0)
    top_p: Optional[float] = Field(default=None, ge=0.0, le=1.0)
    num_ctx: Optional[int] = Field(default=None, ge=1024, le=131072)
    stop: Optional[List[str]] = Field(default=None)

    def merge_with(self, other: 'ModelOptions') -> 'ModelOptions':
        """Merges this options with another, prioritizing 'other'."""
        data = self.model_dump(exclude_none=True)
        other_data = other.model_dump(exclude_none=True)
        data.update(other_data)
        return ModelOptions(**data)

class AgentModelConfig(BaseModel):
    model: str
    options: Optional[ModelOptions] = None

class AgentConfig(BaseModel):
    global_options: ModelOptions = Field(default_factory=ModelOptions)
    models: Dict[str, Union[str, AgentModelConfig]]

    @model_validator(mode='after')
    def validate_models(self) -> 'AgentConfig':
        required = {"planner", "executor", "critic", "researcher", "coder", "extraction"}
        missing = required - set(self.models.keys())
        if missing:
            logger.warning(f"Missing recommended roles in config: {missing}")
        return self

class ConfigLoader:
    _instance: Optional['ConfigLoader'] = None
    _config: Optional[AgentConfig] = None

    def __new__(cls):
        if cls._instance is None:
            cls._instance = super(ConfigLoader, cls).__new__(cls)
            cls._instance._load()
        return cls._instance

    def _load(self, path: str = "config/models.yaml"):
        try:
            if not os.path.exists(path):
                logger.error(f"Config file not found: {path}")
                self._config = self._get_default_config()
                return

            with open(path, "r", encoding="utf-8") as f:
                raw = yaml.safe_load(f) or {}
            
            self._config = AgentConfig(**raw)
            logger.info(f"Configuration loaded successfully from {path}")
        except Exception as e:
            logger.error(f"Failed to load config: {e}. Using defaults.")
            self._config = self._get_default_config()

    def _get_default_config(self) -> AgentConfig:
        return AgentConfig(
            global_options=ModelOptions(temperature=0.2, num_ctx=4096),
            models={role: FALLBACK_MODEL for role in ["planner", "executor", "critic", "researcher", "coder", "extraction"]}
        )

    def get_model(self, role: str) -> str:
        entry = self._config.models.get(role, FALLBACK_MODEL)
        if isinstance(entry, AgentModelConfig):
            return entry.model
        return entry

    def get_options(self, role: str) -> Dict[str, Any]:
        """Returns merged options for the specific role."""
        effective_options = self._config.global_options
        
        entry = self._config.models.get(role)
        if isinstance(entry, AgentModelConfig) and entry.options:
            effective_options = effective_options.merge_with(entry.options)
        
        # Clean up None values for Ollama API
        return effective_options.model_dump(exclude_none=True)

# Singleton instance
config_loader = ConfigLoader()
