import pytest
import asyncio
import time
from unittest.mock import MagicMock, AsyncMock, patch
from aetox.interfaces.discord_bot import MemoryManager, DiscordInterface

@pytest.mark.asyncio
async def test_memory_manager_lifecycle():
    with patch("aetox.interfaces.discord_bot.WorkingMemory"), \
         patch("aetox.interfaces.discord_bot.Dispatcher"):
        manager = MemoryManager(max_users=2, ttl=1)
        
        # Test creation
        mem1, disp1 = await manager.get_resources(1)
        assert 1 in manager.memories
        
        # Test capacity / eviction
        await manager.get_resources(2)
        await manager.get_resources(3) # Should evict 1 if we force age or just FIFO
        assert len(manager.memories) == 2
        assert 1 not in manager.memories

@pytest.mark.asyncio
async def test_memory_manager_cleanup():
    with patch("aetox.interfaces.discord_bot.WorkingMemory"), \
         patch("aetox.interfaces.discord_bot.Dispatcher"):
        manager = MemoryManager(ttl=0.1)
        await manager.get_resources(1)
        
        await asyncio.sleep(0.2)
        await manager.get_resources(2) # Triggers cleanup
        assert 1 not in manager.memories

@pytest.mark.asyncio
async def test_discord_interface_send_progress():
    mock_ctx = MagicMock()
    mock_ctx.send = AsyncMock()
    
    interface = DiscordInterface(mock_ctx)
    await interface.send_progress("Work in progress")
    
    mock_ctx.send.assert_called_once_with("⏳ Work in progress")

@pytest.mark.asyncio
async def test_discord_interface_request_approval():
    mock_ctx = MagicMock()
    mock_ctx.send = AsyncMock()
    mock_msg = MagicMock()
    mock_msg.add_reaction = AsyncMock()
    mock_ctx.send.return_value = mock_msg
    
    interface = DiscordInterface(mock_ctx)
    
    # Mock bot.wait_for
    mock_reaction = MagicMock()
    mock_reaction.emoji = "✅"
    
    with patch("aetox.interfaces.discord_bot.bot.wait_for", new_callable=AsyncMock) as mock_wait:
        mock_wait.return_value = (mock_reaction, MagicMock())
        result = await interface.request_approval("test_action", "details")
        assert result is True
        assert mock_msg.add_reaction.call_count == 2
