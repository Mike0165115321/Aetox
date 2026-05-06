import pytest
import json
from unittest.mock import MagicMock, AsyncMock, patch
from aetox.agents.executor import ExecutorAgent

@pytest.fixture
def executor(mock_ollama_client):
    with patch("aetox.agents.executor.PermissionManager"), \
         patch("aetox.agents.executor.create_default_registry"):
        agent = ExecutorAgent(client=mock_ollama_client)
        # Mock the tool registry
        agent.tools = MagicMock()
        return agent

@pytest.mark.asyncio
async def test_extract_action_success(executor, mock_ollama_client, mock_extraction_response):
    mock_ollama_client.chat.return_value = {
        "message": {"content": json.dumps(mock_extraction_response)}
    }
    
    result = await executor.extract_action({"description": "list files"})
    
    assert result["tool"] == "master_file_manager"
    assert result["action"] == "list_dir"

@pytest.mark.asyncio
async def test_extract_action_low_confidence(executor, mock_ollama_client):
    mock_ollama_client.chat.return_value = {
        "message": {"content": json.dumps({"tool": "t", "confidence": 0.2})}
    }
    
    result = await executor.extract_action({"description": "hi"})
    
    assert result["tool"] == "chat"
    assert result["confidence"] == 1.0

@pytest.mark.asyncio
async def test_run_action_tool(executor):
    extraction = {
        "tool": "file_tool",
        "action": "delete",
        "params": {"path": "test.txt"}
    }
    executor.tools.execute.return_value = {"status": "success", "output": "deleted"}
    
    result = await executor.run_action(extraction, {})
    
    assert result["status"] == "success"
    executor.tools.execute.assert_called_once_with("file_tool", {"path": "test.txt", "action": "delete"})

@pytest.mark.asyncio
async def test_run_action_chat(executor, mock_ollama_client):
    extraction = {
        "tool": "chat",
        "params": {"message": "hello"}
    }
    mock_ollama_client.chat.return_value = {"message": {"content": "Hi there"}}
    
    result = await executor.run_action(extraction, {})
    
    assert result["status"] == "success"
    assert result["output"] == "Hi there"

@pytest.mark.asyncio
async def test_run_chat_stream(executor, mock_ollama_client):
    async def mock_stream(*args, **kwargs):
        yield "Token 1"
        yield "Token 2"
    
    mock_ollama_client.chat_stream = mock_stream
    
    tokens = []
    async for token in executor.run_chat_stream("hello"):
        tokens.append(token)
    
    assert tokens == ["Token 1", "Token 2"]
