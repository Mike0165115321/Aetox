import logging
from typing import Dict, Any, List, Optional
from aetox.memory.episodic import EpisodicMemory
from aetox.memory.preference import PreferenceMemory

class MemoryManager:
    """
    Coordinator for AetoxOS memory systems.
    Links Working, Episodic, and Preference memory.
    """
    def __init__(self, db_path: str = "aetox_memory.db", pref_path: str = "config/preferences.json"):
        self.logger = logging.getLogger("aetox.memory.manager")
        self.episodic = EpisodicMemory(db_path)
        self.preference = PreferenceMemory(pref_path)

    def save_episode(self, event_id: str, event_type: str, summary: str, outcome: str, facts: Dict[str, Any], tags: List[str]):
        self.episodic.save_episode(event_id, event_type, summary, outcome, facts, tags)

    def update_preference(self, key: str, value: Any):
        self.preference.update(key, value)

    def learn_from_denial(self, action: str, details: str):
        self.preference.learn_from_denial(action, details)

    def get_context_for_planner(self, goal: str) -> str:
        """
        Fetches relevant past episodes and preferences to guide the Planner.
        """
        recent = self.episodic.query_recent(limit=3)
        prefs = self.preference.preferences
        
        context_parts = []
        
        if recent:
            context_parts.append("### RECENT TASK HISTORY:")
            for ep in recent:
                context_parts.append(f"- Goal: {ep['task_summary']} | Outcome: {ep['outcome']} | Facts: {ep['key_facts']}")
        
        if prefs.get("custom_rules"):
            context_parts.append("### USER PREFERENCES & RULES:")
            for rule in prefs["custom_rules"]:
                context_parts.append(f"- {rule}")
                
        return "\n".join(context_parts)
