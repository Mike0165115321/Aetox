import pytest
from aetox.memory.working import SessionContext


class TestSessionContext:
    @pytest.fixture
    def session(self):
        return SessionContext(chat_history_limit=3)

    def test_add_exchange(self, session):
        session.add_exchange("Hello", "Hi there")
        assert len(session) == 1
        assert session.chat_history[0]["q"] == "Hello"
        assert session.chat_history[0]["a"] == "Hi there"

    def test_sliding_window(self, session):
        for i in range(5):
            session.add_exchange(f"Q{i}", f"A{i}")
        
        # Limit is 3, so only last 3 should remain
        assert len(session) == 3
        assert session.chat_history[0]["q"] == "Q2"
        assert session.chat_history[-1]["q"] == "Q4"

    def test_get_chat_history(self, session):
        session.add_exchange("Q1", "A1")
        session.add_exchange("Q2", "A2")
        
        history = session.get_chat_history()
        assert len(history) == 2
        assert history[0]["q"] == "Q1"

    def test_get_history_as_string(self, session):
        session.add_exchange("Hello", "Hi")
        result = session.get_history_as_string()
        assert "ถาม: Hello" in result
        assert "ตอบ: Hi" in result

    def test_get_history_as_string_empty(self, session):
        result = session.get_history_as_string()
        assert result == "ไม่มี"

    def test_truncation(self, session):
        long_text = "A" * 500
        session.add_exchange(long_text, long_text, truncate_chars=100)
        assert len(session.chat_history[0]["q"]) == 100
        assert len(session.chat_history[0]["a"]) == 100

    def test_clear(self, session):
        session.add_exchange("Q1", "A1")
        session.add_exchange("Q2", "A2")
        session.clear()
        assert len(session) == 0

    def test_repr(self, session):
        session.add_exchange("Q", "A")
        assert "exchanges=1" in repr(session)
        assert "limit=3" in repr(session)
