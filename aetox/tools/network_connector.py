import httpx
import logging
from typing import Dict, Any, Optional

class NetworkConnectorTool:
    """
    Safely perform HTTP requests with domain restrictions.
    """
    def __init__(self, whitelist: Optional[list] = None):
        # Default whitelist for safety
        self.whitelist = whitelist or [
            "google.com", 
            "github.com", 
            "api.coingecko.com", # Example: Crypto Prices
            "api.exchangerate-api.com", # Example: FX Rates
            "wikipedia.org",
            "stackoverflow.com"
        ]
        self.logger = logging.getLogger("aetox.tools.network")

    def _is_allowed(self, url: str) -> bool:
        """Checks if the domain is in the whitelist."""
        for domain in self.whitelist:
            if domain in url:
                return True
        return False

    async def get_data(self, url: str, params: Optional[Dict] = None) -> str:
        """Performs an async GET request."""
        if not self._is_allowed(url):
            return f"Error: Domain of '{url}' is not in the whitelist. Access restricted."
        
        try:
            async with httpx.AsyncClient(timeout=10.0) as client:
                response = await client.get(url, params=params)
                response.raise_for_status()
                
                # If content is JSON, format it nicely, otherwise return text snippet
                if "application/json" in response.headers.get("Content-Type", ""):
                    import json
                    return json.dumps(response.json(), indent=2, ensure_ascii=False)[:2000]
                return response.text[:2000] # Limit to 2000 chars for LLM
                
        except Exception as e:
            self.logger.error(f"Network Error: {e}")
            return f"Error fetching data: {str(e)}"

    def update_whitelist(self, domain: str):
        """Allows adding new domains dynamically if needed."""
        if domain not in self.whitelist:
            self.whitelist.append(domain)
            self.logger.info(f"Added {domain} to network whitelist.")
