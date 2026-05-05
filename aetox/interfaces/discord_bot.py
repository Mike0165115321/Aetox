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
from aetox.core.config_loader import config_loader

# Initialize Discord Bot
intents = discord.Intents.default()
intents.message_content = True
bot = commands.Bot(command_prefix="!", intents=intents)

# Persistent Instance
persistent_memory = WorkingMemory(config_loader.get_memory_config())
shared_dispatcher = Dispatcher(persistent_memory)

class DiscordInterface:
    """
    Asynchronous Interface Layer for AetoxClaw.
    Pipe mode: Discord for output, Terminal for debug/analysis.
    """
    def __init__(self, context: commands.Context):
        self.context = context

    async def stream_chat(self, stream_generator):
        """Streams AI response tokens asynchronously to Discord."""
        message = None
        full_content = ""
        last_update = 0
        update_interval = 0.5

        try:
            async for token in stream_generator:
                if token == "__NOT_CHAT__":
                    return False
                
                full_content += token
                current_time = time.time()
                
                if (current_time - last_update) > update_interval or not message:
                    if not message:
                        message = await self.context.send(full_content + " ▌")
                    else:
                        await message.edit(content=full_content + " ▌")
                    last_update = current_time
            
            if message:
                await message.edit(content=full_content)
            elif full_content:
                await self.context.send(full_content)
            return full_content
        except Exception as e:
            print(f"\n[ERROR] Streaming failed: {e}")
            await self.context.send(f"❌ **ขออภัย ระบบขัดข้อง:** {str(e)}")
            return ""

    async def send_progress(self, message: str):
        """Log progress to Terminal and Discord for visibility."""
        print(f"[PROGRESS] {message}")
        try:
            await self.context.send(f"⏳ {message}")
        except:
            pass

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
    logger.info(f"AetoxClaw Interface ready: {bot.user}")

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
    """Main entry point - Unified Streaming Pipeline."""
    if not goal: return
    
    interface = DiscordInterface(ctx)
    shared_dispatcher.progress_callback = interface.send_progress
    shared_dispatcher.executor.permission_manager.approval_callback = interface.request_approval
    
    task_id = f"discord_{ctx.author.id}_{int(time.time())}"
    await persistent_memory.update_context(task_id, {"guild_id": ctx.guild.id if ctx.guild else None})

    async with ctx.typing():
        # ดึงประวัติแบบดิบมาแปลงเป็น String
        history_list = shared_dispatcher.executor.history
        history_str = "\n".join([f"Q: {h['q']}\nA: {h['a']}" for h in history_list])
        
        minimal_context = {"history": history_str} 
        extraction = await shared_dispatcher.executor.extract_action({"description": goal}, minimal_context)
        
        analysis = extraction.get("analysis", "วิเคราะห์งาน...")
        print(f"\n[GOAL] {goal}\n[ANALYSIS] {analysis}\n")

        if extraction.get("estimated_steps", 1) > 2:
            # --- PLANNING LANE ---
            await ctx.send(f"🤖 **วิเคราะห์:** {analysis}")
            status_msg = await ctx.send("⏳ **กำลังเตรียมแผนงาน...**")
            planner = AetoxPlanner()
            plan = await planner.create_plan(goal)
            await status_msg.delete()
            
            steps_msg = "📝 **แผนการทำงาน:**\n" + "\n".join([f"- {s.get('description')}" for s in plan.get('steps', [])])
            await ctx.send(steps_msg)
            
            if await interface.request_approval("ดำเนินการตามแผน", goal):
                await shared_dispatcher.run_plan(plan)
                await ctx.send("🏁 **ภารกิจเสร็จสิ้น!**")
        else:
            # --- UNIFIED STREAMING PIPE ---
            result = await shared_dispatcher.executor.run_action(extraction, minimal_context)
            
            async def unified_generator():
                if result.get("status") == "success":
                    output = result.get("output", "")
                    if len(output) > 600:
                        yield "📝 **[สรุปเนื้อหาสำคัญ]:**\n\n"
                        sum_prompt = f"สรุปเนื้อหาต่อไปนี้ให้สั้นและเป็นประเด็นสำคัญในภาษาไทย:\n\n{output[:8000]}"
                        async for token in shared_dispatcher.executor.run_chat_stream(sum_prompt):
                            yield token
                    else:
                        yield output or "สำเร็จเรียบร้อยครับ"
                elif result.get("status") == "chat":
                    async for token in shared_dispatcher.executor.run_chat_stream(goal, context=result.get("output")):
                        yield token
                else:
                    yield f"❌ **ผิดพลาด:** {result.get('error', 'Unknown Error')}"

            final_response = await interface.stream_chat(unified_generator())
            if final_response:
                shared_dispatcher.executor.add_to_history(goal, final_response)

if __name__ == "__main__":
    if TOKEN: bot.run(TOKEN)
    else: logger.error("No TOKEN found.")
