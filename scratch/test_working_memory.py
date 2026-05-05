import pytest
import asyncio
from aetox.memory.working import WorkingMemory
from aetox.core.dispatcher import Dispatcher
from aetox.core.config_loader import config_loader

@pytest.mark.asyncio
async def test_memory_layer1_working():
    """ทดสอบการเพิ่มข้อมูลลง Working Memory (Layer 1)"""
    # ใช้ config มาตรฐานจาก config_loader
    config = config_loader.get_memory_config()
    mem = WorkingMemory(config)
    mem.goal = "ทดสอบการจำ"
    
    # เพิ่มข้อมูลจำลอง
    content = "นี่คือผลการทำงานจำลองที่ยาวมากๆ " * 20
    mem.add_to_working(
        content=content,
        source="test_tool",
        keywords=["test", "memory"]
    )
    
    # ตรวจสอบว่าข้อมูลถูกเก็บใน active_chunks
    assert len(mem.active_chunks) == 1
    assert mem.active_chunks[0].source == "test_tool"
    
    # ตรวจสอบ format_history
    history = mem.format_history()
    assert "test_tool" in history
    assert "🎯 เป้าหมาย" not in history # history ในเวอร์ชันใหม่เก็บแค่สรุป chunk

@pytest.mark.asyncio
async def test_dispatcher_retry_logic_mock():
    """ทดสอบว่า retry ทำงานเมื่อ critic ไม่ผ่าน (ใช้ Mock)"""
    config = config_loader.get_memory_config()
    memory = WorkingMemory(config)
    dispatcher = Dispatcher(memory)
    
    # Mock executor ให้ล้มเหลว 2 รอบ แล้วสำเร็จรอบที่ 3
    call_count = 0
    
    # จำลองการรัน action
    async def mock_run_action(*args, **kwargs):
        nonlocal call_count
        call_count += 1
        if call_count < 3:
            return {"status": "failure", "output": "error", "error": "test error"}
        return {"status": "success", "output": "done"}
    
    # จำลองการสกัด action
    async def mock_extract_action(*args, **kwargs):
        return {"tool": "test", "action": "test"}

    dispatcher.executor.run_action = mock_run_action
    dispatcher.executor.extract_action = mock_extract_action
    
    # ตั้งค่า Critic ให้ผ่านในรอบที่ 3 เท่านั้น
    async def mock_evaluate(step, result, context):
        if result["status"] == "success":
            return {"verdict": "pass", "score": 1.0}
        return {"verdict": "fail", "suggestion": "try again", "score": 0.0}
        
    dispatcher.critic.evaluate = mock_evaluate

    result = await dispatcher.run_plan({
        "plan_id": "test_plan",
        "goal": "ทดสอบระบบ Retry",
        "steps": [{"step_id": 1, "description": "ทดสอบขั้นตอนที่ 1"}]
    }, max_retries=3)
    
    assert result["status"] == "success"
    assert call_count == 3  # ต้องพยายามทั้งหมด 3 รอบถึงจะสำเร็จ