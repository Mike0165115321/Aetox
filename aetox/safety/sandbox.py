import os
import yaml
from pathlib import Path
from typing import List, Optional

class SafetyViolation(Exception):
    """Raised when an operation violates security boundaries."""
    pass

class Sandbox:
    """
    Enforces file system access boundaries.
    """
    def __init__(self, config_path: str = "config/permissions.yaml"):
        self.allowed_paths = []
        self.forbidden_paths = []
        self._load_config(config_path)

    def _load_config(self, config_path: str):
        try:
            with open(config_path, 'r') as f:
                config = yaml.safe_load(f)
                
            username = os.getlogin()
            
            for p in config.get('allowed_paths', []):
                p = p.replace("{username}", username)
                self.allowed_paths.append(str(Path(p).resolve()))
                
            for p in config.get('forbidden_paths', []):
                p = p.replace("{username}", username)
                self.forbidden_paths.append(str(Path(p).resolve()))
                
        except Exception as e:
            # Fallback to current directory if config fails
            self.allowed_paths = [str(Path(".").resolve())]

    def validate_path(self, path: str) -> Path:
        """
        Validates if the given path is within allowed boundaries.
        Returns resolved Path if safe, raises SafetyViolation otherwise.
        """
        resolved_path = Path(path).resolve()
        path_str = str(resolved_path)

        # 1. Check forbidden paths first (explicit denial)
        for forbidden in self.forbidden_paths:
            if path_str.startswith(forbidden):
                raise SafetyViolation(f"ACCESS DENIED: Path '{path}' is in a forbidden system area.")

        # 2. Check allowed paths
        is_allowed = False
        for allowed in self.allowed_paths:
            if path_str.startswith(allowed):
                is_allowed = True
                break
        
        if not is_allowed:
            raise SafetyViolation(f"SAFETY VIOLATION: Path '{path}' is outside of allowed workspace boundaries.")
            
        return resolved_path
