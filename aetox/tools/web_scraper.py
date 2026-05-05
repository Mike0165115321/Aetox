# aetox/tools/web_scraper.py
import logging
import asyncio
from typing import Dict, Any, List, Optional
from pathlib import Path
from urllib.parse import urlparse, urljoin
import httpx
from bs4 import BeautifulSoup
from aetox.tools.base import BaseTool

logger = logging.getLogger("aetox.tools.web_scraper")

class WebPulseScraper(BaseTool):
 """
 🌐 WebPulse: Intelligent Web Scraper for AetoxClaw
 ออกแบบมาเพื่อ "ดึงข้อมูลเว็บ → ประมวลผล → ส่งต่อให้ WorkingMemory"
 รองรับ: ดึงเนื้อหา, ค้นหาลิงก์, สรุปหน้าเว็บ, ดึงข้อมูลเฉพาะส่วน
 """
 
 def __init__(self, memory_ref=None):
  super().__init__(
   name="web_pulse_scraper",
   description="ดึงข้อมูลจากเว็บไซต์: อ่านเนื้อหา, ค้นหาลิงก์, สรุปหน้า, ดึงข้อมูลเฉพาะส่วน (CSS Selector/XPath)",
   actions=["fetch_content", "extract_links", "summarize_page", "extract_by_selector", "crawl_sitemap"]
  )
  self.memory = memory_ref  # Inject WorkingMemory reference (optional)
  self.timeout = 30
  self.headers = {
   "User-Agent": "Mozilla/5.0 (AetoxClaw/1.0; +https://aetox.dev) AppleWebKit/537.36"
  }

 def get_prompt_doc(self) -> str:
  return (
   f"Tool: {self.name}\n"
   f"หน้าที่: ดึงและประมวลผลข้อมูลจากเว็บไซต์สำหรับงานอัตโนมัติ\n"
   f"คำสั่งที่รองรับ:\n"
   f"1. fetch_content: ดึงเนื้อหาเต็มของหน้าเว็บ (params: url, max_length: int)\n"
   f"2. extract_links: ดึงลิงก์ทั้งหมดหรือกรองด้วย keyword (params: url, filter: str)\n"
   f"3. summarize_page: สรุปใจความสำคัญของหน้าเว็บ (params: url, focus: str)\n"
   f"4. extract_by_selector: ดึงข้อมูลเฉพาะส่วนด้วย CSS Selector (params: url, selector: str)\n"
   f"5. crawl_sitemap: ดึงลิงก์ทั้งหมดจาก sitemap.xml (params: base_url)\n\n"
   f"⚠️ หมายเหตุ: ข้อมูลที่ดึงมาจะส่งกลับเป็นข้อความสั้นๆ เพื่อประหยัดบริบทของโมเดลเล็ก\n"
   f"💡 ทริค: ใช้ร่วมกับ WorkingMemory.add_to_working() เพื่อเก็บข้อมูลสำคัญไว้เรียกใช้ภายหลัง\n"
   f"\nตัวอย่าง JSON:\n"
   f' {{"tool": "{self.name}", "action": "fetch_content", "params": {{"url": "https://example.com", "max_length": 2000}}, "confidence": 0.95}}\n'
   f' {{"tool": "{self.name}", "action": "extract_by_selector", "params": {{"url": "https://blog.com", "selector": "article.post > h2"}}, "confidence": 0.9}}\n'
  )

 async def _fetch_html(self, url: str) -> Optional[str]:
  """ดึง HTML จาก URL พร้อมจัดการ error"""
  try:
   async with httpx.AsyncClient(timeout=self.timeout, follow_redirects=True) as client:
    response = await client.get(url, headers=self.headers)
    response.raise_for_status()
    # Detect encoding
    response.encoding = response.apparent_encoding
    return response.text
  except httpx.RequestError as e:
   logger.error(f"Request failed for {url}: {e}")
   return None
  except Exception as e:
   logger.error(f"Unexpected error fetching {url}: {e}")
   return None

 def _clean_text(self, html: str, max_length: int = None) -> str:
  """แปลง HTML → ข้อความสะอาด พร้อมตัดความยาว"""
  soup = BeautifulSoup(html, 'html.parser')
  
  # ลบส่วนที่ไม่จำเป็น
  for tag in soup(['script', 'style', 'nav', 'footer', 'header']):
   tag.decompose()
  
  # ดึงข้อความและจัดรูปแบบ
  text = soup.get_text(separator=' ', strip=True)
  # ลบช่องว่างซ้ำ
  text = ' '.join(text.split())
  
  if max_length and len(text) > max_length:
   text = text[:max_length] + "...[truncated]"
  
  return text

 def _extract_links(self, soup: BeautifulSoup, base_url: str, filter_keyword: str = None) -> List[Dict]:
  """ดึงลิงก์จากหน้าเว็บ พร้อมแปลงเป็น absolute URL"""
  links = []
  for tag in soup.find_all('a', href=True):
   href = tag['href']
   absolute = urljoin(base_url, href)
   text = tag.get_text(strip=True)[:100]
   
   # กรองตาม keyword ถ้ามี
   if filter_keyword and filter_keyword.lower() not in text.lower() and filter_keyword.lower() not in absolute.lower():
    continue
   
   # กรองลิงก์ภายนอก/ภายใน (ถ้าต้องการ)
   if urlparse(absolute).netloc != urlparse(base_url).netloc:
    continue  # ข้ามลิงก์ไปเว็บอื่น (ปรับได้ตามต้องการ)
   
   links.append({"url": absolute, "text": text})
  
  return links[:20]  # จำกัดจำนวนเพื่อประหยัดบริบท

 def execute(self, params: Dict[str, Any]) -> Dict[str, Any]:
  """Main entry point — ใช้ asyncio.run สำหรับ sync call"""
  action = params.get("action")
  url = params.get("url")
  
  if not action or not url:
   return {"status": "failure", "error": "Missing 'action' or 'url' parameter"}
  
  try:
   # รัน async function ใน sync context
   if action == "fetch_content":
    max_len = params.get("max_length", 3000)
    return asyncio.run(self._fetch_content(url, max_len))
   elif action == "extract_links":
    filter_kw = params.get("filter")
    return asyncio.run(self._extract_links_action(url, filter_kw))
   elif action == "summarize_page":
    focus = params.get("focus", "ใจความสำคัญ")
    return asyncio.run(self._summarize_page(url, focus))
   elif action == "extract_by_selector":
    selector = params.get("selector")
    if not selector:
     return {"status": "failure", "error": "Missing 'selector' parameter"}
    return asyncio.run(self._extract_by_selector(url, selector))
   elif action == "crawl_sitemap":
    return asyncio.run(self._crawl_sitemap(url))
   else:
    return {"status": "failure", "error": f"Unknown action: {action}"}
  except Exception as e:
   logger.error(f"Execution error in {action}: {e}")
   return {"status": "failure", "error": f"Internal error: {str(e)}"}

 async def _fetch_content(self, url: str, max_length: int) -> Dict[str, Any]:
  html = await self._fetch_html(url)
  if not html:
   return {"status": "failure", "error": f"Failed to fetch {url}"}
  
  text = self._clean_text(html, max_length)
  
  # ถ้ามี memory → เก็บข้อมูลสำคัญลง WorkingMemory อัตโนมัติ
  if self.memory and len(text) > 500:
   self.memory.add_to_working(
    content=text[:1000],  # เก็บส่วนหัวไว้
    source=f"web:{urlparse(url).netloc}",
    keywords=[urlparse(url).netloc],
    metadata={"full_url": url, "type": "web_content"}
   )
  
  return {
   "status": "success",
   "output": text,
   "meta": {
    "url": url,
    "length": len(text),
    "truncated": len(text) >= max_length
   }
  }

 async def _extract_links_action(self, url: str, filter_keyword: str = None) -> Dict[str, Any]:
  html = await self._fetch_html(url)
  if not html:
   return {"status": "failure", "error": f"Failed to fetch {url}"}
  
  soup = BeautifulSoup(html, 'html.parser')
  links = self._extract_links(soup, url, filter_keyword)
  
  return {
   "status": "success",
   "output": "\n".join([f"- [{l['text']}]({l['url']})" for l in links]),
   "meta": {"count": len(links), "base_url": url}
  }

 async def _summarize_page(self, url: str, focus: str) -> Dict[str, Any]:
  """ดึงเนื้อหา + สร้างสรุปสั้นๆ (ให้โมเดลหลักทำสรุปละเอียดต่อ)"""
  html = await self._fetch_html(url)
  if not html:
   return {"status": "failure", "error": f"Failed to fetch {url}"}
  
  text = self._clean_text(html, max_length=2000)
  
  # สรุปอย่างง่าย: เอาประโยคแรก + กลาง + ท้าย + เน้นคำที่เกี่ยวกับ focus
  sentences = [s.strip() for s in text.split('.') if len(s.strip()) > 20]
  if len(sentences) >= 3:
   summary = f"หัวข้อ: {focus}\n" + ". ".join([sentences[0], sentences[len(sentences)//2], sentences[-1]]) + "."
  else:
   summary = text[:500] + "..."
  
  return {
   "status": "success",
   "output": summary,
   "meta": {"url": url, "focus": focus, "method": "extractive_summary"}
  }

 async def _extract_by_selector(self, url: str, selector: str) -> Dict[str, Any]:
  html = await self._fetch_html(url)
  if not html:
   return {"status": "failure", "error": f"Failed to fetch {url}"}
  
  soup = BeautifulSoup(html, 'html.parser')
  elements = soup.select(selector)
  
  if not elements:
   return {"status": "failure", "error": f"No elements found for selector: {selector}"}
  
  results = []
  for el in elements[:10]:  # จำกัดจำนวน
   text = el.get_text(strip=True)
   if text:
    results.append(text[:300])  # ตัดสั้น
  
  return {
   "status": "success",
   "output": "\n---\n".join(results),
   "meta": {"selector": selector, "count": len(results), "url": url}
  }

 async def _crawl_sitemap(self, base_url: str) -> Dict[str, Any]:
  """ดึงลิงก์จาก sitemap.xml"""
  sitemap_url = urljoin(base_url, "/sitemap.xml")
  html = await self._fetch_html(sitemap_url)
  
  if not html:
   # ลองแบบ /sitemap_index.xml
   sitemap_url = urljoin(base_url, "/sitemap_index.xml")
   html = await self._fetch_html(sitemap_url)
   if not html:
    return {"status": "failure", "error": "Could not find sitemap.xml"}
  
  soup = BeautifulSoup(html, 'lxml-xml')  # ใช้ lxml-xml สำหรับ XML
  urls = [loc.text for loc in soup.find_all('loc')[:50]]  # จำกัด 50 ลิงก์
  
  return {
   "status": "success",
   "output": "\n".join(urls),
   "meta": {"sitemap": sitemap_url, "count": len(urls)}
  }