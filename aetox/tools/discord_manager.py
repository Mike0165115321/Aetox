import logging
import discord
from typing import Optional

class DiscordTool:
    """
    Tool for managing Discord servers (channels, categories, roles).
    """
    def __init__(self, bot: discord.Client):
        self.bot = bot
        self.logger = logging.getLogger("aetox.tools.discord")

    async def create_category(self, guild_id: int, name: str) -> str:
        guild = self.bot.get_guild(guild_id)
        if not guild:
            return f"Error: Guild {guild_id} not found."
        
        category = await guild.create_category(name)
        return f"Successfully created category: {category.name} (ID: {category.id})"

    async def create_channel(self, guild_id: int, name: str, category_id: Optional[int] = None) -> str:
        guild = self.bot.get_guild(guild_id)
        if not guild:
            return f"Error: Guild {guild_id} not found."
        
        category = guild.get_channel(category_id) if category_id else None
        channel = await guild.create_text_channel(name, category=category)
        return f"Successfully created channel: {channel.name} in {category.name if category else 'root'}"
