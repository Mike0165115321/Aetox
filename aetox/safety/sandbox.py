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
        if not path:
            raise SafetyViolation("SAFETY VIOLATION: Empty path provided.")

        # Resolve and normalize the input path
        resolved_path = Path(path).resolve()
        path_str = str(resolved_path).lower()

        # 1. Check forbidden paths first (explicit denial)
        for forbidden in self.forbidden_paths:
            forbidden_resolved = str(Path(forbidden).resolve()).lower()
            if path_str.startswith(forbidden_resolved):
                raise SafetyViolation(f"ACCESS DENIED: Path '{path}' is in a forbidden system area.")

        # 2. Check allowed paths
        is_allowed = False
        for allowed in self.allowed_paths:
            allowed_resolved = str(Path(allowed).resolve()).lower()
            # Check if path is same as allowed or inside it
            if path_str.startswith(allowed_resolved):
                is_allowed = True
                break
        
        if not is_allowed:
            self.logger.warning(f"Path validation failed: {path_str} not in {[str(Path(p).resolve()).lower() for p in self.allowed_paths]}")
            raise SafetyViolation(f"SAFETY VIOLATION: Path '{path}' is outside of allowed workspace boundaries.")
            
        return resolved_path
