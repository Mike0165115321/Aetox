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
        options: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """
        Sends a chat request to Ollama.
        
        Args:
            model: Name of the model to use.
            messages: List of message dicts (role, content).
            format: Set to "json" to enforce JSON output.
            options: Additional model parameters (temperature, etc.).
        """
        payload = {
            "model": model,
            "messages": messages,
            "stream": False
        }
        
        if format:
            payload["format"] = format
            
        if options:
            payload["options"] = options

        try:
            with httpx.Client(timeout=self.timeout) as client:
                response = client.post(self.chat_url, json=payload)
                response.raise_for_status()
                result = response.json()
                
                # Check if it's JSON format and try to parse it early if needed
                if format == "json":
                    content = result.get("message", {}).get("content", "")
                    try:
                        # We just verify it's valid JSON here
                        json.loads(content)
                    except json.JSONDecodeError:
                        logger.error(f"Model {model} returned invalid JSON even with format='json'")
                
                return result
        except httpx.ConnectError:
            logger.error(f"Could not connect to Ollama at {self.host}. Is it running?")
            raise ConnectionError(f"Ollama connection failed at {self.host}")
        except Exception as e:
            logger.error(f"Error calling Ollama API: {str(e)}")
            raise

    def check_health(self) -> bool:
        """Checks if Ollama is accessible."""
        try:
            with httpx.Client(timeout=5) as client:
                response = client.get(f"{self.host}/api/tags")
                return response.status_code == 200
        except:
            return False
