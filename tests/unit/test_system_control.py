import pytest
from unittest.mock import MagicMock
from aetox.tools.system_control import SystemControl

class TestSystemControl:
    @pytest.fixture
    def mock_registry(self):
        registry = MagicMock()
        mock_tool = MagicMock()
        mock_tool.description = "Test Description"
        registry.get_all.return_value = {"mock_tool": mock_tool}
        return registry

    @pytest.fixture
    def tool(self, mock_registry):
        return SystemControl(registry=mock_registry)

    def test_identity_initialized(self, tool):
        assert "AetoxClaw" in tool.identity

    def test_chat_action(self, tool):
        result = tool.execute({"action": "chat", "message": "hello"})
        assert result["status"] == "chat"
        assert "บริบทตัวตน:" in result["output"]
        assert "hello" in result["output"]

    def test_get_status_action(self, tool):
        result = tool.execute({"action": "get_status"})
        assert result["status"] == "success"
        assert "🟢 ระบบออนไลน์" in result["output"]

    def test_list_capabilities_success(self, tool):
        result = tool.execute({"action": "list_capabilities"})
        assert result["status"] == "success"
        # Check for the key phrase without sensitive emoji encoding
        assert "เครื่องมือที่พร้อมใช้งาน" in result["output"]
        assert "mock_tool" in result["output"]
        assert "Test Description" in result["output"]

    def test_list_capabilities_no_registry(self):
        tool_no_reg = SystemControl(registry=None)
        result = tool_no_reg.execute({"action": "list_capabilities"})
        assert result["status"] == "failure"
        assert "ไม่สามารถดึงข้อมูล Registry ได้" in result["error"]

    def test_invalid_action(self, tool):
        result = tool.execute({"action": "invalid"})
        assert result["status"] == "failure"
        assert "ไม่พบคำสั่ง" in result["error"]

    def test_get_prompt_doc(self, tool):
        doc = tool.get_prompt_doc()
        assert "Tool: system_control" in doc
        assert "chat" in doc
        assert "get_status" in doc
        assert "list_capabilities" in doc
