import json
import logging
import os
from datetime import datetime
from typing import Dict, Any, List

class PreferenceMemory:
    """
    JSON-based storage for user preferences and learned rules.
    """
    def __init__(self, path: str = "config/preferences.json"):
        self.path = path
        self.logger = logging.getLogger("aetox.memory.preference")
        self.preferences = {
            "file_naming": "descriptive",
            "output_format": "markdown",
            "forbidden_paths": [],
            "custom_rules": [],
            "last_updated": datetime.now().isoformat()
        }
        self._load()

    def _load(self):
        if os.path.exists(self.path):
            try:
                with open(self.path, 'r') as f:
                    self.preferences.update(json.load(f))
            except Exception as e:
                self.logger.error(f"Failed to load preferences: {e}")

    def _save(self):
        try:
            os.makedirs(os.path.dirname(self.path), exist_ok=True)
            self.preferences["last_updated"] = datetime.now().isoformat()
            with open(self.path, 'w') as f:
                json.dump(self.preferences, f, indent=2)
        except Exception as e:
            self.logger.error(f"Failed to save preferences: {e}")

    def update(self, key: str, value: Any):
        if key in self.preferences and isinstance(self.preferences[key], list):
            if value not in self.preferences[key]:
                self.preferences[key].append(value)
        else:
            self.preferences[key] = value
        self._save()

    def get(self, key: str, default: Any = None) -> Any:
        return self.preferences.get(key, default)

    def learn_from_denial(self, action: str, details: str):
        """
        Learns from a user's security denial.
        """
        rule = f"User denied action '{action}' with details: {details}"
        if rule not in self.preferences["custom_rules"]:
            self.preferences["custom_rules"].append(rule)
            self.logger.info(f"Learned new preference: {rule}")
            self._save()
