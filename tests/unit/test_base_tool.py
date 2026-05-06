import pytest
from typing import Dict, Any
from aetox.tools.base import BaseTool

class MockTool(BaseTool):
    def execute(self, params: Dict[str, Any]) -> Dict[str, Any]:
        return {"status": "success", "output": "mocked"}

class TestBaseTool:
    @pytest.fixture
    def tool(self):
        return MockTool(
            name="test_tool",
            description="A tool for testing",
            actions=["action1", "action2"]
        )

    def test_default_prompt_doc(self, tool):
        doc = tool.get_prompt_doc()
        assert "Tool: test_tool" in doc
        assert "หน้าที่: A tool for testing" in doc
        assert "คำสั่งที่รองรับ (action): action1, action2" in doc

    def test_get_schema(self, tool):
        schema = tool.get_schema()
        assert schema["tool"] == "test_tool"
        assert schema["actions"] == ["action1", "action2"]
        assert "params" in schema

    def test_get_metadata(self, tool):
        metadata = tool.get_metadata()
        assert metadata["name"] == "test_tool"
        assert metadata["description"] == "A tool for testing"

    def test_init_no_actions(self):
        simple_tool = MockTool(name="simple", description="simple desc")
        assert simple_tool.actions == []
        doc = simple_tool.get_prompt_doc()
        assert "คำสั่งที่รองรับ (action): ไม่ระบุ" in doc
