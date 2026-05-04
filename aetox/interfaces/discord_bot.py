import os
import discord
import logging
import asyncio
import time
from discord.ext import commands
from dotenv import load_dotenv

# Load environment variables
load_dotenv()
TOKEN = os.getenv("DISCORD_TOKEN")
ALLOWED_USERS = os.getenv("ALLOWED_USER_IDS", "").split(",")

# Setup logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("aetox.interfaces.discord")

from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.planner import AetoxPlanner
from aetox.core.dispatcher import Dispatcher
from aetox.memory.working import WorkingMemory

# Initialize Discord Bot
intents = discord.Intents.default()
intents.message_content = True
bot = commands.Bot(command_prefix="!", intents=intents)

# Persistent Instance
persistent_memory = WorkingMemory("")
shared_dispatcher = Dispatcher(persistent_memory)

class DiscordInterface:
    """
    Asynchronous Interface Layer for AetoxOS.
    """
    def __init__(self, context: commands.Context):
        self.context = context

    async def stream_chat(self, stream_generator):
        """Streams AI response tokens asynchronously with optimized buffering."""
        message = None
        full_content = ""
        last_update = 0
        update_interval = 0.5  # Optimized interval for speed (0.5s)

        try:
            async for token in stream_generator:
                if token == "__NOT_CHAT__":
                    return False
                
                full_content += token
                current_time = time.time()
                
                # Update Discord if interval passed or it's the first message
                if (current_time - last_update) > update_interval or not message:
                    if not message:
                        message = await self.context.send(full_content + " ▌")
                    else:
                        await message.edit(content=full_content + " ▌")
                    last_update = current_time
            
            # Final polish
            if message:
                await message.edit(content=full_content)
            elif full_content:
                await self.context.send(full_content)
            return True
        except Exception as e:
            logger.error(f"Streaming error: {e}")
            await self.context.send(f"❌ **ระบบสตรีมขัดข้อง:** {str(e)}")
            return True

    async def send_progress(self, message: str):
        """Send progress updates asynchronously."""
        await self.context.send(f"⏳ {message}")

    async def request_approval(self, action: str, details: str) -> bool:
        """Asynchronous permission request."""
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
    logger.info(f"AetoxOS Async Interface ready: {bot.user}")

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
    """Main entry point - Optimized for speed and async flow."""
    if not goal: return
    
    interface = DiscordInterface(ctx)
    shared_dispatcher.progress_callback = interface.send_progress if "!plan" in ctx.message.content else None
    shared_dispatcher.executor.permission_manager.approval_callback = interface.request_approval
    persistent_memory.update_context({"guild_id": ctx.guild.id if ctx.guild else None})

    async with ctx.typing():
        # Lane 1: Smart Intent Extraction & Step Estimation
        minimal_context = {"context": {}} 
        extraction = await shared_dispatcher.executor.extract_action({"description": goal}, minimal_context)
        
        est_steps = extraction.get("estimated_steps", 1)
        analysis = extraction.get("analysis", "")
        
        # Always send the text analysis/response first (What the user sees)
        if analysis:
            await ctx.send(f"🤖 **AetoxOS:** {analysis}")

        # Lane 2: Execution Lane Decision (Internal Calculation)
        if est_steps > 1:
            # --- AUTO-PLANNING LANE (Invisible to user, but triggered by the 'Number') ---
            try:
                planner = AetoxPlanner()
                # Link callbacks for progress visibility in planning mode
                shared_dispatcher.progress_callback = interface.send_progress
                
                plan = await planner.create_plan(goal)
                await shared_dispatcher.run_plan(plan)
                await ctx.send("🏁 **ภารกิจซับซ้อนดำเนินการเสร็จสิ้นเรียบร้อยครับ!**")
            except Exception as e:
                await ctx.send(f"❌ **เกิดข้อผิดพลาดในการประมวลผลหลายขั้นตอน:** {str(e)}")
        
        elif extraction.get("tool") == "chat":
            # --- SIMPLE CHAT LANE ---
            stream_gen = shared_dispatcher.executor.run_chat_stream(goal)
            await interface.stream_chat(stream_gen)
        else:
            # --- SINGLE ACTION LANE ---
            try:
                result = await shared_dispatcher.executor.run_action(extraction, minimal_context)
                if result.get("status") == "success":
                    output = result.get("output", "")
                    if output:
                        if len(output) > 1900: output = output[:1900] + "..."
                        await ctx.send(output)
                else:
                    await ctx.send(f"❌ **ล้มเหลว:** {result.get('error')}")
            except Exception as e:
                await ctx.send(f"❌ **ผิดพลาด:** {str(e)}")

@bot.command(name="task")
async def start_task(ctx, *, goal: str):
    await handle_task_pipe(ctx, goal)

@bot.command(name="plan")
async def start_plan_task(ctx, *, goal: str):
    interface = DiscordInterface(ctx)
    planner = AetoxPlanner()
    
    shared_dispatcher.progress_callback = interface.send_progress
    shared_dispatcher.executor.permission_manager.approval_callback = interface.request_approval

    async with ctx.typing():
        try:
            plan = await planner.create_plan(goal)
            await ctx.send(f"📝 **แผนงาน:** {len(plan.get('steps', []))} ขั้นตอน กำลังเริ่มดำเนินการ...")
            await shared_dispatcher.run_plan(plan)
            await ctx.send("🏁 **เสร็จสิ้นภารกิจ!**")
        except Exception as e:
            await ctx.send(f"❌ **ผิดพลาด:** {str(e)}")

if __name__ == "__main__":
    if TOKEN: bot.run(TOKEN)
    else: logger.error("No TOKEN found.")
