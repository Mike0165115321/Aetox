import yaml
import logging
import os
from typing import Dict, Any, Optional, List, Union
from pydantic import BaseModel, Field, model_validator
from dotenv import load_dotenv

# Load .env at the very beginning
load_dotenv()

logger = logging.getLogger("aetox.core.config_loader")

# 🏛️ GLOBAL DEFAULTS
FALLBACK_MODEL = "qwen3:8b"

class ModelOptions(BaseModel):
    temperature: Optional[float] = Field(default=None, ge=0.0, le=2.0)
    top_p: Optional[float] = Field(default=None, ge=0.0, le=1.0)
    num_ctx: Optional[int] = Field(default=None, ge=1024, le=131072)
    stop: Optional[List[str]] = Field(default=None)

    def merge_with(self, other: 'ModelOptions') -> 'ModelOptions':
        data = self.model_dump(exclude_none=True)
        other_data = other.model_dump(exclude_none=True)
        data.update(other_data)
        return ModelOptions(**data)

class AgentModelConfig(BaseModel):
    model: str
    options: Optional[ModelOptions] = None

class SessionConfig(BaseModel):
    """Lightweight session config — no RAG, no vector DB"""
    chat_history_limit: int = 5
    history_truncate_chars: int = 200

class AgentConfig(BaseModel):
    global_options: ModelOptions = Field(default_factory=ModelOptions)
    session: SessionConfig = Field(default_factory=SessionConfig)
    models: Dict[str, Union[str, AgentModelConfig]]

    @model_validator(mode='after')
    def validate_models(self) -> 'AgentConfig':
        required = {"main"}
        missing = required - set(self.models.keys())
        if missing:
            logger.warning(f"Missing essential roles in config: {missing}. Fallback to {FALLBACK_MODEL}")
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
            global_options=ModelOptions(temperature=0.1, num_ctx=8192),
            models={"main": FALLBACK_MODEL}
        )

    def get_model(self, role: str) -> str:
        entry = self._config.models.get(role) or self._config.models.get("main") or FALLBACK_MODEL
        if isinstance(entry, AgentModelConfig):
            return entry.model
        return entry

    def get_options(self, role: str) -> Dict[str, Any]:
        effective_options = self._config.global_options
        entry = self._config.models.get(role) or self._config.models.get("main")
        if isinstance(entry, AgentModelConfig) and entry.options:
            effective_options = effective_options.merge_with(entry.options)
        return effective_options.model_dump(exclude_none=True)

    def get_session_config(self) -> Dict[str, Any]:
        """Returns session configuration (history limits etc.)"""
        return self._config.session.model_dump()

    def get_ollama_url(self) -> str:
        return os.getenv("OLLAMA_BASE_URL", "http://localhost:11434")

# Singleton instance
config_loader = ConfigLoader()
