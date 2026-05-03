import sqlite3
import json
import logging
from datetime import datetime
from typing import List, Dict, Any, Optional

class EpisodicMemory:
    """
    Persistent SQLite-based storage for task events and outcomes.
    """
    def __init__(self, db_path: str = "aetox_memory.db"):
        self.db_path = db_path
        self.logger = logging.getLogger("aetox.memory.episodic")
        self._init_db()

    def _init_db(self):
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("""
                CREATE TABLE IF NOT EXISTS episodes (
                    event_id TEXT PRIMARY KEY,
                    timestamp DATETIME,
                    event_type TEXT,
                    task_summary TEXT,
                    outcome TEXT,
                    key_facts TEXT,
                    tags TEXT
                )
            """)
            conn.commit()

    def save_episode(self, event_id: str, event_type: str, task_summary: str, outcome: str, key_facts: Dict[str, Any], tags: List[str]):
        try:
            with sqlite3.connect(self.db_path) as conn:
                conn.execute(
                    "INSERT INTO episodes VALUES (?, ?, ?, ?, ?, ?, ?)",
                    (
                        event_id,
                        datetime.now().isoformat(),
                        event_type,
                        task_summary,
                        outcome,
                        json.dumps(key_facts),
                        json.dumps(tags)
                    )
                )
                conn.commit()
            self.logger.info(f"Episode saved: {event_id} ({outcome})")
        except Exception as e:
            self.logger.error(f"Failed to save episode: {e}")

    def query_recent(self, limit: int = 5) -> List[Dict[str, Any]]:
        try:
            with sqlite3.connect(self.db_path) as conn:
                conn.row_factory = sqlite3.Row
                cursor = conn.execute("SELECT * FROM episodes ORDER BY timestamp DESC LIMIT ?", (limit,))
                rows = cursor.fetchall()
                
                results = []
                for row in rows:
                    item = dict(row)
                    item['key_facts'] = json.loads(item['key_facts'])
                    item['tags'] = json.loads(item['tags'])
                    results.append(item)
                return results
        except Exception as e:
            self.logger.error(f"Failed to query episodes: {e}")
            return []

    def find_by_tag(self, tag: str) -> List[Dict[str, Any]]:
        # Simple string search in JSON tags column
        try:
            with sqlite3.connect(self.db_path) as conn:
                conn.row_factory = sqlite3.Row
                cursor = conn.execute("SELECT * FROM episodes WHERE tags LIKE ? ORDER BY timestamp DESC", (f'%"{tag}"%',))
                rows = cursor.fetchall()
                
                results = []
                for row in rows:
                    item = dict(row)
                    item['key_facts'] = json.loads(item['key_facts'])
                    item['tags'] = json.loads(item['tags'])
                    results.append(item)
                return results
        except Exception as e:
            self.logger.error(f"Failed to find episodes by tag: {e}")
            return []
