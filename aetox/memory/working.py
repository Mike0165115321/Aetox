# aetox/memory/working.py
"""
SessionContext — Lightweight session state for AetoxClaw.
ไม่มี RAG, ไม่มี auto-summarize, ไม่มี vector store
เก็บแค่ประวัติการคุย (chat history) แบบ Sliding Window
"""
import logging
from typing import List, Dict, Any

logger = logging.getLogger("aetox.memory.session")


class SessionContext:
    """
    Lightweight session state — ไม่มี processing, แค่เก็บ history
    
    Usage:
        Chat Mode:  get_chat_history() → ส่ง 3-5 ข้อความล่าสุด
        Plan Mode:  Dispatcher จัดการ plan_history ภายในเอง
    """
    
    def __init__(self, chat_history_limit: int = 5):
        self.chat_history: List[Dict[str, str]] = []  # [{q: str, a: str}]
        self.chat_history_limit = chat_history_limit
        logger.debug(f"SessionContext initialized (history_limit={chat_history_limit})")

    def add_exchange(self, question: str, answer: str, truncate_chars: int = 200):
        """บันทึกการคุย 1 รอบ (คำถาม + คำตอบ) พร้อมตัดความยาว"""
        q_trunc = question[:truncate_chars] if question else ""
        a_trunc = answer[:truncate_chars] if isinstance(answer, str) else str(answer)[:truncate_chars]
        
        self.chat_history.append({"q": q_trunc, "a": a_trunc})
        
        # Sliding window: ตัดข้อความเก่าออก
        if len(self.chat_history) > self.chat_history_limit:
            self.chat_history.pop(0)

    def get_chat_history(self) -> List[Dict[str, str]]:
        """คืนประวัติการคุยล่าสุดตาม limit"""
        return self.chat_history[-self.chat_history_limit:]

    def get_history_as_string(self) -> str:
        """แปลงประวัติเป็น string สำหรับใส่ใน prompt"""
        if not self.chat_history:
            return "ไม่มี"
        lines = []
        for i, h in enumerate(self.get_chat_history(), 1):
            lines.append(f"{i}. ถาม: {h['q']} -> ตอบ: {h['a']}")
        return "\n".join(lines)

    def clear(self):
        """ล้างประวัติทั้งหมด"""
        self.chat_history.clear()

    def __len__(self):
        return len(self.chat_history)

    def __repr__(self):
        return f"SessionContext(exchanges={len(self.chat_history)}, limit={self.chat_history_limit})"