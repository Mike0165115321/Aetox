graph TD
    A[ผู้ใช้: ย้ายบทความจาก blog.example.com ไป mysite.com] --> B[Intent Extractor]
    B --> C{วิเคราะห์: งานซับซ้อน?}
    C -->|ใช่ | D[สร้างแผน 3 ขั้น:<br/>1. ดึงลิงก์จาก sitemap<br/>2. ดึงเนื้อหาแต่ละลิงก์ → สรุป → เก็บ Memory<br/>3. แปลงรูปแบบ → โพสต์ API]
    C -->|ไม่ | E[รันขั้นเดียวโดยตรง]
    
    D --> F[Dispatcher.run_plan\(\)]
    F --> G[ขั้นที่ 1: WebPulse.crawl_sitemap\(\)]
    G --> H[ได้ 50 ลิงก์ → เก็บลง WorkingMemory]
    
    H --> I[ขั้นที่ 2: ลูป 50 รอบ]
    I --> J[WebPulse.fetch_content\(\) + _clean_text\(\)]
    J --> K[WorkingMemory.add_to_working\(\)<br/>• เก็บ summary<br/>• เก็บ keywords<br/>• เก็บ metadata]
    K --> L[VectorStore.add\(\) ด้วย BGE-M3 embedding]
    
    L --> M[ขั้นที่ 3: สำหรับแต่ละบทความ]
    M --> N[VectorStore.query\(\) ด้วยคำสำคัญ]
    N --> O[ดึงเฉพาะเนื้อหาที่เกี่ยวข้อง → ส่งให้ LLM เขียนโค้ดแปลงรูปแบบ]
    O --> P[เรียก mysite.com API → โพสต์]
    
    P --> Q[Critic.evaluate\(\): ตรวจสอบว่าโพสต์สำเร็จ?]
    Q -->|ผ่าน | R[บันทึก episodic memory: "เรียนรู้วิธีแปลงรูปแบบ X"]
    Q -->|ไม่ผ่าน | S[ส่ง feedback → รีทรายขั้นนี้]
    
    R --> T[บันทึก WorkingMemory ลง disk → จบงาน]
    S --> I