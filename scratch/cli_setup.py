import os
import discord
import asyncio
from dotenv import load_dotenv
from aetox.tools.discord_manager import DiscordTool

load_dotenv()
TOKEN = os.getenv("DISCORD_TOKEN")

async def run_setup():
    intents = discord.Intents.default()
    bot = discord.Client(intents=intents)

    @bot.event
    async def on_ready():
        print(f"Connected as {bot.user}")
        if not bot.guilds:
            print("Error: Bot is not in any server. Please invite it first!")
            await bot.close()
            return

        guild = bot.guilds[0] # เลือกเซิร์ฟเวอร์แรกที่เจอ
        print(f"Setting up workspace in: {guild.name} ({guild.id})")
        
        discord_tool = DiscordTool(bot)
        
        try:
            # Helper to create category and return ID
            async def get_cat_id(name):
                res = await discord_tool.create_category(guild.id, name)
                import re
                match = re.search(r"ID: (\d+)", res)
                return int(match.group(1)) if match else None

            # 1. Control Center
            print("Creating Control Center...")
            cat_control = await get_cat_id("🌌 ศูนย์ควบคุม AETOX")
            await discord_tool.create_channel(guild.id, "🎮-ห้องสั่งการ", cat_control)
            await discord_tool.create_channel(guild.id, "📜-บันทึกระบบ", cat_control)

            # 2. Projects
            print("Creating Projects...")
            cat_projects = await get_cat_id("📂 จัดการโปรเจกต์")
            await discord_tool.create_channel(guild.id, "🛠️-งานปัจจุบัน", cat_projects)
            await discord_tool.create_channel(guild.id, "🗄️-คลังไฟล์เก่า", cat_projects)

            # 3. Brain
            print("Creating Knowledge Base...")
            cat_brain = await get_cat_id("🧠 คลังความรู้ AETOX")
            await discord_tool.create_channel(guild.id, "💡-ระดมสมอง", cat_brain)

            print("\n✅ DONE! Please check your Discord server.")
        except Exception as e:
            print(f"Error during setup: {e}")
        
        await bot.close()

    await bot.start(TOKEN)

if __name__ == "__main__":
    asyncio.run(run_setup())
