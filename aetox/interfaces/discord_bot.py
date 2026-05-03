import os
import discord
import logging
import asyncio
import threading
import json
from discord.ext import commands
from dotenv import load_dotenv
from concurrent.futures import ThreadPoolExecutor

from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.core.planner import Planner
from aetox.core.dispatcher import Dispatcher
from aetox.memory.working import WorkingMemory
from aetox.memory.manager import MemoryManager
from aetox.tools.discord_manager import DiscordTool

# Load environment variables
load_dotenv()
TOKEN = os.getenv("DISCORD_TOKEN")
ALLOWED_USERS = os.getenv("ALLOWED_USER_IDS", "").split(",")

# Setup logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("aetox.interfaces.discord")

# Initialize Discord Bot
intents = discord.Intents.default()
intents.message_content = True
bot = commands.Bot(command_prefix="!", intents=intents)

# Global state for task tracking
active_tasks = {} # user_id -> task_info

class DiscordInterface:
    """
    Handles communication between AetoxOS and Discord.
    """
    def __init__(self, context: commands.Context):
        self.context = context
        self.loop = asyncio.get_event_loop()

    def send_progress(self, message: str):
        """Callback for Dispatcher progress updates."""
        asyncio.run_coroutine_threadsafe(
            self.context.send(f"**Progress:** {message}"), 
            self.loop
        )

    def request_approval(self, action: str, details: str) -> bool:
        """Callback for PermissionManager to ask via Discord reactions."""
        future = asyncio.run_coroutine_threadsafe(
            self._ask_discord(action, details), 
            self.loop
        )
        return future.result()

    async def _ask_discord(self, action: str, details: str) -> bool:
        msg = await self.context.send(
            f"⚠️ **SECURITY APPROVAL REQUIRED**\n"
            f"**Action:** `{action}`\n"
            f"**Details:** `{details}`\n"
            f"Please react with ✅ to Approve or ❌ to Deny."
        )
        await msg.add_reaction("✅")
        await msg.add_reaction("❌")

        def check(reaction, user):
            return user == self.context.author and str(reaction.emoji) in ["✅", "❌"] and reaction.message.id == msg.id

        try:
            reaction, user = await bot.wait_for("reaction_add", timeout=60.0, check=check)
            return str(reaction.emoji) == "✅"
        except asyncio.TimeoutError:
            await self.context.send("⌛ Permission request timed out. Denying for safety.")
            return False

@bot.event
async def on_ready():
    logger.info(f"AetoxOS Discord Bot connected as {bot.user}")

@bot.command(name="task")
async def start_task(ctx: commands.Context, *, goal: str):
    """Direct execution lane - No planning, just do it."""
    if str(ctx.author.id) not in ALLOWED_USERS and "*" not in ALLOWED_USERS:
        return

    await ctx.send(f"⚡ **Direct Action:** {goal}\nExecuting...")

    client = OllamaClient()
    engine = PromptEngine()
    memory = WorkingMemory(goal)
    dispatcher = Dispatcher(memory)
    interface = DiscordInterface(ctx)
    discord_tool = DiscordTool(bot)

    dispatcher.progress_callback = interface.send_progress
    dispatcher.executor.permission_manager.approval_callback = interface.request_approval
    dispatcher.executor.discord_tool = discord_tool
    memory.update_context({"guild_id": ctx.guild.id if ctx.guild else None})

    def run_direct():
        try:
            # 1. Execute via Dispatcher (New centralized logic)
            result = dispatcher.run_direct_step(goal)
            
            # 2. Smart Error Suggestion
            if result.get("status") == "failure":
                error_msg = result.get("error", "Unknown error")
                if result.get("needs_planning") or "too complex" in error_msg.lower():
                    asyncio.run_coroutine_threadsafe(
                        ctx.send(f"❌ **Direct Task Failed:** {error_msg}\n💡 *งานนี้ดูซับซ้อนเกินไป ลองใช้ `!plan` ดูไหมครับ?*"), 
                        interface.loop
                    )
                else:
                    asyncio.run_coroutine_threadsafe(
                        ctx.send(f"❌ **Direct Task Failed:** {error_msg}"), 
                        interface.loop
                    )
                return

            # 3. Final Summary
            output = result.get("output", "Done")
            if isinstance(output, str) and len(output) > 1500: output = output[:1500] + "..."
            summary = f"🏁 **Direct Task Completed!**\n**Result:**\n```\n{output}\n```"
            asyncio.run_coroutine_threadsafe(ctx.send(summary), interface.loop)
        except Exception as e:
            asyncio.run_coroutine_threadsafe(ctx.send(f"❌ **Direct Task System Error:** {str(e)}"), interface.loop)

    threading.Thread(target=run_direct).start()

