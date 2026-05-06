import pytest
from unittest.mock import AsyncMock, MagicMock
from aetox.agents.executor import ExecutorAgent
from aetox.tools.loader import create_default_registry

class TestExecutorFlow:
    @pytest.fixture
    def executor(self, mock_ollama_client):
        # Executor needs a client and engine (DI)
        from aetox.core.prompt_engine import PromptEngine
        engine = PromptEngine()
        agent = ExecutorAgent(client=mock_ollama_client, engine=engine)
        return agent

    async def test_extract_action_flow(self, executor, mock_ollama_client, mock_extraction_response):
        # Mock LLM response for extraction
        import json
        mock_ollama_client.chat.return_value = {
            "message": {"content": json.dumps(mock_extraction_response)}
        }
        
        goal = "List files in current directory"
        extraction = await executor.extract_action({"description": goal}, {})
        
        assert extraction["tool"] == "master_file_manager"
        assert extraction["action"] == "list_dir"
        assert extraction["params"]["path"] == "."

    async def test_run_action_tool_success(self, executor, mock_ollama_client):
        # 1. Extraction says: use file_manager list_dir
        extraction = {
            "tool": "master_file_manager",
            "action": "list_dir",
            "params": {"path": "."}
        }
        
        # 2. Mocking actual tool execution within the executor
        # Since we use real tools, it should actually run the tool in our workspace
        # But we need to make sure the tool uses a safe path if we don't mock it.
        # Actually, let's just verify it calls the registry.
        
        result = await executor.run_action(extraction, {})
        assert result["status"] == "success"
        assert "### โครงสร้างไฟล์" in result["output"]

    async def test_handle_chat_fallback(self, executor, mock_ollama_client):
        # Test when no tool is suitable
        async def fake_stream(*args, **kwargs):
            for i in range(3):
                yield f"Token {i} "
        
        mock_ollama_client.chat_stream = fake_stream
        
        tokens = []
        async for token in executor.run_chat_stream("Hello, how are you?"):
            tokens.append(token)
            
        assert len(tokens) > 0
        assert "Token" in tokens[0]
