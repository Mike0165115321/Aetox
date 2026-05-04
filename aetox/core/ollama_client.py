import httpx
import json
import logging
from typing import Optional, Dict, Any, List

logger = logging.getLogger("aetox.core.ollama")

class OllamaClient:
    """
    Client for interacting with local Ollama REST API.
    """
    def __init__(self, host: str = "http://localhost:11434", timeout: int = 120):
        self.host = host.rstrip("/")
        self.timeout = timeout
        self.chat_url = f"{self.host}/api/chat"

    def chat(
        self, 
        model: str, 
        messages: List[Dict[str, str]], 
        format: Optional[str] = None,
        options: Optional[Dict[str, Any]] = None,
        keep_alive: str = "-1"
    ) -> Dict[str, Any]:
        """Sends a chat request to Ollama (Synchronous)."""
        payload = {
            "model": model, 
            "messages": messages, 
            "stream": False,
            "keep_alive": keep_alive
        }
        if format: payload["format"] = format
        if options: payload["options"] = options

        try:
            with httpx.Client(timeout=self.timeout) as client:
                response = client.post(self.chat_url, json=payload)
                response.raise_for_status()
                return response.json()
        except Exception as e:
            logger.error(f"Error calling Ollama API: {str(e)}")
            raise

    def chat_stream(
        self, 
        model: str, 
        messages: List[Dict[str, str]], 
        options: Optional[Dict[str, Any]] = None,
        keep_alive: str = "-1"
    ):
        """Sends a chat request to Ollama and yields response tokens (Streaming)."""
        payload = {
            "model": model, 
            "messages": messages, 
            "stream": True,
            "keep_alive": keep_alive
        }
        if options: payload["options"] = options

        try:
            with httpx.Client(timeout=self.timeout) as client:
                with client.stream("POST", self.chat_url, json=payload) as response:
                    response.raise_for_status()
                    for line in response.iter_lines():
                        if line:
                            chunk = json.loads(line)
                            if chunk.get("done"): break
                            yield chunk.get("message", {}).get("content", "")
        except Exception as e:
            logger.error(f"Error in Ollama chat stream: {str(e)}")
            raise

    def check_health(self) -> bool:
        """Checks if Ollama is accessible."""
        try:
            with httpx.Client(timeout=5) as client:
                response = client.get(f"{self.host}/api/tags")
                return response.status_code == 200
        except:
            return False