@bot.command(name="plan")
async def start_plan_task(ctx: commands.Context, *, goal: str):
    """Planned execution lane - Complex tasks with multi-step plans."""
    if str(ctx.author.id) not in ALLOWED_USERS and "*" not in ALLOWED_USERS:
        return

    await ctx.send(f"🧠 **Planning Mode:** {goal}\nAnalyzing steps...")

    client = OllamaClient()
    engine = PromptEngine()
    planner = Planner(client, engine)
    memory = WorkingMemory(goal)
    dispatcher = Dispatcher(memory)
    interface = DiscordInterface(ctx)
    discord_tool = DiscordTool(bot)

    dispatcher.progress_callback = interface.send_progress
    dispatcher.executor.permission_manager.approval_callback = interface.request_approval
    dispatcher.executor.discord_tool = discord_tool
    memory.update_context({"guild_id": ctx.guild.id if ctx.guild else None})

    def run_planned():
        try:
            plan = planner.create_plan(goal)
            asyncio.run_coroutine_threadsafe(
                ctx.send(f"✅ **Plan Created:** {len(plan.get('steps', []))} steps. Executing..."), 
                interface.loop
            )
            dispatcher.run_plan(plan)
            
            final_context = memory.get_full_context()
            step_results = final_context.get("step_results", [])
            summary = f"🏁 **Planned Task Completed!**\n**Goal:** {goal}\n"
            results_text = ""
            for i, res in enumerate(step_results):
                output = res.get("output", "Done")
                if isinstance(output, str) and len(output) > 500: output = output[:500] + "..."
                results_text += f"\n**Step {i+1}:** ```\n{output}\n```"
            
            asyncio.run_coroutine_threadsafe(ctx.send(summary + results_text), interface.loop)
        except Exception as e:
            asyncio.run_coroutine_threadsafe(ctx.send(f"❌ **Planning Failed:** {str(e)}"), interface.loop)

    threading.Thread(target=run_planned).start()

@bot.command(name="setup")
async def setup_server(ctx: commands.Context):
    """One-click setup for a professional Aetox workspace."""
    if str(ctx.author.id) not in ALLOWED_USERS and "*" not in ALLOWED_USERS:
        return

    await ctx.send("🏗️ **Starting Professional Workspace Setup...**")
    guild_id = ctx.guild.id
    discord_tool = DiscordTool(bot)

    try:
        # 1. Control Center
        cat_control = await guild_id_to_cat_id(guild_id, "🌌 ศูนย์ควบคุม AETOX", discord_tool)
        await discord_tool.create_channel(guild_id, "🎮-ห้องสั่งการ", cat_control)
        await discord_tool.create_channel(guild_id, "📜-บันทึกระบบ", cat_control)

        # 2. Projects
        cat_projects = await guild_id_to_cat_id(guild_id, "📂 จัดการโปรเจกต์", discord_tool)
        await discord_tool.create_channel(guild_id, "🛠️-งานปัจจุบัน", cat_projects)
        await discord_tool.create_channel(guild_id, "🗄️-คลังไฟล์เก่า", cat_projects)

        # 3. Brain
        cat_brain = await guild_id_to_cat_id(guild_id, "🧠 คลังความรู้ AETOX", discord_tool)
        await discord_tool.create_channel(guild_id, "💡-ระดมสมอง", cat_brain)

        await ctx.send("✅ **ตั้งค่า Workspace เสร็จเรียบร้อย!** ยินดีต้อนรับสู่ห้องสั่งการ AetoxOS ครับ")
    except Exception as e:
        await ctx.send(f"❌ Setup failed: {str(e)}")

async def guild_id_to_cat_id(guild_id, name, tool):
    """Helper to create category and return ID."""
    res = await tool.create_category(guild_id, name)
    # Extract ID from string result "Successfully created category: Name (ID: 123)"
    import re
    match = re.search(r"ID: (\d+)", res)
    return int(match.group(1)) if match else None

@bot.command(name="memory")
async def show_memory(ctx: commands.Context):
    """Shows what AetoxOS knows about you."""
    manager = MemoryManager()
    prefs = manager.preference.preferences
    recent = manager.episodic.query_recent(limit=3)
    
    msg = "**🧠 AetoxOS Memory**\n\n"
    msg += "**Preferences:**\n"
    msg += f"- File Naming: {prefs.get('file_naming')}\n"
    msg += f"- Custom Rules: {len(prefs.get('custom_rules', []))} rules learned.\n\n"
    
    msg += "**Recent Activity:**\n"
    for ep in recent:
        msg += f"- {ep['timestamp'][:10]}: {ep['task_summary']} ({ep['outcome']})\n"
        
    await ctx.send(msg)

@bot.command(name="help_aetox")
async def custom_help(ctx: commands.Context):
    """Custom help message."""
    help_text = (
        "**🌌 AetoxOS Discord Interface**\n"
        "`!task [goal]` - Send a goal to AetoxOS\n"
        "`!status`      - Check current progress\n"
        "`!memory`      - View learned preferences and history\n"
        "`!cancel`      - Stop current task\n"
    )
    await ctx.send(help_text)

if __name__ == "__main__":
    if not TOKEN:
        logger.error("No DISCORD_TOKEN found in environment.")
    else:
        bot.run(TOKEN)
