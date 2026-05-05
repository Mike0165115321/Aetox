# core/memory/embedder.py
import logging
import numpy as np
from typing import List, Union, Optional
from sentence_transformers import SentenceTransformer

logger = logging.getLogger("aetox.memory.embedder")

class BGE3Embedder:
 """
 🧠 BGE-M3 Embedder สำหรับ AetoxClaw
 รองรับ: Dense Embedding, Sparse Embedding (optional), Multi-lingual
 """
 
 def __init__(self, model_path: str = "BAAI/bge-m3", device: str = "cpu"):
  """
  Args:
   model_path: Path ไปยังโมเดล (โหลดจาก HF หรือ local path)
   device: "cpu" หรือ "cuda" — แนะนำ "cpu" เพื่อประหยัด VRAM
  """
  logger.info(f"Loading BGE-M3 embedder on {device}...")
  
  self.model = SentenceTransformer(
   model_path,
   device=device,
   trust_remote_code=True  # BGE-M3 ต้องการ flag นี้
  )
  
  # BGE-M3 default dimension
  self.dimension = 1024
  self.device = device
  
  logger.info(f"✓ BGE-M3 loaded | dim={self.dimension} | device={self.device}")
 
 def encode(
  self, 
  texts: Union[str, List[str]], 
  normalize: bool = True,
  batch_size: int = 8
 ) -> Union[List[float], List[List[float]]]:
  """
  แปลงข้อความเป็นเวกเตอร์
  """
  if isinstance(texts, str):
   texts = [texts]
   single = True
  else:
   single = False
  
  embeddings = self.model.encode(
   texts,
   normalize_embeddings=normalize,
   batch_size=batch_size,
   show_progress_bar=False
  )
  
  # แปลงเป็น list ธรรมดา (สำหรับเก็บลง JSON/ChromaDB)
  if isinstance(embeddings, np.ndarray):
   embeddings = embeddings.tolist()
  
  return embeddings[0] if single else embeddings
 
 def encode_multi_vector(
  self,
  texts: Union[str, List[str]],
  return_sparse: bool = False
 ) -> dict:
  """
  [Advanced] ใช้ความสามารถเต็มของ BGE-M3
  - dense: เวกเตอร์ปกติ
  - sparse: คำสำคัญแบบ weighted (คล้าย BM25)
  - colbert: multi-vector สำหรับความแม่นยำสูง
  """
  if isinstance(texts, str):
   texts = [texts]
  
  # BGE-M3 รองรับ multi-embedding types
  result = self.model.encode(
   texts,
   return_dense=True,
   return_sparse=return_sparse,
   return_colbert_vecs=False,  # เปิดถ้าต้องการความแม่นยำสูงแต่ช้ากว่า
   normalize_embeddings=True
  )
  
  return {
   "dense": result["dense_embeddings"].tolist(),
   "sparse": result["lexical_weights"] if return_sparse else None,
   # "colbert": result["colbert_vecs"] if return_colbert else None
  }
 
 def similarity(self, text1: str, text2: str) -> float:
  """คำนวณความคล้ายคลึงแบบเร็ว (ใช้สำหรับตรวจสอบเบื้องต้น)"""
  emb1 = self.encode(text1)
  emb2 = self.encode(text2)
  # Cosine similarity
  return float(np.dot(emb1, emb2) / (np.linalg.norm(emb1) * np.linalg.norm(emb2)))