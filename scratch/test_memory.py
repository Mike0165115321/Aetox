import sys
import os
import json

# Add project root to path
sys.path.append(os.getcwd())

# Ensure UTF-8 output for Windows terminal
if sys.platform == "win32":
    import io
    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8')

from aetox.memory.working import WorkingMemory

def test_enhanced_memory():
    print("--- Testing Enhanced WorkingMemory ---")
    
    # 1. Initialization
    mem = WorkingMemory(goal="จัดระเบียบไฟล์ในโฟลเดอร์ Downloads")
    print(f"Task ID: {mem.task_id}")
    print(f"Goal: {mem.goal}")
    
    # 2. Adding Steps
    mem.add_step_result(1, "พบไฟล์ทั้งหมด 5 ไฟล์", status="success")
    mem.add_step_result(2, "ลบไฟล์ temp.txt สำเร็จ", status="success")
    mem.add_step_result(3, "ไม่สามารถย้ายไฟล์ document.pdf", status="failed", error="Permission Denied")
    
    # 3. Artifacts and Context
    mem.add_artifact("target_folder", "C:/Users/Test/Downloads")
    mem.update_context({"last_scanned_file": "document.pdf"})
    
    # 4. Format History (Check if it's 'smart' and readable)
    history = mem.format_history()
    print("\nFormatted History:")
    print(history)
    
    # 5. Serialization Test
    data = mem.to_dict()
    print("\nSerialized Data Keys:", list(data.keys()))
    
    # 6. Deserialization Test
    new_mem = WorkingMemory.from_dict(data)
    print(f"\nRestored Goal: {new_mem.goal}")
    print(f"Restored Steps Count: {len(new_mem.step_results)}")
    print(f"Restored Context: {new_mem.context}")
    
    assert new_mem.task_id == mem.task_id
    assert len(new_mem.step_results) == 3
    assert new_mem.artifacts["target_folder"] == "C:/Users/Test/Downloads"
    
    print("\n✅ Verification Successful: Data is intact and system is 'smarter'.")

if __name__ == "__main__":
    test_enhanced_memory()
