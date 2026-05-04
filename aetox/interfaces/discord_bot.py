import os
import discord
import logging
import asyncio
import time
from discord.ext import commands
from dotenv import load_dotenv

from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.planner import AetoxPlanner
from aetox.core.dispatcher import Dispatcher
from aetox.memory.working import WorkingMemory

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

# Persistent Instance for Shared State
persistent_memory = WorkingMemory("")
shared_dispatcher = Dispatcher(persistent_memory)

class DiscordInterface:
    """
    Pure Interface Layer - The 'Pipe' for AetoxOS.
    Handles streaming and message rendering.
    """
    def __init__(self, context: commands.Context):
        self.context = context
        self.loop = asyncio.get_event_loop()

    async def stream_chat(self, stream_generator):
        """Streams AI response tokens to Discord with buffering."""
        message = None
        full_content = ""
        buffer = ""
        last_update = 0
        update_interval = 0.8  # Seconds between Discord message edits

        try:
            for token in stream_generator:
                if token == "__NOT_CHAT__":
                    return False # Signal to fallback to direct step
                
                full_content += token
                buffer += token
                
                # Update Discord if buffer is meaningful or time interval passed
                current_time = time.time()
                if (current_time - last_update) > update_interval:
                    if not message:
                        message = await self.context.send(full_content + " ▌")
                    else:
                        await message.edit(content=full_content + " ▌")
                    last_update = current_time
                    buffer = ""
            
            # Final update
            if message:
                await message.edit(content=full_content)
            elif full_content:
                await self.context.send(full_content)
            
            return True
        except Exception as e:
            logger.error(f"Streaming error: {e}")
            await self.context.send(f"❌ **ขออภัย ระบบสตรีมขัดข้อง:** {str(e)}")
            return True

    def send_progress(self, message: str):
        """Callback for Dispatcher progress updates."""
        asyncio.run_coroutine_threadsafe(
            self.context.send(f"⏳ {message}"), 
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
            f"⚠️ **ความปลอดภัย:** ต้องการ `{action}`\n"
            f"**รายละเอียด:** `{details}`\n"
            f"กด ✅ เพื่ออนุมัติ หรือ ❌ เพื่อปฏิเสธ"
        )
        await msg.add_reaction("✅")
        await msg.add_reaction("❌")

        def check(reaction, user):
            return user == self.context.author and str(reaction.emoji) in ["✅", "❌"] and reaction.message.id == msg.id

        try:
            reaction, _ = await bot.wait_for("reaction_add", timeout=120.0, check=check)
            return str(reaction.emoji) == "✅"
        except asyncio.TimeoutError:
            await self.context.send("⏳ **หมดเวลา:** ปฏิเสธการดำเนินการ")
            return False

@bot.event
async def on_ready():
    logger.info(f"AetoxOS Interface ready: {bot.user}")

@bot.event
async def on_message(message):
    if message.author == bot.user: return
    if message.content.startswith('!'):
        await bot.process_commands(message)
        return

    if str(message.author.id) not in ALLOWED_USERS and "*" not in ALLOWED_USERS: return
    
    ctx = await bot.get_context(message)
    await handle_task_pipe(ctx, message.content.strip())

async def handle_task_pipe(ctx, goal):
    """The main entry point for all goals - Handled as a thin pipe."""
    if not goal: return
    
    interface = DiscordInterface(ctx)
    shared_dispatcher.progress_callback = None
    shared_dispatcher.executor.permission_manager.approval_callback = interface.request_approval
    persistent_memory.update_context({"guild_id": ctx.guild.id if ctx.guild else None})

    # Start showing typing status immediately
    async with ctx.typing():
        # Lane 1: Try Streaming Chat
        stream_gen = shared_dispatcher.run_direct_chat_stream(goal)
        is_chat = await interface.stream_chat(stream_gen)
        
        if not is_chat:
            # Lane 2: Fallback to Direct Tool Execution
            try:
                result = await asyncio.to_thread(shared_dispatcher.run_direct_step, goal)
                if result.get("status") == "success":
                    output = result.get("output", "ดำเนินการเรียบร้อยครับ")
                    if output:
                        if len(output) > 1900: output = output[:1900] + "..."
                        await ctx.send(output)
                else:
                    await ctx.send(f"❌ **ล้มเหลว:** {result.get('error')}")
            except Exception as e:
                await ctx.send(f"❌ **ผิดพลาด:** {str(e)}")

@bot.command(name="task")
async def start_task(ctx, *, goal: str):
    """Alias for direct execution pipe."""
    await handle_task_pipe(ctx, goal)

@bot.command(name="plan")
async def start_plan_task(ctx, *, goal: str):
    """Planned execution through the pipe."""
    interface = DiscordInterface(ctx)
    client = OllamaClient()
    engine = PromptEngine()
    planner = AetoxPlanner(client, engine)
    
    shared_dispatcher.progress_callback = interface.send_progress
    shared_dispatcher.executor.permission_manager.approval_callback = interface.request_approval

    async with ctx.typing():
        try:
            plan = await asyncio.to_thread(planner.create_plan, goal)
            await ctx.send(f"📝 **แผนงาน:** {len(plan.get('steps', []))} ขั้นตอน กำลังเริ่มดำเนินการ...")
            
            await asyncio.to_thread(shared_dispatcher.run_plan, plan)
            await ctx.send("🏁 **เสร็จสิ้นภารกิจ!**")
        except Exception as e:
            await ctx.send(f"❌ **ผิดพลาด:** {str(e)}")

@bot.command(name="help_aetox")
async def custom_help(ctx):
    await ctx.send(
        "**🌌 AetoxOS Interface**\n"
        "พิมพ์ข้อความหาบอทได้โดยตรง หรือใช้:\n"
        "`!task` - สั่งงานทันที\n"
        "`!plan` - วางแผนและทำงาน\n"
    )

if __name__ == "__main__":
    if TOKEN:
        bot.run(TOKEN)
    else:
        logger.error("No TOKEN found.")
