# aetox/memory/working.py
import json
import hashlib
from datetime import datetime
from typing import List, Dict, Optional, Any, Union
from dataclasses import dataclass, field, asdict
import logging

from .vector_store import VectorMemory
from .embedder import BGE3Embedder

logger = logging.getLogger("aetox.memory.working")

@dataclass
class MemoryChunk:
    """ชิ้นข้อมูลหนึ่งหน่วยในระบบความจำ"""
    id: str
    content: str
    summary: str
    keywords: List[str]
    source: str
    timestamp: float
    relevance_score: float = 1.0
    metadata: Dict = field(default_factory=dict)
    
    def to_dict(self) -> Dict:
        return asdict(self)

class WorkingMemory:
    """ระบบความจำแบบไฮบริด 3 ชั้น สำหรับ AetoxOS (Updated with BGE-M3 RAG)"""
    
    def __init__(self, config: Dict):
        self.config = config
        self.goal = config.get("goal", "No goal set")
        self.max_context_tokens = config.get("max_context_tokens", 4096)
        self.chunk_size = config.get("chunk_size", 512)
        self.summary_ratio = config.get("summary_ratio", 0.1)
        
        # Layer 1: RAM
        self.active_chunks: List[MemoryChunk] = []
        self.task_context: Dict = {}
        self.artifacts: Dict[str, Any] = {}
        
        # Layer 2: Episodic Path
        self.episodic_path = config.get("episodic_path", "data/episodes.jsonl")
        
        # Layer 3: Advanced RAG (BGE-M3 + VectorStore)
        try:
            import os
            embedder_cfg = config.get("embedder", {})
            # ลำดับความสำคัญ: 1. .env (Local) -> 2. config (YAML) -> 3. Default (BAAI/bge-m3)
            model_path = os.getenv("EMBEDDER_MODEL_PATH") or embedder_cfg.get("model", "BAAI/bge-m3")
            
            self.embedder = BGE3Embedder(
                model_path=model_path,
                device=embedder_cfg.get("device", "cpu")
            )
            self.vector_store = VectorMemory(
                path=config.get("vector_db_path", "data/vector_db"),
                embedder=self.embedder
            )
        except Exception as e:
            logger.error(f"Failed to initialize BGE-M3 RAG: {e}")
            self.vector_store = None

    # ========== Layer 1 Actions ==========
    def set_active_context(self, task_id: str, context: Dict):
        self.task_context[task_id] = {
            "state": context,
            "updated_at": datetime.now().timestamp()
        }
        if "instruction" in context:
            self.goal = context["instruction"]

    def get_active_context(self, task_id: str) -> Optional[Dict]:
        ctx = self.task_context.get(task_id)
        return ctx["state"] if ctx else None

    def add_artifact(self, name: str, value: Any):
        self.artifacts[name] = value

    def add_to_working(self, content: str, source: str, keywords: List[str] = None, metadata: Dict = None) -> MemoryChunk:
        chunk_id = hashlib.md5(f"{content[:100]}_{datetime.now().timestamp()}".encode()).hexdigest()[:12]
        summary = self._auto_summarize(content, int(self.chunk_size * self.summary_ratio))
        
        chunk = MemoryChunk(
            id=chunk_id,
            content=content,
            summary=summary,
            keywords=keywords or self._extract_keywords(summary),
            source=source,
            timestamp=datetime.now().timestamp(),
            metadata=metadata or {}
        )
        self.active_chunks.append(chunk)
        self._trim_working_memory()
        return chunk

    def _trim_working_memory(self):
        self.active_chunks.sort(key=lambda c: (c.relevance_score, c.timestamp), reverse=True)
        self.active_chunks = self.active_chunks[:15]

    # ========== Layer 3 Actions (BGE-M3 Updated) ==========
    def store_long_term(self, content: str, metadata: Dict = None):
        """เก็บข้อมูลลง long-term memory ด้วย BGE-M3"""
        if not self.vector_store: return
        
        chunks = self._split_content(content)
        for i, chunk in enumerate(chunks):
            doc_id = f"{metadata.get('source', 'unknown') if metadata else 'unknown'}_{datetime.now().timestamp()}_{i}"
            
            # เก็บลง vector store (BGE3Embedder จะถูกเรียกใช้ภายใน)
            self.vector_store.add(
                docs=[chunk],
                ids=[doc_id],
                metadata=[{**(metadata or {}), "chunk_index": i, "content_preview": chunk[:100]}]
            )

    def retrieve_relevant(self, query: str, limit: int = 5) -> List[Dict]:
        """ค้นหาข้อมูลที่เกี่ยวข้องจาก BGE-M3 Vector Store"""
        if not self.vector_store: return []
        
        results = self.vector_store.query(query, n_results=limit)
        
        return [
            {
                "content": doc,
                "metadata": meta,
                "distance": dist,
                "id": id_
            }
            for doc, meta, dist, id_ in zip(
                results["documents"][0],
                results["metadatas"][0],
                results["distances"][0],
                results["ids"][0]
            )
        ]

    def get_full_context(self) -> Dict[str, Any]:
        """คืนค่าบริบททั้งหมดในรูปแบบ Dictionary สำหรับ Dispatcher"""
        return {
            "goal": self.goal,
            "active_chunks": [c.to_dict() for c in self.active_chunks],
            "artifacts": self.artifacts,
            "task_context": self.task_context
        }

    def add_step_result(self, step_id: Union[int, str], result: Any, status: str = "success", error: str = None):
        """Bridge method สำหรับความเข้ากันได้กับ Dispatcher เวอร์ชั่นเก่า"""
        self.add_to_working(
            content=str(result) if result else "",
            source=f"step_{step_id}",
            metadata={"status": status, "error": error, "step_id": step_id}
        )

    def save_to_disk(self):
        """บันทึกสถานะความจำปัจจุบันลงไฟล์ JSON"""
        import os
        os.makedirs(os.path.dirname(self.episodic_path), exist_ok=True)
        snapshot_path = os.path.join(os.path.dirname(self.episodic_path), "working_snapshot.json")
        
        data = {
            "goal": self.goal,
            "active_chunks": [c.to_dict() for c in self.active_chunks],
            "artifacts": self.artifacts,
            "timestamp": datetime.now().timestamp()
        }
        
        try:
            with open(snapshot_path, "w", encoding="utf-8") as f:
                json.dump(data, f, ensure_ascii=False, indent=2)
            # logger.debug(f"Memory snapshot saved to {snapshot_path}")
        except Exception as e:
            logger.error(f"Failed to save memory snapshot: {e}")

    # ========== Helpers ==========
    def _auto_summarize(self, content: str, max_length: int) -> str:
        if len(content) <= max_length: return content
        sentences = content.split(". ")
        if len(sentences) <= 3: return ". ".join(sentences[:2]) + "."
        return ". ".join([sentences[0], sentences[len(sentences)//2], sentences[-1]]) + "."

    def _extract_keywords(self, text: str) -> List[str]:
        import re
        words = re.findall(r'\b[a-zA-Zก-๋]{3,}\b', text.lower())
        from collections import Counter
        return [w for w, _ in Counter(words).most_common(5)]

    def _split_content(self, content: str) -> List[str]:
        paragraphs = content.split("\n\n")
        chunks = []
        current = ""
        for para in paragraphs:
            if len(current) + len(para) > self.chunk_size * 4:
                chunks.append(current.strip())
                current = para
            else:
                current += "\n\n" + para
        if current.strip(): chunks.append(current.strip())
        return chunks

    def format_history(self) -> str:
        if not self.active_chunks: return "ยังไม่มีประวัติการทำงานในเซสชันนี้"
        lines = ["### ประวัติการทำงานล่าสุด:"]
        for chunk in reversed(self.active_chunks):
            lines.append(f"- [{chunk.source}] {chunk.summary}")
        return "\n".join(lines)

class MemoryContextBuilder:
    """ช่วยสร้าง prompt context ที่ 'พอดี' สำหรับโมเดลเล็ก"""
    
    @staticmethod
    def build_for_task(memory: WorkingMemory, task_type: str, query: str, max_tokens: int = 4000) -> str:
        parts = [f"🎯 เป้าหมายหลัก: {memory.goal}"]
        
        # 1. ข้อมูลจาก Advanced RAG (BGE-M3)
        relevant = memory.retrieve_relevant(query)
        if relevant:
            parts.append("\n📚 ข้อมูลที่เกี่ยวข้องจากความจำระยะยาว (RAG):")
            for i, item in enumerate(relevant, 1):
                parts.append(f"{i}. {item['content'][:300]}...")

        # 2. ข้อมูลตามประเภทงาน
        if task_type == "planning":
            if memory.active_chunks:
                parts.append(f"\n💡 สถานะล่าสุด: {memory.active_chunks[-1].summary}")
        
        elif task_type == "execution":
            if memory.artifacts:
                parts.append(f"\n📦 Artifacts ที่มี: {', '.join(memory.artifacts.keys())}")
            if memory.active_chunks:
                parts.append(f"\n✅ ผลการทำงานล่าสุด: {memory.active_chunks[-1].content[:500]}")

        # 3. ประวัติย่อ
        parts.append("\n📜 " + memory.format_history())

        if query:
            parts.append(f"\n❓ คำถาม/คำสั่งปัจจุบัน: {query}")

        full = "\n".join(parts)
        if len(full) > max_tokens * 4:
            full = full[:max_tokens * 4] + "\n...[บริบทถูกตัด]"
        return full