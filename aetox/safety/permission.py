import logging
import yaml
from typing import Dict, Any

class PermissionManager:
    """
    Manages user approvals based on operation risk levels.
    """
    def __init__(self, config_path: str = "config/permissions.yaml"):
        self.logger = logging.getLogger("aetox.safety.permission")
        self.risk_rules = {}
        self.approval_callback = None # Set by interfaces (e.g. Discord)
        self._load_config(config_path)

    def _load_config(self, config_path: str):
        try:
            with open(config_path, 'r') as f:
                config = yaml.safe_load(f)
                self.risk_rules = config.get('risk_rules', {})
        except:
            self.risk_rules = {"high": ["delete_file"], "medium": ["write_file"]}

    def get_risk_level(self, action: str, params: Dict[str, Any]) -> str:
        """
        Determines the risk level of an action.
        """
        action = action.lower()
        
        if action in self.risk_rules.get('high', []):
            return "high"
            
        # Special rule: write_file outside project folder is always HIGH
        if action == "write_file":
            path = params.get("path", "")
            if path and not path.startswith(".") and "/" in path:
                # Simple check for path outside relative project root
                return "high"
            return "medium"

        if action in self.risk_rules.get('medium', []):
            return "medium"
            
        return "low"

    def request_permission(self, action: str, details: str) -> bool:
        """
        Requests permission from the user via CLI or External Callback (Discord).
        """
        if self.approval_callback:
            return self.approval_callback(action, details)
            
        print(f"\n[SECURITY ACTION REQUIRED]")
        print(f"Action: {action}")
        print(f"Details: {details}")
        
        choice = input("Allow this action? (y/N): ").strip().lower()
        if choice == 'y':
            self.logger.info(f"User APPROVED action: {action}")
            return True
        else:
            self.logger.warning(f"User DENIED action: {action}")
            return False
