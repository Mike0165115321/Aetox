# core/memory/vector_store.py (อัปเดต)
# import chromadb
# from chromadb.utils.embedding_functions import SentenceTransformerEmbeddingFunction
from .embedder import BGE3Embedder

class VectorMemory:
 def __init__(
  self, 
  path: str = "data/vector_db",
  embedder: BGE3Embedder = None,
  collection_name: str = "aetox_memory"
 ):
  self.path = path
  self.collection_name = collection_name
  
  # ใช้ embedder ที่ส่งมา หรือสร้างใหม่ (แนะนำ: device="cpu")
  self.embedder = embedder or BGE3Embedder(device="cpu")
  
  # สร้าง embedding function สำหรับ ChromaDB
  from chromadb import EmbeddingFunction
  
  class BGE3ChromaEF(EmbeddingFunction):
   def __init__(self, embedder: BGE3Embedder):
    self.embedder = embedder
   def __call__(self, input: list[str]) -> list[list[float]]:
    return self.embedder.encode(input, batch_size=16)
  
  chroma_ef = BGE3ChromaEF(self.embedder)
  
  # init ChromaDB client
  import chromadb
  self.client = chromadb.PersistentClient(path=path)
  self.collection = self.client.get_or_create_collection(
   name=collection_name,
   embedding_function=chroma_ef,
   metadata={"hnsw:space": "cosine", "dimension": self.embedder.dimension}
  )
  
  logger.info(f"✓ VectorMemory initialized | collection: {collection_name}")
 
 def add(self, docs: list[str], ids: list[str], metadata: list[dict] = None):
  """เพิ่มข้อมูลลง vector store"""
  if metadata is None:
   metadata = [{} for _ in docs]
  
  self.collection.add(
   documents=docs,
   ids=ids,
   metadatas=metadata
  )
 
 def query(self, text: str, n_results: int = 5, filter: dict = None) -> dict:
  """ค้นหาข้อมูลที่เกี่ยวข้อง"""
  return self.collection.query(
   query_texts=[text],
   n_results=n_results,
   where=filter  # กรองด้วย metadata เช่น {"source": "web"}
  )
 
 def delete_by_id(self, ids: list[str]):
  """ลบข้อมูลตาม ID"""
  self.collection.delete(ids=ids)