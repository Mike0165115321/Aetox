import os
import logging
from pathlib import Path
from typing import List, Optional, Union

from aetox.safety.sandbox import Sandbox, SafetyViolation

logger = logging.getLogger("aetox.tools.file_manager")

class FileManagerTool:
    """
    Standard Windows file system operations with path safety via Sandbox.
    """
    def __init__(self, allowed_paths: Optional[List[str]] = None):
        self.sandbox = Sandbox()
        # If allowed_paths provided, we could override but Sandbox handles config now

    def _validate_path(self, path: Union[str, Path]) -> Path:
        return self.sandbox.validate_path(path)

    def list_files(self, directory: str = ".") -> List[str]:
        """Lists all files in the given directory."""
        safe_path = self._validate_path(directory)
        if not safe_path.is_dir():
            raise NotADirectoryError(f"'{directory}' is not a directory.")
        
        return [f.name for f in safe_path.iterdir() if f.is_file()]

    def read_file(self, file_path: str) -> str:
        """Reads and returns the content of a file."""
        safe_path = self._validate_path(file_path)
        if not safe_path.is_file():
            raise FileNotFoundError(f"File '{file_path}' not found.")
            
        with open(safe_path, 'r', encoding='utf-8') as f:
            return f.read()

    def write_file(self, file_path: str, content: str) -> str:
        """Writes content to a file (creates or overwrites)."""
        safe_path = self._validate_path(file_path)
        
        # Ensure parent directory exists
        safe_path.parent.mkdir(parents=True, exist_ok=True)
        
        with open(safe_path, 'w', encoding='utf-8') as f:
            f.write(content)
        
        return f"Successfully wrote to {file_path}"

    def create_directory(self, dir_path: str) -> str:
        """Creates a new directory."""
        safe_path = self._validate_path(dir_path)
        safe_path.mkdir(parents=True, exist_ok=True)
        return f"Directory created: {dir_path}"
