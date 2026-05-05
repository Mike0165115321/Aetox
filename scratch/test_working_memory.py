import asyncio
from aetox.memory.working import WorkingMemory

async def test_memory_logic():
    """ทดสอบ Logic ของ WorkingMemory (Async, Locks, Truncation)"""
    print("--- [TEST] Starting WorkingMemory Unit Test ---")
    
    # ใช้ config "none" เพื่อข้ามการโหลดโมเดลหนัก
    config = {"episodic_path": "data/test_working.json", "embedder": {"model": "none"}}
    mem = WorkingMemory(config)
    task_id = "test_123"
    
    # 1. ทดสอบ Partial Update (Update เฉพาะจุด ข้อมูลเก่าไม่หาย)
    print("[1/3] Testing Update Context...")
    await mem.set_active_context(task_id, {"key1": "old_val", "status": "init"})
    await mem.update_context(task_id, {"key2": "new_val", "status": "updated"})
    
    ctx = await mem.get_active_context(task_id)
    assert ctx["key1"] == "old_val"
    assert ctx["key2"] == "new_val"
    assert ctx["status"] == "updated"
    print("   Update Context passed")
    
    # 2. ทดสอบ Truncation (ตัดคำอัตโนมัติถ้าผลลัพธ์ยาวเกิน)
    print("[2/3] Testing Result Truncation...")
    long_text = "Data " * 500 # > 2000 chars
    mem.add_step_result(1, long_text)
    
    latest_content = mem.active_chunks[0].content
    assert len(latest_content) <= 1050
    assert "... [Output Truncated]" in latest_content
    print("   Truncation passed")
    
    # 3. ทดสอบ Async Safety (Copy Check)
    print("[3/3] Testing Async Safety (Copy Check)...")
    data = {"meta": {"ver": 1}}
    await mem.set_active_context("safety", data)
    
    ctx_copy = await mem.get_active_context("safety")
    ctx_copy["meta"]["ver"] = 999 # แก้ไขสำเนา
    
    ctx_real = await mem.get_active_context("safety")
    assert ctx_real["meta"]["ver"] == 1 # ข้อมูลจริงต้องไม่เปลี่ยน
    print("   Async Safety passed")
    
    print("--- [TEST] MEMORY LOGIC NOMINAL ---")

if __name__ == "__main__":
    asyncio.run(test_memory_logic())
