# เพิ่มไฟล์ใหม่: aetox/memory/episodic.py
import json
from pathlib import Path
from typing import List, Dict, Optional
from datetime import datetime

class EpisodicMemory:
 """
 เก็บ "บทเรียน" จากงานเก่า เพื่อให้ระบบเรียนรู้จากประสบการณ์
 เช่น: "เว็บ X มักมีโครงสร้างแบบ Y", "คำสั่ง Z มักล้มเหลวถ้าไม่ทำ A ก่อน"
 """
 
 def __init__(self, storage_path: str = "data/episodes.jsonl"):
  self.path = Path(storage_path)
  self.path.parent.mkdir(parents=True, exist_ok=True)
  self.episodes: List[Dict] = []
  self._load()
 
 def _load(self):
  if self.path.exists():
   with open(self.path, 'r', encoding='utf-8') as f:
    for line in f:
     if line.strip():
      self.episodes.append(json.loads(line))
 
 def save_lesson(self, task_goal: str, outcome: str, 
                 key_learnings: List[str], success: bool):
  """บันทึกบทเรียนจากงานหนึ่งชิ้น"""
  episode = {
   "timestamp": datetime.now().isoformat(),
   "goal": task_goal[:200],
   "outcome": "success" if success else "failed",
   "summary": outcome[:300],
   "learnings": key_learnings,
   "tags": self._extract_tags(task_goal + " " + " ".join(key_learnings))
  }
  with open(self.path, 'a', encoding='utf-8') as f:
   f.write(json.dumps(episode, ensure_ascii=False) + '\n')
  self.episodes.append(episode)
 
 def search_lessons(self, query: str, limit: int = 3) -> List[Dict]:
  """ค้นหาคำแนะนำจากงานเก่าที่เกี่ยวข้อง"""
  query_lower = query.lower()
  results = []
  for ep in reversed(self.episodes[-50:]):  # ค้นจาก 50 งานล่าสุด
   if (query_lower in ep['goal'].lower() or 
       query_lower in ep['summary'].lower() or
       any(query_lower in tag.lower() for tag in ep.get('tags', []))):
    results.append(ep)
    if len(results) >= limit:
     break
  return results
 
 def _extract_tags(self, text: str) -> List[str]:
  """สกัดคำสำคัญอย่างง่าย"""
  import re
  words = re.findall(r'\b[a-zA-Zก-๋]{4,}\b', text.lower())
  from collections import Counter
  return [w for w, _ in Counter(words).most_common(5)]
 
 def get_advice_for_task(self, goal: str) -> str:
  """ให้คำแนะนำจากประสบการณ์เก่า"""
  lessons = self.search_lessons(goal)
  if not lessons:
   return ""
  
  advice = "\n💡 คำแนะนำจากประสบการณ์เก่า:"
  for i, ep in enumerate(lessons, 1):
   status = "✅" if ep['outcome'] == 'success' else "⚠️"
   advice += f"\n{i}. {status} {ep['summary']}"
   if ep['learnings']:
    advice += f" → เรียนรู้: {', '.join(ep['learnings'][:2])}"
  return advice