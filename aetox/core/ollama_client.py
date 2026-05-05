import httpx
import json
import logging
from typing import Optional, Dict, Any, List

logger = logging.getLogger("aetox.core.ollama")

class OllamaClient:
    """
    Asynchronous Client for interacting with local Ollama REST API.
    Optimized for speed and non-blocking performance.
    """
    def __init__(self, host: str = "http://localhost:11434", timeout: int = 120):
        self.host = host.rstrip("/")
        self.timeout = timeout
        self.chat_url = f"{self.host}/api/chat"

    async def chat(
        self, 
        model: str, 
        messages: List[Dict[str, str]], 
        format: Optional[str] = None,
        options: Optional[Dict[str, Any]] = None,
        keep_alive: int = -1
    ) -> Dict[str, Any]:
        """Sends an asynchronous chat request to Ollama."""
        payload = {
            "model": model, 
            "messages": messages, 
            "stream": False,
            "keep_alive": keep_alive
        }
        if format: payload["format"] = format
        if options: payload["options"] = options

        logger.debug(f"[OLLAMA] Calling {model} | options: {options}")

        try:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                response = await client.post(self.chat_url, json=payload)
                response.raise_for_status()
                return response.json()
        except Exception as e:
            logger.error(f"Async Ollama Error: {str(e)}")
            raise

    async def chat_stream(
        self, 
        model: str, 
        messages: List[Dict[str, str]], 
        options: Optional[Dict[str, Any]] = None,
        keep_alive: int = -1
    ):
        """Sends an asynchronous chat request to Ollama and yields tokens (Streaming)."""
        payload = {
            "model": model, 
            "messages": messages, 
            "stream": True,
            "keep_alive": keep_alive
        }
        if options: payload["options"] = options

        logger.debug(f"[OLLAMA-STREAM] Calling {model} | options: {options}")

        try:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                async with client.stream("POST", self.chat_url, json=payload) as response:
                    response.raise_for_status()
                    async for line in response.aiter_lines():
                        if line:
                            chunk = json.loads(line)
                            if chunk.get("done"): break
                            yield chunk.get("message", {}).get("content", "")
        except Exception as e:
            logger.error(f"Async Ollama Stream Error: {str(e)}")
            raise

    async def check_health(self) -> bool:
        """Checks if Ollama is accessible asynchronously."""
        try:
            async with httpx.AsyncClient(timeout=5) as client:
                response = await client.get(f"{self.host}/api/tags")
                return response.status_code == 200
        except:
            return False
